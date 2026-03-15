# Work Runtime Foundations

This is the canonical home for the Prometheus Runtime design material that
still matters after the Phase 4 migration work.

`work` remains the product and the user-facing CLI. "Prometheus Runtime" was a
planning name for the runtime architecture under that CLI, not a separate tool
to ship.

## Why This Exists

`work` already orchestrates the real operator path:

- bead lifecycle through `br`
- context gathering from repo state and prior work
- worker spawn/send control
- gate enforcement
- trace capture
- relay notifications
- learning-loop feed output

The Phase 4 runtime work keeps that product surface stable while making the
integration edges explicit, typed, and recoverable.

## Runtime Invariants

- Keep `work` as the orchestration authority.
- Keep `relay` as the transport and coordination bus.
- Keep `work run`, `work resume`, `work feed`, `work history`, and `work trace`
  as the operator-facing commands.
- Fail closed when integration state is ambiguous.
- Treat every tool boundary as a contract, not as a best-effort shell scrape.
- Persist enough run state to recover post-worker steps without replaying
  unknown side effects.

## Architecture Layers

The current runtime shape is:

1. CLI entrypoints under `cmd/work` and `internal/cli`
2. Explicit adapters for `beads`, `gate`, and `relay`
3. Shared ecosystem/runtime helpers for orchestrated tool calls
4. Run-state persistence under `internal/runstate`
5. Trace plus spine dual-write under `internal/trace` and `internal/spine`
6. Context gathering and learning-loop feed/export surfaces

That preserves the existing product while steadily replacing ad hoc command
coupling with narrower owned modules.

## Tool Contract Rules

- `br`, `gate`, and `tmux` remain required for the full `work run` path.
- `relay`, `bv`, and `loop` are optional integrations unless a specific command
  requires them.
- Optional integrations may degrade only when strict mode is disabled.
- Every adapter change should land with command-contract tests before it is used
  on the main orchestration path.

## Recovery Rules

- Checkpoint run state under `~/.work/runs/<run-id>/`.
- Store the latest resumable state separately from raw trace output.
- Resume only steps whose side effects are explicitly known and guarded.
- Require explicit override for lease theft or other operator intervention.
- Prefer stopping with a clear error over guessing which external action ran.

## Observability Rules

- Keep legacy `~/.work` traces authoritative until spine parity passes.
- Treat spine dual-write as additive until parity evidence is green.
- Keep parity tooling operator-visible via `work spine-parity`.
- Keep `work feed -> loop ingest -> loop query` as one tested contract.

## Exec Boundary Rule

Raw process execution belongs at owned boundaries:

- adapters
- worker runtime launchers
- tightly scoped integration helpers with contract tests

Do not spread new direct CLI scraping across unrelated packages when an adapter
or shared helper already owns that surface.

## Compatibility Rule

When an upstream tool changes:

1. tighten or repair the adapter
2. update the contract test for that boundary
3. keep the operator command stable unless there is a deliberate migration bead

Compatibility wrappers stay in place until the caller matrix is clean.

## What This Replaces

This doc replaces the still-useful design material from
`/home/polis/projects/prometheus-runtime/PRD.md`:

- why the runtime exists
- layering and adapter boundaries
- dependency rules
- fail-closed recovery expectations
- compatibility discipline

The old PRD should now be treated as archival planning context, not the current
authority.
