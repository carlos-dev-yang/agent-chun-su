// Package runtimeenv describes the enforced execution location independently
// of an AI provider. Drivers translate this boundary into their native runtime.
package runtimeenv

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const NativeRestricted = "native-restricted"
const PolicyVersion = 1

type Boundary struct {
	Version         int      `json:"version"`
	Kind            string   `json:"kind"`
	OS              string   `json:"os"`
	Architecture    string   `json:"architecture"`
	ReadRoots       []string `json:"read_roots"`
	WriteRoots      []string `json:"write_roots"`
	ToolNetwork     bool     `json:"tool_network"`
	ProviderTraffic string   `json:"provider_traffic"`
}

type Environment interface {
	CheckRoot(string) error
	Boundary(string, string) (Boundary, error)
}

func Select(kind string) (Environment, error) {
	if kind == "" || kind == NativeRestricted {
		return native{}, nil
	}
	return nil, errors.New("execution environment is not implemented; choose a supported boundary")
}

type native struct{}

func (native) CheckRoot(root string) error {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return errors.New("native execution boundary supports macOS and Linux only")
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return errors.New("cannot resolve execution data root")
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return err
	}
	if runtime.GOOS != "darwin" {
		return nil
	}
	// The verified macOS native profile has temporary-directory write exceptions.
	const systemTemporaryDirectory = "/tmp"
	for _, temporary := range []string{os.TempDir(), systemTemporaryDirectory} {
		canonical, err := filepath.EvalSymlinks(temporary)
		if err != nil {
			return errors.New("cannot verify the macOS temporary-directory boundary")
		}
		if within(canonical, resolved) {
			return errors.New("executor data root is inside a macOS temporary-directory write exception; use a private data directory outside system temporary storage")
		}
	}
	return nil
}

func (n native) Boundary(root, directory string) (Boundary, error) {
	b := Boundary{Version: PolicyVersion, Kind: NativeRestricted, OS: runtime.GOOS, Architecture: runtime.GOARCH, WriteRoots: []string{}, ProviderTraffic: "selected driver provider; tool network restriction does not block inference traffic"}
	if err := n.CheckRoot(root); err != nil {
		return b, err
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return b, err
	}
	canonicalDirectory, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return b, err
	}
	if !within(canonicalRoot, canonicalDirectory) || canonicalRoot == canonicalDirectory {
		return b, errors.New("execution package must be a dedicated directory inside its data root")
	}
	b.ReadRoots = []string{canonicalDirectory}
	return b, nil
}

func within(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
