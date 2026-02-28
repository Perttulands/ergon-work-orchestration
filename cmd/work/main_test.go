package main

import (
	"bytes"
	"strings"
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

func TestSubcommandRegistration(t *testing.T) {
	root := cli.NewRoot("1.0.0")

	expected := []string{"version", "context", "run", "status", "history", "trace", "feed", "deliberate", "decide"}
	for _, name := range expected {
		found := false
		for _, cmd := range root.Commands() {
			if cmd.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected subcommand %q not found", name)
		}
	}
}

func TestSubcommandHelp(t *testing.T) {
	subcommands := []string{"context", "run", "status", "history", "trace", "feed", "deliberate", "decide"}

	for _, sub := range subcommands {
		t.Run(sub, func(t *testing.T) {
			root := cli.NewRoot("1.0.0")
			buf := new(bytes.Buffer)
			root.SetOut(buf)
			root.SetErr(buf)
			root.SetArgs([]string{sub, "--help"})

			if err := root.Execute(); err != nil {
				t.Fatalf("%s --help failed: %v", sub, err)
			}

			output := buf.String()
			if !strings.Contains(output, "Usage:") && !strings.Contains(output, "Flags:") {
				t.Errorf("%s --help produced unexpected output: %s", sub, output)
			}
		})
	}
}

func TestUnknownSubcommand(t *testing.T) {
	root := cli.NewRoot("1.0.0")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"nonexistent-command"})

	err := root.Execute()
	if err == nil {
		t.Error("expected error for unknown subcommand")
	}
}

func TestRootNoArgs(t *testing.T) {
	root := cli.NewRoot("1.0.0")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{})

	// Root with no args should show help (no error)
	if err := root.Execute(); err != nil {
		t.Fatalf("root with no args should not error: %v", err)
	}
}
