// Package updateguard coordinates the short self-update activation window.
package updateguard

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"chunsu/internal/files"
	"chunsu/internal/platform"
)

const relative = "state/self-update/activation"

type record struct {
	ID      string                   `json:"id"`
	Nonce   string                   `json:"nonce"`
	Owner   platform.ProcessIdentity `json:"owner"`
	Started time.Time                `json:"started"`
}
type Guard struct {
	lock   *platform.Lock
	root   string
	Record record
}

func Acquire(ctx context.Context, root, id string, timeout time.Duration) (*Guard, error) {
	dir := filepath.Join(root, relative)
	if err := files.PrivateDir(dir); err != nil {
		return nil, err
	}
	if err := files.PrivateDir(filepath.Join(dir, "state")); err != nil {
		return nil, err
	}
	lock, err := platform.Acquire(ctx, dir, timeout)
	if err != nil {
		return nil, err
	}
	owner, err := platform.Identify(os.Getpid())
	if err != nil {
		_ = lock.Close()
		return nil, err
	}
	raw := make([]byte, 24)
	if _, err = rand.Read(raw); err != nil {
		_ = lock.Close()
		return nil, err
	}
	g := &Guard{lock: lock, root: root, Record: record{ID: id, Nonce: hex.EncodeToString(raw), Owner: owner, Started: time.Now().UTC()}}
	b, err := json.Marshal(g.Record)
	if err == nil {
		err = files.Write(dir, "active.json", b, true)
	}
	if err != nil {
		_ = lock.Close()
		return nil, err
	}
	return g, nil
}
func (g *Guard) Close() error {
	err := files.RemoveTree(g.root, filepath.Join(relative, "active.json"))
	return errors.Join(err, g.lock.Close())
}
func Active(root string, limit int64) (bool, error) {
	_, err := read(root, limit)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}
func Valid(root, id, nonce string, limit int64) bool {
	r, err := read(root, limit)
	if err != nil || r.ID != id || r.Nonce != nonce {
		return false
	}
	active, err := platform.IdentityActive(r.Owner)
	return err == nil && active
}
func read(root string, limit int64) (record, error) {
	var r record
	b, err := files.Read(filepath.Join(root, relative), "active.json", limit)
	if err != nil {
		return r, err
	}
	err = json.Unmarshal(b, &r)
	return r, err
}
