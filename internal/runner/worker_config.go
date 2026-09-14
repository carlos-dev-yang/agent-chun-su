package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	"chunsu/internal/config"
	"chunsu/internal/workerconfig"
)

type workerConfigRequest struct {
	Operation string `json:"operation"`
	Value     string `json:"value"`
}

func (r *Runner) workerConfig(ctx context.Context, input json.RawMessage) (workerconfig.Result, error) {
	request, err := decodeWorkerConfigRequest(input)
	if err != nil {
		return workerconfig.Result{}, err
	}
	if request.Operation != "source" && request.Operation != "model" {
		return workerconfig.Result{}, errors.New("worker configuration operation is unavailable")
	}
	if err = r.workerConfigAllowed(); err != nil {
		return workerconfig.Result{}, err
	}
	current, err := config.Load(r.Store.Root)
	if err != nil {
		return workerconfig.Result{}, errors.New("task route configuration is unavailable")
	}
	updated := current
	var next config.Executor
	switch request.Operation {
	case "model":
		if err = workerconfig.ValidateModel(request.Value); err != nil {
			return workerconfig.Result{}, err
		}
		if current.Executor.Model == request.Value {
			return workerConfigUnchanged(current), nil
		}
		next = current.Executor
		next.Model = request.Value
		if err = workerconfig.ValidateExecutor(next); err != nil {
			return workerconfig.Result{}, err
		}
		next.RevokeDisclosure()
	case "source":
		source, sourceErr := workerConfigSource(current, request.Value)
		if sourceErr != nil {
			return workerconfig.Result{}, sourceErr
		}
		if sameWorkerIdentity(current.Executor, source) {
			return workerConfigUnchanged(current), nil
		}
		if err = workerconfig.ValidateExecutor(source); err != nil {
			return workerconfig.Result{}, err
		}
		// A source is a snapshot of its execution identity only. Its source-data
		// disclosure grants and Jira policy proof never transfer to the task route.
		next = config.Executor{Kind: source.Kind, Path: source.Path, Model: source.Model, Environment: source.Environment}
		next.RevokeDisclosure()
	}
	preserveInheritedRoutes(&updated)
	updated.Executor = next
	if err = config.Save(r.Store.Root, updated); err != nil {
		return workerconfig.Result{}, errors.New("task route configuration could not be saved")
	}
	r.mu.Lock()
	r.Config = updated
	if r.workerErr == "worker_not_configured" || r.workerErr == "config_unavailable" {
		r.workerErr = ""
	}
	r.mu.Unlock()
	if r.setupHost != nil {
		r.setupHost.Config = updated
	}
	return workerconfig.Result{Message: "The task worker configuration was saved. Use /worker start when ready.", Settings: workerconfig.Project(updated)}, nil
}

func decodeWorkerConfigRequest(input json.RawMessage) (workerConfigRequest, error) {
	var request workerConfigRequest
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return request, errors.New("invalid worker configuration request")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return request, errors.New("invalid worker configuration request")
	}
	return request, nil
}

func (r *Runner) workerConfigAllowed() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopping {
		return errors.New("controller_stopping")
	}
	if r.recoveryBlocked {
		return errors.New("recovery_blocked")
	}
	if r.active != "" || (r.setupHost != nil && r.setupHost.Busy()) {
		return errors.New("worker_busy")
	}
	if r.dispatch || r.workerRequested {
		return errors.New("worker_stop_required")
	}
	return nil
}

func workerConfigSource(c config.Config, value string) (config.Executor, error) {
	switch value {
	case config.RoleReception:
		if c.Routes.Reception != nil {
			return *c.Routes.Reception, nil
		}
	case config.RoleReview:
		if c.Routes.Review != nil {
			return *c.Routes.Review, nil
		}
	}
	return config.Executor{}, errors.New("requested worker configuration source is unavailable")
}

func preserveInheritedRoutes(c *config.Config) {
	old := c.Executor
	if c.Routes.Reception == nil {
		snapshot := old
		c.Routes.Reception = &snapshot
	}
	if c.Routes.Review == nil {
		snapshot := old
		c.Routes.Review = &snapshot
	}
}

func sameWorkerIdentity(a, b config.Executor) bool {
	return a.Kind == b.Kind && a.Path == b.Path && a.Model == b.Model && a.Environment == b.Environment
}

func workerConfigUnchanged(c config.Config) workerconfig.Result {
	return workerconfig.Result{Message: "The task worker configuration is unchanged.", Settings: workerconfig.Project(c)}
}
