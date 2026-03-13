# Ergon

![Ergon](images/ergon.jpg)

*A forge-scarred hand on the bellows. The other holds a manifest. Every task enters raw. Every task leaves shaped.*

---

Ergon is a task orchestrator for AI coding agents. You give it a task description and it handles everything around the actual work: gathering context from past runs, spawning a worker session in tmux, tracing what happens, checking quality on the way out, and closing the loop. One command takes a task from "somebody should do this" to "done, here's the record." It's the part of the workshop that isn't the hammer — it's the bench, the vise, the logbook.

In Aristotle's ethics, every thing has an *ergon* — a function, the activity it exists to perform. The ergon of a knife is to cut. The ergon of an eye is to see. The ergon of a craftsman is to take raw material and return it finished. Not faster. Not louder. *Finished.*

The forge doesn't care about your intentions. It cares about what comes out the other side.

## Quick CLI

```bash
work run "add JWT authentication" --repo myproject
work spawn hugo --repo myproject     # spawn a ready worker session
work send agent-hugo "now fix the tests"  # inject a prompt into a running session
work run "fix flaky auth test" --repo myproject  # strict mode is on by default
work --strict=false run "fix flaky auth test" --repo myproject  # explicitly relax optional integrations
work context <bead-id>         # what should I know before starting this?
work status                    # what's active right now
work history                   # recent runs with outcomes
work trace <bead-id>           # replay a run's timeline
work spine-parity              # compare legacy work traces with the Polis spine shadow stream
work feed --since 24h          # structured JSONL for learning-loop
work deliberate "should we split the auth module?" --type architecture
work decide "approve deploy?" --evidence pol-abc1,pol-abc2
```

## Lifecycle

```
description + bead
       │
       ▼
   ┌─────────┐
   │ context  │  ← past work, relevant beads, cass memory
   └────┬─────┘
        │
        ▼
   ┌─────────┐
   │  spawn   │  ← runtime-profile worker in tmux session
   └────┬─────┘
        │
        ▼
   ┌─────────┐
   │  trace   │  ← capture run output, decisions, errors
   └────┬─────┘
        │
        ▼
   ┌─────────┐
   │   gate   │  ← quality check (Cerberus, if on PATH)
   └────┬─────┘
        │
        ▼
   ┌─────────┐
   │  close   │  ← bead closed with outcome + trace
   └─────────┘
```

## Install

```bash
go build -o work ./cmd/work/
mv work ~/.local/bin/
```

## Design Docs

- `docs/runtime-foundations.md` — canonical runtime layering, contracts, recovery, and compatibility rules
- `docs/runtime-migration-status.md` — what migrated out of the old Prometheus Runtime plan and what remains active

## CLI Commands

### Global Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--strict` | `true` | Fail on optional integration errors by default (set `--strict=false` or `WORK_STRICT=0` to relax) |

---

### Observability Env

| Env var | Default | Description |
|---------|---------|-------------|
| `WORK_SPINE_DUAL_WRITE` | unset | When `1` or `true`, `work run` shadow-writes spine events alongside the legacy trace JSONL/index |
| `POLIS_SPINE_DIR` | `~/.polis/spine/events` | Override the spine event directory for dual-write and parity checks |

Legacy `work history` and `work trace` remain the operator-facing views during the D15 parity period. Spine output is additive until parity is proven.

---

### `work run <task>`

Run a task with the full orchestration lifecycle: bead creation, context gathering, worker session spawn, gate check, trace recording, and relay notification.

```
work run <task> [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--repo` | cwd | Repository path |
| `--citizen` | `worker` | Citizen name |
| `--runtime` | (profile default) | Worker runtime profile override |
| `--deadline` | `30m0s` | Maximum worker time before timeout kill |
| `--notify` | `` | Additional agent to notify on completion |

`<task>` may be a free-text description or a bead ID (format: `pol-<2..6 lowercase alnum>`). Bead IDs trigger additional validation via `br show`.

---

### `work resume <bead-id>`

Resume the latest unfinished `work run` for a bead from checkpointed run state under `~/.work/runs/<run-id>/`.

```
work resume <bead-id> [flags]
```

Current scope is intentionally narrow and fail-closed:

- latest unfinished run only
- post-worker recovery only
- `--force` steals a fresh lease when the original worker is gone

| Flag | Default | Description |
|------|---------|-------------|
| `--force` | `false` | Steal an active lease for the latest unfinished run |

---

### `work spawn <citizen>`

Spawn a ready worker session in `tmux` without running a full task lifecycle.

```
work spawn <citizen> [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--repo` | cwd | Repository path |
| `--session` | `agent-<citizen>` | tmux session name |
| `--runtime` | (profile default) | Worker runtime profile override |

---

### `work send <session> <prompt>`

Inject a prompt into a running tmux worker session via `tmux send-keys`.

```
work send <session> <prompt> [flags]
work send <session> --file prompt.txt
```

| Flag | Default | Description |
|------|---------|-------------|
| `--file` | `` | Read prompt from file instead of args |

---

### `work context [bead-id]`

Gather and print context for a bead or task. Sources include `bv` search/related/plan, `br` closed bead search, repo `PRD.md`, recent git log, citizen experience history, and learning-loop query results.

```
work context [bead-id] [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--citizen` | `` | Citizen name |
| `--repo` | cwd | Repository path |
| `--task` | `` | Task description for search |

---

### `work status`

Show active work runs (tmux sessions prefixed `work-`).

```
work status [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--json` | `false` | Output as JSON |

---

### `work history`

List recent completed runs from the SQLite index.

```
work history [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `-n, --limit` | `20` | Max runs to show |
| `--json` | `false` | Output as JSON |

---

### `work trace <bead-id>`

Pretty-print the trace timeline for a completed run.

```
work trace <bead-id> [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--json` | `false` | Output raw JSONL events instead of formatted timeline |

---

### `work spine-parity`

Compare the legacy `~/.work/traces` store against the Polis spine shadow stream. The command exits non-zero if runs are missing or if terminal state, gate verdict, or event ordering drift.

```
work spine-parity [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--work-dir` | `~/.work` | Work state directory containing legacy traces |
| `--spine-dir` | `~/.polis/spine/events` | Spine event directory |
| `--json` | `false` | Output the parity report as JSON |

---

### `work feed`

Output structured JSONL feed entries for learning-loop consumption.

```
work feed [flags]
```

Pass each emitted JSON line to `loop ingest -`, then query the same task later with `loop query "<task>" --json`. This is the supported D17 round-trip contract.

| Flag | Default | Description |
|------|---------|-------------|
| `--since` | `24h` | Time window filter (`<n>h`, `<n>d`, `<n>m`) |

---

### `work deliberate <question>`

Structured deliberation via Senate with bead tracking. Requires the `senate` binary.

```
work deliberate <question> [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--type` | `general` | Case type: `rule_evolution`, `gate_criteria`, `dispute`, `priority`, `architecture`, `general` |
| `--participants` | `3` | Panel agent count |
| `--evidence` | (empty) | Evidence paths or `bead:id` references |
| `--filed-by` | `` | Who files the case |
| `--state-dir` | `` | Senate state directory override |
| `--no-handoff` | `false` | Skip senate handoff bead creation |

---

### `work decide <question>`

Quick ruling workflow: creates a gate bead and sends a relay notification to the designated decider.

```
work decide <question> [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--evidence` | (empty) | Evidence bead IDs (comma-separated) |
| `--decider` | `athena` | Relay target agent |
| `--priority` | `normal` | Priority: `low`, `normal`, `high`, `urgent` |

---

### `work version`

Print the work version.

```
work version
```

---

### `work completion [bash|fish|powershell|zsh]`

Generate shell completion scripts.

```
work completion bash [--no-descriptions]
work completion fish [--no-descriptions]
work completion powershell [--no-descriptions]
work completion zsh [--no-descriptions]
```

---

## Configuration

### Runtime Profiles

Worker launch behavior is configured in JSON profiles, not hardcoded:

1. `$WORK_RUNTIME_CONFIG` (if set)
2. `~/.work/worker_profiles.json` (if present)
3. Built-in default profile

Profiles define runtime command + args, ready/trust detection patterns, model label, and optional agent-to-runtime mapping.

```json
{
  "default_runtime": "claude",
  "runtimes": {
    "<name>": {
      "command": "<binary>",
      "args": [],
      "model": "<model-id>",
      "ready_patterns": [],
      "trust_patterns": []
    }
  },
  "agents": {
    "<agent>": { "runtime": "<runtime-name>" }
  }
}
```

The built-in default defines two runtimes:
- `codex`: runs `codex --dangerously-bypass-approvals-and-sandbox`, model `gpt-5.3-codex`
- `claude`: runs `claude --dangerously-skip-permissions`, model `claude-sonnet`

Both `work run` and `work spawn` resolve runtime from this profile chain. `--runtime` overrides only for that invocation.

### Environment Variables

| Variable | Description |
|----------|-------------|
| `WORK_STRICT` | Override strict mode: `1`/`true`/`yes`/`on` enables, `0`/`false`/`no`/`off` relaxes |
| `WORK_RUNTIME_CONFIG` | Path to runtime profile JSON file (highest priority) |
| `LEARNING_LOOP_DIR` | Base directory for learning-loop scripts; falls back to `~/tools/learning-loop` |
| `HOME` | Used for all `~/.work` paths and fallback runtime config location |

### Failure Policy

Default mode is strict: optional integration errors become hard failures. Relaxed mode requires `--strict=false` or `WORK_STRICT=0`.

### Filesystem Layout

| Path | Contents |
|------|----------|
| `~/.work/` | Work root directory |
| `~/.work/traces/YYYY/MM/DD/trace-<bead>.jsonl` | JSONL trace files |
| `~/.work/index.db` | SQLite run index (auto-rebuilds from traces when empty) |
| `~/.work/citizens/<citizen>.md` | Per-citizen experience log |
| `~/.work/run-records/<bead>.json` | Run record files |
| `~/.work/feedback/<bead>.json` | Feedback collector output |
| `~/.work/senate-cases/senate-<bead>.json` | Senate deliberation case files |

## Dependencies

### Required

| Tool | Purpose |
|------|---------|
| `tmux` | Worker session management and status |

### Optional (degrade gracefully)

| Tool | Purpose |
|------|---------|
| `br` | Bead creation, search, and close |
| `gate` | Quality gate checks after worker completion |
| `relay` | Agent bus registration and result notifications |
| `bv` | Bead intelligence search, related, and plan |
| `loop` | Learning-loop query and run ingest |
| `senate` | Deliberation and handoff workflows (`work deliberate`) |

`git`, `bash`, and the runtime binaries configured in worker profiles (`codex`, `claude` by default) are also invoked during normal operation.

### Go Module Dependencies

- `github.com/spf13/cobra v1.10.2` — CLI framework
- `modernc.org/sqlite v1.46.1` — Pure-Go SQLite driver for run index
- `github.com/google/uuid v1.6.0`, `github.com/dustin/go-humanize v1.0.1` — Utilities

## Current Status

✅ Full run lifecycle: context gather, spawn, trace, gate, close
✅ Runtime profiles with fallback chain (env var, user config, built-in default)
✅ JSONL trace capture with SQLite index and auto-rebuild
✅ Strict mode for CI/production use
✅ `send` command for prompt injection into running sessions
✅ `deliberate` and `decide` governance flows
✅ `feed` export for learning-loop consumption
✅ Shell completions (bash, zsh, fish, powershell)

⚠️ `BrAgentState` calls (set agent working/idle state) are no-ops by design
⚠️ `deep` currently adds full `truthsayer` and `ubs` scans only. There is no separate risk gate until that check is real.
⚠️ Learning-loop template selection relies on an external `select-template.sh` script; skipped if absent
⚠️ SQLite index rebuild skips individually corrupted trace lines

## Part of Polis

Ergon is the doing-layer of the city. It doesn't exist alone.

- [Chiron](https://github.com/Perttulands/chiron-trainer) — trains the agents
- [Cerberus](https://github.com/Perttulands/cerberus-gate) — guards the gate
- [Hermes](https://github.com/Perttulands/hermes-relay) — carries the messages
- [Senate](https://github.com/Perttulands/senate) — deliberation and governance
- [Learning Loop](https://github.com/Perttulands/learning-loop) — memory across runs
- [Beads](https://github.com/Perttulands/beads-polis) — work unit tracking
- [Truthsayer](https://github.com/Perttulands/truthsayer) — verification
- [Horkos](https://github.com/Perttulands/horkos-oathkeeper) — oath enforcement
- [Argus](https://github.com/Perttulands/argus-watcher) — observation
- [UBS](https://github.com/Perttulands/ultimate_bug_scanner) — bug scanning
- [Polis Utils](https://github.com/Perttulands/polis-utils) — shared utilities

See `PRD.md` for full design details.

## License

MIT
