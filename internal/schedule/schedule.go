package schedule

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/gmail"
	"chunsu/internal/jira"
	"chunsu/internal/store"
)

const (
	Pending     = "pending"
	RetryWait   = "retry_wait"
	WaitingAuth = "waiting_auth"
	Blocked     = "blocked"
	Linked      = "linked"
	Done        = "done"
	Skipped     = "skipped"
)

type Manager struct {
	Store  *store.Store
	Config config.Config
}

// Step admits at most one bounded collection. The controller is the sole writer;
// launchd manages that process, while these database rows own job scheduling.
func (m Manager) Step(ctx context.Context) (*store.Tick, error) {
	paused, err := m.Store.Paused(ctx)
	if err != nil || paused {
		return nil, err
	}
	schedules, err := m.Store.Schedules(ctx)
	if err != nil {
		return nil, err
	}
	for _, s := range schedules {
		if !s.Enabled {
			continue
		}
		var previous store.Tick
		if s.LastTickID != "" {
			previous, err = m.Store.Tick(ctx, s.LastTickID)
			if err != nil {
				return nil, err
			}
			if previous.JobID != "" && previous.Status != Done && previous.Status != Skipped {
				j, e := m.Store.Job(ctx, previous.JobID)
				if e != nil {
					return nil, e
				}
				switch j.Status {
				case store.Completed, store.Partial:
					previous.Status = Done
				case store.Queued, store.Running, store.RetryWait:
					previous.Status = Linked
				default:
					previous.Status = Blocked
				}
				previous.Diagnostic = j.Diagnostic
				if e = m.Store.SaveTick(ctx, previous); e != nil {
					return nil, e
				}
			}
			if previous.Status != Done && previous.Status != Skipped {
				if previous.JobID == "" && (previous.Status == Pending || previous.Status == RetryWait) && previous.NotBefore <= time.Now().UnixMilli() {
					return m.collect(ctx, s, previous)
				}
				continue
			}
		}
		if s.NextRunAt > time.Now().UnixMilli() {
			continue
		}
		jiraProfile, e := m.isJiraProfile(s.ConnectionID)
		if e != nil {
			return nil, e
		}
		continuation := ""
		if previous.Status == Done && previous.AcquisitionID != "" && previous.JobID != "" && !jiraProfile {
			collector := gmail.Collector{Store: m.Store, Config: m.Config}
			more, e := collector.HasContinuation(ctx, previous.AcquisitionID)
			if e != nil {
				return nil, e
			}
			if more {
				continuation = previous.AcquisitionID
			}
		}
		tick, e := m.Store.BeginTick(ctx, s.ID, continuation)
		if e != nil {
			return nil, e
		}
		return m.collect(ctx, s, tick)
	}
	return nil, nil
}

func (m Manager) isJiraProfile(id string) (bool, error) {
	path, err := jira.ProfilePath(id)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(filepath.Join(m.Store.Root, path))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func (m Manager) collect(ctx context.Context, s store.Schedule, tick store.Tick) (*store.Tick, error) {
	jiraProfile, err := m.isJiraProfile(s.ConnectionID)
	if err != nil {
		return nil, err
	}
	if jiraProfile {
		return m.collectJira(ctx, s, tick)
	}
	persist := func() (*store.Tick, error) { return &tick, m.Store.SaveTick(context.WithoutCancel(ctx), tick) }
	connection, err := gmail.LoadConnection(m.Store.Root, s.ConnectionID, m.Config)
	if err != nil {
		tick.Status = WaitingAuth
		tick.Diagnostic = "connection unavailable; repair it and explicitly retry this occurrence"
		return persist()
	}
	if connection.Policy.Timezone != s.Timezone {
		tick.Status = Blocked
		tick.Diagnostic = "connection timezone changed; review and replace the schedule"
		return persist()
	}
	collector := gmail.Collector{Store: m.Store, Config: m.Config, ReservedID: tick.AcquisitionID}
	a, readErr := m.Store.Acquisition(ctx, tick.AcquisitionID)
	if readErr != nil && !errors.Is(readErr, sql.ErrNoRows) {
		return nil, readErr
	}
	var collectErr error
	if readErr != nil || (a.Status != "collected" && a.Status != "partial") {
		if tick.CollectionAttempts >= m.Config.Limits.MaxAttempts {
			tick.Status = Blocked
			tick.Diagnostic = "collection attempt budget exhausted; review the limit or explicitly skip this occurrence"
			return persist()
		}
		tick.CollectionAttempts++
		if err = m.Store.SaveTick(ctx, tick); err != nil {
			return nil, err
		}
		a, collectErr = collector.Collect(ctx, connection, "", tick.ContinueID)
	}
	if collectErr != nil {
		tick.Status = Blocked
		if a.Status == RetryWait {
			tick.Status = RetryWait
			if tick.CollectionAttempts >= m.Config.Limits.MaxAttempts {
				tick.Status = Blocked
			}
		}
		if a.Status == WaitingAuth {
			tick.Status = WaitingAuth
		}
		tick.NotBefore = a.NotBefore
		tick.Diagnostic = collectErr.Error()
		return persist()
	}
	tick.AcquisitionID = a.ID
	j, err := collector.Queue(ctx, a.ID)
	if err != nil {
		tick.Status = Blocked
		tick.Diagnostic = err.Error()
		return persist()
	}
	tick.JobID = j.ID
	tick.Status = Linked
	tick.Diagnostic = ""
	tick.NotBefore = 0
	return persist()
}

func (m Manager) collectJira(ctx context.Context, s store.Schedule, tick store.Tick) (*store.Tick, error) {
	persist := func() (*store.Tick, error) { return &tick, m.Store.SaveTick(context.WithoutCancel(ctx), tick) }
	profile, err := jira.LoadProfile(m.Store.Root, s.ConnectionID, m.Config)
	if err != nil {
		tick.Status = WaitingAuth
		tick.Diagnostic = "Jira profile is unavailable; repair it and explicitly retry this occurrence"
		return persist()
	}
	if profile.Timezone != s.Timezone {
		tick.Status = Blocked
		tick.Diagnostic = "Jira profile timezone changed; review and replace the schedule"
		return persist()
	}
	collector := jira.Collector{Store: m.Store, Config: m.Config, ReservedID: tick.AcquisitionID}
	a, readErr := m.Store.Acquisition(ctx, tick.AcquisitionID)
	if readErr != nil && !errors.Is(readErr, sql.ErrNoRows) {
		return nil, readErr
	}
	var (
		collectErr error
		record     jira.Record
	)
	if readErr != nil || (a.Status != jira.Collected && a.Status != jira.Partial) {
		if tick.CollectionAttempts >= m.Config.Limits.MaxAttempts {
			tick.Status = Blocked
			tick.Diagnostic = "collection attempt budget exhausted; review the limit or explicitly skip this occurrence"
			return persist()
		}
		tick.CollectionAttempts++
		if err = m.Store.SaveTick(ctx, tick); err != nil {
			return nil, err
		}
		if readErr != nil {
			record, collectErr = collector.CollectLive(ctx, profile)
		} else {
			record, collectErr = collector.ResumeLive(ctx, profile, tick.AcquisitionID)
		}
		a = record.Acquisition
	}
	if collectErr != nil {
		tick.Status = Blocked
		if a.Status == store.RetryWait || record.State.StopReason == "retry_wait" {
			tick.Status = RetryWait
		}
		if a.Status == store.WaitingInput || record.State.StopReason == "waiting_auth" {
			tick.Status = WaitingAuth
		}
		tick.NotBefore = a.NotBefore
		tick.Diagnostic = collectErr.Error()
		return persist()
	}
	tick.AcquisitionID = a.ID
	j, err := collector.Queue(ctx, a.ID)
	if err != nil {
		tick.Status = Blocked
		tick.Diagnostic = err.Error()
		return persist()
	}
	tick.JobID = j.ID
	tick.Status = Linked
	tick.Diagnostic = ""
	tick.NotBefore = 0
	return persist()
}

func (m Manager) Retry(ctx context.Context, id string) (store.Tick, error) {
	t, err := m.Store.Tick(ctx, id)
	if err != nil {
		return t, err
	}
	if t.JobID != "" {
		return t, errors.New("this occurrence has a job; resolve or retry that job instead")
	}
	if t.Status != Blocked && t.Status != WaitingAuth && t.Status != RetryWait {
		return t, errors.New("occurrence is not blocked")
	}
	if t.NotBefore > time.Now().UnixMilli() {
		return t, errors.New("provider retry is not yet eligible")
	}
	t.Status = Pending
	t.Diagnostic = "human requested collection retry"
	return t, m.Store.SaveTick(ctx, t)
}

func (m Manager) Acknowledge(ctx context.Context, id, reason string) (store.Tick, error) {
	t, err := m.Store.Tick(ctx, id)
	if err != nil {
		return t, err
	}
	if reason == "" || t.Status == Done || t.Status == Skipped {
		return t, errors.New("an unfinished occurrence and explicit reason are required")
	}
	if t.JobID != "" {
		j, e := m.Store.Job(ctx, t.JobID)
		if e != nil {
			return t, e
		}
		if j.Status == store.Queued || j.Status == store.Running || j.Status == store.RetryWait {
			return t, errors.New("cancel or finish the linked job first")
		}
	}
	// Skipping explicitly abandons this pagination chain, retaining all linkage.
	t.Status = Skipped
	t.Diagnostic = "human acknowledged incomplete occurrence: " + reason
	return t, m.Store.SaveTick(ctx, t)
}
