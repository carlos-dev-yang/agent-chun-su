// Package chatlanguage stores the host-owned language selection for ordinary chat replies.
package chatlanguage

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"chunsu/internal/config"
	"chunsu/internal/files"
	"golang.org/x/text/language"
)

const (
	File                     = "preferences/chat.json"
	Version                  = 1
	Auto                     = "auto"
	MaxPreferenceBytes int64 = 4096
)

// Error omits filesystem details and the contents of the preference file.
type Error struct{ Operation string }

func (e *Error) Error() string { return "chat language " + e.Operation + " failed" }

type preference struct {
	Version  int    `json:"version"`
	Language string `json:"language"`
}

func preferenceLimit(limits config.Limits) int64 {
	return min(MaxPreferenceBytes, limits.MaxSourceBytes, limits.MaxArtifactBytes)
}

func canonical(value string) (string, error) {
	if value == Auto {
		return Auto, nil
	}
	tag, err := language.Parse(value)
	if err != nil || tag.String() == language.Und.String() {
		return "", errors.New("invalid reply language")
	}
	return tag.String(), nil
}

func decode(data []byte) (string, error) {
	var saved preference
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&saved); err != nil {
		return "", err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return "", errors.New("unexpected preference data")
	}
	if saved.Version != Version {
		return "", errors.New("unsupported preference version")
	}
	value, err := canonical(saved.Language)
	if err != nil || value != saved.Language {
		return "", errors.New("invalid reply language")
	}
	return value, nil
}

// Load returns auto for an absent preference file. It never rewrites a bad file.
func Load(root string, limits config.Limits) (string, error) {
	limit := preferenceLimit(limits)
	if limit <= 0 {
		return "", &Error{Operation: "read"}
	}
	preferencesDir := filepath.Join(root, filepath.Dir(File))
	if err := files.RequirePrivateDir(preferencesDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Auto, nil
		}
		return "", &Error{Operation: "read"}
	}
	if err := files.RequirePrivateFile(filepath.Join(root, File)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Auto, nil
		}
		return "", &Error{Operation: "read"}
	}
	data, err := files.Read(root, File, limit)
	if errors.Is(err, os.ErrNotExist) {
		return Auto, nil
	}
	if err != nil {
		return "", &Error{Operation: "read"}
	}
	value, err := decode(data)
	if err != nil {
		return "", &Error{Operation: "read"}
	}
	return value, nil
}

func write(root string, limits config.Limits, value, operation string) error {
	limit := preferenceLimit(limits)
	if limit <= 0 {
		return &Error{Operation: operation}
	}
	data, err := json.Marshal(preference{Version: Version, Language: value})
	if err != nil || int64(len(data)) > limit {
		return &Error{Operation: operation}
	}
	if err := files.RequirePrivateDir(root); err != nil {
		return &Error{Operation: operation}
	}
	if err := files.PrivateDir(filepath.Join(root, filepath.Dir(File))); err != nil {
		return &Error{Operation: operation}
	}
	if err := files.Write(root, File, data, true); err != nil {
		return &Error{Operation: operation}
	}
	return nil
}

// Save validates and canonically stores a BCP 47 tag or auto.
func Save(root string, limits config.Limits, value string) (string, error) {
	canonicalValue, err := canonical(strings.TrimSpace(value))
	if err != nil {
		return "", &Error{Operation: "save"}
	}
	if err := write(root, limits, canonicalValue, "save"); err != nil {
		return "", err
	}
	return canonicalValue, nil
}

// Reset repairs the preference file by restoring the default auto selection.
func Reset(root string, limits config.Limits) error {
	return write(root, limits, Auto, "reset")
}

func help() string {
	return "Use /language or /언어 with auto, reset (or 초기화), or a BCP 47 tag; for example, /language auto, /language reset, /language en, or /언어 ko-KR."
}

func current(root string, limits config.Limits, includeUsage bool) (string, bool, error) {
	value, err := Load(root, limits)
	if err != nil {
		return "", true, err
	}
	response := "Current reply language: " + value + "."
	if value == Auto {
		response += " Ordinary replies follow your current language."
	}
	if includeUsage {
		response += "\n" + help()
	}
	return response, true, nil
}

// Handle is stateless so language commands never create a pending menu.
func Handle(root string, limits config.Limits, text string) (string, bool, error) {
	parts := strings.Fields(strings.TrimSpace(text))
	if len(parts) == 0 || (parts[0] != "/언어" && !strings.EqualFold(parts[0], "/language")) {
		return "", false, nil
	}
	if len(parts) == 1 {
		return current(root, limits, true)
	}
	if len(parts) == 2 && (strings.EqualFold(parts[1], "current") || parts[1] == "현재") {
		return current(root, limits, false)
	}
	if len(parts) != 2 {
		return help(), true, nil
	}
	value := strings.ToLower(parts[1])
	switch value {
	case Auto:
		if _, err := Save(root, limits, Auto); err != nil {
			return "", true, err
		}
		return "Reply language set to auto. Ordinary replies will follow your current language starting with the next generated reply.", true, nil
	case "reset", "초기화":
		if err := Reset(root, limits); err != nil {
			return "", true, err
		}
		return "Reply language reset to auto. Ordinary replies will follow your current language starting with the next generated reply.", true, nil
	}
	canonicalValue, err := canonical(parts[1])
	if err != nil {
		return help(), true, nil
	}
	if _, err := Save(root, limits, canonicalValue); err != nil {
		return "", true, err
	}
	return "Reply language set to " + canonicalValue + ". It takes effect with the next generated reply.", true, nil
}
