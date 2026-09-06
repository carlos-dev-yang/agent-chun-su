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

// Security.framework OSStatus values, as declared in Apple's SecBase.h.
const (
	statusUserCanceled          = -128
	statusNotAvailable          = -25291
	statusAuthFailed            = -25293
	statusNoSuchKeychain        = -25294
	statusInvalidKeychain       = -25295
	statusDuplicateItem         = -25299
	statusItemNotFound          = -25300
	statusInteractionNotAllowed = -25308
	processExitMask             = 0xff
	unknownExitStatus           = -1
)

var ErrUnavailable = errors.New("macOS Keychain operation failed; no plaintext fallback is used")
var ErrMissing = errors.New("credential reference is missing; reconnect the account")

type nativeStatus struct {
	code        int
	name        string
	message     string
	explanation string
}

// Never surface raw security output: interactive mode can echo its secret-bearing
// input. Only recognize exact known message suffixes with matching exit status.
var nativeStatuses = [...]nativeStatus{
	{statusUserCanceled, "errSecUserCanceled", "User canceled the operation.", "the Keychain operation was canceled"},
	{statusNotAvailable, "errSecNotAvailable", "No keychain is available. You may need to restart your computer.", "macOS could not make a Keychain available"},
	{statusAuthFailed, "errSecAuthFailed", "The user name or passphrase you entered is not correct.", "macOS rejected Keychain authentication; an unlocked status does not prove storage access"},
	{statusNoSuchKeychain, "errSecNoSuchKeychain", "The specified keychain could not be found.", "macOS could not find the selected Keychain"},
	{statusInvalidKeychain, "errSecInvalidKeychain", "The specified keychain is not a valid keychain file.", "macOS reported an invalid Keychain"},
	{statusDuplicateItem, "errSecDuplicateItem", "The specified item already exists in the keychain.", "the credential item already exists"},
	{statusItemNotFound, "errSecItemNotFound", "The specified item could not be found in the keychain.", "the credential item was not found"},
	{statusInteractionNotAllowed, "errSecInteractionNotAllowed", "User interaction is not allowed.", "macOS could not show or complete required Keychain interaction"},
}

type nativeError struct {
	operation string
	exitCode  int
	status    *nativeStatus
}

func (e *nativeError) Error() string {
	message := fmt.Sprintf("macOS Keychain %s failed", e.operation)
	if e.exitCode != unknownExitStatus {
		message += fmt.Sprintf(" (security exit %d)", e.exitCode)
	}
	if e.status != nil {
		message += fmt.Sprintf(": %s (%d): %s", e.status.name, e.status.code, e.status.explanation)
	} else {
		message += ": native failure details were not recognized and are withheld to protect credentials"
	}
	return message + "; no plaintext fallback is used"
}

func (e *nativeError) Unwrap() error {
	if e.status != nil && e.status.code == statusItemNotFound {
		return ErrMissing
	}
	return ErrUnavailable
}

func nativeFailure(ctx context.Context, operation, diagnostic string, cause error) error {
	failure := &nativeError{operation: operation, exitCode: unknownExitStatus}
	var exit *exec.ExitError
	if errors.As(cause, &exit) {
		failure.exitCode = exit.ExitCode()
		for _, line := range strings.Split(diagnostic, "\n") {
			for i := range nativeStatuses {
				status := &nativeStatuses[i]
				if failure.exitCode == status.code&processExitMask && strings.HasSuffix(strings.TrimSpace(line), status.message) {
					failure.status = status
				}
			}
		}
	}
	return errors.Join(failure, ctx.Err())
}

type Keychain struct{ Program string }

func Open() (Keychain, error) {
	if runtime.GOOS != "darwin" {
		return Keychain{}, errors.New("live Gmail secret storage currently requires macOS Keychain")
	}
	path, err := exec.LookPath("security")
	if err != nil {
		return Keychain{}, fmt.Errorf("Keychain helper executable is unavailable: %w", ErrUnavailable)
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
	if _, diagnostic, err := k.invoke(ctx, command, "-i"); err != nil {
		return nativeFailure(ctx, "store", diagnostic, err)
	}
	stored, err := k.Get(ctx, ref)
	if err != nil {
		return fmt.Errorf("verify stored Keychain credential: %w", err)
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
		return "", nativeFailure(ctx, "read", diagnostic, err)
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
	if err != nil {
		failure := nativeFailure(ctx, "delete", diagnostic, err)
		if !errors.Is(failure, ErrMissing) || ctx.Err() != nil {
			return failure
		}
	}
	return nil
}
