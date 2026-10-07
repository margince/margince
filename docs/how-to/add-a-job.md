# Add a background job

Add a job kind for [River](https://riverqueue.com), the Postgres-backed queue the backend runs its
background work on. You declare the kind in `backend/api/jobs.yaml`, run `make gen`, then write the
args type and the worker; a kind the file does not declare does not compile. Background:
[explanation/job-fleet.md](../explanation/job-fleet.md) and
[explanation/write-backbone.md](../explanation/write-backbone.md).

Most new work is two kinds: a dispatcher that enumerates the fleet and enqueues, and a
worker that does the work. Declare both. A pass that touches every tenant is a dispatcher plus a
workspace worker, never a fleet loop inside one job row.

## Steps

1. **Declare the kind** in `backend/api/jobs.yaml`, under `kinds:`, alphabetically. Every kind
   requires these fields:

   - `role`: `dispatcher` or `worker`.
   - `go_type`: the compose args struct that returns this kind. It must match
     `^[A-Z][A-Za-z0-9]*Args$` and be unique across the file (one args struct is one River kind).
   - `queue`: must name an entry in the file's own `queues:` block. Reuse `default` unless the
     work is long or outbound-bound. A new queue owes a `reason` for having been split out of
     the default pool, and the census holds its `max_workers` equal to compose's `jobQueues()`.
   - `timeout`: there is no default. Pick one of the three forms: a literal (`2m`),
     `{derived: goConstant, value: 4h, reason: …}`, or
     `{none: true, reason: …}`. Every dispatcher in the file takes `2m`, for one shared reason: a
     dispatcher runs one indexed scan and one insert-many, database-bound, with no model call, crawl
     or outbound request in it.
   - `opts_owner`: `fan_out` if a dispatcher's fan-out builds this kind's insert options, `args`
     if the args type's own `InsertOpts()` does, `caller` if scattered enqueue sites do.

   Then by role. A dispatcher also declares `cadence` (a duration, `{setting: key}`, or
   `on_demand`) and the pair `fans_out_to` + `fan_out_unit` (`workspace`, `connection` or `build`).
   A fan-out child (`opts_owner: fan_out`) also declares `max_attempts`. Three is the house
   number; a different one says why in the entry's `reason:`.

   And by exception:
   - `registration: {when: [ConfigField, …], absent: registers_nothing |
     registers_anyway}` when the kind needs a dependency the deployment may not have wired.
   - `args:` for every field of the args struct (step 3).
   - `fault: {nil_after_logging: …}` only if the worker logs and returns `nil`. Name the durable
     retry policy that later redoes the work.

2. **Regenerate** with `make gen`. `backend/tools/gen-jobs` validates the whole contract and rewrites
   two files (never hand-edit either): `backend/internal/platform/jobs/specs_gen.go`, the Spec table every
   reader walks, and `backend/internal/compose/jobkinds_gen.go`, the closed `declaredJobArgs` union plus one
   compile-time role assertion per kind. This step fails on a contract that cannot hold: a
   missing timeout, a fan-out to a kind nobody declares, a cadence on an enqueued worker.

   The build does not compile yet: the generated assertions name args types you have not written.

3. **Write the args type** in the matching `backend/internal/compose/jobs_<concern>.go` (create one for a
   new concern). A tenant-scoped worker carries its tenant in a field named `Workspace`, because Go forbids
   a method and a field of the same name, and the wire key is fixed:

   ```go
   type CloseDateWorkspaceArgs struct {
       Workspace ids.UUID `json:"workspace_id"`
   }

   func (CloseDateWorkspaceArgs) Kind() string { return "close_date_workspace" }

   // WorkspaceID binds this pass to its tenant (jobs.WorkspaceScoped).
   func (a CloseDateWorkspaceArgs) WorkspaceID() ids.UUID { return a.Workspace }
   ```

   A dispatcher declares the empty marker instead, and carries no workspace key at all:

   ```go
   func (CloseDateSweepArgs) Kind() string { return "close_date_sweep" }

   // FleetWide marks this a dispatcher: it enumerates and enqueues,
   // and does no tenant work of its own (jobs.FleetWide).
   func (CloseDateSweepArgs) FleetWide() {}
   ```

   Check that `Kind()` returns this kind's string. If you copy the args struct beside it and keep
   the neighbour's string, every other gate on this path still passes. Every args
   field must be declared in step 1, as `id` or as a scalar with the reason it is safe: `river_job`
   has no workspace column and no RLS, so a job names a row and the worker reads it.

4. **Write `Work`**, a method on your worker type, and nothing else. Do not declare `Timeout`,
   `NextRetry` or `Middleware`; the worker reaches River only as `jobs.WorkOnly[T]`, so those are the
   declaration's to answer. Bind the workspace through the shared helper and return through
   `jobs.FaultContext`:

   ```go
   func (w *closeDateWorkspaceWorker) Work(ctx context.Context, job *river.Job[CloseDateWorkspaceArgs]) error {
       wsCtx, err := workspaceJobCtx(ctx, job.Args)   // refuses a zero workspace, binds it to the context
       if err != nil {
           return jobs.FaultContext(ctx, err)
       }
       wsCtx = principal.WithActor(wsCtx, principal.Principal{Type: principal.PrincipalSystem, ID: "system:close-date"})
       wsCtx = principal.WithCorrelationID(wsCtx, ids.NewV7())
       return jobs.FaultContext(ctx, w.corrector.SweepWorkspace(wsCtx))
   }
   ```

   A pass that writes binds its own actor and mints its own correlation id, as above: there is no
   HTTP middleware behind a job, and `storekit.Emit` errors without a correlation id
   ([write-backbone.md](../explanation/write-backbone.md#6-correlation--causation-the-trace)).
   `workspaceJobCtx` binds the tenant and only the tenant.

   A dispatcher's `Work` instead reaches one of the fleet helpers (`dispatchWith`, `dispatchOne`,
   `runPerWorkspace`, `runPerEveryWorkspace`, `runEach`) and issues no tenant write of its own:

   ```go
   func (w *closeDateSweepWorker) Work(ctx context.Context, _ *river.Job[CloseDateSweepArgs]) error {
       return jobs.FaultContext(ctx, runPerWorkspace(ctx, w.pool, w.correctWorkspace))
   }
   ```

5. **Register in `backend/internal/compose/jobs.go`** through `addDeclaredWorker`, from the wiring helper
   that matches the kind's gating (`addModelLaneJobs`, `addDatabaseOnlySweepJobs`,
   `addCapturePipelineJobs`, `addGmailCaptureJobs`, or a self-registering helper of
   your own that also returns its periodic entries):

   ```go
   addDeclaredWorker[CloseDateWorkspaceArgs](reg, &closeDateWorkspaceWorker{corrector: NewCloseDateCorrector(pool, log)})
   ```

   Never call `river.AddWorker` / `AddWorkerArgs` / `AddWorkerSafely`; forbidigo blocks all three
   outside the one sanctioned line in `jobregistry.go`. The `backend/cmd/worker` binary runs every
   registered worker.

   If the kind's registration is gated on a new `JobRunnerConfig` field, add the field, name it in
   the entry's `registration.when`, and answer it in `configDependencies`
   (`backend/internal/compose/jobcensusconfig.go`). `periodicFor` panics at boot on a path that file does
   not answer.

6. **Place the schedule, or the fan-out.**

   - A dispatcher with a clock gets one line in `wireJobs`' `slices.Concat` block:
     `periodicFor(cfg, CloseDateSweepArgs{})`. `periodicFor` reads the cadence, the registration
     posture, and whether there is a schedule at all from the declaration, never from where the call
     sits. Do not build a `river.PeriodicJob` by hand, and do not touch River's runtime
     `PeriodicJobBundle`; forbidigo blocks the latter.
   - A fan-out over the fleet calls
     `dispatchWith(ctx, workspaces, insert, workspaceSweepOpts(ChildArgs{}.Kind()), argsFor)`. Pass the
     insert of a transaction you already hold when the fan-out must join it. It inserts the whole
     fan-out as one `InsertMany`, because a partial fan-out that fails and retries re-runs the
     workspaces whose children already completed.
   - A pass that runs each workspace in this process calls `runPerWorkspace(ctx, pool, run)`, which
     attempts every workspace and joins the failures.
   - A fan-out per connection or per build loops `dispatchOne(ctx, args, callerOpts)`, whose
     options are decided by the child's declared `opts_owner`: pass `callerOpts` for `caller` and
     `nil` for the other two. Passing the wrong one panics rather than silently dropping a uniqueness
     window.

7. **Verify** with `make check`. The census, the job gates under `backend/gates/` and the boot-time
   totality check all run here. Add `make test-integration` if the pass touches tenant tables: the real-Postgres lane is
   what proves the workspace binding, and the timeout wiring suite reads the declared wall clock back
   off a live River client.

8. **Commit the contract and the generated output together**: `api/jobs.yaml`, `specs_gen.go` and
   `jobkinds_gen.go` in the same commit. A missed `make gen` fails the drift gate, and a
   half-regenerated pair is caught separately by the contract-hash check the two generated files
   share.

## What each gate is telling you

The failures you are most likely to meet, in the order you would meet them:

| Where | Message | What it means |
|---|---|---|
| `make gen` | `kind "x": declares no timeout — an absent one is River's silent 1-minute default, which is what this contract removes` | Pick one of the three `timeout` forms. There is no default |
| `make gen` | `kind "x": is a dispatcher that fans out to nothing` / `fans_out_to "y", whose role is "dispatcher"` | A dispatcher must declare `fans_out_to` + `fan_out_unit`, and the child must be `role: worker` |
| `make gen` | `kind "x": declares a cadence but its role is "worker"` | An enqueued worker is never ticked. Move the cadence to the dispatcher |
| `make gen` | `kind "x": opts_owner is fan_out but no max_attempts is declared` | The fan-out helper reads that number and nothing else supplies it; its absence is River's silent 25-rung ladder |
| `make gen` | `kind "x": args field "F" is declared a scalar with no reason` | A value that is not an id must say why it is safe in a table Art. 17 erasure never reaches |
| `go build` | `CloseDateWorkspaceArgs does not satisfy declaredJobArgs` | The kind is not in `api/jobs.yaml`, or you have not run `make gen` since declaring it |
| `golangci-lint` | `register through addDeclaredWorker — a kind absent from api/jobs.yaml is not in declaredJobArgs, and a direct registration also escapes jobs.Govern and the boot-time totality check` | You called River's registration API directly |
| `golangci-lint` | `a periodic tick is api/jobs.yaml's to declare — give the kind a cadence: and let periodicFor build it` | You reached River's runtime `PeriodicJobBundle` from inside a worker |
| boot | `jobs: N kind(s) not declared in api/jobs.yaml: … — add them there and run` `make gen` | A kind got past the compiler: a kind alias, a fixture path, or a hand-edited generated union |
| boot | `compose: a worker is registered under a kind the contract pairs with another args type` | `Kind()` returns the neighbouring kind's string. River would work those rows under the neighbour's timeout, queue and cap, and your kind would have no worker |
| boot | `compose: fanning out to "y", which no declared kind names in fans_out_to` | Declare the dispatcher's fan-out edge; `fans_out_to` is the registry of what may be fanned out to at all |
| `jobrole_test.go` | `X is a River job (it declares Kind()) but is not in api/jobs.yaml` | An args type with no declaration. It would run on River's one-minute default and be invisible to both job surfaces |
| `jobrole_test.go` | `X declares both WorkspaceID() and FleetWide()` | A job does one workspace's work or dispatches, never both |
| `jobwirekey_test.go` | `X.F ships as json:"ws", want json:"workspace_id"` | A divergent key is invisible to `args->>'workspace_id'`, and a null there reads as a dispatcher instead of tenant work the query cannot see |
| `jobwirekey_test.go` | `X is a dispatcher (it declares FleetWide()) but ships a json:"workspace_id" key` | Put the workspace on the children it enqueues; the dispatcher's own args carry none |
| `jobfleetwide_test.go` | `W works FleetWide args X but never fans out` | A dispatcher must reach one of the fleet helpers (`dispatchWith`, `dispatchOne`, `runPerWorkspace`, `runPerEveryWorkspace`, `runEach`). If it does tenant work instead, it is `WorkspaceScoped` |
| `jobfleetwide_test.go` | `W works FleetWide args X and issues a tenant write` | Move the write into the workspace worker, where it can succeed or fail as its own row |
| `jobfault_test.go` | `a worker return must be nil, jobs.Fault(...), or a river control return — a raw cause is written verbatim into river_job.errors` | Wrap the return in `jobs.FaultContext(ctx, err)` |
| `jobfault_test.go` | `W logs an error and returns nil — River will record this job as completed while the work failed` | Return the failure, or ratify it with `fault: {nil_after_logging: …}` naming the retry policy that later redoes the work |
| `jobargscontent_test.go` | `X.F is not declared in api/jobs.yaml — say what it carries` | Every compiled args field needs a declaration; nothing is inferred from its name |
| `jobargscontent_test.go` | `X.F is declared an id but its name reads like content … and nothing says why` | Give the field a `{reason: …}` or make it carry an id. The word list cannot decide whether it is safe; it only requires a stated reason |
| `jobcensus_test.go` | `job census: api/jobs.yaml and the wiring disagree: …` | The two ends drifted: a kind declared and never wired, a `{derived: …}` constant that moved, a queue bound that changed in one place only |

## Notes

- **Adding a queue** is a contract change plus a compose change, and the census holds the two equal in
  both directions. A bound moved in one place only shows operators a number no client runs at. A
  queue declared but never built takes fan-out children that no client works.
- **Kind strings are persisted state.** Renaming one strands every live row that carries it. Correct
  the Go type name if it reads wrong; never the kind.
- `opts_owner: caller` is not checked: the options live at the enqueue sites, so the declared queue
  only documents them. Prefer `fan_out` when the file can own the options.
- A one-off enqueue of a fan-out child (an event enqueuing one known workspace's pass)
  uses `oneOffChildOpts`, not `workspaceSweepOpts`. It omits the `sweep` tag and the active-state
  uniqueness. Such a job is not that workspace's share of a fleet pass, and deduplicating it against
  a running pass would drop the rows the event fired about.
- Reading the fleet back: the gauges, `GET /v1/admin/job-health`, and the caveats that apply to
  a kind's rows are documented in
  [reference/configuration.md → Reading the job surfaces](../reference/configuration.md#reading-the-job-surfaces).
