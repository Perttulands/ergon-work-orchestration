// Package testutil provides test helpers shared across work's internal packages.
package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
)

// TB is the subset of testing.TB used by test helpers in this package.
// Both *testing.T and *testing.B satisfy it without importing "testing".
type TB interface {
	Helper()
	TempDir() string
	Fatalf(format string, args ...any)
	Setenv(key, value string)
}

// SandboxPATH restricts PATH to a temp directory containing only the named
// tool scripts. Each entry in tools maps a binary name to a shell script body;
// the helper writes them as executable scripts under a fresh temp dir and sets
// PATH to that directory alone.
//
// When tools is nil or empty, PATH points to an empty directory so no external
// commands are found by exec.LookPath.
//
// Basic system tools (sh, bash, env, cat, etc.) are always symlinked from
// the real PATH so that shell scripts and subprocesses work.
func SandboxPATH(t TB, tools map[string]string) string {
	t.Helper()

	binDir := t.TempDir()

	// Symlink basic system tools that tests or their shell scripts may need.
	system := []string{
		"sh", "bash", "env", "cat", "ls", "mkdir", "rm", "cp", "mv",
		"chmod", "stat", "grep", "sed", "awk", "head", "tail", "wc",
		"sort", "uniq", "tr", "date", "dirname", "basename", "realpath",
		"test", "printf", "echo", "true", "false",
	}
	for _, tool := range system {
		if real, err := exec.LookPath(tool); err == nil {
			// Ignore symlink errors (e.g. duplicates).
			os.Symlink(real, filepath.Join(binDir, filepath.Base(real)))
		}
	}

	// Write user-provided tool scripts.
	for name, body := range tools {
		path := filepath.Join(binDir, name)
		script := "#!/bin/sh\nset -e\n" + body + "\n"
		if writeErr := os.WriteFile(path, []byte(script), 0o755); writeErr != nil {
			t.Fatalf("write %s: %v", name, writeErr)
		}
	}

	t.Setenv("PATH", binDir)
	return binDir
}
