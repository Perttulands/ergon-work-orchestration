# Prometheus Runtime → Work Migration

**Date:** 2026-03-15
**Source:** `/home/polis/projects/prometheus-runtime/PRD.md` (Draft v2, 2026-03-04)
**Status:** Mostly implemented. Remaining gaps noted below.

## What the PRD Proposed

A "Prometheus Runtime" layer inside `work` to replace raw `exec.Command()` calls with:
1. Typed adapter interfaces per tool (br, relay, gate, bv, loop, senate)
2. Persisted state machine with checkpoints and resume
3. Preflight capability probing
4. Error classification with remediation hints
5. Shared runtime services across run/decide/deliberate

## What Already Exists in Work

| PRD Feature | Status | Location |
|-------------|--------|----------|
| Adapter: br | Done | `internal/adapters/beads/client.go` via `brclient` |
| Adapter: gate | Done | `internal/adapters/gate/` |
| Adapter: relay | Done | `internal/adapters/relay/` |
| Ecosystem abstraction | Done | `internal/ecosystem/ecosystem.go` |
| State machine + checkpoints | Done | `internal/runstate/runstate.go` (17 phases, journal, lease) |
| Resume from any phase | Done | `internal/cli/resume.go` (Phase 4 extended to all phases) |
| Idempotency keys | Done | `CompletedSteps` + `CompletedEffects` with keys |
| Crash recovery | Done | Lease detection, steal, resume path |
| Preflight tool checks | Done | `checkTools()` in `run.go`, graceful degradation |
| Trace capture | Done | `internal/trace/trace.go` + spine dual-write |
| Contract tests | Done | `probes/contract_tests.sh` (34 tests, 7 contracts) |

## Remaining Gaps

| PRD Feature | Status | Priority |
|-------------|--------|----------|
| Typed error classification (Unsupported/Transient/InvalidOutput/Fatal) | Not done | P2 |
| `work preflight` standalone command | Not done | P2 |
| Shared runtime struct across run/decide/deliberate | Partial (ecosystem.go is shared) | P2 |
| Delete `ecosystem.go` when adapters fully migrated | Not done (still main integration layer) | P3 |
| Zero `exec.Command` in CLI/orchestrator code | Not done (ecosystem.go uses exec) | P3 |
| Contract test fixtures (fake binaries) | Partial (test_smoke.sh has mocks) | P2 |

## Decision

The PRD's core value — typed contracts, state persistence, crash recovery — is implemented.
The remaining gaps (error classification, preflight CLI, adapter purity) are evolutionary improvements, not blocking issues. They are tracked as P2 beads.

The `prometheus-runtime` repo contains only the PRD. No code. It can be archived in Phase 5 after confirming all useful concepts are captured here.
