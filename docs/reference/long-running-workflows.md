# Long-running workflows registry

Registry required by [ADR-0302](../adr/0302-temporal.md). A workflow whose wall-clock exceeds one prod deploy cycle (~1 week) is permitted only if it is listed here with a versioning plan and replay tests.

## Why this registry exists

A workflow that outlives a deploy cycle will span code changes, so it must use `workflow.GetVersion` patching and carry replay tests (`workflow.NewReplayer`) proving old event histories still replay. Listing it here makes that obligation explicit and reviewable.

## Registered workflows

_None yet._ Liberal use of workflows for multi-step / compensable / cross-system operations; conservative use of long wall-clocks.

| Workflow | Owning service | Expected wall-clock | Versioning plan | Replay tests |
| --- | --- | --- | --- | --- |
| — | — | — | — | — |

## Adding an entry

1. Add a row above with the workflow, its owning service, and expected wall-clock.
2. Link the `workflow.GetVersion` plan (which change points are versioned).
3. Link the replay test covering representative historical event histories in CI.
