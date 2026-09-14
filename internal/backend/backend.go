// Package backend owns the durable local controller lifecycle.
package backend

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"chunsu/internal/audit"
	"chunsu/internal/config"
	"chunsu/internal/control"
	"chunsu/internal/files"
	"chunsu/internal/runner"
	"chunsu/internal/service"
)

const (
	Controller    = "controller"
	Worker        = "worker"
	lifecyclePath = "state/backend-lifecycle.lock"
	pollInterval  = 50 * time.Millisecond
)

type Status struct {
	Owner                   string `json:"owner,omitempty"`
	ActiveJob               string `json:"active_job,omitempty"`
	QueuePaused             bool   `json:"queue_paused"`
	WorkerRunning           bool   `json:"worker_running"`
	ControllerReady         bool   `json:"controller_ready"`
	ControllerManaged       bool   `json:"controller_managed"`
	ControllerRunning       bool   `json:"controller_running"`
	ControllerServiceLoaded bool   `json:"controller_service_loaded"`
	WorkerRequested         bool   `json:"worker_requested"`
	WorkerError             string `json:"worker_error"`
}
type RuntimeStatus = Status
type Result struct {
	Message string `json:"message"`
	Status  Status `json:"status"`
}

func Serve(ctx context.Context, root string, c config.Config, managed bool) error {
	return runner.ServeController(ctx, root, c, managed)
}

func Command(ctx context.Context, root string, c config.Config, component, operation string) (Result, error) {
	if component != Controller && component != Worker {
		return Result{Message: "That runtime component is unavailable."}, errors.New("unsupported backend component")
	}
	if operation != "start" && operation != "stop" && operation != "restart" && operation != "status" {
		return Result{Message: "That runtime operation is unavailable."}, errors.New("unsupported backend operation")
	}
	// A read must never queue behind a lifecycle mutation.
	if operation == "status" {
		return observe(ctx, root, c)
	}
	lock, err := acquireLifecycle(ctx, root, lifecycleTimeout(c))
	if err != nil {
		return Result{Message: "Another runtime change is still in progress."}, err
	}
	defer lock.Close()
	if component == Controller {
		return controllerCommand(ctx, root, c, operation)
	}
	return workerCommand(ctx, root, c, operation)
}

func controllerCommand(ctx context.Context, root string, c config.Config, operation string) (out Result, resultErr error) {
	auditLifecycle(root, Controller, operation, "requested")
	defer func() { auditLifecycle(root, Controller, operation, auditResult(resultErr)) }()
	d, registrationErr := service.ReadFor(root, service.Controller)
	if registrationErr != nil && !errors.Is(registrationErr, os.ErrNotExist) {
		return Result{Message: "The controller service could not be verified."}, registrationErr
	}
	if registrationErr == nil {
		if err := verifyDefinition(d); err != nil {
			return Result{Message: "The controller service could not be verified."}, err
		}
	}
	status, endpoint, probeErr := call(ctx, root, c, "status")
	if endpoint && probeErr != nil {
		return Result{Message: "The controller endpoint could not be verified."}, probeErr
	}
	if endpoint && !status.ControllerManaged {
		return Result{Message: "A foreground controller owns this data home. Use its local controls."}, errors.New("foreign controller")
	}
	if endpoint && registrationErr != nil {
		return Result{Message: "The running controller is not a verified managed service."}, errors.New("controller registration missing")
	}
	if operation == "start" && endpoint && status.ControllerReady {
		if err := service.SetControllerEnabled(root, true); err != nil {
			return Result{Message: "The controller service could not be prepared."}, err
		}
		opCtx, cancel := bounded(ctx, lifecycleTimeout(c))
		serviceState, serviceErr := service.CommandFor(opCtx, root, service.Controller, "status", lifecycleTimeout(c))
		cancel()
		status.ControllerServiceLoaded = serviceErr == nil && serviceState.Running
		status.ControllerRunning = true
		return Result{Message: "The controller is ready.", Status: status}, nil
	}
	if !endpoint {
		busy, err := controllerLockBusy(root)
		if err != nil {
			return Result{Message: "The existing controller ownership could not be verified."}, err
		}
		if busy {
			return Result{Message: "Another controller owns this data home. Use its local controls."}, errors.New("foreign controller")
		}
	}

	if operation == "stop" || operation == "restart" {
		if registrationErr != nil {
			if operation == "stop" {
				if err := service.SetControllerEnabled(root, false); err != nil {
					return Result{Message: "The controller stop intent could not be recorded."}, err
				}
				return Result{Message: "The controller is stopped."}, nil
			}
		} else {
			quiesced := false
			var priorEnabled *bool
			if operation == "stop" {
				enabled, markerErr := service.ControllerEnabled(root)
				if markerErr != nil {
					return Result{Message: "The controller stop intent could not be verified."}, markerErr
				}
				priorEnabled = &enabled
			}
			if endpoint {
				if _, _, err := call(ctx, root, c, "controller_stop"); err != nil {
					return Result{Message: "The controller cannot be stopped until its current work is resolved."}, err
				}
				quiesced = true
			}
			// Explicit stop persists even if the service is already inactive; that
			// prevents launchd/systemd from restoring it later.
			if operation == "stop" {
				if err := service.SetControllerEnabled(root, false); err != nil {
					if quiesced && rollbackQuiesce(root, c, priorEnabled) != nil {
						return Result{Message: "The controller stop could not be confirmed and recovery could not be fully restored. Check local runtime diagnostics."}, err
					}
					return Result{Message: "The controller service could not be stopped."}, err
				}
			}
			opCtx, cancel := bounded(ctx, lifecycleTimeout(c))
			state, stateErr := service.CommandFor(opCtx, root, service.Controller, "status", lifecycleTimeout(c))
			cancel()
			if stateErr != nil {
				if quiesced && rollbackQuiesce(root, c, priorEnabled) != nil {
					return Result{Message: "The controller stop could not be confirmed and recovery could not be fully restored. Check local runtime diagnostics."}, stateErr
				}
				return Result{Message: "The controller service could not be verified."}, stateErr
			}
			if state.Running {
				opCtx, cancel = bounded(ctx, lifecycleTimeout(c))
				_, stopErr := service.CommandFor(opCtx, root, service.Controller, "stop", lifecycleTimeout(c))
				cancel()
				if stopErr != nil {
					if quiesced && rollbackQuiesce(root, c, priorEnabled) != nil {
						return Result{Message: "The controller stop could not be confirmed and recovery could not be fully restored. Check local runtime diagnostics."}, stopErr
					}
					return Result{Message: "The controller service could not be stopped."}, stopErr
				}
			}
			if operation == "stop" {
				if err := waitStopped(ctx, root, c); err != nil {
					if quiesced && rollbackQuiesce(root, c, priorEnabled) != nil {
						return Result{Message: "The controller stop could not be confirmed and recovery could not be fully restored. Check local runtime diagnostics."}, err
					}
					return Result{Message: "The controller service could not be stopped."}, err
				}
				return Result{Message: "The controller is stopped."}, nil
			}
			if err := waitStopped(ctx, root, c); err != nil {
				if quiesced && rollbackQuiesce(root, c, priorEnabled) != nil {
					return Result{Message: "The controller stop could not be confirmed and recovery could not be fully restored. Check local runtime diagnostics."}, err
				}
				return Result{Message: "The controller service could not be stopped."}, err
			}
		}
	}
	busy, err := controllerLockBusy(root)
	if err != nil {
		return Result{Message: "The existing controller ownership could not be verified."}, err
	}
	if busy {
		return Result{Message: "Another controller owns this data home. Use its local controls."}, errors.New("foreign controller")
	}
	if registrationErr != nil {
		if _, err := service.InstallFor(root, service.Controller, true); err != nil {
			return Result{Message: "The controller service could not be prepared."}, err
		}
	}
	if err := service.SetControllerEnabled(root, true); err != nil {
		return Result{Message: "The controller service could not be prepared."}, err
	}
	opCtx, cancel := bounded(ctx, lifecycleTimeout(c))
	_, startErr := service.CommandFor(opCtx, root, service.Controller, "start", lifecycleTimeout(c))
	cancel()
	if startErr != nil {
		return Result{Message: "The controller service could not be started."}, startErr
	}
	readyCtx, readyCancel := bounded(ctx, lifecycleTimeout(c))
	defer readyCancel()
	for {
		ready, err := observe(readyCtx, root, c)
		if err == nil && ready.Status.ControllerRunning && ready.Status.ControllerReady && ready.Status.ControllerManaged {
			ready.Message = "The controller is ready."
			return ready, nil
		}
		select {
		case <-readyCtx.Done():
			return Result{Message: "The controller did not become ready."}, readyCtx.Err()
		case <-time.After(pollInterval):
		}
	}
}

func workerCommand(ctx context.Context, root string, c config.Config, operation string) (out Result, resultErr error) {
	auditLifecycle(root, Worker, operation, "requested")
	defer func() { auditLifecycle(root, Worker, operation, auditResult(resultErr)) }()
	status, endpoint, err := call(ctx, root, c, "worker_"+operation)
	if !endpoint {
		return Result{Message: "The controller is unavailable. Start the controller, then try again."}, errors.New("controller unavailable")
	}
	if err != nil {
		if observed, observeErr := observe(ctx, root, c); observeErr == nil {
			status = observed.Status
		}
		return Result{Message: workerMessage(err.Error(), status), Status: status}, err
	}
	switch operation {
	case "start":
		return Result{Message: "The worker is ready to accept new work.", Status: status}, nil
	case "stop":
		return Result{Message: "The worker will not start new work; an active job may finish.", Status: status}, nil
	case "restart":
		return Result{Message: "The worker was restarted and is ready to accept new work.", Status: status}, nil
	}
	return Result{Message: "The worker request failed. Check controller status."}, errors.New("unsupported worker operation")
}

func workerMessage(code string, status Status) string {
	if code == "" {
		code = status.WorkerError
	}
	switch code {
	case "worker_busy":
		return "The worker cannot change while active work or setup is in progress."
	case "controller_stopping":
		return "The controller is stopping and cannot change worker dispatch."
	case "recovery_blocked":
		return "The controller is ready, but recovery must be reviewed before work can start."
	case "config_unavailable", "worker_not_configured":
		return "The controller is ready, but the worker needs configuration."
	case "worker_intent_unavailable":
		return "The controller is ready, but the saved worker setting needs attention."
	case "worker_prerequisites_unavailable":
		return "The controller is ready, but the worker prerequisites need attention."
	case "dispatch_failed", "schedule_blocked", "store_unavailable":
		return "The controller is ready, but worker dispatch is blocked for review."
	}
	return "The worker request failed. Check controller status."
}

func observe(ctx context.Context, root string, c config.Config) (Result, error) {
	loaded := false
	if _, err := service.ReadFor(root, service.Controller); err == nil {
		opCtx, cancel := bounded(ctx, lifecycleTimeout(c))
		state, stateErr := service.CommandFor(opCtx, root, service.Controller, "status", lifecycleTimeout(c))
		cancel()
		loaded = stateErr == nil && state.Running
	}
	status, endpoint, err := call(ctx, root, c, "status")
	if !endpoint {
		return Result{Message: "The controller is not running.", Status: Status{ControllerServiceLoaded: loaded}}, nil
	}
	if err != nil {
		return Result{Message: "The controller endpoint could not be verified.", Status: Status{ControllerServiceLoaded: loaded}}, err
	}
	status.ControllerRunning, status.ControllerServiceLoaded = true, loaded
	message := "The controller is ready."
	if !status.WorkerRunning && status.WorkerError != "" {
		message = workerMessage(status.WorkerError, status)
	}
	return Result{Message: message, Status: status}, nil
}

// call treats malformed or legacy replies as endpoint-present failures. Each
// field is pointer-validated so omitted false/empty values cannot pass status.
func call(ctx context.Context, root string, c config.Config, operation string) (Status, bool, error) {
	callCtx, cancel := bounded(ctx, lifecycleTimeout(c))
	defer cancel()
	data, handled, err := control.Call(callCtx, root, lifecycleTimeout(c), c.Limits.MaxArtifactBytes, control.Request{Operation: operation})
	if err != nil {
		return Status{}, handled, err
	}
	if !handled {
		return Status{}, false, nil
	}
	var wire struct {
		Owner             *string `json:"owner"`
		ActiveJob         *string `json:"active_job"`
		QueuePaused       *bool   `json:"queue_paused"`
		WorkerRunning     *bool   `json:"worker_running"`
		ControllerReady   *bool   `json:"controller_ready"`
		ControllerManaged *bool   `json:"controller_managed"`
		ControllerRunning *bool   `json:"controller_running"`
		WorkerRequested   *bool   `json:"worker_requested"`
		WorkerError       *string `json:"worker_error"`
	}
	if err := json.Unmarshal(data, &wire); err != nil || wire.Owner == nil || *wire.Owner != "controller" || wire.ActiveJob == nil || wire.QueuePaused == nil || wire.WorkerRunning == nil || wire.ControllerReady == nil || wire.ControllerManaged == nil || wire.ControllerRunning == nil || wire.WorkerRequested == nil || wire.WorkerError == nil {
		return Status{}, true, errors.New("invalid controller status")
	}
	return Status{Owner: *wire.Owner, ActiveJob: *wire.ActiveJob, QueuePaused: *wire.QueuePaused, WorkerRunning: *wire.WorkerRunning, ControllerReady: *wire.ControllerReady, ControllerManaged: *wire.ControllerManaged, ControllerRunning: *wire.ControllerRunning, WorkerRequested: *wire.WorkerRequested, WorkerError: *wire.WorkerError}, true, nil
}

func verifyDefinition(d service.Definition) error {
	digest, _, err := files.HashFile(filepath.Dir(d.Path), filepath.Base(d.Path), config.DefaultMaxArtifactBytes)
	if err != nil {
		return err
	}
	if digest != d.Digest {
		return errors.New("installed controller definition changed")
	}
	return nil
}
func rollbackQuiesce(root string, c config.Config, priorEnabled *bool) error {
	if priorEnabled != nil {
		if err := service.SetControllerEnabled(root, *priorEnabled); err != nil {
			return err
		}
	}
	return abortStop(root, c)
}

func abortStop(root string, c config.Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), lifecycleTimeout(c))
	defer cancel()
	_, endpoint, err := call(ctx, root, c, "controller_abort_stop")
	if !endpoint && err == nil {
		return errors.New("controller endpoint disappeared during rollback")
	}
	return err
}

// waitStopped proves both that the management endpoint is absent and that no
// writer still owns the root before a restart can install a new service state.
func waitStopped(parent context.Context, root string, c config.Config) error {
	ctx, cancel := bounded(parent, lifecycleTimeout(c))
	defer cancel()
	for {
		_, endpoint, _ := call(ctx, root, c, "status")
		busy, lockErr := controllerLockBusy(root)
		if lockErr != nil {
			return lockErr
		}
		if !endpoint && !busy {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.New("controller stop completion could not be confirmed")
		case <-time.After(pollInterval):
		}
	}
}
func bounded(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, d)
}
func lifecycleTimeout(c config.Config) time.Duration {
	return time.Duration(c.Limits.LockWaitSeconds) * time.Second
}
func auditResult(err error) string {
	if err != nil {
		return "failed"
	}
	return "succeeded"
}
func auditLifecycle(root, component, operation, outcome string) {
	_ = audit.Record(root, "backend."+component+"."+operation+"."+outcome, "", "")
}

type lifecycleLock struct{ file *os.File }

func acquireLifecycle(ctx context.Context, root string, wait time.Duration) (*lifecycleLock, error) {
	if err := files.RequirePrivateDir(root); err != nil {
		return nil, err
	}
	if err := files.RequirePrivateDir(filepath.Join(root, "state")); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(root, lifecyclePath), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, files.FileMode)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		f.Close()
		return nil, errors.New("backend lifecycle lock is not private")
	}
	if err := files.RequireOwner(info); err != nil {
		f.Close()
		return nil, err
	}
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	for {
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
			return &lifecycleLock{f}, nil
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, ctx.Err()
		case <-deadline.C:
			f.Close()
			return nil, errors.New("backend lifecycle lock busy")
		case <-time.After(pollInterval):
		}
	}
}
func (l *lifecycleLock) Close() error {
	_ = syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	return l.file.Close()
}
func controllerLockBusy(root string) (bool, error) {
	f, err := os.OpenFile(filepath.Join(root, "state", "controller.lock"), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, files.FileMode)
	if err != nil {
		return false, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return false, errors.New("controller lock is not private")
	}
	if err := files.RequireOwner(info); err != nil {
		return false, err
	}
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		return false, nil
	}
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
		return true, nil
	}
	return false, err
}
