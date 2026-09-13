// Package chatstyle stores and selects a host-owned response-tone instruction.
package chatstyle

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"chunsu/internal/config"
	"chunsu/internal/files"
)

const StyleFile = "instructions/chat-tone.md"
const MaxStyleBytes int64 = 4096
const defaultChoice = "0"

var (
	ErrEmpty   = errors.New("chat tone must not be empty")
	ErrInvalid = errors.New("chat tone must be valid UTF-8")
	ErrTooLong = errors.New("chat tone exceeds its byte limit")
)

// Error omits filesystem details and user-authored instruction contents.
type Error struct{ Operation string }

func (e *Error) Error() string { return "chat tone " + e.Operation + " failed" }

//go:embed presets/*.md
var presetFiles embed.FS

type preset struct{ Choice, Name, File string }

var presets = []preset{
	{"1", "Polite formal", "presets/polite.md"},
	{"2", "Relaxed polite", "presets/casual-honorific.md"},
	{"3", "Friendly informal", "presets/friendly-informal.md"},
	{"4", "Concise work style", "presets/concise-work.md"},
}

type pending uint8

const (
	pendingNone pending = iota
	pendingSelection
	pendingCustom
)

// Dialogue keeps uncommitted menu state in memory. Markdown is the only durable setting.
type Dialogue struct{ pending pending }

func (d *Dialogue) Pending() bool { return d.pending != pendingNone }

func styleLimit(limits config.Limits) int64 {
	return min(MaxStyleBytes, limits.MaxSourceBytes, limits.MaxArtifactBytes)
}

func normalize(text string, limits config.Limits) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", ErrEmpty
	}
	if !utf8.ValidString(text) {
		return "", ErrInvalid
	}
	if int64(len(text)) > styleLimit(limits) {
		return "", ErrTooLong
	}
	return text, nil
}

// Load treats an absent file and the empty-file reset as no added instruction.
func Load(root string, limits config.Limits) (string, error) {
	if styleLimit(limits) <= 0 {
		return "", &Error{Operation: "read"}
	}
	if err := files.RequirePrivateFile(filepath.Join(root, StyleFile)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", &Error{Operation: "read"}
	}
	data, err := files.Read(root, StyleFile, styleLimit(limits))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil || !utf8.Valid(data) {
		return "", &Error{Operation: "read"}
	}
	return string(data), nil
}

func write(root, text, operation string) error {
	if err := files.RequirePrivateDir(root); err != nil {
		return &Error{Operation: operation}
	}
	if err := files.PrivateDir(filepath.Join(root, filepath.Dir(StyleFile))); err != nil {
		return &Error{Operation: operation}
	}
	if err := files.Write(root, StyleFile, []byte(text), true); err != nil {
		return &Error{Operation: operation}
	}
	return nil
}

// Save publishes the caller's trimmed text without asking an AI to rewrite it.
func Save(root string, limits config.Limits, text string) error {
	text, err := normalize(text, limits)
	if err != nil {
		return err
	}
	return write(root, text, "save")
}

// Reset atomically writes empty Markdown without adding another settings format.
func Reset(root string, limits config.Limits) error {
	if styleLimit(limits) <= 0 {
		return &Error{Operation: "reset"}
	}
	return write(root, "", "reset")
}

func customChoice() string { return strconv.Itoa(len(presets) + 1) }

func menu() string {
	var text strings.Builder
	fmt.Fprintf(&text, "Choose a tone.\n%s. Default tone\n", defaultChoice)
	for _, item := range presets {
		fmt.Fprintf(&text, "%s. %s\n", item.Choice, item.Name)
	}
	fmt.Fprintf(&text, "%s. Custom tone\nSend a number, or use /tone current · /tone reset · /tone cancel. Korean aliases remain available: /말투 현재 · /말투 초기화 · /말투 취소.", customChoice())
	return text.String()
}

func presetFor(choice string) (preset, bool) {
	for _, item := range presets {
		if item.Choice == choice {
			return item, true
		}
	}
	return preset{}, false
}

func presetText(item preset) (string, error) {
	data, err := presetFiles.ReadFile(item.File)
	if err != nil || !utf8.Valid(data) {
		return "", &Error{Operation: "preset"}
	}
	return string(data), nil
}

func customWord(text string) bool  { return text == "커스텀" || strings.EqualFold(text, "custom") }
func currentWord(text string) bool { return text == "현재" || strings.EqualFold(text, "current") }
func resetWord(text string) bool   { return text == "초기화" || strings.EqualFold(text, "reset") }
func cancelWord(text string) bool {
	return text == "취소" || text == "뒤로" || strings.EqualFold(text, "cancel") || strings.EqualFold(text, "back")
}

func (d *Dialogue) save(root string, limits config.Limits, text, label string) (string, bool, error) {
	if err := Save(root, limits, text); err != nil {
		switch {
		case errors.Is(err, ErrEmpty):
			return "Tone text is empty. Enter it again or use /tone cancel.", true, nil
		case errors.Is(err, ErrInvalid):
			return "Enter tone text as UTF-8.", true, nil
		case errors.Is(err, ErrTooLong):
			return "Tone text is too long. Enter a shorter version.", true, nil
		default:
			return "", true, err
		}
	}
	d.pending = pendingNone
	return label + " tone saved. It will apply to the next generated reply.", true, nil
}

func (d *Dialogue) selectStyle(root string, limits config.Limits, choice string) (string, bool, error) {
	if choice == defaultChoice {
		if err := Reset(root, limits); err != nil {
			return "", true, err
		}
		d.pending = pendingNone
		return "Tone reset to the default.", true, nil
	}
	if item, ok := presetFor(choice); ok {
		text, err := presetText(item)
		if err != nil {
			return "", true, err
		}
		return d.save(root, limits, text, item.Name)
	}
	if choice == customChoice() || customWord(choice) {
		d.pending = pendingCustom
		return "Enter the tone you want. It is saved without AI rewriting. Use cancel or /tone cancel to stop.", true, nil
	}
	d.pending = pendingSelection
	return menu(), true, nil
}

func (d *Dialogue) current(root string, limits config.Limits) (string, bool, error) {
	style, err := Load(root, limits)
	if err != nil {
		return "", true, err
	}
	if style == "" {
		return "Current tone: Default tone.", true, nil
	}
	for _, item := range presets {
		text, err := presetText(item)
		if err != nil {
			return "", true, err
		}
		if style == strings.TrimSpace(text) {
			return "Current tone: " + item.Name + "\n" + style, true, nil
		}
	}
	return "Current tone: Custom tone\n" + style, true, nil
}

// Handle never sends menu inputs to AI. Slash commands retain their normal handlers.
func (d *Dialogue) Handle(root string, limits config.Limits, text string) (string, bool, error) {
	text = strings.TrimSpace(text)
	if text == "/cancel" || text == "/reset" || text == "/새대화" {
		d.pending = pendingNone
		return "", false, nil
	}
	if d.pending != pendingNone && cancelWord(text) {
		d.pending = pendingNone
		return "Tone selection cancelled.", true, nil
	}
	parts := strings.Fields(text)
	if len(parts) > 0 && (parts[0] == "/말투" || strings.EqualFold(parts[0], "/tone")) {
		if len(parts) == 1 || (len(parts) == 2 && (parts[1] == "옵션" || strings.EqualFold(parts[1], "options"))) {
			d.pending = pendingSelection
			return menu(), true, nil
		}
		if len(parts) != 2 {
			d.pending = pendingSelection
			return menu(), true, nil
		}
		if currentWord(parts[1]) {
			return d.current(root, limits)
		}
		if resetWord(parts[1]) {
			return d.selectStyle(root, limits, defaultChoice)
		}
		if cancelWord(parts[1]) {
			d.pending = pendingNone
			return "Tone selection cancelled.", true, nil
		}
		return d.selectStyle(root, limits, parts[1])
	}
	if d.pending == pendingNone || strings.HasPrefix(text, "/") {
		return "", false, nil
	}
	if d.pending == pendingSelection {
		return d.selectStyle(root, limits, text)
	}
	return d.save(root, limits, text, "Custom")
}
