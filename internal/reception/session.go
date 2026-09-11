// Package reception owns conversational intake independently of UI transports.
// It can discuss intent and request bounded host actions; task execution has its
// own queued job, attempt, Skill, sources and process.
package reception

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"

	"chunsu/internal/config"
	"chunsu/internal/conversation"
	"chunsu/internal/executor"
	"chunsu/internal/files"
)

const Local = "local_terminal"
const Telegram = "telegram_paired_private_dm"

type Generation = executor.Result
type Generator func(context.Context, string, []byte, []byte, []byte) (Generation, error)

func Generate(ctx context.Context, root, directory string, c config.Config, prompt, schema, skill []byte) (Generation, error) {
	return executor.Converse(ctx, root, directory, c.Executor, c.Limits, prompt, schema, skill)
}

type Event struct {
	Kind   string
	Text   string
	Action string
}
type Session struct {
	ID        string
	Root      string
	Config    config.Config
	Channel   string
	History   []conversation.Event
	KnownJobs map[string]bool
}

func New(root string, c config.Config, channel string) *Session {
	return &Session{ID: files.ID(), Root: root, Config: c, Channel: channel, KnownJobs: map[string]bool{}}
}

func Capabilities(channel string) []conversation.Capability {
	allowed := []conversation.Capability{}
	for _, capability := range conversation.Capabilities() {
		if channel == Local {
			allowed = append(allowed, capability)
		} else if channel == Telegram {
			switch capability.Name {
			case conversation.None, conversation.RuntimeStatus, conversation.ReadGuide, conversation.InstallGuide, conversation.ListJobs, conversation.DelegateJob:
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
func (s *Session) Turn(ctx context.Context, request string, host Host, generate Generator, emit func(Event) error) error {
	if s.Channel != Local && s.Channel != Telegram {
		return errors.New("unsupported reception channel")
	}
	if s.KnownJobs == nil {
		s.KnownJobs = map[string]bool{}
	}
	s.History = append(s.History, conversation.Event{Role: "user", Content: request})
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
		prompt, err := conversation.PromptFor(s.History, s.Config.Limits.MaxSourceBytes, Capabilities(s.Channel), s.Channel)
		if err != nil {
			return err
		}
		directory := filepath.Join(s.Root, "chat", s.ID, files.ID())
		if err = emit(Event{Kind: "thinking"}); err != nil {
			return errors.Join(ErrUncertain, err)
		}
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
		if err = emit(Event{Kind: "reply", Text: reply.Message}); err != nil {
			return errors.Join(ErrUncertain, err)
		}
		encoded, _ := json.Marshal(reply)
		s.History = append(s.History, conversation.Event{Role: "assistant", Content: string(encoded)})
		if reply.Action.Name == conversation.None {
			return nil
		}
		if reply.Action.Name == conversation.DelegateJob && delegated[reply.Action.Reference] {
			return errors.New("같은 요청에서 이미 접수한 입력을 다시 접수하지 않았습니다. 기존 작업 상태를 확인해 주세요.")
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		intent, _ := json.Marshal(map[string]any{"action": reply.Action, "state": "requested"})
		if err = files.Write(directory, "action.json", intent, false); err != nil {
			return err
		}
		outcome, actionErr := host.Dispatch(ctx, s.Channel, reply.Action, request, s.KnownJobs)
		if reply.Action.Name == conversation.DelegateJob {
			delegated[reply.Action.Reference] = true
		}
		if actionErr != nil {
			outcome = HostResult{Status: "failed", Detail: "호스트 작업을 완료하지 못했습니다. 로컬 상태를 확인해 주세요. 자동 재시도하지 마세요."}
		}
		outcomeBytes, _ := json.Marshal(outcome)
		audit, _ := json.Marshal(map[string]any{"action": reply.Action, "state": outcome.Status, "result_digest": files.Digest(outcomeBytes)})
		if err = files.Write(directory, "action.json", audit, true); err != nil {
			return errors.Join(ErrUncertain, err)
		}
		s.History = append(s.History, conversation.Event{Role: "host", Content: string(outcomeBytes)})
		if err = emit(Event{Kind: "action", Action: reply.Action.Name, Text: outcome.Status}); err != nil {
			return errors.Join(ErrUncertain, err)
		}
		if actionErr != nil {
			return actionErr
		}
	}
	return errors.New("이번 요청의 작업 처리 한도에 도달했습니다. 현재 결과를 바탕으로 이어서 요청할 수 있습니다.")
}

var ErrUncertain = errors.New("reception execution or action recording requires inspection before continuing")
