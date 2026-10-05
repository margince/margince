# Create an automation workflow

A task checklist for adding a new automation (a "when X happens, do Y" template a workspace can
enable) to the closed catalog (`internal/modules/automation`). Like the API, automation is
code-and-test, never data: you scaffold a handler, fill in its `Match`/`Plan`, register it, and let
the closure tests prove the wiring. For the closure, the single firing path, and the two permission
gates every firing passes through, see [explanation/automation.md](../explanation/automation.md).

This recipe covers the common case: **adding a new starter workflow handler** over the existing
closed trigger/action vocabulary. Adding a new *trigger kind* or *action type* to the vocabulary
itself is a larger change. Extend `catalog_triggers.go`/`catalog_actions.go` and
`catalog_closure_test.go`'s pinned lists together, and expect to touch the match-time gate's
permission table (`catalog_actions.go`'s `actionDefs`) too (step 6 covers the permission side).

## Steps

1. **Scaffold the handler**: `make gen-workflow NAME=<snake_case_name>` (from `backend/`, or
   `go run ./tools/gen-workflow <snake_case_name>` directly). This writes
   `internal/modules/automation/handlers_<name>.go` and its test stub: a handler that compiles,
   registers, and declares a placeholder trigger + tier, both files carrying the BUSL SPDX header.
   It is **write-once**: it refuses if either file already exists, so re-running it never clobbers
   your edits.

2. **Fill in `Match` and `Plan`** in the scaffolded handler:
   - `Match` decides whether this firing's event/candidate satisfies the automation's condition.
     Read the automation's own params off `ev.Params`. Follow the one-reader-per-knob pattern of an
     existing starter (e.g. `noActivityDays` in `handlers_clock.go`), which keeps a coarse
     pre-filter and the precise recheck from drifting apart.
   - `Plan` builds the `workflow.Effect`: one or more typed `workflow.Action` values from the
     **executor** vocabulary (`ports/workflow.ActionKind`), which differs from the catalog's
     `automation.ActionType`. `ApplyActions`' switch in `engine.go` lists the full closed set.
   - Set `Spec().Trigger` to **either** `EventType` (a bus event, e.g. `"deal.stage_changed"`)
     **or** `Schedule`, never both. `Schedule` is a non-empty marker string; `noActivityScheduleMarker`'s
     doc says why it documents intent only and is never parsed as a cron expression.
     `RegisterWorkflow` panics on a handler declaring neither, or both.
   - Set `Spec().Tier` to the risk tier the *action* carries (`mcp.TierGreen` for an
     auto-executing effect, `mcp.TierYellow` for one that must stage for approval).

3. **Add a `Catalog()` entry** in `automations_catalog.go` whose `Key` equals the handler's
   `Spec().Name` **character for character**. `Key == Spec().Name` is the only link between a
   catalog row a workspace enables and the handler that runs it.

   **The orphan-key trap:** a catalog entry whose `Key` has no matching registered handler is not
   an error. It reports `Active` in the UI, accepts params, and validates fine, then never fires and
   never logs anything. `HandleEvent` and the time-scan both dispatch by looking up
   `instances[h.Spec().Name]` for each *registered handler*; they never walk catalog entries. A
   typo in either name produces a silent no-op that nobody sees until someone notices the
   automation never ran.

   Set `Seeded: true` only if this becomes one of the starter templates every fresh workspace
   enrolls (step 7). Most new catalog entries are **authorable-only** (`Seeded: false`): fully
   instantiable through the API, never auto-enrolled.

4. **Register the handler** in `StarterWorkflows()` (`handlers_event.go`). `compose/workflows.go`
   already ranges over that slice when it builds the engine, so nothing else needs to change to wire
   a new starter into the running binary.

5. **Run the closure tests** (`go test ./internal/modules/automation/...`, part of `make check`):
   - `catalog_closure_test.go` proves every catalog action has a definition with a resolvable
     permission shape, and (for the vocabulary itself) that the trigger/action sets match the pinned
     lists in both directions.
   - `seed_test.go`'s `TestEveryCatalogKeyResolvesToARegisteredHandler` catches the orphan-key
     trap. It derives the registered-handler-name set from `StarterWorkflows()` (plus the
     externally-registered `assign_lead_owner`) and fails if any catalog key has no match. A starter
     you forgot to register, or a handler whose name does not match its catalog key, fails here.
   - If your new entry is a seeded one, `TestExactlySixSeededTemplatesWithPinnedNames` and
     `TestNonSeededCatalogEntriesStayOutOfTheSeed` both need updating to keep the pinned set in sync.

6. **Declare the permission tier** the action requires (`catalog_actions.go`'s `actionDefs`, if
   you are adding a new action type instead of reusing an existing one). The author-time ceiling
   (`ceiling.go`) checks it at creation time, and the match-time gate (`gate.go`) re-checks it
   against the *owner's* live RBAC on every firing. Get the `PermissionShape` right:
   - `PermissionPinned` if the action always touches one fixed entity type (e.g. `create_task`
     always creates an `activity`);
   - `PermissionTargetScoped` if the real entity type comes from whatever the trigger fired on
     (e.g. `assign_owner`/`set_field`, which route to `provider.Update{Ref: action.Target}`).

   Getting this wrong either gates the wrong object or leaves an action ungated.
   `assertPermissionIsExactlyOneShape` (`catalog_closure_test.go`) proves every action has one of
   the two shapes.

7. **Seed it, if it's a starter.** Only if `Seeded: true`, confirm `SeedStarterAutomationsTx`
   (`automations.go`) enrolls it and that its default (nil) params pass its own `Validate`
   (`TestSeededEntriesDefaultParamsPassTheirOwnValidate` proves this for every seeded entry).

8. **Verify**: run `make check` (the catalog closure tests, the orphan-key trap, `arch-lint`, the
   license header on your new files). Run `make test-integration` too if your handler reads through
   a new cross-module seam. If it needs a store this module cannot reach yet, add a seam in
   `seams.go` first; [explanation/automation.md](../explanation/automation.md#two-vocabularies-one-layer-apart)
   explains why a seam replaces a direct import of a sibling module.

## Notes

- A new cross-module read or write needs **its own seam**, declared in `seams.go` with only
  `ids`/`json`/stdlib types, and its real implementation wired as a compose adapter
  (`internal/compose/workflows.go`). Every module follows the same rule: a module never imports a
  sibling.
- A handler that needs another module's store is registered from that module's compose wiring.
  `assign_lead_owner`'s routing logic lives in `contacts`, so it registers there.
  `compose/workflows.go`'s doc says why `route_lead` and `assign_lead_owner` are two handlers under
  two names instead of one handler with two meanings.
- **There is no generated manifest.** `gen-workflow` scaffolds the handler and its test only; it
  never touches `Catalog()` or `StarterWorkflows()`. Steps 3 and 4 are reviewed hand-edits. The
  generator prints them as its "next steps" output instead of making them, so an automated pass
  never wires a catalog key to the wrong handler.
