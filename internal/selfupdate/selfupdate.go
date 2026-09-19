// Package selfupdate implements the deliberately narrow owner-configured
// update path. It never accepts a repository URL, ref, shell fragment, model,
// or configuration change from an update request.
package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"chunsu/internal/backend"
	"chunsu/internal/config"
	"chunsu/internal/files"
	"chunsu/internal/platform"
	"chunsu/internal/secrets"
	"chunsu/internal/service"
	"chunsu/internal/store"
	"chunsu/internal/telegram"
	"chunsu/internal/updateguard"
)

const (
	Version        = 1
	directory      = "state/self-update"
	configName     = "config.json"
	stateName      = "state.json"
	lockName       = "lock"
	master         = "master"
	updateRef      = "refs/chunsu-update/master"
	maxOutput      = 64 << 10
	binaryMaxBytes = 128 << 20
	metadataArg    = "metadata"
	RunTimeout     = 30 * time.Minute
)

type Settings struct {
	Version           int    `json:"version"`
	Repository        string `json:"repository"`
	Origin            string `json:"origin"`
	GoPath            string `json:"go_path"`
	GitPath           string `json:"git_path"`
	InstalledPath     string `json:"installed_path"`
	InstalledDigest   string `json:"installed_digest"`
	InstalledRevision string `json:"installed_revision"`
}

type Notification struct {
	UpdateID  int64 `json:"update_id,omitempty"`
	ChatID    int64 `json:"chat_id,omitempty"`
	Attempted bool  `json:"attempted"`
}

type State struct {
	Version           int          `json:"version"`
	ID                string       `json:"id,omitempty"`
	Status            string       `json:"status"`
	PreviousRevision  string       `json:"previous_revision,omitempty"`
	CandidateRevision string       `json:"candidate_revision,omitempty"`
	StartedAt         time.Time    `json:"started_at,omitempty"`
	CompletedAt       time.Time    `json:"completed_at,omitempty"`
	Message           string       `json:"message,omitempty"`
	Notification      Notification `json:"notification"`
}

type Metadata struct {
	SchemaVersion   int    `json:"schema_version"`
	MigrationDigest string `json:"migration_digest"`
}

type Status struct {
	Configured bool      `json:"configured"`
	Settings   *Settings `json:"-"`
	State      *State    `json:"state,omitempty"`
	Available  bool      `json:"available,omitempty"`
	Revision   string    `json:"revision,omitempty"`
}

func path(root, name string) string { return filepath.Join(root, directory, name) }

func prepare(root string) error { return files.PrivateDir(filepath.Join(root, directory)) }

func readJSON(root, name string, v any, limit int64) error {
	b, err := files.Read(filepath.Join(root, directory), name, limit)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func writeJSON(root, name string, v any) error {
	if err := prepare(root); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return files.Write(filepath.Join(root, directory), name, b, true)
}

func Load(root string, limit int64) (Settings, error) {
	var s Settings
	if err := readJSON(root, configName, &s, limit); err != nil {
		return s, err
	}
	if s.Version != Version || !filepath.IsAbs(s.Repository) || !filepath.IsAbs(s.GoPath) || !filepath.IsAbs(s.GitPath) || !filepath.IsAbs(s.InstalledPath) || len(s.InstalledDigest) != 64 || strings.TrimSpace(s.Origin) == "" || !validRevision(s.InstalledRevision) {
		return s, errors.New("invalid self-update settings")
	}
	return s, nil
}

func ReadState(root string, limit int64) (State, error) {
	var s State
	if err := readJSON(root, stateName, &s, limit); err != nil {
		return s, err
	}
	if s.Version != Version || (s.ID != "" && !files.ValidID(s.ID)) {
		return s, errors.New("invalid self-update state")
	}
	return s, nil
}

func Configure(ctx context.Context, root, repository, goPath, gitPath, installedRevision string, limit int64) (Settings, error) {
	abs, err := filepath.Abs(repository)
	if err != nil {
		return Settings{}, err
	}
	goPath, err = executable(goPath, "go")
	if err != nil {
		return Settings{}, err
	}
	gitPath, err = executable(gitPath, "git")
	if err != nil {
		return Settings{}, err
	}
	if err = validateRepository(ctx, abs, gitPath); err != nil {
		return Settings{}, err
	}
	origin, err := git(ctx, gitPath, abs, "remote", "get-url", "origin")
	if err != nil {
		return Settings{}, errors.New("configured repository must have an origin remote")
	}
	if !validRevision(installedRevision) {
		return Settings{}, errors.New("--installed-revision must be the verified installed source revision")
	}
	revision, err := git(ctx, gitPath, abs, "rev-parse", installedRevision+"^{commit}")
	if err != nil || strings.TrimSpace(revision) != installedRevision {
		return Settings{}, errors.New("installed revision is not available in the configured repository")
	}
	installed, digest, err := installedExecutable()
	if err != nil {
		return Settings{}, err
	}
	s := Settings{Version: Version, Repository: abs, Origin: strings.TrimSpace(origin), GoPath: goPath, GitPath: gitPath, InstalledPath: installed, InstalledDigest: digest, InstalledRevision: installedRevision}
	if err = writeJSON(root, configName, s); err != nil {
		return Settings{}, err
	}
	return s, nil
}

func Check(ctx context.Context, root string, limit int64) (Status, error) {
	settings, err := Load(root, limit)
	if err != nil {
		return Status{}, err
	}
	if err = validateConfiguredRepository(ctx, settings); err != nil {
		return Status{Configured: true, Settings: &settings}, err
	}
	if _, err = git(ctx, settings.GitPath, settings.Repository, "fetch", "--no-tags", "origin", "refs/heads/"+master+":"+updateRef); err != nil {
		return Status{Configured: true, Settings: &settings}, err
	}
	revision, err := git(ctx, settings.GitPath, settings.Repository, "rev-parse", updateRef)
	if err != nil {
		return Status{Configured: true, Settings: &settings}, err
	}
	result := Status{Configured: true, Settings: &settings, Available: strings.TrimSpace(revision) != settings.InstalledRevision, Revision: strings.TrimSpace(revision)}
	if state, stateErr := ReadState(root, limit); stateErr == nil {
		result.State = &state
	} else if !errors.Is(stateErr, os.ErrNotExist) {
		return result, stateErr
	}
	return result, nil
}

func Inspect(root string, limit int64) (Status, error) {
	settings, err := Load(root, limit)
	if errors.Is(err, os.ErrNotExist) {
		return Status{}, nil
	}
	if err != nil {
		return Status{}, err
	}
	out := Status{Configured: true, Settings: &settings}
	if state, err := ReadState(root, limit); err == nil {
		out.State = &state
	} else if !errors.Is(err, os.ErrNotExist) {
		return out, err
	}
	return out, nil
}

func SelfMetadata() (Metadata, error) {
	digest, err := store.MigrationDigest()
	return Metadata{SchemaVersion: store.SchemaVersion, MigrationDigest: digest}, err
}

func Launch(ctx context.Context, root string, updateID, chatID int64) (string, error) {
	if runtime.GOOS != "linux" {
		return "", errors.New("self-update activation requires a Linux systemd user manager")
	}
	if updateID < 0 || chatID <= 0 {
		return "", errors.New("an acknowledged paired Telegram request is required")
	}
	if _, err := Load(root, config.DefaultMaxArtifactBytes); err != nil {
		return "", err
	}
	receipt, err := telegram.ReadReceipt(root, updateID, config.DefaultMaxArtifactBytes)
	if err != nil || receipt.State != "completed" || receipt.ReplyDigest == "" {
		return "", errors.New("update request receipt is not confirmed")
	}
	id := files.ID()
	return launch(ctx, root, id, updateID, chatID)
}

// LaunchManual uses the same independent user service as a Telegram request,
// but intentionally has no completion-notification destination.
func LaunchManual(ctx context.Context, root string) (string, error) {
	return launch(ctx, root, files.ID(), 0, 0)
}

func launch(ctx context.Context, root, id string, updateID, chatID int64) (string, error) {
	binary, err := os.Executable()
	if err != nil {
		return "", err
	}
	if err = prepare(root); err != nil {
		return "", err
	}
	runnerDir := filepath.Join(root, directory, "runners", id)
	if err = files.PrivateDir(runnerDir); err != nil {
		return "", err
	}
	runner := filepath.Join(runnerDir, "chunsu")
	source, err := os.Open(binary)
	if err != nil {
		return "", err
	}
	defer source.Close()
	target, err := os.OpenFile(runner, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		return "", err
	}
	if _, err = io.Copy(target, source); err != nil {
		_ = target.Close()
		return "", err
	}
	if err = target.Close(); err != nil {
		return "", err
	}
	manager, err := exec.LookPath("systemd-run")
	if err != nil {
		return "", errors.New("systemd-run is required for an independent update service")
	}
	unit := "chunsu-update-" + id
	command := exec.CommandContext(ctx, manager, "--user", "--collect", "--quiet", "--property", "RuntimeMaxSec=1800", "--unit", unit, runner, "--home", root, "update", "run", "--id", id, "--update-id", fmt.Sprint(updateID), "--chat-id", fmt.Sprint(chatID))
	var output boundedWriter
	command.Stdout, command.Stderr = &output, &output
	err = command.Run()
	if err != nil {
		return "", boundedError("could not queue independent update service", output.Bytes(), err)
	}
	return id, nil
}

func Run(ctx context.Context, root, id string, updateID, chatID int64, c config.Config) (out State, resultErr error) {
	if !files.ValidID(id) {
		return out, errors.New("invalid update request ID")
	}
	if err := prepare(root); err != nil {
		return out, err
	}
	lockDir := filepath.Join(root, directory)
	if err := files.PrivateDir(filepath.Join(lockDir, "state")); err != nil {
		return out, err
	}
	lock, err := platform.Acquire(ctx, lockDir, time.Duration(c.Limits.LockWaitSeconds)*time.Second)
	if err != nil {
		return out, errors.New("another update or recovery is in progress")
	}
	defer lock.Close()
	if prior, stateErr := ReadState(root, c.Limits.MaxArtifactBytes); stateErr == nil {
		if prior.Status == "running" || prior.Status == "recovery_required" {
			return prior, errors.New("an interrupted update requires local reconciliation before another apply")
		}
	} else if !errors.Is(stateErr, os.ErrNotExist) {
		return out, errors.New("self-update state must be repaired locally before another apply")
	}
	settings, err := Load(root, c.Limits.MaxArtifactBytes)
	if err != nil {
		return out, err
	}
	if err = validateConfiguredRepository(ctx, settings); err != nil {
		return out, err
	}
	if updateID > 0 && func() bool {
		receipt, e := telegram.ReadReceipt(root, updateID, c.Limits.MaxArtifactBytes)
		return e != nil || receipt.State != "completed" || receipt.ReplyDigest == ""
	}() {
		return out, errors.New("update request receipt is not confirmed")
	}
	state := State{Version: Version, ID: id, Status: "running", PreviousRevision: settings.InstalledRevision, StartedAt: time.Now().UTC(), Notification: Notification{UpdateID: updateID, ChatID: chatID}}
	if err = writeJSON(root, stateName, state); err != nil {
		return out, err
	}
	defer func() {
		if resultErr != nil && state.Status == "running" {
			state.Status, state.Message = "failed", safeMessage(resultErr)
			state.CompletedAt = time.Now().UTC()
			_ = writeJSON(root, stateName, state)
		}
		if resultErr != nil && chatID > 0 && !state.Notification.Attempted {
			// A completion/failure notice is attempted once after the accepted
			// Telegram receipt. An uncertain send is deliberately never retried.
			_ = notify(context.Background(), root, c, &state, "Update did not complete. Check /update status and /errors.")
		}
		out = state
	}()
	if _, err = git(ctx, settings.GitPath, settings.Repository, "fetch", "--no-tags", "origin", "refs/heads/"+master+":"+updateRef); err != nil {
		return state, err
	}
	candidateRevision, err := git(ctx, settings.GitPath, settings.Repository, "rev-parse", updateRef)
	if err != nil {
		return state, err
	}
	state.CandidateRevision = strings.TrimSpace(candidateRevision)
	if state.CandidateRevision == settings.InstalledRevision {
		state.Status, state.Message, state.CompletedAt = "completed", "already_current", time.Now().UTC()
		resultErr = writeJSON(root, stateName, state)
		if resultErr == nil && chatID > 0 {
			resultErr = notify(ctx, root, c, &state, "Update completed: already current.")
		}
		return state, resultErr
	}
	candidate, cleanup, err := buildCandidate(ctx, settings.Repository, state.CandidateRevision, settings.GoPath, settings.GitPath)
	if err != nil {
		return state, err
	}
	defer cleanup()
	if err = verifyCandidate(ctx, candidate, root, c); err != nil {
		return state, err
	}
	if err = idle(ctx, root, c); err != nil {
		return state, err
	}
	// From this point a termination can leave installed files and service state
	// split. Preserve that fact durably instead of allowing another apply.
	state.Message = "activation_started"
	if err = writeJSON(root, stateName, state); err != nil {
		return state, err
	}
	if err = install(ctx, candidate, root, c, &state, settings); err != nil {
		if state.Status != "failed" {
			state.Status = "recovery_required"
			state.Message, state.CompletedAt = "activation_interrupted", time.Now().UTC()
			_ = writeJSON(root, stateName, state)
		}
		return state, err
	}
	settings.InstalledRevision = state.CandidateRevision
	if settings.InstalledDigest, _, err = files.HashFile(filepath.Dir(settings.InstalledPath), filepath.Base(settings.InstalledPath), binaryMaxBytes); err != nil {
		state.Status = "recovery_required"
		return state, err
	}
	if err = writeJSON(root, configName, settings); err != nil {
		state.Status, state.Message, state.CompletedAt = "recovery_required", "installed_revision_not_persisted", time.Now().UTC()
		_ = writeJSON(root, stateName, state)
		return state, err
	}
	state.Status, state.Message, state.CompletedAt = "completed", "installed", time.Now().UTC()
	if err = writeJSON(root, stateName, state); err != nil {
		state.Status, state.Message = "recovery_required", "completion_not_persisted"
		_ = writeJSON(root, stateName, state)
		return state, err
	}
	if chatID > 0 {
		resultErr = notify(ctx, root, c, &state, "Update completed: "+short(state.CandidateRevision))
	}
	return state, resultErr
}

func validateRepository(ctx context.Context, repository, gitPath string) error {
	if info, err := os.Stat(repository); err != nil || !info.IsDir() {
		return errors.New("repository must be an existing directory")
	}
	branch, err := git(ctx, gitPath, repository, "branch", "--show-current")
	if err != nil || strings.TrimSpace(branch) != master {
		return errors.New("repository must be on master")
	}
	if dirty, err := git(ctx, gitPath, repository, "status", "--porcelain"); err != nil || strings.TrimSpace(dirty) != "" {
		return errors.New("repository must have no local changes")
	}
	_, err = git(ctx, gitPath, repository, "rev-parse", "--verify", "refs/remotes/origin/"+master)
	return err
}

func validateConfiguredRepository(ctx context.Context, s Settings) error {
	if err := validateRepository(ctx, s.Repository, s.GitPath); err != nil {
		return err
	}
	origin, err := git(ctx, s.GitPath, s.Repository, "remote", "get-url", "origin")
	if err != nil || strings.TrimSpace(origin) != s.Origin {
		return errors.New("configured repository origin changed")
	}
	return nil
}

func validRevision(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, b := range value {
		if !strings.ContainsRune("0123456789abcdef", b) {
			return false
		}
	}
	return true
}

func executable(value, name string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("--%s-path is required; choose the installed %s executable explicitly", name, name)
	}
	abs, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil || info.IsDir() || info.Mode()&0111 == 0 {
		return "", fmt.Errorf("%s path must name an executable regular file", name)
	}
	return abs, nil
}

func installedExecutable() (string, string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", "", err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil || filepath.Base(path) != "chunsu" {
		return "", "", errors.New("installed command must be named chunsu")
	}
	digest, _, err := files.HashFile(filepath.Dir(path), filepath.Base(path), binaryMaxBytes)
	if err != nil {
		return "", "", err
	}
	return path, digest, nil
}

func verifyInstalled(s Settings) error {
	info, err := os.Stat(s.InstalledPath)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("configured installed command changed")
	}
	digest, _, err := files.HashFile(filepath.Dir(s.InstalledPath), filepath.Base(s.InstalledPath), binaryMaxBytes)
	if err != nil || digest != s.InstalledDigest {
		return errors.New("configured installed command changed")
	}
	return nil
}

func git(ctx context.Context, gitPath, repository string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, gitPath, append([]string{"-C", repository}, args...)...)
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var output boundedWriter
	command.Stdout, command.Stderr = &output, &output
	err := command.Run()
	if err != nil {
		return "", boundedError("git operation failed", output.Bytes(), err)
	}
	return string(output.Bytes()), nil
}

func buildCandidate(ctx context.Context, repository, revision, goPath, gitPath string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "chunsu-update-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	archive := exec.CommandContext(ctx, gitPath, "-C", repository, "archive", "--format=tar", revision)
	untar := exec.CommandContext(ctx, "tar", "-x", "-C", dir)
	pipe, err := archive.StdoutPipe()
	if err != nil {
		cleanup()
		return "", nil, err
	}
	untar.Stdin = pipe
	var archiveErr, tarErr boundedWriter
	archive.Stderr, untar.Stderr = &archiveErr, &tarErr
	if err = untar.Start(); err != nil {
		cleanup()
		return "", nil, err
	}
	if err = archive.Start(); err != nil {
		cleanup()
		return "", nil, err
	}
	archiveWait := archive.Wait()
	tarWait := untar.Wait()
	if archiveWait != nil || tarWait != nil {
		cleanup()
		return "", nil, errors.Join(boundedError("source archive failed", archiveErr.Bytes(), archiveWait), boundedError("source extraction failed", tarErr.Bytes(), tarWait))
	}
	for _, spec := range []struct{ output, pkg string }{{"chunsu", "./cmd/chunsu"}} {
		cmd := exec.CommandContext(ctx, goPath, "build", "-o", filepath.Join(dir, spec.output), spec.pkg)
		cmd.Dir, cmd.Env = dir, append(os.Environ(), "PATH="+os.Getenv("PATH"))
		var output boundedWriter
		cmd.Stdout, cmd.Stderr = &output, &output
		e := cmd.Run()
		if e != nil {
			cleanup()
			return "", nil, boundedError("candidate build failed", output.Bytes(), e)
		}
	}
	return dir, cleanup, nil
}

func verifyCandidate(ctx context.Context, candidate, root string, c config.Config) error {
	// This opens the production database query-only. It proves the live data is
	// already at the running schema; candidate setup/migration is never invoked.
	live, err := store.OpenReadOnly(ctx, root)
	if err != nil {
		return errors.New("production database schema is not eligible for self-update")
	}
	if err = live.Close(); err != nil {
		return err
	}
	current, err := SelfMetadata()
	if err != nil {
		return err
	}
	meta, err := candidateMetadata(ctx, filepath.Join(candidate, "chunsu"), root)
	if err != nil {
		return err
	}
	if meta != current {
		return errors.New("candidate database metadata differs; schema migrations are not allowed through self-update")
	}
	cmd := exec.CommandContext(ctx, filepath.Join(candidate, "chunsu"), "--home", root, "doctor")
	var output boundedWriter
	cmd.Stdout, cmd.Stderr = &output, &output
	err = cmd.Run()
	if err != nil {
		return boundedError("candidate read-only readiness check failed", output.Bytes(), err)
	}
	return nil
}

func candidateMetadata(ctx context.Context, binary, root string) (Metadata, error) {
	var meta Metadata
	cmd := exec.CommandContext(ctx, binary, "--home", root, "update", metadataArg)
	output, err := cmd.Output()
	if err != nil {
		return meta, err
	}
	if err = json.Unmarshal(output, &meta); err != nil {
		return meta, err
	}
	if meta.SchemaVersion < 1 || meta.MigrationDigest == "" {
		return meta, errors.New("candidate metadata is invalid")
	}
	return meta, nil
}

func idle(ctx context.Context, root string, c config.Config) error {
	status, err := backend.Command(ctx, root, c, backend.Controller, "status")
	if err != nil {
		return errors.New("backend status must be readable before update activation")
	}
	if status.Status.ActiveJob != "" {
		return errors.New("backend is busy; wait for active work to finish")
	}
	health, alive, err := telegram.ReadHealth(root, c.Limits)
	if err != nil {
		return errors.New("chat health must be readable before update activation")
	}
	if alive && health.ActiveUpdate != 0 {
		return errors.New("chat is handling a request; retry after its reply")
	}
	return nil
}

func install(ctx context.Context, candidate, root string, c config.Config, state *State, settings Settings) (resultErr error) {
	if err := verifyInstalled(settings); err != nil {
		return err
	}
	lock, err := acquireBinaryLock(ctx, settings.InstalledPath, time.Duration(c.Limits.LockWaitSeconds)*time.Second)
	if err != nil {
		return errors.New("another installed-binary update is in progress")
	}
	defer lock.Close()
	if err = idle(ctx, root, c); err != nil {
		return err
	}
	chatEnabled, err := service.Enabled(root)
	if err != nil {
		return err
	}
	controllerEnabled, err := service.ControllerEnabled(root)
	if err != nil {
		return err
	}
	workerEnabled, err := service.ControllerWorkerIntent(root)
	if err != nil {
		return err
	}
	monitorEnabled, err := service.MonitorEnabled(root)
	if err != nil {
		return err
	}
	backup := filepath.Join(filepath.Dir(settings.InstalledPath), ".chunsu-update", "backups", state.ID)
	if err = os.MkdirAll(backup, 0700); err != nil {
		return err
	}
	oldMain := filepath.Join(backup, "chunsu")
	// Copy the complete rollback artifact before disturbing services or target.
	if err = copyPrivate(oldMain, settings.InstalledPath); err != nil {
		return err
	}
	activation, err := updateguard.Acquire(ctx, root, state.ID, time.Duration(c.Limits.LockWaitSeconds)*time.Second)
	if err != nil {
		return err
	}
	defer activation.Close()
	run := func(args ...string) ([]byte, error) {
		return runInstalled(ctx, settings.InstalledPath, root, c, state.ID, activation.Record.Nonce, args...)
	}
	stopped := false
	restoreAll := func() error {
		if !stopped {
			return nil
		}
		// Stop every candidate component before restoring its executable bytes.
		if chatEnabled {
			if _, stopErr := run("update", "chat-quiesce"); stopErr != nil {
				return stopErr
			}
		}
		if monitorEnabled {
			if stopErr := quiesceService(ctx, root, c, service.Monitor); stopErr != nil {
				return stopErr
			}
		}
		if controllerEnabled {
			if _, stopErr := run("controller", "stop"); stopErr != nil {
				return stopErr
			}
		}
		if err := restore(settings.InstalledPath, oldMain); err != nil {
			return err
		}
		if controllerEnabled {
			if _, err := run("controller", "start"); err != nil {
				return err
			}
			if err := verifyController(ctx, settings.InstalledPath, root, c, state.ID, activation.Record.Nonce, workerEnabled); err != nil {
				return err
			}
		}
		if workerEnabled {
			if err := service.SetControllerWorkerIntent(root, true); err != nil {
				return err
			}
		}
		if monitorEnabled {
			if err := resumeService(ctx, root, c, service.Monitor); err != nil {
				return err
			}
			if err := verifyMonitor(ctx, root, c); err != nil {
				return err
			}
		}
		if chatEnabled {
			if _, err := run("update", "chat-resume"); err != nil {
				return err
			}
			if err := verifyChat(root, c); err != nil {
				return err
			}
		}
		return nil
	}
	defer func() {
		if resultErr != nil && stopped {
			if rollbackErr := restoreAll(); rollbackErr != nil {
				resultErr = fmt.Errorf("activation rollback requires local recovery: %w", errors.Join(resultErr, rollbackErr))
			} else {
				state.Status = "failed"
				state.Message = "rolled_back"
				state.CompletedAt = time.Now().UTC()
				_ = writeJSON(root, stateName, *state)
			}
		}
	}()
	// Stop chat after its acknowledged request so no new admission/config/control
	// command can race the critical swap. It is restarted last.
	if chatEnabled {
		stopped = true
		if _, err = run("update", "chat-quiesce"); err != nil {
			return err
		}
	}
	if !stopped {
		stopped = true
	}
	if err = idle(ctx, root, c); err != nil {
		return err
	}
	if controllerEnabled {
		if _, err = run("controller", "stop"); err != nil {
			return err
		}
	}
	if monitorEnabled {
		if err = quiesceService(ctx, root, c, service.Monitor); err != nil {
			return err
		}
	}
	// With controller and chat quiesced, the existing root lock fences ordinary
	// configuration and store writers during the binary replacement. It must be
	// released before starting a controller, which owns the same lock itself.
	rootLock, err := platform.Acquire(ctx, root, time.Duration(c.Limits.LockWaitSeconds)*time.Second)
	if err != nil {
		return err
	}
	if err = replace(settings.InstalledPath, filepath.Join(candidate, "chunsu")); err != nil {
		_ = rootLock.Close()
		return err
	}
	if err = rootLock.Close(); err != nil {
		return err
	}
	if controllerEnabled {
		if _, err = run("controller", "start"); err != nil {
			return err
		}
		if err = verifyController(ctx, settings.InstalledPath, root, c, state.ID, activation.Record.Nonce, workerEnabled); err != nil {
			return err
		}
	}
	if workerEnabled {
		if err = service.SetControllerWorkerIntent(root, true); err != nil {
			return err
		}
	}
	if monitorEnabled {
		if err = resumeService(ctx, root, c, service.Monitor); err != nil {
			return err
		}
		if err = verifyMonitor(ctx, root, c); err != nil {
			return err
		}
	}
	if chatEnabled {
		if _, err = run("update", "chat-resume"); err != nil {
			return err
		}
		if err = verifyChat(root, c); err != nil {
			return err
		}
	}
	return nil
}

// These service operations preserve the separate enabled markers. Public
// telegram/monitor stop commands intentionally change user intent and cannot
// be used as an updater quiesce primitive.
func quiesceService(ctx context.Context, root string, c config.Config, kind string) error {
	d, err := service.ReadFor(root, kind)
	if err != nil {
		return err
	}
	_ = d
	timeout := time.Duration(c.Limits.LockWaitSeconds+c.Limits.PollSeconds) * time.Second
	status, err := service.CommandFor(ctx, root, kind, "status", timeout)
	if err != nil {
		return err
	}
	if status.Running {
		_, err = service.CommandFor(ctx, root, kind, "stop", timeout)
	}
	return err
}
func resumeService(ctx context.Context, root string, c config.Config, kind string) error {
	d, err := service.ReadFor(root, kind)
	if err != nil {
		return err
	}
	_ = d
	timeout := time.Duration(c.Limits.LockWaitSeconds+c.Limits.PollSeconds) * time.Second
	_, err = service.CommandFor(ctx, root, kind, "start", timeout)
	return err
}

func verifyChat(root string, c config.Config) error {
	_, alive, err := telegram.ReadHealth(root, c.Limits)
	if err != nil || !alive {
		return errors.New("chat readiness was not verified")
	}
	return nil
}
func verifyMonitor(ctx context.Context, root string, c config.Config) error {
	d, err := service.ReadFor(root, service.Monitor)
	if err != nil {
		return errors.New("monitor readiness was not verified")
	}
	timeout := time.Duration(c.Limits.LockWaitSeconds+c.Limits.PollSeconds) * time.Second
	status, err := service.CommandFor(ctx, root, service.Monitor, "status", timeout)
	if err != nil || !status.Running || d.Kind != service.Monitor {
		return errors.New("monitor readiness was not verified")
	}
	return nil
}

type binaryLock struct{ file *os.File }

func acquireBinaryLock(ctx context.Context, target string, timeout time.Duration) (*binaryLock, error) {
	dir := filepath.Join(filepath.Dir(target), ".chunsu-update")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "activation.lock"), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
			return &binaryLock{f}, nil
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, ctx.Err()
		case <-deadline.C:
			_ = f.Close()
			return nil, err
		case <-tick.C:
		}
	}
}
func (l *binaryLock) Close() error {
	_ = syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	return l.file.Close()
}

func runInstalled(ctx context.Context, binary, root string, c config.Config, id, nonce string, args ...string) ([]byte, error) {
	timeout := time.Duration(c.Limits.LockWaitSeconds+c.Limits.PollSeconds) * time.Second
	if len(args) == 2 && args[0] == "update" && (args[1] == "chat-quiesce" || args[1] == "chat-resume") {
		// Account for the helper's own lifecycle budget plus the outer process
		// startup/lock handoff; normal backend commands keep their short bound.
		timeout = max(timeout, telegram.LifecycleBudget(c.Limits)+time.Duration(c.Limits.LockWaitSeconds)*time.Second)
	}
	call, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	prefix := []string{"--home", root, "--update-activation-id", id, "--update-activation-nonce", nonce}
	cmd := exec.CommandContext(call, binary, append(prefix, args...)...)
	var output boundedWriter
	cmd.Stdout, cmd.Stderr = &output, &output
	err := cmd.Run()
	if call.Err() != nil {
		return output.Bytes(), call.Err()
	}
	return output.Bytes(), boundedError("installed lifecycle command failed", output.Bytes(), err)
}

func verifyController(ctx context.Context, binary, root string, c config.Config, id, nonce string, workerRequested bool) error {
	output, err := runInstalled(ctx, binary, root, c, id, nonce, "controller", "status")
	if err != nil {
		return err
	}
	var value struct {
		Status struct {
			ControllerReady   bool `json:"controller_ready"`
			ControllerRunning bool `json:"controller_running"`
			ControllerManaged bool `json:"controller_managed"`
			WorkerRequested   bool `json:"worker_requested"`
		} `json:"status"`
	}
	if json.Unmarshal(output, &value) != nil || !value.Status.ControllerReady || !value.Status.ControllerRunning || !value.Status.ControllerManaged || value.Status.WorkerRequested != workerRequested {
		return errors.New("candidate controller readiness was not verified")
	}
	return nil
}

func copyPrivate(target, source string) error {
	if _, err := os.Stat(target); err == nil {
		return errors.New("rollback binary already exists")
	}
	from, err := os.Open(source)
	if err != nil {
		return err
	}
	defer from.Close()
	to, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		return err
	}
	_, err = io.Copy(to, from)
	if err == nil {
		err = to.Sync()
	}
	closeErr := to.Close()
	if err != nil || closeErr != nil {
		return errors.Join(err, closeErr)
	}
	return nil
}

func replace(target, candidate string) error {
	info, err := os.Stat(target)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("installed binary must be a regular file")
	}
	staging, err := os.CreateTemp(filepath.Dir(target), ".chunsu-update-new.")
	if err != nil {
		return err
	}
	stagingName := staging.Name()
	defer os.Remove(stagingName)
	source, err := os.Open(candidate)
	if err != nil {
		_ = staging.Close()
		return err
	}
	_, copyErr := io.Copy(staging, source)
	closeErr := source.Close()
	if copyErr == nil {
		copyErr = staging.Sync()
	}
	if closeErr == nil {
		closeErr = staging.Close()
	}
	if copyErr != nil || closeErr != nil {
		return errors.Join(copyErr, closeErr)
	}
	if err = os.Chmod(stagingName, 0755); err != nil {
		return err
	}
	return os.Rename(stagingName, target)
}
func restore(target, backup string) error {
	return replace(target, backup)
}

func notify(ctx context.Context, root string, c config.Config, state *State, text string) error {
	if state.Notification.Attempted {
		return nil
	}
	state.Notification.Attempted = true
	if err := writeJSON(root, stateName, *state); err != nil {
		return err
	}
	binding, err := telegram.Load(root, c.Limits.MaxArtifactBytes)
	if err != nil || binding.ChatID != state.Notification.ChatID {
		return errors.New("completion notification could not verify the paired chat")
	}
	store, err := secrets.Open()
	if err != nil {
		return err
	}
	token, err := store.Get(ctx, binding.TokenRef)
	if err != nil {
		return err
	}
	client, err := telegram.NewClient(binding.APIBase, token, c.Limits)
	if err != nil {
		return err
	}
	defer client.Close()
	_, err = client.Send(ctx, binding.ChatID, text)
	return err
}

type boundedWriter struct{ b []byte }

func (w *boundedWriter) Write(p []byte) (int, error) {
	n := len(p)
	left := maxOutput - len(w.b)
	if left > 0 {
		if len(p) > left {
			p = p[:left]
		}
		w.b = append(w.b, p...)
	}
	return n, nil
}
func (w *boundedWriter) Bytes() []byte { return w.b }
func boundedError(prefix string, _ []byte, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", prefix, err)
}
func safeMessage(err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	switch {
	case strings.Contains(text, "database schema"), strings.Contains(text, "migration"):
		return "schema_rejected"
	case strings.Contains(text, "candidate build"):
		return "candidate_build_failed"
	case strings.Contains(text, "busy"), strings.Contains(text, "handling a request"):
		return "busy"
	case strings.Contains(text, "rollback"):
		return "rollback_failed"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "interrupted"
	default:
		return "operation_failed"
	}
}
func short(revision string) string {
	if len(revision) > 12 {
		return revision[:12]
	}
	return revision
}

var _ io.Writer = (*boundedWriter)(nil)
