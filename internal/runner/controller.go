package runner

import (
	"context"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/control"
	"chunsu/internal/platform"
	"chunsu/internal/schedule"
	"chunsu/internal/service"
	"chunsu/internal/store"
)

// ServeController is the single durable SQLite/control owner.  A missing or
// invalid task executor deliberately leaves this server running for recovery.
func ServeController(ctx context.Context, root string, c config.Config, managed bool) error {
	if managed {
		enabled, err := service.ControllerEnabled(root)
		if err != nil {
			return err
		}
		if !enabled {
			return nil
		}
	}
	lock, err := platform.Acquire(ctx, root, time.Duration(c.Limits.LockWaitSeconds)*time.Second)
	if err != nil {
		return err
	}
	defer lock.Close()
	s, err := store.Open(ctx, root, c, false)
	if err != nil {
		return err
	}
	defer s.Close()
	r := &Runner{Store: s, Config: c, WorkerMode: true, controller: true, managed: managed}
	defer r.CloseSetup()
	if _, err = Recover(ctx, s, c); err != nil {
		r.recoveryBlocked = true
		r.workerErr = "recovery_blocked"
	}
	requested, intentErr := service.ControllerWorkerIntent(root)
	if intentErr != nil {
		r.workerErr = "worker_intent_unavailable"
	} else {
		r.workerRequested = requested
	}
	// Hold mutation admission across listener installation and remembered-intent
	// reconciliation. Status bypasses admission, so management readiness stays
	// observable while a missing executor is being inspected.
	r.admission.Lock()
	server, err := control.Listen(ctx, root, c.Limits.MaxArtifactBytes, time.Duration(c.Limits.LockWaitSeconds)*time.Second, r.Handle)
	if err != nil {
		r.admission.Unlock()
		return err
	}
	defer server.Close()
	// IPC readiness is independent of executor validation. A remembered worker
	// intent is reconciled after the control endpoint is available, so an absent
	// task executor cannot make controller start appear to have failed.
	if requested {
		_ = r.setWorker(ctx, true, false)
	}
	r.admission.Unlock()
	ticker := time.NewTicker(time.Duration(c.Limits.PollSeconds) * time.Second)
	defer ticker.Stop()
	for {
		if err := r.step(ctx); err != nil {
			r.blockDispatch("store_unavailable")
		}
		select {
		case <-ctx.Done():
			r.beginStop()
			if managed {
				enabled, err := service.ControllerEnabled(root)
				if err != nil {
					return err
				}
				if enabled {
					return ctx.Err()
				}
			}
			return nil
		case <-ticker.C:
		}
	}
}

func (r *Runner) step(ctx context.Context) error {
	r.mu.Lock()
	dispatch, stopping := r.dispatch, r.stopping
	r.mu.Unlock()
	if stopping || !dispatch {
		return nil
	}
	paused, err := r.Store.Paused(ctx)
	if err != nil || paused {
		return err
	}
	jobs, err := r.Store.Jobs(ctx)
	if err != nil {
		return err
	}
	for i := len(jobs) - 1; i >= 0; i-- {
		j := jobs[i]
		if (j.Status == store.Queued || j.Status == store.RetryWait) && j.NotBefore <= time.Now().UnixMilli() {
			out, runErr := r.Run(ctx, j.ID, "")
			if runErr != nil && out.Status == "" {
				r.mu.Lock()
				dispatchStillEnabled := r.dispatch
				r.mu.Unlock()
				// worker_stop may arrive while Run is returning. It intentionally
				// disables only future dispatch and must not be relabeled as a fault.
				if dispatchStillEnabled {
					r.blockDispatch("dispatch_failed")
				}
			}
			return nil
		}
	}
	r.admission.Lock()
	defer r.admission.Unlock()
	r.mu.Lock()
	stopping, dispatch = r.stopping, r.dispatch
	r.mu.Unlock()
	if stopping || !dispatch {
		return nil
	}
	_, err = (schedule.Manager{Store: r.Store, Config: r.Config}).Step(ctx)
	if err != nil {
		r.blockDispatch("schedule_blocked")
		return nil
	}
	return nil
}

func (r *Runner) blockDispatch(code string) {
	r.mu.Lock()
	r.dispatch = false
	r.workerErr = code
	r.mu.Unlock()
}
