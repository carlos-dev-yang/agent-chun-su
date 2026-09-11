package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/files"
)

const SystemctlName = "systemctl"
const LinuxDefinitionPath = "state/service/worker.service"

// systemd expands percent specifiers even in quoted strings; ExecStart also
// expands dollar variables. Values are escaped as data, never shell commands.
func unitValue(value string) string {
	return strconv.Quote(strings.ReplaceAll(value, "%", "%%"))
}

func renderLinux(root string, atLogin bool) (Definition, error) {
	label, path, domain, err := identity(root)
	if err != nil {
		return Definition{}, err
	}
	executable, err := os.Executable()
	if err != nil {
		return Definition{}, err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return Definition{}, err
	}
	c, err := config.Load(root)
	if err != nil {
		return Definition{}, err
	}
	var body strings.Builder
	body.WriteString("[Unit]\nDescription=Chun-su agent controller\nAfter=network-online.target\n\n[Service]\nType=simple\nUMask=0077\nNoNewPrivileges=true\nKillMode=mixed\nRestart=no\n")
	args := []string{executable, "--home", root, "worker"}
	quoted := []string{}
	for _, arg := range args {
		quoted = append(quoted, unitValue(strings.ReplaceAll(arg, "$", "$$")))
	}
	fmt.Fprintf(&body, "ExecStart=%s\nTimeoutStopSec=%d\n", strings.Join(quoted, " "), c.Limits.LockWaitSeconds+c.Limits.PollSeconds)
	for _, key := range hostEnvironmentKeys() {
		if value := os.Getenv(key); value != "" {
			fmt.Fprintf(&body, "Environment=%s\n", unitValue(key+"="+value))
		}
	}
	body.WriteString("\n[Install]\nWantedBy=default.target\n")
	text := body.String()
	return Definition{Label: label, Path: path, Domain: domain, Digest: files.Digest([]byte(text)), AtLogin: atLogin, Body: text}, nil
}

func loginLink(d Definition, remove bool) error {
	path := filepath.Join(filepath.Dir(d.Path), "default.target.wants", filepath.Base(d.Path))
	if target, err := os.Readlink(path); err == nil {
		if target != d.Path {
			return errors.New("existing user-service startup link differs; preserve it for inspection")
		}
		if remove {
			return os.Remove(path)
		}
		if !d.AtLogin {
			return errors.New("unexpected startup link for a manual service; inspect the user service registration")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if remove || !d.AtLogin {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), files.DirMode); err != nil {
		return err
	}
	return os.Symlink(d.Path, path)
}

func systemdCommand(ctx context.Context, root, action string, timeout time.Duration, d Definition) (Result, error) {
	result := Result{Label: d.Label, Action: action}
	program, err := exec.LookPath(SystemctlName)
	if err != nil {
		return result, err
	}
	run := func(args ...string) error {
		callCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		command := exec.CommandContext(callCtx, program, append([]string{"--user"}, args...)...)
		output := &boundedOutput{maximum: MaxCommandOutput}
		command.Stdout = output
		command.Stderr = output
		err := command.Run()
		result.Output = output.String()
		result.ExitCode = 0
		if command.ProcessState != nil {
			result.ExitCode = command.ProcessState.ExitCode()
		}
		if callCtx.Err() != nil {
			return callCtx.Err()
		}
		return err
	}
	unit := filepath.Base(d.Path)
	switch action {
	case "status":
		err = run("is-active", unit)
		// Inactive is status, but a missing user manager is an operational error.
		state := strings.TrimSpace(result.Output)
		if (result.ExitCode == 3 || result.ExitCode == 4) && (state == "inactive" || state == "failed" || state == "unknown") {
			err = nil
		}
	case "start":
		c, e := config.Load(root)
		if e != nil {
			return result, e
		}
		if c.Executor.Kind == "" || !filepath.IsAbs(c.Executor.Path) {
			return result, errors.New("configure an absolute executor path before starting the service")
		}
		if _, e = exec.LookPath(c.Executor.Path); e != nil {
			return result, e
		}
		digest, _, e := files.HashFile(filepath.Dir(d.Path), filepath.Base(d.Path), config.DefaultMaxArtifactBytes)
		if e != nil {
			return result, e
		}
		if digest != d.Digest {
			return result, errors.New("installed service definition changed; inspect it before starting")
		}
		if err = run("daemon-reload"); err != nil {
			return result, err
		}
		err = run("start", unit)
	case "stop":
		err = run("stop", unit)
	default:
		err = errors.New("unsupported service action")
	}
	return result, err
}
