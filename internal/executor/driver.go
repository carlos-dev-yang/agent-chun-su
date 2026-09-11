package executor

import (
	"context"
	"errors"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/runtimeenv"
	"chunsu/internal/workgroup"
)

// Driver is the provider adapter. Adding a driver requires its own compatibility
// and boundary evidence, without changing reception or review semantics.
type Driver interface {
	Report(context.Context, string, workgroup.Package) (Result, error)
	Structured(context.Context, StructuredRequest) (Result, error)
}

type StructuredRequest struct {
	Role      string
	Root      string
	Directory string
	Executor  config.Executor
	Limits    config.Limits
	Prompt    []byte
	Schema    []byte
	Skill     []byte
}

type codexDriver struct{}

func (codexDriver) Report(ctx context.Context, root string, p workgroup.Package) (Result, error) {
	return runCodexReport(ctx, root, p)
}
func (codexDriver) Structured(ctx context.Context, r StructuredRequest) (Result, error) {
	return runCodexStructured(ctx, r.Role, r.Root, r.Directory, r.Executor, r.Limits, r.Prompt, r.Schema, r.Skill)
}

func Select(kind string) (Driver, error) {
	if kind == "codex" {
		return codexDriver{}, nil
	}
	return nil, errors.New("AI driver is not implemented or selected; configure an available driver")
}

type Compatibility struct {
	Driver          string `json:"driver"`
	Environment     string `json:"environment"`
	Model           string `json:"model"`
	ObservedVersion string `json:"observed_version,omitempty"`
	Status          string `json:"status"`
	Detail          string `json:"detail,omitempty"`
}

// Inspect checks installed compatibility without model traffic or authentication.
// It is a prerequisite check, not proof that a new host's isolation has passed.
func Inspect(ctx context.Context, root string, selected config.Executor, limits config.Limits) Compatibility {
	result := Compatibility{Driver: selected.Kind, Environment: selected.Environment, Model: selected.Model, Status: "unavailable"}
	if result.Environment == "" {
		result.Environment = runtimeenv.NativeRestricted
	}
	if _, err := selection(selected, root); err != nil {
		result.Detail = err.Error()
		return result
	}
	checkCtx, cancel := context.WithTimeout(ctx, time.Duration(limits.LockWaitSeconds)*time.Second)
	defer cancel()
	version, err := Version(checkCtx, selected.Path)
	result.ObservedVersion = version
	if err != nil {
		result.Detail = err.Error()
		return result
	}
	if version != TestedVersion || selected.Model != TestedModel {
		result.Status = "needs_revalidation"
		result.Detail = "this initial driver is temporarily pinned to " + TestedVersion + " / " + TestedModel
		return result
	}
	result.Status = "prerequisites_match"
	result.Detail = "authentication and enforcement on this host require their own checks"
	return result
}

func selection(selected config.Executor, root string) (Driver, error) {
	driver, err := Select(selected.Kind)
	if err != nil {
		return nil, err
	}
	environment, err := runtimeenv.Select(selected.Environment)
	if err != nil {
		return nil, err
	}
	if err = environment.CheckRoot(root); err != nil {
		return nil, err
	}
	return driver, nil
}

func Run(ctx context.Context, root string, p workgroup.Package) (Result, error) {
	driver, err := selection(p.Executor, root)
	if err != nil {
		return Result{ExitCode: -1, Outcome: "not_started"}, err
	}
	return driver.Report(ctx, root, p)
}

func Structured(ctx context.Context, r StructuredRequest) (Result, error) {
	if r.Role != config.RoleReception && r.Role != config.RoleReview {
		return Result{}, errors.New("unsupported structured agent role")
	}
	driver, err := selection(r.Executor, r.Root)
	if err != nil {
		return Result{ExitCode: -1, Outcome: "not_started"}, err
	}
	return driver.Structured(ctx, r)
}

// Converse preserves the original call surface; role owners should use
// Structured with their explicit role and independently selected route.
func Converse(ctx context.Context, root, directory string, selected config.Executor, limits config.Limits, prompt, schema, skill []byte) (Result, error) {
	return Structured(ctx, StructuredRequest{Role: config.RoleReception, Root: root, Directory: directory, Executor: selected, Limits: limits, Prompt: prompt, Schema: schema, Skill: skill})
}
