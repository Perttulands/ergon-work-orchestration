# work

Task orchestration for coding agents. Context, spawn, trace, gate, close.

## Usage

```bash
work run "add JWT authentication" --repo myproject
work context <bead-id>         # what should I know before starting this?
work status                    # what's active right now
work history                   # recent runs with outcomes
```

`work run` creates a bead, gathers context from past work, spawns a Claude Code worker in tmux, traces the run, optionally runs a quality gate, and closes the bead with the outcome.

## Dependencies

Requires: `tmux` -- workers are spawned as tmux sessions.
Optional: `gate` -- if on PATH, runs quality checks on completed work.
Optional: `relay` -- publishes run events to other agents.

## Install

```bash
go build -o work ./cmd/work/
mv work ~/.local/bin/
```

See `PRD.md` for full design details.
