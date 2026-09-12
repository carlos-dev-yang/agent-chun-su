package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/files"
	"chunsu/internal/mail"
	"chunsu/internal/platform"
)

const Version = 1
const BindingFile = "binding.json"
const CursorFile = "cursor.json"
const PairSeconds = 300

type Binding struct {
	Version  int    `json:"version"`
	Bot      User   `json:"bot"`
	TokenRef string `json:"token_ref"`
	APIBase  string `json:"api_base"`
	UserID   int64  `json:"user_id"`
	ChatID   int64  `json:"chat_id"`
}

func Directory(root string) string { return filepath.Join(root, "state", "telegram") }
func Lock(ctx context.Context, root string, limits config.Limits) (*platform.Lock, error) {
	dir := Directory(root)
	for _, p := range []string{dir, filepath.Join(dir, "state"), filepath.Join(dir, "receipts")} {
		if e := files.PrivateDir(p); e != nil {
			return nil, e
		}
	}
	return platform.Acquire(ctx, dir, time.Duration(limits.LockWaitSeconds)*time.Second)
}
func Load(root string, limit int64) (Binding, error) {
	var b Binding
	raw, e := files.Read(Directory(root), BindingFile, limit)
	if e != nil {
		return b, e
	}
	if e = mail.Decode(raw, &b); e != nil {
		return b, e
	}
	if b.Version != Version || !b.Bot.IsBot || b.Bot.ID <= 0 || !files.ValidID(b.TokenRef) || b.APIBase == "" || b.UserID < 0 || b.ChatID != b.UserID {
		return b, errors.New("invalid Telegram binding")
	}
	return b, nil
}
func Save(root string, b Binding, replace bool) error {
	raw, e := json.MarshalIndent(b, "", "  ")
	if e != nil {
		return e
	}
	return files.Write(Directory(root), BindingFile, raw, replace)
}
func Offset(root string, limit int64) (int64, error) {
	var v struct {
		Next int64 `json:"next_update"`
	}
	raw, e := files.Read(Directory(root), CursorFile, limit)
	if errors.Is(e, os.ErrNotExist) {
		return 0, nil
	}
	if e != nil {
		return 0, e
	}
	if e = mail.Decode(raw, &v); e != nil {
		return 0, e
	}
	if v.Next < 0 {
		return 0, errors.New("invalid Telegram cursor")
	}
	return v.Next, nil
}
func Advance(root string, id int64) error {
	if id < 0 || id == math.MaxInt64 {
		return errors.New("invalid update ID")
	}
	raw, _ := json.Marshal(map[string]int64{"next_update": id + 1})
	return files.Write(Directory(root), CursorFile, raw, true)
}

type Receipt struct {
	UpdateID       int64     `json:"update_id"`
	State          string    `json:"state"`
	At             time.Time `json:"at,omitempty"`
	SessionID      string    `json:"session_id,omitempty"`
	Acknowledgment string    `json:"acknowledgment,omitempty"`
	AckMessageIDs  []int64   `json:"ack_message_ids,omitempty"`
	ErrorID        string    `json:"error_id,omitempty"`
	InputDigest    string    `json:"input_digest,omitempty"`
	ReplyDigest    string    `json:"reply_digest,omitempty"`
	MessageIDs     []int64   `json:"message_ids,omitempty"`
}

func Record(root string, r Receipt, replace bool) error {
	if r.UpdateID < 0 {
		return errors.New("invalid update ID")
	}
	r.At = time.Now().UTC()
	raw, e := json.Marshal(r)
	if e != nil {
		return e
	}
	return files.Write(Directory(root), filepath.Join("receipts", ReceiptName(r.UpdateID)), raw, replace)
}

func ReadReceipt(root string, id int64, limit int64) (Receipt, error) {
	var receipt Receipt
	if id < 0 {
		return receipt, errors.New("invalid update ID")
	}
	raw, err := files.Read(Directory(root), filepath.Join("receipts", ReceiptName(id)), limit)
	if err != nil {
		return receipt, err
	}
	if err = mail.Decode(raw, &receipt); err != nil {
		return receipt, err
	}
	if receipt.UpdateID != id {
		return receipt, errors.New("receipt identity mismatch")
	}
	return receipt, nil
}

func ReceiptName(id int64) string { return strconv.FormatInt(id, 10) + ".json" }
