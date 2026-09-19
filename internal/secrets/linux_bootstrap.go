package secrets

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"

	"chunsu/internal/files"
)

// LinuxBootstrapPath is deliberately outside the backup allow-list. It holds
// only paths; the adjacent key and ciphertext directory must be provisioned
// again when restoring a data home.
const LinuxBootstrapPath = "state/credential-store/config.json"

const (
	linuxBootstrapVersion  = 1
	linuxBootstrapLimit    = 8 << 10
	linuxBootstrapKeyPath  = "state/credential-store/master.key"
	linuxBootstrapDataPath = "state/credential-store/ciphertext"
)

type linuxBootstrap struct {
	Version             int    `json:"version"`
	Helper              string `json:"helper"`
	KeyFile             string `json:"key_file"`
	CiphertextDirectory string `json:"ciphertext_directory"`
}

// RestoreLinuxBootstrap applies only saved non-secret helper paths. It does
// not inspect the helper, key or ciphertext: ordinary commands remain usable
// when a credential store was removed or needs recovery. An explicitly
// selected helper, including partial explicit helper settings, remains in
// charge.
func RestoreLinuxBootstrap(root string) (bool, error) {
	if runtime.GOOS != "linux" || os.Getenv(HelperEnv) != "" || os.Getenv(KeyFileEnv) != "" || os.Getenv(StoreDirectoryEnv) != "" {
		return false, nil
	}
	saved, found, err := loadLinuxBootstrap(root)
	if err != nil || !found {
		// Credential setup will report malformed metadata when it needs the
		// store. Do not make controller/status/configuration unavailable.
		return false, nil
	}
	if err = os.Setenv(HelperEnv, saved.Helper); err != nil {
		return false, err
	}
	if err = os.Setenv(KeyFileEnv, saved.KeyFile); err != nil {
		return false, err
	}
	if err = os.Setenv(StoreDirectoryEnv, saved.CiphertextDirectory); err != nil {
		return false, err
	}
	return true, nil
}

func loadLinuxBootstrap(root string) (linuxBootstrap, bool, error) {
	var saved linuxBootstrap
	if _, err := os.Lstat(root); errors.Is(err, os.ErrNotExist) {
		return saved, false, nil
	} else if err != nil {
		return saved, false, err
	}
	b, err := files.Read(root, LinuxBootstrapPath, linuxBootstrapLimit)
	if errors.Is(err, os.ErrNotExist) {
		return saved, false, nil
	}
	if err != nil {
		return saved, false, err
	}
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&saved); err != nil || !json.Valid(b) || saved.Version != linuxBootstrapVersion || !filepath.IsAbs(saved.Helper) || !privateBootstrapPath(root, saved.KeyFile) || !privateBootstrapPath(root, saved.CiphertextDirectory) {
		return linuxBootstrap{}, false, errors.New("invalid saved Linux credential-store configuration")
	}
	return saved, true, nil
}

func privateBootstrapPath(root, path string) bool {
	if !filepath.IsAbs(root) || !filepath.IsAbs(path) {
		return false
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && filepath.IsLocal(rel) && rel != "."
}

// BootstrapLinuxStore creates the bundled encrypted helper configuration on a
// first Linux chat setup. The random key is generated only for an empty
// ciphertext directory: losing a key with existing ciphertext is fatal.
func BootstrapLinuxStore(root string) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	if helper := os.Getenv(HelperEnv); helper != "" {
		// Root initialization may have restored our saved paths into this
		// process. Validate that store at the credential boundary, while any
		// different explicit helper remains user-owned and untouched.
		if saved, found, err := loadLinuxBootstrap(root); err == nil && found && helper == saved.Helper && os.Getenv(KeyFileEnv) == saved.KeyFile && os.Getenv(StoreDirectoryEnv) == saved.CiphertextDirectory {
			return validateLinuxBootstrap(saved)
		}
		return nil
	}
	if os.Getenv(KeyFileEnv) != "" || os.Getenv(StoreDirectoryEnv) != "" {
		return errors.New("configure CHUNSU_SECRET_HELPER with any explicit credential-store paths")
	}
	if saved, found, err := loadLinuxBootstrap(root); err != nil {
		return err
	} else if found {
		if err = validateLinuxBootstrap(saved); err != nil {
			return err
		}
		_, err = RestoreLinuxBootstrap(root)
		return err
	}
	if !filepath.IsAbs(root) {
		return errors.New("credential-store data root must be absolute")
	}
	state := filepath.Join(root, "state", "credential-store")
	if err := files.PrivateDir(state); err != nil {
		return err
	}
	data := filepath.Join(root, linuxBootstrapDataPath)
	if err := files.PrivateDir(data); err != nil {
		return err
	}
	entries, err := os.ReadDir(data)
	if err != nil {
		return err
	}
	key := filepath.Join(root, linuxBootstrapKeyPath)
	if _, err = os.Lstat(key); errors.Is(err, os.ErrNotExist) {
		if len(entries) != 0 {
			return errors.New("encrypted credential data exists but its master key is missing; restore the original key instead of generating a replacement")
		}
		value := make([]byte, MasterKeyBytes)
		if _, err = rand.Read(value); err != nil {
			return err
		}
		err = files.Write(root, linuxBootstrapKeyPath, value, false)
		clear(value)
		if err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
	} else if err != nil {
		return err
	}
	if err = files.RequirePrivateFile(key); err != nil {
		return errors.New("saved encrypted credential master key must be a private regular file")
	}
	value, err := files.Read(root, linuxBootstrapKeyPath, MasterKeyBytes)
	if err != nil || len(value) != MasterKeyBytes {
		clear(value)
		return errors.New("saved encrypted credential master key must contain exactly 32 random bytes")
	}
	clear(value)
	helper, err := bundledHelperPath()
	if err != nil {
		return err
	}
	saved := linuxBootstrap{Version: linuxBootstrapVersion, Helper: helper, KeyFile: key, CiphertextDirectory: data}
	if err = validateLinuxBootstrap(saved); err != nil {
		return err
	}
	metadata, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return err
	}
	if err = files.Write(root, LinuxBootstrapPath, append(metadata, '\n'), false); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	_, err = RestoreLinuxBootstrap(root)
	return err
}

func bundledHelperPath() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return "", err
	}
	helper := filepath.Join(filepath.Dir(executable), "chunsu-secret-store")
	if err = validateHelperProgram(helper); err != nil {
		return "", errors.New("bundled encrypted credential helper is unavailable or unsafe")
	}
	return helper, nil
}

func validateLinuxBootstrap(saved linuxBootstrap) error {
	if err := validateHelperProgram(saved.Helper); err != nil {
		return err
	}
	if err := files.RequirePrivateFile(saved.KeyFile); err != nil {
		return errors.New("saved encrypted credential master key must be a private regular file")
	}
	key, err := files.Read(filepath.Dir(saved.KeyFile), filepath.Base(saved.KeyFile), MasterKeyBytes)
	if err != nil || len(key) != MasterKeyBytes {
		clear(key)
		return errors.New("saved encrypted credential master key must contain exactly 32 random bytes")
	}
	clear(key)
	if err := files.PrivateDir(saved.CiphertextDirectory); err != nil {
		return err
	}
	return nil
}
