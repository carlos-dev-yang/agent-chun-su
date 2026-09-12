package service

import (
	"errors"
	"os"
	"path/filepath"

	"chunsu/internal/files"
)

const Worker = "worker"
const Chat = "chat"
const ChatLabelPrefix = "local.chunsu.chat."
const ChatDirectory = "state/chat-service"
const EnabledPath = ChatDirectory + "/enabled"
const WorkerEnabledPath = ChatDirectory + "/worker-enabled"

func directory(kind string) string {
	if kind == Chat {
		return ChatDirectory
	}
	return "state/service"
}
func arguments(executable, root, kind string) []string {
	args := []string{executable, "--home", root}
	if kind == Chat {
		return append(args, "telegram", "supervise")
	}
	return append(args, "worker")
}

// SetEnabled persists user intent separately from current process state.
func SetEnabled(root string, enabled bool) error {
	return setMarker(root, EnabledPath, enabled)
}
func SetWorkerEnabled(root string, enabled bool) error {
	return setMarker(root, WorkerEnabledPath, enabled)
}
func WorkerEnabled(root string) (bool, error) { return marker(root, WorkerEnabledPath) }
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
		return false, errors.New("invalid chat service desired state")
	}
	return true, nil
}
func removeRegistration(root, kind string) error {
	if kind != Chat {
		return files.RemoveTree(root, directory(kind))
	}
	// Retain process/recovery records and diagnostics when unregistering a chat.
	for _, name := range []string{"registration.json", "launch-agent.plist", "worker.service"} {
		if err := files.RemoveTree(root, filepath.Join(directory(kind), name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
