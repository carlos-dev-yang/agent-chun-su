// Package reception owns conversational intake independently of UI transports.
// It can discuss intent and request bounded host actions; task execution has its
// own queued job, attempt, Skill, sources and process.
package reception

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"

	"chunsu/internal/chatlanguage"
	"chunsu/internal/chatstyle"
	"chunsu/internal/config"
	"chunsu/internal/conversation"
	"chunsu/internal/conversationstate"
	"chunsu/internal/executor"
	"chunsu/internal/files"
)

const Local = "local_terminal"
const Telegram = "telegram_paired_private_dm"

type Generation = executor.Result
type Generator func(context.Context, string, []byte, []byte, []byte) (Generation, error)

func Generate(ctx context.Context, root, directory string, c config.Config, prompt, schema, skill []byte) (Generation, error) {
	return executor.Structured(ctx, executor.StructuredRequest{Role: config.RoleReception, Root: root, Directory: directory, Executor: c.ExecutorFor(config.RoleReception), Limits: c.Limits, Prompt: prompt, Schema: schema, Skill: skill})
}

type Event struct {
	Kind   EventKind
	Text   string
	Action string
}
type Session struct {
	ID             string
	Root           string
	Config         config.Config
	Channel        string
	UpdateID       int64
	History        []conversation.Event
	KnownJobs      map[string]bool
	State          *conversationstate.Store
	ConversationID string
	Revision       uint64
}

func New(root string, c config.Config, channel string) *Session {
	return &Session{ID: files.ID(), Root: root, Config: c, Channel: channel, UpdateID: -1, KnownJobs: map[string]bool{}}
}

func (s *Session) Attach(ctx context.Context, owner string) error {
	s.State = conversationstate.New(s.Root, s.Config.Limits)
	value, err := s.State.Ensure(ctx, s.Channel, owner)
	if err != nil {
		return err
	}
	if value.TurnState == "running" {
		value, err = s.State.Mark(ctx, value.ID, value.Revision, "uncertain")
		if err != nil {
			return err
		}
	}
	history, err := s.State.History(value)
	if err != nil {
		return err
	}
	s.ConversationID, s.Revision, s.History = value.ID, value.Revision, history
	if s.Channel == Telegram {
		s.ID = "telegram-" + value.ID
	} else {
		s.ID = value.ID
	}
	s.KnownJobs = make(map[string]bool, len(value.JobIDs))
	for _, id := range value.JobIDs {
		s.KnownJobs[id] = true
	}
	if value.TurnState == "uncertain" {
		s.History = append(s.History, conversation.Event{Role: "host", Content: "The prior reply was interrupted or its delivery is uncertain. Do not replay an action automatically."})
	}
	return nil
}

func (s *Session) recordMessage(ctx context.Context, role, content, state string) error {
	if s.State == nil {
		return nil
	}
	value, err := s.State.Append(ctx, s.ConversationID, s.Revision, role, content, state)
	if err != nil {
		return err
	}
	s.Revision = value.Revision
	return nil
}

func (s *Session) recordAction(ctx context.Context, id, name, status string, result HostResult) error {
	if s.State == nil {
		return nil
	}
	if name == conversation.DelegateJob || name == conversation.StartWebJob {
		if detail, ok := result.Detail.(map[string]string); ok && files.ValidID(detail["job_id"]) {
			value, err := s.State.LinkJob(ctx, s.ConversationID, s.Revision, detail["job_id"])
			if err != nil {
				return err
			}
			s.Revision = value.Revision
		}
	}
	value, err := s.State.IngestEvent(ctx, s.ConversationID, s.Revision, conversationstate.HostReference{ID: id, Kind: name, Summary: "Host action status: " + status})
	if err != nil {
		return err
	}
	s.Revision = value.Revision
	return nil
}

// SelectPriorJob makes one old link available in the current chat only after
// the owner names both the prior conversation and job. Reset never copies links.
func (s *Session) SelectPriorJob(ctx context.Context, priorID, jobID string) error {
	if s.State == nil || !files.ValidID(priorID) || !files.ValidID(jobID) {
		return errors.New("select a valid prior conversation and job ID")
	}
	prior, err := s.State.Load(priorID)
	if err != nil {
		return err
	}
	current, err := s.State.Load(s.ConversationID)
	if err != nil {
		return err
	}
	if prior.Channel != current.Channel || prior.Owner != current.Owner || !slices.Contains(prior.JobIDs, jobID) {
		return errors.New("the job is not linked to that conversation and owner")
	}
	current, err = s.State.LinkJob(ctx, s.ConversationID, s.Revision, jobID)
	if err != nil {
		return err
	}
	s.Revision = current.Revision
	current, err = s.State.IngestEvent(ctx, s.ConversationID, s.Revision, conversationstate.HostReference{ID: files.ID(), Kind: "selected_job", Summary: "The owner selected existing job " + jobID + " from conversation " + priorID})
	if err != nil {
		return err
	}
	s.Revision = current.Revision
	s.KnownJobs[jobID] = true
	s.History, err = s.State.History(current)
	return err
}

func Capabilities(channel string) []conversation.Capability {
	allowed := []conversation.Capability{}
	for _, capability := range conversation.Capabilities() {
		if channel == Local {
			allowed = append(allowed, capability)
		} else if channel == Telegram {
			switch capability.Name {
			case conversation.None, conversation.RuntimeStatus, conversation.ReadGuide, conversation.InstallGuide, conversation.ListJobs, conversation.DelegateJob, conversation.StartWebJob,
				conversation.WebSearch, conversation.WebOpen,
				conversation.ListErrors, conversation.AcknowledgeError, conversation.PauseQueue, conversation.ResumeQueue, conversation.CancelJob, conversation.RetryJob, conversation.ListFeatures, conversation.InstallFeature,
				conversation.StartWorker, conversation.StopWorker, conversation.RestartWorker, conversation.ReadWorkerConfig, conversation.ConfigureWorker:
				allowed = append(allowed, capability)
			}
		}
	}
	return allowed
}

func allowed(channel, name string) bool {
	for _, capability := range Capabilities(channel) {
		if capability.Name == name {
			return true
		}
	}
	return false
}

// Turn never replays a partial turn. Transport cancellation must finish the
// generator before another turn is accepted on this session.
func (s *Session) Turn(ctx context.Context, request string, host Host, generate Generator, emit func(Event) error) (turnErr error) {
	if s.Channel != Local && s.Channel != Telegram {
		return errors.New("unsupported reception channel")
	}
	if s.KnownJobs == nil {
		s.KnownJobs = map[string]bool{}
	}
	requestID := files.ID()
	if err := s.recordMessage(ctx, "user", request, "running"); err != nil {
		return err
	}
	defer func() {
		if s.State != nil && turnErr != nil {
			if value, err := s.State.Mark(context.Background(), s.ConversationID, s.Revision, "uncertain"); err == nil {
				s.Revision = value.Revision
			}
		}
	}()
	s.History = append(s.History, conversation.Event{Role: "user", Content: request})
	if len(s.History) > s.Config.Limits.MaxMessages {
		return errors.New("conversation context limit reached; start a new conversation with /reset or select a relevant job")
	}
	schema, err := conversation.SchemaFor(Capabilities(s.Channel))
	if err != nil {
		return err
	}
	skill, err := conversation.Skill()
	if err != nil {
		return err
	}
	if generate == nil {
		generate = func(ctx context.Context, directory string, prompt, schema, skill []byte) (executor.Result, error) {
			return Generate(ctx, s.Root, directory, s.Config, prompt, schema, skill)
		}
	}
	delegated := map[string]bool{}
	for step := 0; step < min(conversation.MaxActionsPerTurn, s.Config.Limits.MaxToolCalls); step++ {
		style, err := chatstyle.Load(s.Root, s.Config.Limits)
		if err != nil {
			return err
		}
		replyLanguage, err := chatlanguage.Load(s.Root, s.Config.Limits)
		if err != nil {
			return err
		}
		prompt, err := conversation.PromptForWithPreferences(s.History, s.Config.Limits.MaxSourceBytes, Capabilities(s.Channel), s.Channel, style, replyLanguage)
		if err != nil {
			return err
		}
		directory := filepath.Join(s.Root, "chat", s.ID, files.ID())
		generated, err := generate(ctx, directory, prompt, schema, skill)
		if generated.Outcome == "orphaned" {
			return errors.Join(ErrUncertain, err)
		}
		if err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		reply, err := conversation.Decode(generated.Final)
		if err != nil || !allowed(s.Channel, reply.Action.Name) {
			return errors.New("reception action is outside this channel's supported contract")
		}
		ordinal := step + 1
		event := Event{Kind: EventProgress, Text: reply.Message}
		if reply.Action.Name == conversation.None {
			event.Kind = EventReply
			if err = s.recordMessage(ctx, "assistant", reply.Message, "idle"); err != nil {
				return err
			}
			if err = s.deliver(directory, requestID, ordinal, event, emit); err != nil {
				return err
			}
			encoded, _ := json.Marshal(reply)
			s.History = append(s.History, conversation.Event{Role: "assistant", Content: string(encoded)})
			return nil
		}
		if reply.Action.Name == conversation.DelegateJob && delegated[reply.Action.Reference] {
			return errors.New("같은 요청에서 이미 접수한 입력을 다시 접수하지 않았습니다. 기존 작업 상태를 확인해 주세요.")
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		dispatch := func(actionCtx context.Context) error {
			if err := s.deliver(directory, requestID, ordinal, event, emit); err != nil {
				return err
			}
			encoded, _ := json.Marshal(reply)
			s.History = append(s.History, conversation.Event{Role: "assistant", Content: string(encoded)})
			traceActionValue, inputDigest := traceAction(reply.Action)
			intent := actionRecord{TraceContext: s.traceContext(requestID, ordinal), EventKind: EventAction, Audience: audienceInternal, Delivery: deliverySuppressed, Action: traceActionValue, InputDigest: inputDigest, State: "requested"}
			if err = s.writeJSON(directory, "action.json", intent, false); err != nil {
				return err
			}
			host.ConversationID, host.MessageID, host.RequestRevision = s.ConversationID, requestID, int64(s.Revision)
			outcome, actionErr := host.Dispatch(actionCtx, s.Channel, reply.Action, request, s.KnownJobs)
			if reply.Action.Name == conversation.DelegateJob {
				delegated[reply.Action.Reference] = true
			}
			if actionErr != nil {
				var workerFailure *workerRuntimeError
				if !errors.As(actionErr, &workerFailure) {
					outcome = HostResult{Status: "failed", Detail: "호스트 작업을 완료하지 못했습니다. 로컬 상태를 확인해 주세요. 자동 재시도하지 마세요."}
				}
			}
			outcomeBytes, err := json.Marshal(outcome)
			if err != nil {
				return errors.Join(ErrUncertain, err)
			}
			stored := traceResult(reply.Action.Name, outcome)
			storedBytes, err := json.Marshal(stored)
			if err != nil {
				return errors.Join(ErrUncertain, err)
			}
			audit := actionRecord{TraceContext: s.traceContext(requestID, ordinal), EventKind: EventAction, Audience: audienceInternal, Delivery: deliverySuppressed, Action: traceActionValue, InputDigest: inputDigest, State: outcome.Status, ResultDigest: files.Digest(outcomeBytes), StoredResultDigest: files.Digest(storedBytes), Result: &stored}
			if err = s.writeJSON(directory, "action.json", audit, true); err != nil {
				return errors.Join(ErrUncertain, err)
			}
			if err = s.recordAction(actionCtx, files.ID(), reply.Action.Name, outcome.Status, outcome); err != nil {
				return errors.Join(ErrUncertain, err)
			}
			s.History = append(s.History, conversation.Event{Role: "host", Content: string(outcomeBytes)})
			if actionErr != nil {
				return &ActionError{Cause: actionErr}
			}
			return nil
		}
		if s.State != nil {
			err = s.State.GuardAction(ctx, s.ConversationID, s.Revision, dispatch)
		} else {
			err = dispatch(ctx)
		}
		if err != nil {
			return err
		}
	}
	return errors.New("이번 요청의 작업 처리 한도에 도달했습니다. 현재 결과를 바탕으로 이어서 요청할 수 있습니다.")
}

var ErrUncertain = errors.New("reception execution or action recording requires inspection before continuing")
