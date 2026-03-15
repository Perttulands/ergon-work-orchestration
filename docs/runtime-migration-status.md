# Work Runtime Migration Status

This document tracks which parts of the former Prometheus Runtime plan are
already shipped in `work`, which remain active migration work, and which stay
explicitly deferred.

## Shipped Foundations

The following Phase 4 runtime items are already in the `work` authority:

- explicit adapters for `beads`, `gate`, and `relay`
- shared `br` client pilot in `work` and `gate`
- `relay spawn` compatibility wrapper over `work run`
- spine dual-write contract and `work spine-parity`
- checkpointed run state under `~/.work/runs/<run-id>/`
- `work resume <bead-id>` with lease and side-effect guards
- `work feed -> loop ingest -> loop query` contract coverage

These shipped pieces mean the migration is already a `work` migration, not a
new runtime product standing beside it.

## Active Migration Themes

The remaining runtime work should continue inside `work` and related master-plan
beads:

- expand resume crash coverage and recovery reporting
- keep caller-by-caller migration onto shared clients and adapters
- preserve observability parity evidence while dual-write remains active
- continue deleting duplicate planning or orchestration surfaces only after
  quiet-period and rollback gates pass

## Deferred Or Non-Goals

The following remain out of scope until a later bead explicitly revives them:

- a new standalone `prometheus-runtime` executable
- direct replacement of `work` commands or flags
- deleting legacy stores before parity and rollback proof
- broad runtime rewrites that bypass the current adapter ownership model

## Source Mapping

Useful material from the old PRD now lives here:

- design and invariants: [runtime-foundations.md](/home/polis/tools/work/docs/runtime-foundations.md)
- user-facing behavior and flags: [README.md](/home/polis/tools/work/README.md)

The old planning repo should be considered a Phase 5 deletion candidate only
after these docs remain current and the quiet-period checks succeed.
