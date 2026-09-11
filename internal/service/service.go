package service

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/files"
	"chunsu/internal/mail"
	"chunsu/internal/secrets"
)

const RecordPath = "state/service/registration.json"
const DefinitionPath = "state/service/launch-agent.plist"
const LabelPrefix = "local.chunsu.worker."
const LabelDigestLength = 16
const LaunchctlName = "launchctl"
const MaxCommandOutput = 64 << 10

type Definition struct {
	Label   string `json:"label"`
	Path    string `json:"path"`
	Domain  string `json:"domain"`
	Digest  string `json:"digest"`
	AtLogin bool   `json:"at_login"`
	Body    string `json:"body"`
}
type Result struct {
	Label    string `json:"label"`
	Action   string `json:"action"`
	Output   string `json:"output"`
	ExitCode int    `json:"exit_code"`
}

func identity(root string) (label, path, domain string, err error) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		err = errors.New("user-service lifecycle supports macOS and Linux; use an explicitly supported environment")
		return
	}
	if !filepath.IsAbs(root) {
		err = errors.New("service data root must be absolute")
		return
	}
	home, e := os.UserHomeDir()
	if e != nil {
		err = e
		return
	}
	label = LabelPrefix + files.Digest([]byte(root))[:LabelDigestLength]
	if runtime.GOOS == "linux" {
		base, e := os.UserConfigDir()
		if e != nil {
			err = e
			return
		}
		path = filepath.Join(base, "systemd", "user", label+".service")
		domain = "user/" + strconv.Itoa(os.Getuid())
		return
	}
	path = filepath.Join(home, "Library", "LaunchAgents", label+".plist")
	domain = "gui/" + strconv.Itoa(os.Getuid())
	return
}
func escaped(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func Render(root string, atLogin bool) (Definition, error) {
	if runtime.GOOS == "linux" {
		return renderLinux(root, atLogin)
	}
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
	home, err := os.UserHomeDir()
	if err != nil {
		return Definition{}, err
	}
	var b bytes.Buffer
	b.WriteString(xml.Header + `<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n<plist version=\"1.0\"><dict>\n")
	fmt.Fprintf(&b, "<key>Label</key><string>%s</string>\n<key>ProgramArguments</key><array>", escaped(label))
	for _, arg := range []string{executable, "--home", root, "worker"} {
		fmt.Fprintf(&b, "<string>%s</string>", escaped(arg))
	}
	b.WriteString("</array>\n<key>EnvironmentVariables</key><dict>")
	// Persist the explicitly captured user environment; no shell startup is used.
	for _, key := range hostEnvironmentKeys() {
		value := os.Getenv(key)
		if key == "HOME" {
			value = home
		}
		if value != "" {
			fmt.Fprintf(&b, "<key>%s</key><string>%s</string>", key, escaped(value))
		}
	}
	b.WriteString("</dict>\n<key>RunAtLoad</key>")
	if atLogin {
		b.WriteString("<true/>")
	} else {
		b.WriteString("<false/>")
	}
	b.WriteString("\n<key>KeepAlive</key><false/>\n<key>ProcessType</key><string>Background</string>\n</dict></plist>\n")
	body := b.String()
	return Definition{Label: label, Path: path, Domain: domain, Digest: files.Digest([]byte(body)), AtLogin: atLogin, Body: body}, nil
}

func hostEnvironmentKeys() []string {
	return []string{"HOME", "PATH", "TMPDIR", "LANG", "LC_ALL", "CODEX_HOME", "SSL_CERT_FILE", "SSL_CERT_DIR", secrets.HelperEnv, secrets.KeyFileEnv, secrets.StoreDirectoryEnv}
}

func Install(root string, atLogin bool) (Definition, error) {
	d, err := Render(root, atLogin)
	if err != nil {
		return d, err
	}
	// Reuse the reviewed definition on an interrupted installation.
	if existing, e := Read(root); e == nil {
		if existing.AtLogin != atLogin {
			return d, errors.New("service settings differ; remove the existing registration before changing login behavior")
		}
		d = existing
	} else if !errors.Is(e, os.ErrNotExist) {
		return d, e
	}
	definitionPath := DefinitionPath
	if runtime.GOOS == "linux" {
		definitionPath = LinuxDefinitionPath
	}
	if err = files.Write(root, definitionPath, []byte(d.Body), true); err != nil {
		return d, err
	}
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return d, err
	}
	if err = files.Write(root, RecordPath, b, true); err != nil {
		return d, err
	}
	parent := filepath.Dir(d.Path)
	if err = os.MkdirAll(parent, files.DirMode); err != nil {
		return d, err
	}
	if info, e := os.Lstat(d.Path); e == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return d, errors.New("service target is not a regular file")
		}
		digest, _, e := files.HashFile(parent, filepath.Base(d.Path), config.DefaultMaxArtifactBytes)
		if e != nil {
			return d, e
		}
		if digest != d.Digest {
			return d, errors.New("service target differs; existing user configuration was preserved")
		}
		if runtime.GOOS == "linux" {
			return d, loginLink(d, false)
		}
		return d, nil
	} else if !errors.Is(e, os.ErrNotExist) {
		return d, e
	}
	// The launchd filename intentionally differs from our internal definition path.
	err = files.Write(parent, filepath.Base(d.Path), []byte(d.Body), false)
	if err == nil && runtime.GOOS == "linux" {
		err = loginLink(d, false)
	}
	return d, err
}

func Read(root string) (Definition, error) {
	var d Definition
	b, err := files.Read(root, RecordPath, config.DefaultMaxArtifactBytes)
	if err != nil {
		return d, err
	}
	if err = mail.Decode(b, &d); err != nil {
		return d, err
	}
	label, path, domain, err := identity(root)
	if err != nil {
		return d, err
	}
	if d.Label != label || d.Path != path || d.Domain != domain || files.Digest([]byte(d.Body)) != d.Digest {
		return d, errors.New("service registration does not match this root and user")
	}
	return d, nil
}

type boundedOutput struct {
	bytes.Buffer
	maximum int
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	n := len(p)
	remaining := b.maximum - b.Len()
	if remaining > 0 {
		if remaining > n {
			remaining = n
		}
		_, _ = b.Buffer.Write(p[:remaining])
	}
	return n, nil
}

func Command(ctx context.Context, root, action string, timeout time.Duration) (Result, error) {
	d, err := Read(root)
	if err != nil {
		return Result{}, err
	}
	result := Result{Label: d.Label, Action: action}
	if runtime.GOOS == "linux" {
		return systemdCommand(ctx, root, action, timeout, d)
	}
	launchctl, err := exec.LookPath(LaunchctlName)
	if err != nil {
		return result, err
	}
	run := func(args ...string) error {
		callCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		cmd := exec.CommandContext(callCtx, launchctl, args...)
		out := &boundedOutput{maximum: MaxCommandOutput}
		cmd.Stdout = out
		cmd.Stderr = out
		err := cmd.Run()
		result.Output = out.String()
		result.ExitCode = 0
		if cmd.ProcessState != nil {
			result.ExitCode = cmd.ProcessState.ExitCode()
		}
		if callCtx.Err() != nil {
			return callCtx.Err()
		}
		return err
	}
	target := d.Domain + "/" + d.Label
	switch action {
	case "status":
		err = run("print", target)
		// A launchctl exit code is returned explicitly, including not-loaded status.
		var exit *exec.ExitError
		if errors.As(err, &exit) {
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
		if e = run("print", target); e != nil {
			if err = run("bootstrap", d.Domain, d.Path); err != nil {
				return result, err
			}
		}
		err = run("kickstart", target)
	case "stop":
		err = run("bootout", target)
	default:
		err = errors.New("unsupported service action")
	}
	return result, err
}

func Remove(ctx context.Context, root string, timeout time.Duration) (Definition, error) {
	d, err := Read(root)
	if err != nil {
		return d, err
	}
	status, err := Command(ctx, root, "status", timeout)
	if err != nil {
		return d, err
	}
	if status.ExitCode == 0 {
		if _, err = Command(ctx, root, "stop", timeout); err != nil {
			return d, err
		}
	}
	if _, err = os.Lstat(d.Path); err == nil {
		digest, _, e := files.HashFile(filepath.Dir(d.Path), filepath.Base(d.Path), config.DefaultMaxArtifactBytes)
		if e != nil {
			return d, e
		}
		if digest != d.Digest {
			return d, errors.New("installed definition differs; refusing to remove it")
		}
		if runtime.GOOS == "linux" {
			if err = loginLink(d, true); err != nil {
				return d, err
			}
		}
		if err = os.Remove(d.Path); err != nil {
			return d, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return d, err
	}
	return d, files.RemoveTree(root, "state/service")
}
