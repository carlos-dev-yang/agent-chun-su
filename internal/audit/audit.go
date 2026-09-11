// Package audit records host-observed ownership without treating payload actor
// names or AI text as authentication. It does not attest physical human presence.
package audit

import (
	"chunsu/internal/files"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const Directory = "state/audit"
const Version = 1
const FilenameTimeFormat = "20060102T150405.000000000Z"

type Event struct {
	Version   int    `json:"version"`
	ID        string `json:"id"`
	At        string `json:"at"`
	OSUserID  int    `json:"os_user_id"`
	ProcessID int    `json:"process_id"`
	Action    string `json:"action"`
	Subject   string `json:"subject,omitempty"`
	Digest    string `json:"digest,omitempty"`
}

func Record(root, action, subject, digest string) error {
	if err := files.RequirePrivateDir(root); err != nil {
		return err
	}
	if err := files.PrivateDir(filepath.Join(root, Directory)); err != nil {
		return err
	}
	event := Event{Version: Version, ID: files.ID(), At: time.Now().UTC().Format(time.RFC3339Nano), OSUserID: os.Geteuid(), ProcessID: os.Getpid(), Action: action, Subject: subject, Digest: digest}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return files.Write(root, filepath.Join(Directory, time.Now().UTC().Format(FilenameTimeFormat)+"-"+event.ID+".json"), data, false)
}
