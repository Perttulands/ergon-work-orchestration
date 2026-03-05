# Ergon

![Ergon](images/ergon.jpg)

*A forge-scarred hand on the bellows. The other holds a manifest. Every task enters raw. Every task leaves shaped.*

---

In Aristotle's ethics, every thing has an *ergon* — a function, the activity it exists to perform. The ergon of a knife is to cut. The ergon of an eye is to see. The ergon of a craftsman is to take raw material and return it finished. Not faster. Not louder. *Finished.*

This is the orchestration layer. A task enters as a description and a bead. Ergon gathers what you need to know (context from past work), spawns a worker (Claude Code in tmux), traces the run, gates the output, and closes the bead with the outcome. One command. Full lifecycle.

The forge doesn't care about your intentions. It cares about what comes out the other side.

## Quick CLI

```bash
work run "add JWT authentication" --repo myproject
work context <bead-id>         # what should I know before starting this?
work status                    # what's active right now
work history                   # recent runs with outcomes
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
   │  spawn   │  ← Claude Code worker in tmux session
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

## Dependencies

Requires: `tmux` — workers are spawned as tmux sessions.
Optional: `gate` — if on PATH, runs quality checks on completed work.
Optional: `relay` — publishes run events to other agents.

## Install

```bash
go build -o work ./cmd/work/
mv work ~/.local/bin/
```

## Part of Polis

Ergon is the doing-layer of the city. [Chiron](https://github.com/Perttulands/chiron-trainer) trains the agents. [Cerberus](https://github.com/Perttulands/cerberus-gate) guards the gate. [Hermes](https://github.com/Perttulands/hermes-relay) carries the messages. Ergon puts them to work.

See `PRD.md` for full design details.

## License

MIT
