package telegram

import (
	"encoding/json"
	"errors"
	"os"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/files"
	"chunsu/internal/mail"
	"chunsu/internal/platform"
)

const HealthFile = "health.json"
const MissedHealthIntervals = 3

// Health is written by the intake event loop, not by a detached watchdog. A
// blocked event loop therefore stops advancing its heartbeat.
type Health struct {
	Version           int                      `json:"version"`
	Identity          platform.ProcessIdentity `json:"identity"`
	At                time.Time                `json:"at"`
	StartedAt         time.Time                `json:"started_at"`
	State             string                   `json:"state"`
	Poll              string                   `json:"poll"`
	LastPollAt        time.Time                `json:"last_poll_at,omitempty"`
	LastMessageAt     time.Time                `json:"last_message_at,omitempty"`
	LastReplyAt       time.Time                `json:"last_reply_at,omitempty"`
	ActiveUpdate      int64                    `json:"active_update,omitempty"`
	SessionID         string                   `json:"session_id,omitempty"`
	AIBlocked         bool                     `json:"ai_blocked"`
	UnpersistedErrors uint64                   `json:"unpersisted_errors"`
}

func StallTimeout(limits config.Limits) time.Duration {
	// One bounded multipart send plus controller calls must fit inside the lease.
	ioBudget := time.Duration((PollSeconds+HTTPGraceSeconds)*(MaxReplyUnits/MessageUnits+1)+limits.LockWaitSeconds*4) * time.Second
	return max(ioBudget, HealthInterval(limits)*MissedHealthIntervals)
}

func HealthInterval(limits config.Limits) time.Duration {
	return time.Duration(max(limits.PollSeconds, HTTPGraceSeconds)) * time.Second
}

func WriteHealth(root string, h Health) error {
	h.Version = Version
	h.At = time.Now().UTC()
	raw, err := json.Marshal(h)
	if err != nil {
		return err
	}
	return files.Write(Directory(root), HealthFile, raw, true)
}

func ReadHealth(root string, limits config.Limits) (Health, bool, error) {
	var h Health
	raw, err := files.Read(Directory(root), HealthFile, limits.MaxArtifactBytes)
	if errors.Is(err, os.ErrNotExist) {
		return h, false, nil
	}
	if err != nil {
		return h, false, err
	}
	if err = mail.Decode(raw, &h); err != nil {
		return h, false, err
	}
	if h.Version != Version || h.Identity.PID <= 1 || h.Identity.Start == "" {
		return h, false, errors.New("invalid chat health identity")
	}
	actual, err := platform.Identify(h.Identity.PID)
	alive := err == nil && actual == h.Identity && time.Since(h.At) >= 0 && time.Since(h.At) < StallTimeout(limits) && h.State != "stopped"
	return h, alive, nil
}
