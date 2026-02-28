# Testing — work

Test quality assessment per the [Polis Test Quality Rubric](../TEST_RUBRIC.md).

## Current Rubric Scores

| Dimension | Score | Notes |
|-----------|-------|-------|
| **E2E Realism** | 3/5 | Two full pipeline integration tests (happy + degraded) exercise context→trace→index→close-reason→citizen. Run orchestration tested with mocked tmux/br/gate. Deliberate and decide workflows tested end-to-end. Missing: no test exercises `work run` with real tmux and a real (or realistic) Claude session. |
| **Unit Test Behaviour Focus** | 4/5 | Detection logic (detectReady, detectCompletion, isStillWorking) tested via observable outputs. Format/parse functions tested by contract. assemblePrompt and buildRunRecord tested by verifying output content, not internals. A few tests are still implementation-coupled (e.g. verifying temp file creation in sendPrompt). |
| **Edge Case & Error Path** | 3/5 | Graceful degradation thoroughly tested (every external tool: br, bv, relay, gate, loop). Invalid JSON, missing files, tool failures, timeouts all covered. Missing: concurrent Spawn with same session name race, partial write failures (disk full), learning-loop script hangs, index SQLite contention. |
| **Test Isolation & Reliability** | 2/5 | Good patterns: t.TempDir(), t.Setenv(), testutil.SandboxPATH(). Bad patterns: 8+ tests use real tmux with time.Sleep() — inherently flaky on slow systems. Worker tests take 30s due to timing. Session names include time.Now().Format("150405") which could collide if two tests run in the same second. |
| **Regression Value** | 4/5 | Integration tests (TestFullPipelineIntegration, TestRunTaskOrchestration) would catch most regressions in the data flow. State machine tests (detectReady, detectCompletion) protect the most fragile logic. Trace format tests ensure JSONL schema stability. Gate result interpretation tested with pass/fail/invalid-JSON. TestRunTaskTraceRecordedOnSpawnError ensures error traces are always written. Spawn orchestration (the highest-risk function) is only tested at the component level, not end-to-end. |

**Total: 16/25 — Grade C** (functional but with known gaps)

## What the Suite is MISSING

**Critical gaps (would catch real bugs agents hit):**

1. **Spawn end-to-end with mock claude** — Spawn is the most complex function and only 22.5% covered. Its orchestration (create session → unset env → launch claude → wait ready → send prompt → wait completion → deadline watchdog) is untested as a unit. The deadline watchdog goroutine and its mutex synchronization are completely untested. A race condition here would cause zombie sessions or lost API credits.

2. **Concurrent run safety** — Two `work run` invocations with overlapping session names. The tmux session naming scheme (`work-<beadID>`) should prevent this, but there's no test proving it.

3. **Large prompt handling** — sendPrompt uses tmux load-buffer which has buffer size limits. No test verifies behaviour with prompts > 64KB (realistic for context-heavy tasks).

4. **Index SQLite contention** — Two goroutines calling index.Record() simultaneously. SQLite handles this with WAL mode, but there's no test proving the locking works.

5. **Trace file on read-only filesystem** — If ~/.work is on a read-only mount (container, CI), trace.Open fails. runTask should handle this gracefully but there's no test.

**Moderate gaps:**

6. **Feed command filtering** — parseSince is tested but the actual time-window filtering in the feed output is only tested in one integration test.

7. **Context section ordering contract** — Agents consume the context markdown and depend on section headers. If a header changes, agent prompts break. No contract test enforces the header names.

8. **Status command with real tmux** — parseTmuxSessions is well-tested but getActiveSessions (which calls real tmux) is only tested via the command, not for edge cases like many sessions or sessions with special characters.

## Test Architecture Notes

- **testutil.SandboxPATH** is the primary mocking pattern. It creates fake shell scripts for external tools (br, bv, gate, relay, loop, tmux) and restricts PATH. System tools (sh, bash, etc.) are symlinked from the real PATH.
- **Worker tests** use real tmux sessions (not mocks) for integration testing. This provides high fidelity but makes tests slow (~30s) and timing-dependent.
- **CLI tests** test through cobra command execution, which exercises flag parsing, argument validation, and output formatting in one shot.
- **Trace tests** are pure — no external dependencies. They verify the JSONL format contract directly.

## Changelog

### 2026-02-28 — Agent: zeus
- Added: TestRunTaskTraceRecordedOnSpawnError — verifies trace file gets begin+end events even when worker.Spawn fails (critical for debugging agent failures)
- Added: TestRunTaskGateFailOutcome — verifies gate_fail outcome is correctly propagated through the pipeline and learning-loop receives accurate data
- Added: TestSpawnSessionConflict — verifies Spawn fails clearly when session already exists (catches zombie session bugs)
- Added: TestWaitForCompletionMaxWait — verifies the timeout safety net returns instead of hanging forever
- Added: TestRebuildSkipsCorruptTraces — verifies index rebuild skips truncated trace files without crashing (real crash-recovery scenario)
- Added: TestFormatCloseReasonShortDuration, TestFormatCloseReasonGateFail — exercises the seconds-only and gate:fail formatting paths
- Added: Mock-based happy+error path tests for all ecosystem functions (GateCheck, RelaySend, BrClose, IngestRun, CollectFeedback, etc.) — verifies correct argument passing and error propagation
- Added: Worker integration tests for sendKeysRaw, sendPrompt, waitForReady (banner/timeout/trust), waitForCompletion (detect/kill)
- Changed: Replaced TestSpawnCleansUpOnCreateFailure (flawed — tmux accepts nonexistent workdirs) with TestSpawnSessionConflict (tests actual failure mode)
- Coverage delta: 72.9% → 84.4% (meaningful: 50+ new tests, ~20 covering real failure modes agents encounter)
