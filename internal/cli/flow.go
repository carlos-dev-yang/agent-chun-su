package cli

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"strconv"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/control"
	"chunsu/internal/conversationstate"
	"chunsu/internal/files"
	"chunsu/internal/reception"
	"chunsu/internal/runner"
	"chunsu/internal/stagedworkflow"
	"chunsu/internal/store"
	"github.com/spf13/cobra"
)

func (o *options) flow() *cobra.Command {
	cmd := &cobra.Command{Use: "flow", Short: "Submit, inspect and locally execute staged conversation work"}
	var objective string
	var checkpoint bool
	submit := &cobra.Command{Use: "submit WORKGROUP INPUT_JSON", Short: "Admit an existing saved snapshot into a staged job", Args: cobra.ExactArgs(2), RunE: func(command *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if err != nil {
			return err
		}
		file, err := os.Open(args[1])
		if err != nil {
			return err
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() > c.Limits.MaxArtifactBytes {
			return errors.New("input must be a bounded regular snapshot file")
		}
		input, err := io.ReadAll(io.LimitReader(file, c.Limits.MaxArtifactBytes+1))
		if err != nil {
			return err
		}
		if int64(len(input)) > c.Limits.MaxArtifactBytes {
			return errors.New("snapshot exceeds artifact limit")
		}
		req := control.Request{Operation: "staged_submit", Workgroup: args[0], Input: json.RawMessage(input), Answer: objective, ManualCheckpoint: checkpoint}
		return o.submitFlow(command, root, c, req)
	}}
	submit.Flags().StringVar(&objective, "objective", "", "User objective for this saved snapshot")
	submit.Flags().BoolVar(&checkpoint, "checkpoint", false, "Wait for owner approval before each later stage")
	_ = submit.MarkFlagRequired("objective")
	var webCheckpoint bool
	web := &cobra.Command{Use: "web QUERY", Short: "Admit public web research as a staged job", Args: cobra.ExactArgs(1), RunE: func(command *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if err != nil {
			return err
		}
		return o.submitFlow(command, root, c, control.Request{Operation: "staged_web", Answer: args[0], ManualCheckpoint: webCheckpoint})
	}}
	web.Flags().BoolVar(&webCheckpoint, "checkpoint", false, "Wait for owner approval before each later stage")
	run := &cobra.Command{Use: "run JOB_ID", Short: "Execute eligible stages locally without activating a service", Args: cobra.ExactArgs(1), RunE: func(command *cobra.Command, args []string) error {
		s, c, closeStore, err := o.open(command.Context(), true)
		if err != nil {
			return err
		}
		defer closeStore()
		if _, err = runner.RecoverStages(command.Context(), s, c); err != nil {
			return err
		}
		r := &runner.Runner{Store: s, Config: c, WorkerMode: true}
		var outcomes []runner.Outcome
		for i := 0; i < store.MaxWorkflowSteps; i++ {
			out, runErr := r.RunStage(command.Context(), args[0])
			outcomes = append(outcomes, out)
			if runErr != nil {
				_ = output(command, outcomes)
				return runErr
			}
			if out.Status == store.Staged {
				break
			}
		}
		return output(command, outcomes)
	}}
	inspect := &cobra.Command{Use: "inspect JOB_ID", Short: "Read staged progress and durable result events", Args: cobra.ExactArgs(1), RunE: func(command *cobra.Command, args []string) error {
		s, _, closeStore, err := o.open(command.Context(), false)
		if err != nil {
			return err
		}
		defer closeStore()
		workflow, err := s.Workflow(command.Context(), args[0])
		if err != nil {
			return err
		}
		steps, err := s.Steps(command.Context(), args[0])
		if err != nil {
			return err
		}
		events, err := s.ResultEvents(command.Context(), workflow.OriginConversationID)
		if err != nil {
			return err
		}
		selected := []store.ResultEvent{}
		for _, event := range events {
			if event.JobID == args[0] {
				selected = append(selected, event)
			}
		}
		return output(command, map[string]any{"workflow": workflow, "steps": steps, "results": selected})
	}}
	result := &cobra.Command{Use: "result EVENT_ID", Short: "Read a complete validated result on the owner CLI", Args: cobra.ExactArgs(1), RunE: func(command *cobra.Command, args []string) error {
		s, c, closeStore, err := o.open(command.Context(), false)
		if err != nil {
			return err
		}
		defer closeStore()
		event, err := s.ResultEvent(command.Context(), args[0])
		if err != nil {
			return err
		}
		artifacts, err := s.Artifacts(command.Context(), event.JobID)
		if err != nil {
			return err
		}
		for _, artifact := range artifacts {
			if artifact.ID != event.ArtifactID {
				continue
			}
			if artifact.Digest != event.ArtifactDigest {
				return errors.New("result artifact digest changed")
			}
			data, readErr := s.ReadArtifact(artifact, c.Limits.MaxArtifactBytes)
			if readErr != nil {
				return readErr
			}
			var validated stagedworkflow.Validated
			if err = json.Unmarshal(data, &validated); err != nil {
				return err
			}
			if validated.Version != 1 || validated.Markdown == "" {
				return errors.New("result artifact is not a validated report")
			}
			return output(command, map[string]any{"event": event, "validated": validated})
		}
		return errors.New("result artifact is unavailable")
	}}
	result.AddCommand(&cobra.Command{Use: "recover EVENT_ID", Short: "Classify an abandoned result send as unconfirmed", Args: cobra.ExactArgs(1), RunE: func(command *cobra.Command, args []string) error {
		return o.flowResultTransition(command, args[0], "result_reconcile", "")
	}})
	result.AddCommand(&cobra.Command{Use: "preview EVENT_ID", Short: "Preview the bounded result visible to the active local chat", Args: cobra.ExactArgs(1), RunE: func(command *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if err != nil {
			return err
		}
		current, err := conversationstate.New(root, c.Limits).Ensure(command.Context(), reception.Local, strconv.Itoa(os.Getuid()))
		if err != nil {
			return err
		}
		req := control.Request{Operation: "result_read", EventID: args[0], ConversationID: current.ID}
		data, handled, err := control.Call(command.Context(), root, time.Duration(c.Limits.LockWaitSeconds)*time.Second, c.Limits.MaxArtifactBytes, req)
		if err != nil {
			return err
		}
		if handled {
			var event store.ResultEvent
			if err = json.Unmarshal(data, &event); err != nil {
				return err
			}
			return output(command, event)
		}
		s, loaded, closeStore, err := o.open(command.Context(), true)
		if err != nil {
			return err
		}
		defer closeStore()
		value, err := (&runner.Runner{Store: s, Config: loaded}).Handle(command.Context(), req)
		if err != nil {
			return err
		}
		return output(command, value)
	}})
	result.AddCommand(&cobra.Command{Use: "resolve EVENT_ID sent|retry", Short: "Resolve an unconfirmed send with an explicit owner decision", Args: cobra.ExactArgs(2), RunE: func(command *cobra.Command, args []string) error {
		if args[1] != store.ResultResolutionSent && args[1] != store.ResultResolutionRetry {
			return errors.New("resolution must be sent or retry")
		}
		return o.flowResultTransition(command, args[0], "result_resolve", args[1])
	}})
	for _, action := range []struct {
		name, operation, argument string
	}{
		{"pause", "staged_pause", "JOB_ID"},
		{"resume", "staged_resume", "JOB_ID"},
		{"approve", "staged_approve", "STEP_ID"},
		{"retry", "staged_retry", "STEP_ID ATTEMPT_ID"},
	} {
		action := action
		argc := 1
		if action.name == "retry" {
			argc = 2
		}
		cmd.AddCommand(&cobra.Command{Use: action.name + " " + action.argument, Short: "Apply an explicit staged job control", Args: cobra.ExactArgs(argc), RunE: func(command *cobra.Command, args []string) error {
			req := control.Request{Operation: action.operation}
			if action.name == "approve" || action.name == "retry" {
				req.StepID = args[0]
			} else {
				req.JobID = args[0]
			}
			if action.name == "retry" {
				req.AttemptID = args[1]
			}
			return o.flowControl(command, req)
		}})
	}
	cmd.AddCommand(submit, web, run, inspect, result)
	return cmd
}

func (o *options) flowResultTransition(command *cobra.Command, eventID, operation, resolution string) error {
	root, err := o.path()
	if err != nil {
		return err
	}
	c, err := config.Load(root)
	if err != nil {
		return err
	}
	read, _, closeRead, err := o.open(command.Context(), false)
	if err != nil {
		return err
	}
	event, err := read.ResultEvent(command.Context(), eventID)
	closeRead()
	if err != nil {
		return err
	}
	state := conversationstate.New(root, c.Limits)
	lease, err := state.AcquireDelivery(command.Context(), event.ConversationID)
	if err != nil {
		return err
	}
	defer lease.Close()
	read, _, closeRead, err = o.open(command.Context(), false)
	if err != nil {
		return err
	}
	event, err = read.ResultEvent(command.Context(), eventID)
	closeRead()
	if err != nil {
		return err
	}
	if operation == "result_reconcile" && event.DeliveryStatus != store.ResultSending || operation == "result_resolve" && event.DeliveryStatus != store.ResultUnconfirmed {
		return errors.New("result delivery state changed; inspect it again")
	}
	req := control.Request{Operation: operation, EventID: event.ID, ConversationID: event.ConversationID, ExpectedUpdatedAt: event.UpdatedAt, DeliveryStatus: resolution}
	return o.flowControl(command, req)
}

func (o *options) flowControl(command *cobra.Command, req control.Request) error {
	root, err := o.path()
	if err != nil {
		return err
	}
	c, err := config.Load(root)
	if err != nil {
		return err
	}
	_, handled, err := control.Call(command.Context(), root, time.Duration(c.Limits.LockWaitSeconds)*time.Second, c.Limits.MaxArtifactBytes, req)
	if err != nil {
		return err
	}
	if !handled {
		s, loaded, closeStore, openErr := o.open(command.Context(), true)
		if openErr != nil {
			return openErr
		}
		defer closeStore()
		_, err = (&runner.Runner{Store: s, Config: loaded}).Handle(command.Context(), req)
		if err != nil {
			return err
		}
	}
	return output(command, map[string]string{"status": "applied", "operation": req.Operation})
}

func (o *options) submitFlow(command *cobra.Command, root string, c config.Config, req control.Request) error {
	if req.Answer == "" || int64(len(req.Answer)) > c.Limits.MaxSourceBytes {
		return errors.New("a bounded objective is required")
	}
	state := conversationstate.New(root, c.Limits)
	owner := strconv.Itoa(os.Getuid())
	current, err := state.Ensure(command.Context(), reception.Local, owner)
	if err != nil {
		return err
	}
	current, err = state.Append(command.Context(), current.ID, current.Revision, "user", req.Answer, "running")
	if err != nil {
		return err
	}
	req.ConversationID, req.MessageID, req.RequestRevision = current.ID, files.ID(), int64(current.Revision)
	data, handled, err := control.Call(command.Context(), root, time.Duration(c.Limits.LockWaitSeconds)*time.Second, c.Limits.MaxArtifactBytes, req)
	var job store.Job
	if err != nil {
		return err
	}
	if handled {
		if err = json.Unmarshal(data, &job); err != nil {
			return err
		}
	} else {
		s, loaded, closeStore, openErr := o.open(command.Context(), true)
		if openErr != nil {
			return openErr
		}
		defer closeStore()
		value, handleErr := (&runner.Runner{Store: s, Config: loaded}).Handle(command.Context(), req)
		if handleErr != nil {
			return handleErr
		}
		var ok bool
		job, ok = value.(store.Job)
		if !ok {
			return errors.New("staged admission returned an unexpected result")
		}
	}
	current, err = state.LinkJob(command.Context(), current.ID, current.Revision, job.ID)
	if err != nil {
		return err
	}
	_, err = state.Mark(command.Context(), current.ID, current.Revision, "idle")
	if err != nil {
		return err
	}
	return output(command, job)
}
