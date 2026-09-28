// Package conversationstate keeps ordinary chat continuity outside the
// controller's long-held SQLite writer lock.
package conversationstate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"chunsu/internal/config"
	"chunsu/internal/conversation"
	"chunsu/internal/files"
)

const directory = "conversations"

var ErrRevision = errors.New("conversation changed while the reply was running; inspect the current conversation before retrying")

type HostReference struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Summary    string `json:"summary"`
	AtRevision uint64 `json:"at_revision"`
}

type Conversation struct {
	ID               string               `json:"id"`
	Channel          string               `json:"channel"`
	Owner            string               `json:"owner"`
	Revision         uint64               `json:"revision"`
	Archived         bool                 `json:"archived"`
	TurnState        string               `json:"turn_state"`
	Messages         []conversation.Event `json:"messages"`
	MessageRevisions []uint64             `json:"message_revisions,omitempty"`
	HostEvents       []HostReference      `json:"host_events,omitempty"`
	JobIDs           []string             `json:"job_ids,omitempty"`
	PendingDecisions []string             `json:"pending_decisions,omitempty"`
	ProcessedEvents  []string             `json:"processed_events,omitempty"`
}

type binding struct {
	Channel string `json:"channel"`
	Owner   string `json:"owner"`
	Active  string `json:"active"`
}

type Store struct {
	Root   string
	Limits config.Limits
}

type actionLeaseKey struct{}

func (s *Store) conversationLease(ctx context.Context, id, kind string) (*lock, error) {
	if !files.ValidID(id) {
		return nil, errors.New("invalid conversation ID")
	}
	if err := files.PrivateDir(filepath.Join(s.Root, "state", kind)); err != nil {
		return nil, err
	}
	return acquireNamed(ctx, s.Root, filepath.Join("state", kind, id+".lock"), s.Limits)
}

// GuardAction serializes only the short host action/receipt window for one
// conversation. Generation runs before this guard, and unrelated chats proceed.
func (s *Store) GuardAction(ctx context.Context, id string, revision uint64, action func(context.Context) error) error {
	lease, err := s.conversationLease(ctx, id, "actions")
	if err != nil {
		return err
	}
	defer lease.Close()
	value, err := s.Load(id)
	if err != nil {
		return err
	}
	if value.Revision != revision || value.Archived {
		return ErrRevision
	}
	return action(context.WithValue(ctx, actionLeaseKey{}, id))
}

// AcquireDelivery is held across one result send and its acknowledgement. A
// later sender can safely classify a leftover sending event as unconfirmed
// only after acquiring this lease.
func (s *Store) AcquireDelivery(ctx context.Context, id string) (*lock, error) {
	return s.conversationLease(ctx, id, "deliveries")
}

func New(root string, limits config.Limits) *Store { return &Store{Root: root, Limits: limits} }

func (s *Store) prepare() error {
	if err := files.RequirePrivateDir(s.Root); err != nil {
		return err
	}
	if err := files.PrivateDir(filepath.Join(s.Root, directory)); err != nil {
		return err
	}
	for _, sub := range []string{"bindings", "items"} {
		if err := files.PrivateDir(filepath.Join(s.Root, directory, sub)); err != nil {
			return err
		}
	}
	return nil
}

func bindingName(channel, owner string) (string, error) {
	if channel == "" || owner == "" || len(channel)+len(owner) > 512 || strings.ContainsAny(channel+owner, "\x00\n\r") {
		return "", errors.New("invalid conversation binding")
	}
	h := sha256.Sum256([]byte(channel + "\x00" + owner))
	return filepath.Join(directory, "bindings", hex.EncodeToString(h[:])+".json"), nil
}

func (s *Store) binding(name string) (binding, error) {
	b, err := files.Read(s.Root, name, s.Limits.MaxArtifactBytes)
	if err != nil {
		return binding{}, err
	}
	var pointer binding
	if err = json.Unmarshal(b, &pointer); err != nil {
		return binding{}, err
	}
	if !files.ValidID(pointer.Active) {
		return binding{}, errors.New("invalid conversation binding")
	}
	return pointer, nil
}

func itemName(id string) (string, error) {
	if !files.ValidID(id) {
		return "", errors.New("invalid conversation ID")
	}
	return filepath.Join(directory, "items", id+".json"), nil
}

func (s *Store) read(id string) (Conversation, error) {
	name, err := itemName(id)
	if err != nil {
		return Conversation{}, err
	}
	b, err := files.Read(s.Root, name, s.Limits.MaxEvidenceBytes)
	if err != nil {
		return Conversation{}, err
	}
	var value Conversation
	if err := json.Unmarshal(b, &value); err != nil {
		return Conversation{}, err
	}
	if value.ID != id || value.Revision == 0 {
		return Conversation{}, errors.New("invalid conversation record")
	}
	return value, nil
}

func (s *Store) write(value Conversation, replace bool) error {
	name, err := itemName(value.ID)
	if err != nil {
		return err
	}
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if int64(len(b)) > s.Limits.MaxEvidenceBytes {
		return errors.New("conversation storage limit reached; start a new conversation")
	}
	return files.Write(s.Root, name, append(b, '\n'), replace)
}

func (s *Store) Ensure(ctx context.Context, channel, owner string) (Conversation, error) {
	if err := s.prepare(); err != nil {
		return Conversation{}, err
	}
	name, err := bindingName(channel, owner)
	if err != nil {
		return Conversation{}, err
	}
	lock, err := acquire(ctx, s.Root, s.Limits)
	if err != nil {
		return Conversation{}, err
	}
	defer lock.Close()
	b, err := files.Read(s.Root, name, s.Limits.MaxArtifactBytes)
	if err == nil {
		var current binding
		if err = json.Unmarshal(b, &current); err != nil {
			return Conversation{}, err
		}
		if current.Channel != channel || current.Owner != owner {
			return Conversation{}, errors.New("conversation binding mismatch")
		}
		value, err := s.read(current.Active)
		if err != nil {
			return Conversation{}, err
		}
		if value.Archived || value.Channel != channel || value.Owner != owner {
			return Conversation{}, errors.New("active conversation identity mismatch")
		}
		return value, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return Conversation{}, err
	}
	value := Conversation{ID: files.ID(), Channel: channel, Owner: owner, Revision: 1, TurnState: "idle"}
	if err = s.write(value, false); err != nil {
		return Conversation{}, err
	}
	b, _ = json.Marshal(binding{Channel: channel, Owner: owner, Active: value.ID})
	if err = files.Write(s.Root, name, b, false); err != nil {
		return Conversation{}, err
	}
	return value, nil
}

func (s *Store) Load(id string) (Conversation, error) {
	if err := s.prepare(); err != nil {
		return Conversation{}, err
	}
	return s.read(id)
}

// Change uses the conversation revision as an optimistic guard against a reply
// generated before newer instructions were admitted.
func (s *Store) Change(ctx context.Context, id string, revision uint64, change func(*Conversation) error) (Conversation, error) {
	if err := s.prepare(); err != nil {
		return Conversation{}, err
	}
	if ctx.Value(actionLeaseKey{}) != id {
		lease, err := s.conversationLease(ctx, id, "actions")
		if err != nil {
			return Conversation{}, err
		}
		defer lease.Close()
	}
	lock, err := acquire(ctx, s.Root, s.Limits)
	if err != nil {
		return Conversation{}, err
	}
	defer lock.Close()
	value, err := s.read(id)
	if err != nil {
		return Conversation{}, err
	}
	if value.Revision != revision || value.Archived {
		return Conversation{}, ErrRevision
	}
	if err = change(&value); err != nil {
		return Conversation{}, err
	}
	value.Revision++
	if err = s.write(value, true); err != nil {
		return Conversation{}, err
	}
	return value, nil
}

func (s *Store) Append(ctx context.Context, id string, revision uint64, role, content, turnState string) (Conversation, error) {
	if role != "user" && role != "assistant" {
		return Conversation{}, errors.New("only ordinary dialogue can be retained")
	}
	if strings.TrimSpace(content) == "" || int64(len(content)) > s.Limits.MaxSourceBytes {
		return Conversation{}, errors.New("conversation message exceeds configured limit")
	}
	return s.Change(ctx, id, revision, func(value *Conversation) error {
		value.Messages = append(value.Messages, conversation.Event{Role: role, Content: content})
		value.MessageRevisions = append(value.MessageRevisions, value.Revision+1)
		value.TurnState = turnState
		return nil
	})
}

func (s *Store) Mark(ctx context.Context, id string, revision uint64, state string) (Conversation, error) {
	if state != "idle" && state != "running" && state != "uncertain" {
		return Conversation{}, errors.New("invalid turn state")
	}
	return s.Change(ctx, id, revision, func(value *Conversation) error { value.TurnState = state; return nil })
}

func (s *Store) LinkJob(ctx context.Context, id string, revision uint64, jobID string) (Conversation, error) {
	if !files.ValidID(jobID) {
		return Conversation{}, errors.New("invalid job ID")
	}
	return s.Change(ctx, id, revision, func(value *Conversation) error {
		if !slices.Contains(value.JobIDs, jobID) {
			value.JobIDs = append(value.JobIDs, jobID)
		}
		return nil
	})
}

func (s *Store) IngestEvent(ctx context.Context, id string, revision uint64, event HostReference) (Conversation, error) {
	if !files.ValidID(event.ID) || event.Kind == "" || int64(len(event.Summary)) > s.Limits.MaxSourceBytes {
		return Conversation{}, errors.New("invalid host event")
	}
	return s.Change(ctx, id, revision, func(value *Conversation) error {
		if slices.Contains(value.ProcessedEvents, event.ID) {
			return nil
		}
		event.AtRevision = value.Revision + 1
		value.ProcessedEvents = append(value.ProcessedEvents, event.ID)
		value.HostEvents = append(value.HostEvents, event)
		return nil
	})
}

func (s *Store) Reset(ctx context.Context, channel, owner string) (Conversation, error) {
	if err := s.prepare(); err != nil {
		return Conversation{}, err
	}
	name, err := bindingName(channel, owner)
	if err != nil {
		return Conversation{}, err
	}
	preflight, err := s.binding(name)
	if err != nil {
		return Conversation{}, err
	}
	actionLease, err := s.conversationLease(ctx, preflight.Active, "actions")
	if err != nil {
		return Conversation{}, err
	}
	defer actionLease.Close()
	lock, err := acquire(ctx, s.Root, s.Limits)
	if err != nil {
		return Conversation{}, err
	}
	defer lock.Close()
	b, err := files.Read(s.Root, name, s.Limits.MaxArtifactBytes)
	if err != nil {
		return Conversation{}, err
	}
	var pointer binding
	if err = json.Unmarshal(b, &pointer); err != nil {
		return Conversation{}, err
	}
	if pointer.Channel != channel || pointer.Owner != owner {
		return Conversation{}, errors.New("conversation binding mismatch")
	}
	if pointer.Active != preflight.Active {
		return Conversation{}, ErrRevision
	}
	prior, err := s.read(pointer.Active)
	if err != nil {
		return Conversation{}, err
	}
	next := Conversation{ID: files.ID(), Channel: channel, Owner: owner, Revision: 1, TurnState: "idle"}
	if err = s.write(next, false); err != nil {
		return Conversation{}, err
	}
	pointer.Active = next.ID
	b, _ = json.Marshal(pointer)
	if err = files.Write(s.Root, name, b, true); err != nil {
		return Conversation{}, err
	}
	prior.Archived = true
	prior.Revision++
	if err = s.write(prior, true); err != nil {
		return Conversation{}, err
	}
	return next, nil
}

func (s *Store) Select(ctx context.Context, channel, owner, id string) (Conversation, error) {
	if err := s.prepare(); err != nil {
		return Conversation{}, err
	}
	name, err := bindingName(channel, owner)
	if err != nil {
		return Conversation{}, err
	}
	preflight, err := s.binding(name)
	if err != nil {
		return Conversation{}, err
	}
	actionLease, err := s.conversationLease(ctx, preflight.Active, "actions")
	if err != nil {
		return Conversation{}, err
	}
	defer actionLease.Close()
	lock, err := acquire(ctx, s.Root, s.Limits)
	if err != nil {
		return Conversation{}, err
	}
	defer lock.Close()
	selected, err := s.read(id)
	if err != nil {
		return Conversation{}, err
	}
	if selected.Channel != channel || selected.Owner != owner {
		return Conversation{}, errors.New("conversation does not belong to this channel and owner")
	}
	b, err := files.Read(s.Root, name, s.Limits.MaxArtifactBytes)
	if err != nil {
		return Conversation{}, err
	}
	var pointer binding
	if err = json.Unmarshal(b, &pointer); err != nil {
		return Conversation{}, err
	}
	if pointer.Channel != channel || pointer.Owner != owner {
		return Conversation{}, errors.New("conversation binding mismatch")
	}
	if pointer.Active != preflight.Active {
		return Conversation{}, ErrRevision
	}
	if pointer.Active != id {
		current, err := s.read(pointer.Active)
		if err != nil {
			return Conversation{}, err
		}
		selected.Archived = false
		selected.Revision++
		if err = s.write(selected, true); err != nil {
			return Conversation{}, err
		}
		pointer.Active = id
		b, _ = json.Marshal(pointer)
		if err = files.Write(s.Root, name, b, true); err != nil {
			return Conversation{}, err
		}
		current.Archived = true
		current.Revision++
		if err = s.write(current, true); err != nil {
			return Conversation{}, err
		}
	}
	return selected, nil
}

func (s *Store) History(value Conversation) ([]conversation.Event, error) {
	history := make([]conversation.Event, 0, len(value.Messages)+len(value.HostEvents))
	if len(value.MessageRevisions) != len(value.Messages) {
		history = append(history, value.Messages...)
		for _, event := range value.HostEvents {
			history = append(history, conversation.Event{Role: "host", Content: event.Kind + ": " + event.Summary})
		}
		return history, nil
	}
	i, j := 0, 0
	for i < len(value.Messages) || j < len(value.HostEvents) {
		if j >= len(value.HostEvents) || i < len(value.Messages) && value.MessageRevisions[i] < value.HostEvents[j].AtRevision {
			history = append(history, value.Messages[i])
			i++
		} else {
			event := value.HostEvents[j]
			history = append(history, conversation.Event{Role: "host", Content: event.Kind + ": " + event.Summary})
			j++
		}
	}
	return history, nil
}
