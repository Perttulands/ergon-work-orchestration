package main

import (
	"bytes"
	"testing"

	"polis/work/internal/cli"
)

func TestVersionCommand(t *testing.T) {
	root := cli.NewRoot("test-version")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("version command failed: %v", err)
	}

	got := buf.String()
	want := "work test-version\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRootHelp(t *testing.T) {
	root := cli.NewRoot("1.0.0")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("help failed: %v", err)
	}

	if buf.Len() == 0 {
		t.Error("expected help output, got empty")
	}
}
