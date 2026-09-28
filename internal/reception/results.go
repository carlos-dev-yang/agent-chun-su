package reception

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"chunsu/internal/control"
	"chunsu/internal/conversationstate"
	"chunsu/internal/store"
)

// ConsumeResults runs only between turns. A result is recorded in the active
// conversation before delivery; an uncertain send is never replayed blindly.
func (s *Session) ConsumeResults(ctx context.Context, emit func(string) error) error {
	if s.State == nil || s.ConversationID == "" {
		return nil
	}
	lease, err := s.State.AcquireDelivery(ctx, s.ConversationID)
	if err != nil {
		return err
	}
	defer lease.Close()
	call := func(req control.Request) (json.RawMessage, bool, error) {
		return control.Call(ctx, s.Root, time.Duration(s.Config.Limits.LockWaitSeconds)*time.Second, s.Config.Limits.MaxArtifactBytes, req)
	}
	raw, handled, err := call(control.Request{Operation: "result_sending", ConversationID: s.ConversationID})
	if err != nil || !handled {
		return err
	}
	var sending []store.ResultEvent
	if err = json.Unmarshal(raw, &sending); err != nil {
		return err
	}
	for _, event := range sending {
		if _, handled, err = call(control.Request{Operation: "result_reconcile", EventID: event.ID, ConversationID: event.ConversationID, ExpectedUpdatedAt: event.UpdatedAt}); err != nil {
			return err
		} else if !handled {
			return errors.New("controller became unavailable during result reconciliation")
		}
	}
	raw, handled, err = call(control.Request{Operation: "result_pending", ConversationID: s.ConversationID})
	if err != nil || !handled {
		return err
	}
	var ids []string
	if err = json.Unmarshal(raw, &ids); err != nil {
		return err
	}
	for _, id := range ids {
		raw, handled, readErr := call(control.Request{Operation: "result_read", EventID: id, ConversationID: s.ConversationID})
		if readErr != nil {
			return readErr
		}
		if !handled {
			return errors.New("controller became unavailable during result handoff")
		}
		var event store.ResultEvent
		if err = json.Unmarshal(raw, &event); err != nil {
			return err
		}
		if event.ID != id || event.DeliveryStatus != store.ResultAvailable {
			return errors.New("result event binding changed")
		}
		current, loadErr := s.State.Load(s.ConversationID)
		if loadErr != nil {
			return loadErr
		}
		if current.Archived || current.TurnState == "running" {
			return nil
		}
		if current.Revision != s.Revision {
			s.Revision = current.Revision
			s.History, err = s.State.History(current)
			if err != nil {
				return err
			}
		}
		err = s.State.GuardAction(ctx, s.ConversationID, s.Revision, func(actionCtx context.Context) error {
			if _, handled, err = call(control.Request{Operation: "result_mark", EventID: id, DeliveryStatus: store.ResultSending}); err != nil {
				return err
			} else if !handled {
				return errors.New("controller became unavailable before result send")
			}
			if !slices.Contains(current.ProcessedEvents, id) {
				current, err = s.State.IngestEvent(actionCtx, s.ConversationID, s.Revision, conversationstate.HostReference{ID: id, Kind: "job_result", Summary: fmt.Sprintf("Job %s: %s (artifact %s)", event.JobID, event.Summary, event.ArtifactID)})
				if err != nil {
					_, _, _ = call(control.Request{Operation: "result_mark", EventID: id, DeliveryStatus: store.ResultUnconfirmed})
					return err
				}
				s.Revision = current.Revision
				s.History, err = s.State.History(current)
				if err != nil {
					_, _, _ = call(control.Request{Operation: "result_mark", EventID: id, DeliveryStatus: store.ResultUnconfirmed})
					return err
				}
			}
			text := fmt.Sprintf("Job %s result: %s\nResult event: %s", event.JobID, event.Summary, event.ID)
			if sendErr := emit(text); sendErr != nil {
				_, _, markErr := call(control.Request{Operation: "result_mark", EventID: id, DeliveryStatus: store.ResultUnconfirmed})
				return errors.Join(sendErr, markErr)
			}
			if _, handled, err = call(control.Request{Operation: "result_mark", EventID: id, DeliveryStatus: store.ResultDelivered}); err != nil {
				return err
			} else if !handled {
				return errors.New("result delivery receipt is unconfirmed")
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}
