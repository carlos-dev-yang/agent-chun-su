package codereview

import (
	"bytes"
	"context"
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

type boundedBuffer struct {
	bytes.Buffer
	maximum  int64
	overflow bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := b.maximum - int64(b.Len())
	if int64(n) > remaining {
		b.overflow = true
		p = p[:remaining]
	}
	_, _ = b.Buffer.Write(p)
	return n, nil
}

// Capture uses read-only Git object operations with literal pathspecs. No
// checkout, filters, hooks, external diff, repository command or network runs.
func Capture(ctx context.Context, repository, base, head string, paths []string, synthetic bool, limits config.Limits) (Input, error) {
	in := Input{Version: Version, Synthetic: synthetic, CapturedAt: time.Now().UTC().Format(time.RFC3339Nano), Sources: []Source{}}
	if len(paths) == 0 || len(paths) > limits.MaxMessages {
		return in, errors.New("select one or more exact repository-relative file paths within the source budget")
	}
	root, err := filepath.Abs(repository)
	if err != nil {
		return in, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return in, err
	}
	in.Repository = filepath.Base(root)
	program, err := exec.LookPath("git")
	if err != nil {
		return in, err
	}
	run := func(args ...string) ([]byte, error) {
		command := exec.CommandContext(ctx, program, append([]string{"--no-pager", "--literal-pathspecs", "-C", root, "-c", "core.fsmonitor=false"}, args...)...)
		// Host Git configuration remains available, but caller-supplied GIT_*
		// overrides cannot silently redirect the selected repository or object DB.
		for _, value := range os.Environ() {
			if !strings.HasPrefix(value, "GIT_") {
				command.Env = append(command.Env, value)
			}
		}
		command.Env = append(command.Env, "GIT_NO_REPLACE_OBJECTS=1", "GIT_NO_LAZY_FETCH=1", "GIT_TERMINAL_PROMPT=0", "GIT_ALLOW_PROTOCOL=")
		output := &boundedBuffer{maximum: limits.MaxSourceBytes}
		command.Stdout = output
		if e := command.Run(); e != nil {
			return nil, errors.New("selected Git object is unavailable; choose locally available committed revisions and regular files")
		}
		if output.overflow {
			return nil, errors.New("Git output exceeds the configured artifact budget")
		}
		return output.Bytes(), nil
	}
	for _, selected := range []struct {
		value  string
		target *string
	}{{base, &in.BaseRevision}, {head, &in.HeadRevision}} {
		value, target := selected.value, selected.target
		if value == "" || strings.HasPrefix(value, "-") || strings.ContainsAny(value, "\x00\r\n") {
			return in, errors.New("explicit base and head revisions are required")
		}
		data, e := run("rev-parse", "--verify", "--end-of-options", value+"^{commit}")
		if e != nil {
			return in, e
		}
		*target = strings.TrimSpace(string(data))
	}
	seen := map[string]bool{}
	for _, name := range paths {
		if !validPath(name) || seen[name] {
			return in, errors.New("select unique exact repository-relative file paths")
		}
		seen[name] = true
		source := Source{ID: files.Digest([]byte(name)), Path: name}
		present := false
		for _, side := range []struct {
			ref     string
			content *string
		}{{in.BaseRevision, &source.Before}, {in.HeadRevision, &source.After}} {
			entry, e := run("ls-tree", "-z", side.ref, "--", name)
			if e != nil {
				return in, e
			}
			if len(entry) == 0 {
				continue
			}
			present = true
			parts := strings.Split(strings.TrimSuffix(string(entry), "\x00"), "\t")
			if len(parts) != 2 || parts[1] != name || strings.ContainsRune(parts[1], '\x00') || (!strings.HasPrefix(parts[0], "100644 blob ") && !strings.HasPrefix(parts[0], "100755 blob ")) {
				return in, errors.New("only regular Git blobs are supported; symlinks, directories and submodules are excluded")
			}
			metadata := strings.Fields(parts[0])
			data, e := run("cat-file", "blob", metadata[2])
			if e != nil {
				return in, e
			}
			*side.content = string(data)
		}
		if !present {
			return in, errors.New("selected path is absent from both revisions")
		}
		diff, e := run("diff", "--no-ext-diff", "--no-textconv", "--no-renames", in.BaseRevision, in.HeadRevision, "--", name)
		if e != nil {
			return in, e
		}
		source.Diff = string(diff)
		source.BeforeDigest = files.Digest([]byte(source.Before))
		source.AfterDigest = files.Digest([]byte(source.After))
		in.Sources = append(in.Sources, source)
	}
	data, err := json.Marshal(in)
	if err != nil {
		return in, err
	}
	return Parse(data, limits)
}
