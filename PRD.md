# work — Working Memory for Polis

## What It Is

The city's active mind. `work` is how Polis orchestrates. It holds what's happening now, what happened before, and what any citizen should know before starting a task. It orchestrates agent work not by dispatching like a factory, but by remembering — what context is relevant, who's done similar work, what went wrong last time.

Starting work is an act of memory.

The traces `work` captures become raw material for learning-loop, which distills patterns and proposes changes. Those proposals go through governance (Senate, Hierophant, Athena, Perttu) before becoming action. The loop closes through citizens, not automation.

## CLI

```
work run "add JWT authentication" --repo myproject
work run "fix the flaky test" --citizen mercury --deadline 30m

work status                    # what's active right now
work context <bead-id>         # what should I know before starting this?
work trace <id>                # what happened during this run?
work history                   # recent runs with outcomes
work feed --since 24h          # structured output for learning-loop
```

## What Happens When You Run `work run`

1. Creates a bead (`br create`) — the work has identity from the start
2. Gathers context — queries past beads, citizen experience, repo history
3. Assembles a rich prompt — task + context + quality expectations + citizen identity
4. Spawns a Claude Code worker in tmux (unsets CLAUDECODE, sets deadline)
5. Opens a trace — timestamped events as the worker proceeds
6. On completion: calls `gate check` for quality
7. Closes the trace with outcome
8. Records the experience — appends to citizen's lived history
9. Closes the bead

If `gate` isn't on PATH: skip step 6, warn, continue.
If `br` isn't available: still works, just no bead tracking.
Graceful degradation everywhere.

## The Context Engine

The most important feature. `work context <bead-id>` returns:

- Past beads by the same citizen in the same repo
- Patterns from learning-loop (if available)
- The citizen's own experience notes
- Known problem areas in the codebase
- What failed last time someone tried similar work

This is what closes the loop. Not a data pipeline — a memory that grows and makes each run wiser than the last.

## Trace Capture

Every run produces a structured JSONL trace:

```jsonl
{"ts":"...","event":"begin","agent":"zeus","task":"add auth","bead":"work-abc123"}
{"ts":"...","event":"tool_call","tool":"bash","cmd":"npm init","duration_ms":1200}
{"ts":"...","event":"file_write","path":"src/auth.ts","lines":45}
{"ts":"...","event":"gate_result","pass":true,"score":0.87}
{"ts":"...","event":"end","outcome":"success","duration_s":340}
```

Traces stored by date: `~/.work/traces/2026/02/23/trace-abc123.jsonl`
SQLite index for fast queries.

## Storage

```
~/.work/
├── traces/           # JSONL per run, organized by date
├── index.db          # SQLite for queries
├── config.toml       # optional tuning
└── citizens/         # per-citizen experience
    ├── mercury.md
    ├── luna.md
    └── ...
```

## Technical

- **Language:** Go
- **Dependencies:** tmux (for workers), br (optional), gate (optional)
- **Integration:** subprocess calls, JSON on stdout, exit codes
- **Concurrency:** multiple workers tracked simultaneously
- **Retention:** configurable, default 30 days for traces

## Dependencies

Requires: `tmux` -- workers are spawned as tmux sessions.
Optional: `gate` -- if on PATH, runs quality checks on completed work.
Optional: `relay` -- publishes run events to other agents.

## What It Does NOT Do

- Quality gating (that's `gate`)
- System monitoring (that's `sentinel`)
- Long-term pattern analysis (that's learning-loop)
- Display a UI (that's Agora when it exists)

## Tmux ENTER Delivery (pol-d401)

Prompt injection into tmux worker sessions relies on sending the ENTER key after
content is pasted. This is fragile — tmux `send-keys ENTER` can silently fail if
the TUI hasn't finished processing the paste, or if the key is sent with the wrong
casing. Every send path must follow the same hardened pattern:

### Requirements

1. **Double-ENTER on all send paths.** First ENTER submits; second is a no-op safety net.
   Applies to: `sendKeys`, `sendPrompt`, `SendFollowUp`, `waitForReady` trust dismiss.
2. **Literal text flag (`-l`) for user-supplied content.** `send-keys` without `-l`
   interprets key names (e.g. "Enter", "Space"). Text from prompts and follow-up
   messages must use `-l` or the load-buffer/paste-buffer path.
3. **Post-paste delay before ENTER.** Currently hardcoded at 200ms. Must be sufficient
   for the TUI to register pasted content before ENTER arrives. Consider making
   configurable via `WORK_ENTER_DELAY_MS` env var (default 200).
4. **ENTER delivery verification (stretch).** After sending ENTER, optionally capture
   the pane and confirm the prompt text is no longer in the input area. Retry once
   if verification fails. Guards against dropped keystrokes under system load.

### Affected Code Paths

| Path | Current | Required |
|------|---------|----------|
| `sendKeys()` | text + 2×ENTER | text via `-l` + 2×ENTER |
| `sendPrompt()` | load-buffer + paste-buffer + 2×ENTER | ✓ (correct) |
| `SendFollowUp()` | `-l` text + **1×ENTER** | `-l` text + **2×ENTER** |
| `waitForReady()` trust | 2×ENTER | ✓ (correct) |

### Non-Goals

- Replacing tmux with a different IPC mechanism
- Adding retry loops that mask deeper failures

## Success

- `work run "task"` completes a full lifecycle without human intervention
- `work context` returns useful, relevant advice from past runs
- After 20 runs, measurably better outcomes than run 1
- Another agent can read a trace and understand what happened
- Works standalone (no gate, no br) with graceful degradation
- Every tmux send path delivers ENTER reliably — no silent failures
