package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"chunsu/internal/chatsupervisor"
	"chunsu/internal/config"
	"chunsu/internal/files"
	"chunsu/internal/platform"
	"chunsu/internal/service"
	"chunsu/internal/telegram"
)

const telegramLifecycleDirectory = service.ChatDirectory + "/lifecycle"

type telegramProcesses struct {
	supervisor platform.ProcessIdentity
	receiver   platform.ProcessIdentity
}

// telegramLifecycleLock serializes only state-changing chat lifecycle commands.
// Status and rendering remain available while a transition is in progress.
func telegramLifecycleLock(ctx context.Context, root string, c config.Config) (*platform.Lock, error) {
	base := filepath.Join(root, telegramLifecycleDirectory)
	if err := files.PrivateDir(base); err != nil {
		return nil, err
	}
	if err := files.PrivateDir(filepath.Join(base, "state")); err != nil {
		return nil, err
	}
	return platform.Acquire(ctx, base, time.Duration(c.Limits.LockWaitSeconds)*time.Second)
}

func verifiedTelegramRegistration(root string) (service.Definition, bool, error) {
	d, err := service.ReadFor(root, service.Chat)
	if errors.Is(err, os.ErrNotExist) {
		return service.Definition{}, false, nil
	}
	if err != nil {
		return service.Definition{}, false, err
	}
	digest, _, err := files.HashFile(filepath.Dir(d.Path), filepath.Base(d.Path), config.DefaultMaxArtifactBytes)
	if err != nil || digest != d.Digest {
		return service.Definition{}, false, errors.New("installed Telegram definition changed")
	}
	return d, true, nil
}

func telegramOwnership(root string, c config.Config) (telegramProcesses, error) {
	var out telegramProcesses
	supervisor, _, err := chatsupervisor.Read(root, c.Limits)
	if err != nil {
		return out, err
	}
	if supervisor.Identity.PID > 1 {
		active, err := platform.IdentityActive(supervisor.Identity)
		if err != nil {
			return out, err
		}
		if active {
			out.supervisor = supervisor.Identity
		}
	}
	health, _, err := telegram.ReadHealth(root, c.Limits)
	if err != nil {
		return out, err
	}
	if health.Identity.PID > 1 {
		active, err := platform.IdentityActive(health.Identity)
		if err != nil {
			return out, err
		}
		if active {
			out.receiver = health.Identity
		}
	}
	return out, nil
}

func telegramLifecycleBudget(c config.Config) time.Duration {
	return telegram.LifecycleBudget(c.Limits)
}

func telegramLifecyclePoll(c config.Config) time.Duration {
	return min(time.Duration(c.Limits.LockWaitSeconds)*time.Second, telegram.HealthInterval(c.Limits))
}

func stopTelegram(parent context.Context, root string, c config.Config, registered bool, old telegramProcesses) error {
	if err := parent.Err(); err != nil {
		return err
	}
	if registered {
		status, err := service.CommandFor(parent, root, service.Chat, "status", telegramLifecycleBudget(c))
		if err != nil {
			return err
		}
		if status.Running {
			if _, err = service.CommandFor(parent, root, service.Chat, "stop", telegramLifecycleBudget(c)); err != nil {
				return err
			}
		}
	}
	if old.receiver.PID > 1 {
		if err := parent.Err(); err != nil {
			return err
		}
		active, err := platform.IdentityActive(old.receiver)
		if err != nil {
			return err
		}
		if active {
			process, err := os.FindProcess(old.receiver.PID)
			if err != nil {
				return err
			}
			if err = process.Signal(os.Interrupt); err != nil {
				return err
			}
		}
	}
	return waitTelegramStopped(parent, root, c, registered, old)
}

func waitTelegramStopped(parent context.Context, root string, c config.Config, registered bool, old telegramProcesses) error {
	ctx, cancel := context.WithTimeout(parent, telegramLifecycleBudget(c))
	defer cancel()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		stopped, err := telegramStopped(ctx, root, c, registered, old)
		if err != nil {
			return err
		}
		if stopped {
			if err := ctx.Err(); err != nil {
				return err
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(telegramLifecyclePoll(c)):
		}
	}
}

// quiesceTelegramForUpdate uses the same ownership and receiver checks as the
// public lifecycle path but deliberately does not write the enabled marker.
// The updater restores the exact prior intent after its binary swap.
func quiesceTelegramForUpdate(parent context.Context, root string, c config.Config) error {
	lock, err := telegramLifecycleLock(parent, root, c)
	if err != nil {
		return err
	}
	defer lock.Close()
	definition, registered, err := verifiedTelegramRegistration(root)
	_ = definition
	if err != nil {
		return err
	}
	old, err := telegramOwnership(root, c)
	if err != nil {
		return err
	}
	if registered {
		state, err := service.CommandFor(parent, root, service.Chat, "status", telegramLifecycleBudget(c))
		if err != nil {
			return err
		}
		if state.Running {
			if _, err = service.CommandFor(parent, root, service.Chat, "stop", telegramLifecycleBudget(c)); err != nil {
				return err
			}
		}
	}
	if old.receiver.PID > 1 {
		active, err := platform.IdentityActive(old.receiver)
		if err != nil {
			return err
		}
		if active {
			process, err := os.FindProcess(old.receiver.PID)
			if err != nil {
				return err
			}
			if err = process.Signal(os.Interrupt); err != nil {
				return err
			}
		}
	}
	ctx, cancel := context.WithTimeout(parent, telegramLifecycleBudget(c))
	defer cancel()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		stopped := true
		if registered {
			state, err := service.CommandFor(ctx, root, service.Chat, "status", telegramLifecycleBudget(c))
			if err != nil || state.Running {
				stopped = false
			}
		}
		if stopped {
			for _, identity := range []platform.ProcessIdentity{old.supervisor, old.receiver} {
				if identity.PID > 1 {
					active, err := platform.IdentityActive(identity)
					if err != nil || active {
						stopped = false
						break
					}
				}
			}
		}
		if stopped {
			released, err := telegramOwnershipReleased(ctx, root, c)
			if err != nil {
				return err
			}
			if released {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(telegramLifecyclePoll(c)):
		}
	}
}

func resumeTelegramForUpdate(parent context.Context, root string, c config.Config) error {
	lock, err := telegramLifecycleLock(parent, root, c)
	if err != nil {
		return err
	}
	defer lock.Close()
	prior, err := telegramOwnership(root, c)
	if err != nil {
		return err
	}
	_, err = startTelegram(parent, root, c, prior, true)
	return err
}

func telegramStopped(ctx context.Context, root string, c config.Config, registered bool, old telegramProcesses) (bool, error) {
	enabled, err := service.Enabled(root)
	if err != nil {
		return false, err
	}
	if enabled {
		return false, nil
	}
	if registered {
		state, err := service.CommandFor(ctx, root, service.Chat, "status", telegramLifecycleBudget(c))
		if err != nil || state.Running {
			return false, err
		}
	}
	for _, identity := range []platform.ProcessIdentity{old.supervisor, old.receiver} {
		if identity.PID <= 1 {
			continue
		}
		active, err := platform.IdentityActive(identity)
		if err != nil || active {
			return false, err
		}
	}
	return telegramOwnershipReleased(ctx, root, c)
}

// telegramOwnershipReleased briefly probes both process locks. It never holds
// either lock while waiting for a new supervisor or receiver to become ready.
func telegramOwnershipReleased(ctx context.Context, root string, c config.Config) (bool, error) {
	probe, cancel := context.WithTimeout(ctx, telegramLifecyclePoll(c))
	defer cancel()
	base := filepath.Join(root, service.ChatDirectory)
	for _, path := range []string{base, filepath.Join(base, "state")} {
		if err := files.PrivateDir(path); err != nil {
			return false, err
		}
	}
	supervisor, err := platform.Acquire(probe, base, telegramLifecyclePoll(c))
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return false, nil
		}
		return false, err
	}
	if err = supervisor.Close(); err != nil {
		return false, err
	}
	receiver, err := telegram.Lock(probe, root, c.Limits)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return false, nil
		}
		return false, err
	}
	return true, receiver.Close()
}

func startTelegram(parent context.Context, root string, c config.Config, old telegramProcesses, fresh bool) (service.Result, error) {
	started := time.Now().UTC()
	if err := parent.Err(); err != nil {
		return service.Result{}, err
	}
	result, err := service.CommandFor(parent, root, service.Chat, "start", telegramLifecycleBudget(c))
	if err != nil {
		return result, err
	}
	if err = waitTelegramReady(parent, root, c, old, started, fresh); err != nil {
		return result, err
	}
	return result, nil
}

func waitTelegramReady(parent context.Context, root string, c config.Config, old telegramProcesses, started time.Time, fresh bool) error {
	ctx, cancel := context.WithTimeout(parent, telegramLifecycleBudget(c))
	defer cancel()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		ready, err := telegramReady(root, c, old, started, fresh)
		if err != nil {
			return err
		}
		if ready {
			if err := ctx.Err(); err != nil {
				return err
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(telegramLifecyclePoll(c)):
		}
	}
}

func telegramReady(root string, c config.Config, old telegramProcesses, started time.Time, fresh bool) (bool, error) {
	enabled, err := service.Enabled(root)
	if err != nil || !enabled {
		return false, err
	}
	supervisor, supervising, err := chatsupervisor.Read(root, c.Limits)
	if err != nil || !supervising || supervisor.Child.PID <= 1 {
		return false, err
	}
	health, alive, err := telegram.ReadHealth(root, c.Limits)
	if err != nil || !alive || health.Identity != supervisor.Child || health.Poll != "connected" || health.AIBlocked {
		return false, err
	}
	if fresh && (supervisor.Identity == old.supervisor || health.Identity == old.receiver || !health.StartedAt.After(started)) {
		return false, nil
	}
	return true, nil
}
