package worker

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

func hasTmux() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

func TestRequireTmux(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}
	if err := requireTmux(); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestCreateAndKillSession(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}
	name := "work-test-" + time.Now().Format("150405")
	defer killSession(name)

	if err := createSession(name, "/tmp"); err != nil {
		t.Fatalf("create session: %v", err)
	}

	if !SessionExists(name) {
		t.Error("session should exist after creation")
	}

	if err := killSession(name); err != nil {
		t.Fatalf("kill session: %v", err)
	}

	if SessionExists(name) {
		t.Error("session should not exist after kill")
	}
}

func TestSendKeysAndCapture(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}
	name := "work-test-capture-" + time.Now().Format("150405")
	defer killSession(name)

	if err := createSession(name, "/tmp"); err != nil {
		t.Fatalf("create session: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	if err := sendKeys(name, "echo HELLO_WORK_TEST"); err != nil {
		t.Fatalf("send keys: %v", err)
	}
	time.Sleep(1 * time.Second)

	output, err := capturePane(name)
	if err != nil {
		t.Fatalf("capture pane: %v", err)
	}

	if !strings.Contains(output, "HELLO_WORK_TEST") {
		t.Errorf("expected output to contain HELLO_WORK_TEST, got: %s", output)
	}
}

func TestSessionExistsNonExistent(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}
	if SessionExists("work-test-nonexistent-99999") {
		t.Error("non-existent session should return false")
	}
}

func TestConfigValidation(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}

	// No session name
	_, err := Spawn(Config{WorkDir: "/tmp"})
	if err == nil || !strings.Contains(err.Error(), "session name required") {
		t.Errorf("expected session name error, got: %v", err)
	}

	// No work dir
	_, err = Spawn(Config{SessionName: "test"})
	if err == nil || !strings.Contains(err.Error(), "work directory required") {
		t.Errorf("expected work directory error, got: %v", err)
	}
}

func TestIsStillWorking(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "idle prompt",
			output: "some output\nDone.\n❯",
			want:   false,
		},
		{
			name:   "reading file",
			output: "some output\nReading src/main.go\n❯",
			want:   true,
		},
		{
			name:   "short output",
			output: "❯",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isStillWorking(tt.output)
			if got != tt.want {
				t.Errorf("isStillWorking(%q) = %v, want %v", tt.output, got, tt.want)
			}
		})
	}
}
