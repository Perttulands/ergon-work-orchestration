package spine

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type ParityMismatch struct {
	RunKey string `json:"run_key"`
	Detail string `json:"detail"`
}

type RunSummary struct {
	RunKey       string   `json:"run_key"`
	BeadID       string   `json:"bead_id,omitempty"`
	TraceID      string   `json:"trace_id,omitempty"`
	SessionID    string   `json:"session_id,omitempty"`
	Outcome      string   `json:"outcome"`
	GateVerdict  string   `json:"gate_verdict,omitempty"`
	OrderedKinds []string `json:"ordered_kinds"`
}

type ParityReport struct {
	LegacyRuns         int              `json:"legacy_runs"`
	SpineRuns          int              `json:"spine_runs"`
	MissingInSpine     []string         `json:"missing_in_spine"`
	MissingInLegacy    []string         `json:"missing_in_legacy"`
	OutcomeMismatches  []ParityMismatch `json:"outcome_mismatches"`
	GateMismatches     []ParityMismatch `json:"gate_mismatches"`
	OrderingMismatches []ParityMismatch `json:"ordering_mismatches"`
}

func (r ParityReport) HasMismatches() bool {
	return r.LegacyRuns != r.SpineRuns ||
		len(r.MissingInSpine) > 0 ||
		len(r.MissingInLegacy) > 0 ||
		len(r.OutcomeMismatches) > 0 ||
		len(r.GateMismatches) > 0 ||
		len(r.OrderingMismatches) > 0
}

func Compare(workDir, spineDir string) (ParityReport, error) {
	legacyRuns, err := loadLegacyRuns(filepath.Join(workDir, "traces"))
	if err != nil {
		return ParityReport{}, err
	}
	spineRuns, err := loadSpineRuns(spineDir)
	if err != nil {
		return ParityReport{}, err
	}

	report := ParityReport{
		LegacyRuns:         len(legacyRuns),
		SpineRuns:          len(spineRuns),
		MissingInSpine:     []string{},
		MissingInLegacy:    []string{},
		OutcomeMismatches:  []ParityMismatch{},
		GateMismatches:     []ParityMismatch{},
		OrderingMismatches: []ParityMismatch{},
	}

	legacyKeys := sortedKeys(legacyRuns)
	spineKeys := sortedKeys(spineRuns)

	legacySet := make(map[string]bool, len(legacyKeys))
	for _, k := range legacyKeys {
		legacySet[k] = true
	}
	spineSet := make(map[string]bool, len(spineKeys))
	for _, k := range spineKeys {
		spineSet[k] = true
	}

	for _, k := range legacyKeys {
		if !spineSet[k] {
			report.MissingInSpine = append(report.MissingInSpine, k)
			continue
		}
		legacy := legacyRuns[k]
		spine := spineRuns[k]
		if legacy.Outcome != spine.Outcome {
			report.OutcomeMismatches = append(report.OutcomeMismatches, ParityMismatch{
				RunKey: k,
				Detail: fmt.Sprintf("legacy=%s spine=%s", legacy.Outcome, spine.Outcome),
			})
		}
		if legacy.GateVerdict != spine.GateVerdict {
			report.GateMismatches = append(report.GateMismatches, ParityMismatch{
				RunKey: k,
				Detail: fmt.Sprintf("legacy=%s spine=%s", legacy.GateVerdict, spine.GateVerdict),
			})
		}
		if strings.Join(legacy.OrderedKinds, ",") != strings.Join(spine.OrderedKinds, ",") {
			report.OrderingMismatches = append(report.OrderingMismatches, ParityMismatch{
				RunKey: k,
				Detail: fmt.Sprintf("legacy=%v spine=%v", legacy.OrderedKinds, spine.OrderedKinds),
			})
		}
	}
	for _, k := range spineKeys {
		if !legacySet[k] {
			report.MissingInLegacy = append(report.MissingInLegacy, k)
		}
	}

	return report, nil
}

type legacyEvent struct {
	EventType string `json:"event"`
	Bead      string `json:"bead,omitempty"`
	TraceID   string `json:"trace_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	RunID     string `json:"run_id,omitempty"`
	Outcome   string `json:"outcome,omitempty"`
	Agent     string `json:"agent,omitempty"`
	Task      string `json:"task,omitempty"`
	Tool      string `json:"tool,omitempty"`
	Cmd       string `json:"cmd,omitempty"`
	Path      string `json:"path,omitempty"`
	Lines     *int   `json:"lines,omitempty"`
	Error     string `json:"error,omitempty"`
	Pass      *bool  `json:"pass,omitempty"`
}

func loadLegacyRuns(tracesDir string) (map[string]RunSummary, error) {
	runs := map[string]RunSummary{}
	if _, err := os.Stat(tracesDir); os.IsNotExist(err) {
		return runs, nil
	}
	err := filepath.Walk(tracesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return ignoreTraceEntryError(path, err)
		}
		if info.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return ignoreTraceEntryError(path, err)
		}
		defer file.Close()

		var events []legacyEvent
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		for scanner.Scan() {
			var e legacyEvent
			if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
				continue
			}
			events = append(events, e)
		}
		if err := scanner.Err(); err != nil {
			return ignoreTraceEntryError(path, err)
		}
		if len(events) == 0 {
			return nil
		}
		summary := summarizeLegacy(events)
		if summary.RunKey != "" {
			runs[summary.RunKey] = summary
		}
		return nil
	})
	return runs, err
}

func ignoreTraceEntryError(_ string, _ error) error {
	return nil
}

func summarizeLegacy(events []legacyEvent) RunSummary {
	var s RunSummary
	for _, e := range events {
		if s.RunKey == "" {
			s.RunKey = runKey(e.RunID, e.Bead, e.TraceID)
			s.BeadID = e.Bead
			s.TraceID = e.TraceID
			s.SessionID = e.SessionID
		}
		switch e.EventType {
		case "begin":
			s.OrderedKinds = append(s.OrderedKinds, "session.start", "agent.start")
		case "tool_call":
			if e.Tool == "bash" {
				s.OrderedKinds = append(s.OrderedKinds, "bash.run")
			}
		case "file_write":
			s.OrderedKinds = append(s.OrderedKinds, "file.edit")
		case "gate_result":
			s.OrderedKinds = append(s.OrderedKinds, "gate.result")
			if e.Pass != nil {
				if *e.Pass {
					s.GateVerdict = "pass"
				} else {
					s.GateVerdict = "fail"
				}
			}
		case "error":
			s.OrderedKinds = append(s.OrderedKinds, "error.tool_failure")
		case "end":
			s.OrderedKinds = append(s.OrderedKinds, "session.end", "agent.end")
			s.Outcome = e.Outcome
		}
	}
	return s
}

func loadSpineRuns(spineDir string) (map[string]RunSummary, error) {
	runs := map[string]RunSummary{}
	events, err := ReadAll(spineDir)
	if err != nil {
		return nil, err
	}
	for _, e := range events {
		key := runKey(e.RunID, deref(e.BeadID), e.TraceID)
		if key == "" {
			continue
		}
		s := runs[key]
		if s.RunKey == "" {
			s.RunKey = key
			s.BeadID = deref(e.BeadID)
			s.TraceID = e.TraceID
			s.SessionID = e.SessionID
		}
		s.OrderedKinds = append(s.OrderedKinds, e.Kind)
		switch e.Kind {
		case "gate.result":
			if verdict, ok := e.Data["verdict"].(string); ok {
				s.GateVerdict = verdict
			}
		case "session.end":
			s.Outcome = deriveOutcome(s.GateVerdict, e.Data)
		}
		runs[key] = s
	}
	return runs, nil
}

func deriveOutcome(gateVerdict string, data map[string]any) string {
	if gateVerdict == "fail" {
		return "gate_fail"
	}
	exitReason, _ := data["exit_reason"].(string)
	switch exitReason {
	case "completed":
		return "success"
	case "error", "aborted":
		return "error"
	case "timeout":
		return "timeout"
	default:
		return exitReason
	}
}

func runKey(runID, beadID, traceID string) string {
	switch {
	case strings.TrimSpace(runID) != "":
		return runID
	case strings.TrimSpace(beadID) != "":
		return beadID
	default:
		return traceID
	}
}

func deref(ptr *string) string {
	if ptr == nil {
		return ""
	}
	return *ptr
}

func sortedKeys[K comparable, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return fmt.Sprint(keys[i]) < fmt.Sprint(keys[j])
	})
	return keys
}
