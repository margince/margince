<!-- prose:plain -->
# Create an automation workflow

This is a task list for adding a new automation to the closed catalog (`internal/modules/automation`).
An automation is a `when X happens, do Y` template that a workspace can turn on. Like the API,
automation is code and tests, never data. You make the handler from a template, fill in its `Match` and
`Plan`, and register it. Then the closure tests prove the wiring.

For the closure, the one path that fires an automation, and the two permission gates every firing passes
through, see [explanation/automation.md](../explanation/automation.md).

The usual case is **adding a new starter workflow handler** over the closed list of
triggers and actions we already have. To add a new *trigger kind* or *action type* to that list is
more work. Change `catalog_triggers.go`/`catalog_actions.go` and the pinned lists in
`catalog_closure_test.go` together. Expect to touch the permission table of the gate at match time
(`actionDefs` in `catalog_actions.go`) too. Step 6 covers the permission side.

## Steps

1. **Make the handler from the template.** Run `make gen-workflow NAME=<snake_case_name>` from `backend/`.

   You can also run `go run ./tools/gen-workflow <snake_case_name>` directly. This writes
   `internal/modules/automation/handlers_<name>.go` and a first test file. The handler builds, registers,
   and declares a stand-in trigger and tier. Both files have the BUSL SPDX header.

   It **writes only once**: it refuses if either file already exists. So to run it again never writes
   over your edits.

2. **Fill in `Match` and `Plan`** in the new handler.

   - `Match` decides whether the event or candidate of this firing meets the rule of the automation.
     Read the automation's own values from `ev.Params`. Follow how a starter we already have does it,
     where one reader serves each setting, such as `noActivityDays` in `handlers_clock.go`. That
     keeps a first filter and the exact second check from moving out of step.
   - `Plan` builds the `workflow.Effect`: one or more typed `workflow.Action` values from the list of the
     **executor** (`ports/workflow.ActionKind`). That list is not the same as the `automation.ActionType`
     of the catalog. The switch in `ApplyActions` in `engine.go` lists the full closed set.
   - Set `Spec().Trigger` to **either** `EventType` (an event on the bus, such as
     `"deal.stage_changed"`) **or** `Schedule`, never both. `Schedule` is a mark that must not be empty.
     The doc of `noActivityScheduleMarker` says why it only records what it is for, and is never read as a
     cron line. `RegisterWorkflow` stops with a panic on a handler that declares neither, or both.
   - Set `Spec().Tier` to the risk tier the *action* carries. That is `mcp.TierAutoExecute` for an action
     that runs on its own, and `mcp.TierConfirmationRequired` for one that must wait for approval.

3. **Add a `Catalog()` entry** in `automations_catalog.go` whose `Key` matches `Spec().Name` of the handler.

   It must match **letter for letter**. `Key == Spec().Name` is the only link between a catalog row that
   a workspace turns on and the handler that runs it.

   **The trap of a key with no handler:** a catalog entry whose `Key` matches no registered handler is
   not an error. It shows as `Active` on the screen, accepts values, and passes its checks. But then it
   never fires, and never logs a line.

   `HandleEvent` and the time scan both find a handler by looking up `instances[h.Spec().Name]` for each
   *registered handler*. They never read the catalog entries. So a typing error in either name gives a
   step that does nothing and says nothing. No one sees it until someone sees the automation never runs.

   Set `Seeded: true` only if this is to be one of the starter templates that every new workspace gets
   (step 7). Most new catalog entries are **not seeded** (`Seeded: false`). You can create them in
   full through the API, but no workspace gets them on its own.

4. **Register the handler** in `StarterWorkflows()` (`handlers_event.go`).

   `compose/workflows.go` already loops over that list when it builds the engine. So nothing else needs to
   change to wire a new starter into the running binary.

5. **Run the closure tests**: `go test ./internal/modules/automation/...`, which is part of `make check`.

   - `catalog_closure_test.go` proves that every catalog action has a permission shape
     that works. For the list of triggers and actions itself, it proves that the sets match the pinned
     lists both ways.
   - `TestEveryCatalogKeyResolvesToARegisteredHandler` in `seed_test.go` finds the trap of a key with no
     handler. It builds the set of registered handler names from `StarterWorkflows()`, plus
     `assign_lead_owner`, which registers from outside. It fails if any catalog key has no match. A starter
     you did not register, or a handler whose name does not match its catalog key, fails here.
   - If your new entry is a seeded one, update both `TestExactlySixSeededTemplatesWithPinnedNames` and
     `TestNonSeededCatalogEntriesStayOutOfTheSeed`, to keep the pinned set in step.

6. **Declare the permission tier** the action needs, in `actionDefs` in `catalog_actions.go`.

   You only do this if you add a new action type, and do not use one we already have. The limit when
   someone writes the automation (`ceiling.go`) checks it when the automation is created. The gate at match time
   (`gate.go`) checks it again against the live RBAC of the *owner* on every firing. Get the
   `PermissionShape` right:

   - `PermissionPinned` if the action always touches one fixed record type. For example, `create_task`
     always creates an `activity`.
   - `PermissionTargetScoped` if the real record type comes from whatever the trigger fired on. For
     example, `assign_owner` and `set_field` go to `provider.Update{Ref: action.Target}`.

   If you get this wrong, it either gates the wrong record or leaves an action with no gate.
   `assertPermissionIsExactlyOneShape` (`catalog_closure_test.go`) proves every action has one of the two
   shapes.

7. **Seed it, if it is a starter.** Do this only if `Seeded: true`.

   Confirm that `SeedStarterAutomationsTx` (`automations.go`) adds it to each new workspace. Also confirm
   that its default (`nil`) values pass its own `Validate`.
   `TestSeededEntriesDefaultParamsPassTheirOwnValidate` proves this for every seeded entry.

8. **Check it**: run `make check`.

   That covers the catalog closure tests, the trap of a key with no handler, `arch-lint`, and the license
   header on your new files. Run `make test-integration` too, if your handler reads through a new seam
   between modules. If it needs a store this module cannot reach yet, add a seam in `seams.go` first.
   The page [explanation/automation.md](../explanation/automation.md#two-vocabularies-one-layer-apart) explains
   why a seam replaces a direct import of a sibling module.

## Notes

- A new read or write between modules needs **its own seam**. Declare it in `seams.go`, with only
  `ids`, `json` and types from the Go `stdlib`. Wire its real code as a compose adapter
  (`internal/compose/workflows.go`). Every module follows the same rule: a module never imports a
  sibling.
- A handler that needs the store of another module registers from the compose wiring of that module. The
  routing code of `assign_lead_owner` lives in `contacts`, so it registers there. The doc of
  `compose/workflows.go` says why `route_lead` and `assign_lead_owner` are two handlers under two names,
  and not one handler with two jobs.
- **There is no generated list of handlers.** `gen-workflow` makes the handler and its test, and nothing
  more. It never touches `Catalog()` or `StarterWorkflows()`. Steps 3 and 4 are edits by hand that a
  human reviews. The tool prints them as its "next steps", and does not make them. So a pass that
  runs on its own never wires a catalog key to the wrong handler.
