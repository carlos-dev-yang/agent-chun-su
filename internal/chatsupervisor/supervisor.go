// Package chatsupervisor owns one receiver process under an OS user service.
package chatsupervisor

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/errorreport"
	"chunsu/internal/executor"
	"chunsu/internal/files"
	"chunsu/internal/platform"
	"chunsu/internal/service"
	"chunsu/internal/telegram"
	"chunsu/internal/telegramchat"
)

const ProcessPath = service.ChatDirectory + "/receiver.json"
const ControllerPath = service.ChatDirectory + "/controller.json"
const StatusPath = service.ChatDirectory + "/supervisor.json"

type Status struct {
	Version           int                      `json:"version"`
	Identity          platform.ProcessIdentity `json:"identity"`
	At                time.Time                `json:"at"`
	State             string                   `json:"state"`
	Restarts          uint64                   `json:"restarts"`
	Child             platform.ProcessIdentity `json:"child"`
	UnpersistedErrors uint64                   `json:"unpersisted_errors"`
}

func Read(root string, limits config.Limits) (Status, bool, error) {
	var s Status
	data, err := files.Read(root, StatusPath, limits.MaxArtifactBytes)
	if errors.Is(err, os.ErrNotExist) {
		return s, false, nil
	}
	if err != nil {
		return s, false, err
	}
	if err = json.Unmarshal(data, &s); err != nil {
		return s, false, err
	}
	if s.Version != 1 || s.Identity.PID <= 1 || s.Identity.Start == "" {
		return s, false, errors.New("invalid supervisor record")
	}
	actual, err := platform.Identify(s.Identity.PID)
	return s, err == nil && actual == s.Identity && time.Since(s.At) >= 0 && time.Since(s.At) < telegram.StallTimeout(limits) && s.State != "stopped", nil
}

func SaveProcess(root, path, state string, identity platform.ProcessIdentity) error {
	data, err := json.Marshal(executor.ProcessRecord{State: state, Identity: identity})
	if err != nil {
		return err
	}
	return files.Write(root, path, data, true)
}

func cleanup(ctx context.Context, root string, limits config.Limits) error {
	if err := executor.Reconcile(ctx, root, ProcessPath, limits); err != nil {
		return err
	}
	// Another foreground receiver may own this home. Reconcile conversation
	// and controller children only after proving exclusive transport ownership.
	lock, err := telegram.Lock(ctx, root, limits)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := executor.Reconcile(ctx, root, ControllerPath, limits); err != nil {
		return err
	}
	return telegramchat.Reconcile(ctx, root, limits)
}

func Run(ctx context.Context, root string, c config.Config) (runErr error) {
	defer func() {
		// An external termination while intent is still enabled is a failure.
		// Explicit OS stop/shutdown suppresses OS restarts, and our stop command
		// removes intent before signalling, so stopped homes remain stopped.
		if ctx.Err() != nil && runErr == nil {
			if enabled, err := service.Enabled(root); err == nil && enabled {
				runErr = errors.New("chat supervisor interrupted while enabled")
				_, _ = errorreport.New(root, c.Limits).Record(context.Background(), "supervisor_interrupted", errorreport.Correlation{})
			}
		}
	}()
	base := filepath.Join(root, service.ChatDirectory)
	for _, path := range []string{base, filepath.Join(base, "state")} {
		if err := files.PrivateDir(path); err != nil {
			return err
		}
	}
	lock, err := platform.Acquire(ctx, base, time.Duration(c.Limits.LockWaitSeconds)*time.Second)
	if err != nil {
		return err
	}
	defer lock.Close()
	identity, err := platform.Identify(os.Getpid())
	if err != nil {
		return err
	}
	status := Status{Version: 1, Identity: identity, State: "starting"}
	prior, _, priorErr := Read(root, c.Limits)
	if priorErr == nil {
		status.Restarts = prior.Restarts
	}
	reports := errorreport.New(root, c.Limits)
	record := func(code string) { _, _ = reports.Record(context.Background(), code, errorreport.Correlation{}) }
	if priorErr == nil && prior.Identity.PID > 1 && prior.State != "stopped" {
		record("supervisor_recovered")
	}
	write := func(state string) {
		status.At = time.Now().UTC()
		status.State = state
		status.UnpersistedErrors = reports.Unpersisted()
		data, _ := json.Marshal(status)
		if err := files.Write(root, StatusPath, data, true); err != nil {
			record("supervisor_state_failed")
		}
	}
	defer write("stopped")
	binary, err := os.Executable()
	if err != nil {
		return err
	}
	ticker := time.NewTicker(telegram.HealthInterval(c.Limits))
	defer ticker.Stop()
	failures := 0
	for ctx.Err() == nil {
		enabled, e := service.Enabled(root)
		if e == nil && !enabled {
			return nil
		}
		if e != nil {
			record("supervisor_state_failed")
		} else {
			write("recovering")
			e = cleanup(ctx, root, c.Limits)
			if e != nil {
				record("recovery_blocked")
				write("recovery_blocked")
			}
		}
		if e == nil {
			e = SaveProcess(root, ProcessPath, "starting", platform.ProcessIdentity{})
			if e == nil {
				child := exec.Command(binary, "--home", root, "telegram", "--paired-only")
				platform.ProcessGroup(child)
				// Raw executor/OS stderr may include private data. Failures are
				// projected into the fixed error catalog by each process instead.
				child.Stdout = io.Discard
				child.Stderr = io.Discard
				e = child.Start()
				if e != nil {
					_ = SaveProcess(root, ProcessPath, "not_started", platform.ProcessIdentity{})
					record("startup_failed")
				} else {
					status.Child, e = platform.Identify(child.Process.Pid)
					if e == nil {
						e = SaveProcess(root, ProcessPath, "running", status.Child)
					}
					if e != nil {
						_ = platform.KillGroup(child.Process.Pid)
						_ = child.Wait()
						record("supervisor_state_failed")
					} else {
						done := make(chan struct{})
						go func() { _ = child.Wait(); close(done) }()
						started := time.Now()
						stopped := false
						write("running")
						for !stopped {
							select {
							case <-ctx.Done():
								stopped = true
							case <-done:
								record("receiver_exited")
								status.Restarts++
								stopped = true
							case <-ticker.C:
								enabled, enableErr := service.Enabled(root)
								if enableErr != nil {
									record("supervisor_state_failed")
								}
								if enableErr == nil && !enabled {
									stopped = true
									break
								}
								health, alive, healthErr := telegram.ReadHealth(root, c.Limits)
								if healthErr == nil && alive && health.Identity == status.Child {
									if time.Since(started) > time.Duration(c.Limits.RetryDelaySeconds)*time.Second {
										failures = 0
									}
								} else if time.Since(started) > telegram.StallTimeout(c.Limits) {
									record("receiver_stalled")
									status.Restarts++
									stopped = true
								}
								_ = reports.Flush(ctx)
								write("running")
							}
						}
						_ = child.Process.Signal(os.Interrupt)
						timer := time.NewTimer(time.Duration(c.Limits.LockWaitSeconds+telegram.HTTPGraceSeconds) * time.Second)
						select {
						case <-done:
						case <-timer.C:
							_ = platform.KillGroup(child.Process.Pid)
							<-done
						}
						timer.Stop()
						// Reap all separately identified children before another receiver.
						if !platform.GroupExists(status.Child.PID) {
							_ = SaveProcess(root, ProcessPath, "exited", status.Child)
						} else {
							record("recovery_blocked")
						}
						cleanupCtx, cancel := context.WithTimeout(context.Background(), telegram.StallTimeout(c.Limits))
						if e = cleanup(cleanupCtx, root, c.Limits); e != nil {
							record("recovery_blocked")
						}
						cancel()
					}
				}
			} else {
				record("supervisor_state_failed")
			}
		}
		failures++
		write("retry_wait")
		timer := time.NewTimer(telegram.RetryDelay(nil, failures, c.Limits))
		waiting := true
		for waiting {
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil
			case <-timer.C:
				waiting = false
			case <-ticker.C:
				enabled, e := service.Enabled(root)
				if e == nil && !enabled {
					timer.Stop()
					return nil
				}
				write("retry_wait")
			}
		}
	}
	return nil
}
