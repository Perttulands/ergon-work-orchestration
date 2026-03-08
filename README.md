# work

`work` is a Go CLI orchestration tool for Polis agent execution lifecycle management. It creates and tracks work beads, gathers contextual memory from prior runs, spawns runtime worker sessions in `tmux`, records JSONL traces, indexes run metadata in SQLite, performs optional quality gating and ecosystem signaling, supports governance flows via `deliberate` and `decide`, and exports structured learning-loop feed data. Optional integrations degrade gracefully when external tools are absent.

## Installation

```sh
# From source
cd /path/to/work
go build -o work ./cmd/work

# Or install to GOBIN
go install ./cmd/work
```

## Quick Start

```sh
# Run a task with full lifecycle
work run "pol-abc1" --citizen hestia

# Check active sessions
work status

# View recent runs
work history

# Gather context for a bead before starting manually
work context pol-abc1 --repo /path/to/repo
```

## CLI Commands

### Global Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--strict` | `false` | Fail on optional integration errors (also set via `WORK_STRICT=1`) |

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

### `work feed`

Output structured JSONL feed entries for learning-loop consumption.

```
work feed [flags]
```

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

### Environment Variables

| Variable | Description |
|----------|-------------|
| `WORK_STRICT` | Enable strict failure mode when set to `1`, `true`, `yes`, or `on` |
| `WORK_RUNTIME_CONFIG` | Path to runtime profile JSON file (highest priority) |
| `LEARNING_LOOP_DIR` | Base directory for learning-loop scripts; falls back to `~/tools/learning-loop` |
| `HOME` | Used for all `~/.work` paths and fallback runtime config location |

### Runtime Profile Resolution Order

1. File path in `WORK_RUNTIME_CONFIG`
2. `~/.work/worker_profiles.json`
3. Embedded default profile (`codex` and `claude` runtimes)

### Runtime Profile Schema

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

The embedded default profile defines:
- `codex`: runs `codex --dangerously-bypass-approvals-and-sandbox`, model `gpt-5.3-codex`
- `claude`: runs `claude --dangerously-skip-permissions`, model `claude-sonnet`

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

### Strict Mode

Strict mode turns fail-open optional integration steps into hard failures. Enable via `--strict` flag or `WORK_STRICT=1`. Without strict mode, missing optional tools emit warnings and execution continues.

## Dependencies

### Required

| Tool | Purpose |
|------|---------|
| `tmux` | Worker session management and status |

### Optional

| Tool | Purpose |
|------|---------|
| `br` | Bead creation, search, and close |
| `gate` | Quality gate checks after worker completion |
| `relay` | Agent bus registration and result notifications |
| `bv` | Bead intelligence search, related, and plan |
| `loop` | Learning-loop query and run ingest |
| `senate` | Deliberation and handoff workflows (`work deliberate`) |

### Supporting Commands

`git` (repo history and diff stat), `bash` (learning-loop scripts), and the runtime binaries configured in worker profiles (`codex`, `claude` by default) are also invoked during normal operation.

### Go Module Dependencies

- `github.com/spf13/cobra v1.10.2` — CLI framework
- `modernc.org/sqlite v1.46.1` — Pure-Go SQLite driver for run index
- `github.com/google/uuid v1.6.0`, `github.com/dustin/go-humanize v1.0.1` — Utilities

## Current Status / Limitations

- `BrAgentState` calls (set agent working/idle state) are currently no-ops by design.
- The `deep` gate level (when using `gate`) includes a `risk` gate that always passes with the message `risk scoring not yet implemented`.
- The learning-loop template selection relies on an external `select-template.sh` script; if absent, that context source is skipped.
- The SQLite index auto-rebuilds from trace JSONL files when the DB is empty, but individual corrupted trace lines are skipped during rebuild.
