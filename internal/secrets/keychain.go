package secrets

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"chunsu/internal/config"
	"chunsu/internal/files"
)

const MaxSecretBytes = 2048
const MaxNativeCommandBytes = 4096
const encodedPrefix = "chunsu-base64:"
const diagnosticLimit = 8192

var ErrUnavailable = errors.New("macOS Keychain is unavailable or locked; no plaintext fallback is used")
var ErrMissing = errors.New("credential reference is missing; reconnect the account")

type Keychain struct{ Program string }

func Open() (Keychain, error) {
	if runtime.GOOS != "darwin" {
		return Keychain{}, errors.New("live Gmail secret storage currently requires macOS Keychain")
	}
	path, err := exec.LookPath("security")
	if err != nil {
		return Keychain{}, ErrUnavailable
	}
	return Keychain{Program: path}, nil
}

type boundedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.Len() {
		return 0, errors.New("native keychain output exceeded its limit")
	}
	return b.Buffer.Write(p)
}
func (k Keychain) invoke(ctx context.Context, input string, args ...string) (string, string, error) {
	cmd := exec.CommandContext(ctx, k.Program, args...)
	cmd.Stdin = strings.NewReader(input)
	out := &boundedBuffer{limit: diagnosticLimit}
	diagnostic := &boundedBuffer{limit: diagnosticLimit}
	cmd.Stdout = out
	cmd.Stderr = diagnostic
	err := cmd.Run()
	return out.String(), diagnostic.String(), err
}
func validReference(ref string) error {
	if !files.ValidID(ref) {
		return errors.New("invalid credential reference")
	}
	return nil
}
func (k Keychain) Set(ctx context.Context, ref, value string) error {
	if err := validReference(ref); err != nil {
		return err
	}
	if value == "" || len(value) > MaxSecretBytes {
		return errors.New("credential exceeds the supported secret size or is empty")
	}
	encoded := encodedPrefix + base64.StdEncoding.EncodeToString([]byte(value))
	command := fmt.Sprintf("add-generic-password -U -s %s -a %s -w %s\n", config.AppName, ref, encoded)
	if len(command) > MaxNativeCommandBytes {
		return errors.New("credential exceeds the native Keychain input limit")
	}
	if _, _, err := k.invoke(ctx, command, "-i"); err != nil {
		return ErrUnavailable
	}
	stored, err := k.Get(ctx, ref)
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare([]byte(stored), []byte(value)) != 1 {
		return errors.New("Keychain write verification failed")
	}
	return nil
}
func (k Keychain) Get(ctx context.Context, ref string) (string, error) {
	if err := validReference(ref); err != nil {
		return "", err
	}
	out, diagnostic, err := k.invoke(ctx, "", "find-generic-password", "-s", config.AppName, "-a", ref, "-w")
	if err != nil {
		if strings.Contains(diagnostic, "could not be found") {
			return "", ErrMissing
		}
		return "", ErrUnavailable
	}
	encoded := strings.TrimSpace(out)
	if !strings.HasPrefix(encoded, encodedPrefix) {
		return "", errors.New("credential reference has an unsupported encoding")
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(encoded, encodedPrefix))
	if err != nil || len(data) > MaxSecretBytes {
		return "", errors.New("credential reference is malformed")
	}
	return string(data), nil
}
func (k Keychain) Delete(ctx context.Context, ref string) error {
	if err := validReference(ref); err != nil {
		return err
	}
	_, diagnostic, err := k.invoke(ctx, "", "delete-generic-password", "-s", config.AppName, "-a", ref)
	if err != nil && !strings.Contains(diagnostic, "could not be found") {
		return ErrUnavailable
	}
	return nil
}
