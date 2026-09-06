package files

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
)

func HashFile(root, name string, limit int64) (string, int64, error) {
	if err := rejectLinks(root, name); err != nil {
		return "", 0, err
	}
	r, err := os.OpenRoot(root)
	if err != nil {
		return "", 0, err
	}
	defer r.Close()
	f, err := r.Open(name)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", 0, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return "", 0, errors.New("invalid or oversized regular file")
	}
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(f, limit+1))
	if err != nil {
		return "", n, err
	}
	if n > limit {
		return "", n, errors.New("file exceeds byte budget")
	}
	return hex.EncodeToString(hash.Sum(nil)), n, nil
}

func Copy(root, name, destination string, limit int64) error {
	if err := rejectLinks(root, name); err != nil {
		return err
	}
	if err := rejectLinks(destination, name); err != nil {
		return err
	}
	source, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer source.Close()
	input, err := source.Open(name)
	if err != nil {
		return err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return errors.New("copy requires a bounded regular file")
	}
	target, err := os.OpenRoot(destination)
	if err != nil {
		return err
	}
	defer target.Close()
	if err = target.MkdirAll(filepath.Dir(name), DirMode); err != nil {
		return err
	}
	output, err := target.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, FileMode)
	if err != nil {
		return err
	}
	defer output.Close()
	n, err := io.Copy(output, io.LimitReader(input, limit+1))
	if err != nil {
		return err
	}
	if n > limit {
		return errors.New("copy exceeds byte budget")
	}
	if err = output.Sync(); err != nil {
		return err
	}
	if err = output.Close(); err != nil {
		return err
	}
	return syncParents(target, filepath.Dir(name))
}

func RemoveTree(root, name string) error {
	if err := rejectLinks(root, name); err != nil {
		return err
	}
	r, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer r.Close()
	if err = r.RemoveAll(name); err != nil {
		return err
	}
	return syncParents(r, filepath.Dir(name))
}
