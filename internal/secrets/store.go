package secrets

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/files"
)

const KeychainStore = "macos-keychain"
const HelperStore = "credential-helper"
const HelperEnv = "CHUNSU_SECRET_HELPER"
const HelperTimeout = 30 * time.Second
const HelperProtocolVersion = 1

type Store interface {
	Set(context.Context, string, string) error
	Get(context.Context, string) (string, error)
	GetExternal(context.Context, string, string) (string, error)
	Delete(context.Context, string) error
}

func Open() (Store, error) {
	if os.Getenv(HelperEnv) != "" {
		return OpenFor(HelperStore)
	}
	return OpenKeychain()
}

func OpenFor(kind string) (Store, error) {
	if kind == KeychainStore {
		return OpenKeychain()
	}
	if kind != HelperStore {
		return nil, errors.New("unsupported credential store")
	}
	path := os.Getenv(HelperEnv)
	if !filepath.IsAbs(path) {
		return nil, errors.New("configure an absolute CHUNSU_SECRET_HELPER path for the host credential store")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 || info.Mode().Perm()&0111 == 0 {
		return nil, errors.New("credential helper must be a regular executable that other users cannot modify")
	}
	owner, err := files.OwnerUID(info)
	if err != nil || (owner != 0 && owner != os.Geteuid()) {
		return nil, errors.New("credential helper must belong to this OS user or the administrator")
	}
	return Helper{Program: path}, nil
}

type HelperRequest struct {
	Version   int    `json:"version"`
	Operation string `json:"operation"`
	Service   string `json:"service"`
	Account   string `json:"account"`
	Value     string `json:"value,omitempty"`
}
type HelperResponse struct {
	Status string `json:"status"`
	Value  string `json:"value,omitempty"`
}
type Helper struct{ Program string }

func (h Helper) invoke(parent context.Context, operation, service, account, value string) (string, error) {
	if service == "" || account == "" || len(service)+len(account) > MaxNativeCommandBytes || strings.ContainsAny(service+account, "\x00\r\n") {
		return "", errors.New("invalid credential reference")
	}
	if len(value) > MaxSecretBytes {
		return "", errors.New("credential exceeds supported size")
	}
	request, _ := json.Marshal(HelperRequest{Version: HelperProtocolVersion, Operation: operation, Service: service, Account: account, Value: value})
	ctx, cancel := context.WithTimeout(parent, HelperTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, h.Program)
	command.Stdin = strings.NewReader(string(request))
	output := &boundedBuffer{limit: diagnosticLimit}
	command.Stdout = output
	// Trusted host helper receives its configured vault environment. None of
	// these values enter the AI driver's restricted environment or arguments.
	if err := command.Run(); err != nil {
		return "", errors.Join(ErrUnavailable, ctx.Err())
	}
	var response HelperResponse
	decoder := json.NewDecoder(strings.NewReader(output.String()))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&response) != nil || !json.Valid(output.Bytes()) {
		return "", errors.New("invalid credential-helper response; raw output withheld")
	}
	if response.Status == "missing" {
		return "", ErrMissing
	}
	if response.Status != "ok" || len(response.Value) > MaxSecretBytes {
		return "", ErrUnavailable
	}
	return response.Value, nil
}

func (h Helper) Set(ctx context.Context, ref, value string) error {
	if err := validReference(ref); err != nil {
		return err
	}
	if value == "" {
		return errors.New("credential is empty")
	}
	if _, err := h.invoke(ctx, "set", config.AppName, ref, value); err != nil {
		return err
	}
	actual, err := h.Get(ctx, ref)
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare([]byte(actual), []byte(value)) != 1 {
		return errors.New("credential write verification failed")
	}
	return nil
}
func (h Helper) Get(ctx context.Context, ref string) (string, error) {
	if err := validReference(ref); err != nil {
		return "", err
	}
	return h.GetExternal(ctx, config.AppName, ref)
}
func (h Helper) GetExternal(ctx context.Context, service, account string) (string, error) {
	value, err := h.invoke(ctx, "get", service, account, "")
	if err == nil && value == "" {
		return "", errors.New("credential helper returned an empty value")
	}
	return value, err
}
func (h Helper) Delete(ctx context.Context, ref string) error {
	if err := validReference(ref); err != nil {
		return err
	}
	_, err := h.invoke(ctx, "delete", config.AppName, ref, "")
	if errors.Is(err, ErrMissing) {
		return nil
	}
	return err
}
