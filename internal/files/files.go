package files

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	DirMode         = 0700
	FileMode        = 0600
	IDBytes         = 16
	MaxIDLength     = 64
	DigestHexLength = sha256.Size * 2
)

func ID() string { return rand.Text() }

func ValidID(id string) bool {
	if len(id) < IDBytes || len(id) > MaxIDLength {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func Digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func ValidDigest(digest string) bool {
	if len(digest) != DigestHexLength {
		return false
	}
	_, err := hex.DecodeString(digest)
	return err == nil
}

func PrivateDir(p string) error {
	info, err := os.Lstat(p)
	if errors.Is(err, os.ErrNotExist) {
		return os.MkdirAll(p, DirMode)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("data directory is not a real directory: %s", p)
	}
	if info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("data directory must be private (0700): %s", p)
	}
	return nil
}

func RequirePrivateDir(p string) error {
	info, err := os.Lstat(p)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return errors.New("managed data directories must be real private directories (0700)")
	}
	return nil
}

func relativeOK(name string) bool {
	return filepath.IsLocal(name) && name != "." && !strings.Contains(name, "\x00")
}

func rejectLinks(root, name string) error {
	info, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("root must be a real directory")
	}
	if !relativeOK(name) {
		return errors.New("path must stay within the data root")
	}
	p := root
	for _, part := range strings.Split(filepath.Clean(name), string(filepath.Separator)) {
		p = filepath.Join(p, part)
		info, err := os.Lstat(p)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("symbolic links are not allowed in managed paths")
		}
	}
	return nil
}

func Read(root, name string, limit int64) ([]byte, error) {
	if limit <= 0 {
		return nil, errors.New("read limit must be positive")
	}
	if err := rejectLinks(root, name); err != nil {
		return nil, err
	}
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	f, err := r.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("expected a regular file")
	}
	if info.Size() > limit {
		return nil, errors.New("file exceeds configured byte limit")
	}
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if int64(len(b)) > limit {
		return nil, errors.New("file exceeds configured byte limit")
	}
	return b, err
}

func Write(root, name string, data []byte, replace bool) error {
	if err := rejectLinks(root, name); err != nil {
		return err
	}
	r, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer r.Close()
	dir := filepath.Dir(name)
	if err := r.MkdirAll(dir, DirMode); err != nil {
		return err
	}
	tmp := filepath.Join(dir, ".staging-"+ID())
	f, err := r.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, FileMode)
	if err != nil {
		return err
	}
	defer r.Remove(tmp)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if !replace {
		// Link publishes a new file atomically and never overwrites an existing artifact.
		if err = r.Link(tmp, name); err != nil {
			return err
		}
	} else if err = r.Rename(tmp, name); err != nil {
		return err
	}
	d, err := r.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
