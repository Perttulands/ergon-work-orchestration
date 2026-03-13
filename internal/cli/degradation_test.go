package cli

import (
	"testing"

	"polis/work/internal/ecosystem"
	"polis/work/internal/loopfeed"
	"polis/work/internal/testutil"
)

// Test that checkTools reports all tools as degraded when none are on PATH.
func TestCheckToolsAllMissing(t *testing.T) {
	testutil.SandboxPATH(t, nil)

	report := checkTools()
	if len(report) < 3 {
		t.Fatalf("expected at least 3 degraded tools, got %d", len(report))
	}

	// Verify each expected tool is in the report
	found := map[string]toolDegradation{}
	for _, d := range report {
		found[d.Name] = d
	}

	// gate → unverified
	if g, ok := found["gate"]; !ok {
		t.Error("expected gate in degradation report")
	} else {
		if g.Mode != "unverified" {
			t.Errorf("gate mode = %q, want unverified", g.Mode)
		}
		if g.Warning == "" {
			t.Error("gate should have a warning message")
		}
	}

	// br → bead-free
	if b, ok := found["br"]; !ok {
		t.Error("expected br in degradation report")
	} else {
		if b.Mode != "bead-free" {
			t.Errorf("br mode = %q, want bead-free", b.Mode)
		}
		if b.Warning == "" {
			t.Error("br should have a warning message")
		}
	}

	// relay → silent-skip (no warning)
	if r, ok := found["relay"]; !ok {
		t.Error("expected relay in degradation report")
	} else {
		if r.Mode != "silent-skip" {
			t.Errorf("relay mode = %q, want silent-skip", r.Mode)
		}
		if r.Warning != "" {
			t.Errorf("relay should have no warning, got %q", r.Warning)
		}
	}
}

// Test that checkTools returns empty when all tools are present.
func TestCheckToolsAllPresent(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"gate":  "echo ok",
		"br":    "echo ok",
		"relay": "echo ok",
		"loop":  "echo ok",
	})

	report := checkTools()
	if len(report) != 0 {
		t.Errorf("expected 0 degradations when all tools present, got %d", len(report))
		for _, d := range report {
			t.Logf("  %s: %s", d.Name, d.Mode)
		}
	}
}

// Test that ecosystem functions return nil when tools are missing.
func TestEcosystemDegradationGate(t *testing.T) {
	testutil.SandboxPATH(t, nil)

	result, err := ecosystem.GateCheck("/tmp", "test")
	if err != nil {
		t.Errorf("gate check should return nil error, got: %v", err)
	}
	if result != nil {
		t.Error("gate check should return nil result when unavailable")
	}
}

func TestEcosystemDegradationBr(t *testing.T) {
	testutil.SandboxPATH(t, nil)

	// BrCreate returns nil, nil when unavailable
	result, err := ecosystem.BrCreate("test", "/tmp")
	if err != nil {
		t.Errorf("br create should return nil error, got: %v", err)
	}
	if result != nil {
		t.Error("br create should return nil result when unavailable")
	}

	// BrClose returns nil when unavailable
	if err := ecosystem.BrClose("id", "reason", "/tmp"); err != nil {
		t.Errorf("br close should return nil, got: %v", err)
	}

	// BrAgentState returns nil when unavailable
	if err := ecosystem.BrAgentState("agent", "working"); err != nil {
		t.Errorf("br agent state should return nil, got: %v", err)
	}
}

func TestEcosystemDegradationRelay(t *testing.T) {
	testutil.SandboxPATH(t, nil)

	if err := ecosystem.RelayHeartbeat("agent"); err != nil {
		t.Errorf("relay heartbeat should return nil, got: %v", err)
	}
	if err := ecosystem.RelaySend("from", "to", "msg", "", "", ""); err != nil {
		t.Errorf("relay send should return nil, got: %v", err)
	}
}

func TestEcosystemDegradationLoop(t *testing.T) {
	testutil.SandboxPATH(t, nil)

	result, err := ecosystem.QueryLearningLoop("test")
	if err != nil {
		t.Errorf("loop query should return nil error, got: %v", err)
	}
	if result != nil {
		t.Error("loop query should return nil result when unavailable")
	}

	dur := 60
	if err := ecosystem.IngestRun(loopfeed.Entry{ID: "id", Task: "task", Outcome: "success", DurationS: &dur, Timestamp: "2026-03-13T00:00:00Z", Agent: "agent"}); err != nil {
		t.Errorf("loop ingest should return nil, got: %v", err)
	}
}

// Test partial degradation: only gate missing.
func TestCheckToolsGateMissing(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"br":    "echo ok",
		"relay": "echo ok",
		"loop":  "echo ok",
	})

	report := checkTools()
	if len(report) != 1 {
		t.Fatalf("expected 1 degradation, got %d", len(report))
	}
	if report[0].Name != "gate" {
		t.Errorf("expected gate, got %s", report[0].Name)
	}
	if report[0].Mode != "unverified" {
		t.Errorf("mode = %q, want unverified", report[0].Mode)
	}
}

// Test partial degradation: only br missing.
func TestCheckToolsBrMissing(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"gate":  "echo ok",
		"relay": "echo ok",
		"loop":  "echo ok",
	})

	report := checkTools()
	if len(report) != 1 {
		t.Fatalf("expected 1 degradation, got %d", len(report))
	}
	if report[0].Name != "br" {
		t.Errorf("expected br, got %s", report[0].Name)
	}
	if report[0].Mode != "bead-free" {
		t.Errorf("mode = %q, want bead-free", report[0].Mode)
	}
}
