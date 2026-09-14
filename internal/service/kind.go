package service

import (
	"errors"
	"os"
	"path/filepath"

	"chunsu/internal/files"
)

const Worker = "worker"
const Controller = "controller"
const Chat = "chat"
const Monitor = "monitor"
const ChatLabelPrefix = "local.chunsu.chat."
const MonitorLabelPrefix = "local.chunsu.monitor."
const ChatDirectory = "state/chat-service"
const MonitorDirectory = "state/monitor-service"
const EnabledPath = ChatDirectory + "/enabled"
const MonitorEnabledPath = MonitorDirectory + "/enabled"
const WorkerEnabledPath = ChatDirectory + "/worker-enabled"
const ControllerDirectory = "state/controller-service"
const ControllerEnabledPath = ControllerDirectory + "/enabled"
const ControllerWorkerIntentPath = ControllerDirectory + "/worker-intent"

func directory(kind string) string {
	switch kind {
	case Controller:
		return ControllerDirectory
	case Chat:
		return ChatDirectory
	case Monitor:
		return MonitorDirectory
	default:
		return "state/service"
	}
}
func arguments(executable, root, kind string) []string {
	args := []string{executable, "--home", root}
	if kind == Controller {
		return append(args, "controller", "serve", "--managed")
	}
	if kind == Chat {
		return append(args, "telegram", "supervise")
	}
	if kind == Monitor {
		return append(args, "monitor", "run", "--managed")
	}
	return append(args, "worker")
}

// SetEnabled persists user intent separately from current process state.
func SetEnabled(root string, enabled bool) error {
	return setMarker(root, EnabledPath, enabled)
}
func SetMonitorEnabled(root string, enabled bool) error {
	return setMarker(root, MonitorEnabledPath, enabled)
}
func SetControllerEnabled(root string, enabled bool) error {
	return setMarker(root, ControllerEnabledPath, enabled)
}
func ControllerEnabled(root string) (bool, error) { return marker(root, ControllerEnabledPath) }
func MonitorEnabled(root string) (bool, error)    { return marker(root, MonitorEnabledPath) }
func SetWorkerEnabled(root string, enabled bool) error {
	return setMarker(root, WorkerEnabledPath, enabled)
}
func WorkerEnabled(root string) (bool, error) { return marker(root, WorkerEnabledPath) }

// ControllerWorkerIntent is an explicit desired state.  Its first read imports
// the legacy receiver-owned marker once, then records both enabled and disabled
// state so a later explicit stop cannot be overwritten by migration.
func ControllerWorkerIntent(root string) (bool, error) {
	data, err := files.Read(root, ControllerWorkerIntentPath, int64(len("disabled\n")))
	if errors.Is(err, os.ErrNotExist) {
		legacy, legacyErr := WorkerEnabled(root)
		if legacyErr != nil {
			return false, legacyErr
		}
		if err = SetControllerWorkerIntent(root, legacy); err != nil {
			return false, err
		}
		return legacy, nil
	}
	if err != nil {
		return false, err
	}
	switch string(data) {
	case "enabled\n":
		return true, nil
	case "disabled\n":
		return false, nil
	}
	return false, errors.New("invalid controller worker desired state")
}

func SetControllerWorkerIntent(root string, enabled bool) error {
	state := []byte("disabled\n")
	if enabled {
		state = []byte("enabled\n")
	}
	return files.Write(root, ControllerWorkerIntentPath, state, true)
}
func setMarker(root, path string, enabled bool) error {
	if enabled {
		return files.Write(root, path, []byte("enabled\n"), true)
	}
	if err := files.RequirePrivateFile(filepath.Join(root, path)); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return files.RemoveTree(root, path)
}
func Enabled(root string) (bool, error) {
	return marker(root, EnabledPath)
}
func marker(root, path string) (bool, error) {
	err := files.RequirePrivateFile(filepath.Join(root, path))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	data, err := files.Read(root, path, int64(len("enabled\n")))
	if err != nil {
		return false, err
	}
	if string(data) != "enabled\n" {
		return false, errors.New("invalid service desired state")
	}
	return true, nil
}
func removeRegistration(root, kind string) error {
	if kind == Worker {
		return files.RemoveTree(root, directory(kind))
	}
	// Retain chat or monitor diagnostics when unregistering its OS service.
	for _, name := range []string{"registration.json", "launch-agent.plist", "worker.service"} {
		if err := files.RemoveTree(root, filepath.Join(directory(kind), name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
