<!-- prose:plain -->
# Add a background job

Add a job kind for [River](https://riverqueue.com), the queue on Postgres that the backend runs its
background work on. You declare the kind in `backend/api/jobs.yaml`, run `make gen`, then write the args
type and the worker. A kind the file does not declare does not compile. Background:
[explanation/job-fleet.md](../explanation/job-fleet.md) and
[explanation/write-backbone.md](../explanation/write-backbone.md).

Most new work is two kinds. A dispatcher lists every workspace and puts jobs in the queue, and a worker does
the work. Declare both. A pass that touches every tenant is a dispatcher plus a workspace worker. It is
never one job row that loops over every workspace.

## Steps

1. **Declare the kind** in `backend/api/jobs.yaml`, under `kinds:`, in name order.
   Every kind needs these fields:

   - `role`: `dispatcher` or `worker`.
   - `go_type`: the args struct in `compose` that returns this kind. It must match
     `^[A-Z][A-Za-z0-9]*Args$` and be the only one of its name in the file (one args struct is one River
     kind).
   - `queue`: must name an entry in the file's own `queues:` block. Use `default` unless the work is long
     or waits on outbound calls. A new queue needs a `reason` for leaving the `default` queue. The census holds
     its `max_workers` equal to `jobQueues()` in `compose`.
   - `timeout`: there is no default. Choose one of the three forms: a plain value (`2m`),
     `{derived: goConstant, value: 4h, reason: …}`, or `{none: true, reason: …}`. Every dispatcher in the
     file takes `2m`, for one shared reason. A dispatcher runs one index scan and one `insert-many` on
     the database, with no model call, site read or outbound request in it.
   - `opts_owner`: `fan_out` if the fan-out of a dispatcher builds this kind's insert options. `args` if the
     args type's own `InsertOpts()` does. `caller` if the places that add the job do.

   Then by role. A dispatcher also declares `cadence` (a length of time, `{setting: key}`, or `on_demand`). It
   also declares the pair `fans_out_to` and `fan_out_unit` (`workspace`, `connection` or `build`). A child
   of a fan-out (`opts_owner: fan_out`) also declares `max_attempts`. Three is the normal number, and a
   different one says why in the entry's `reason:`.

   And in some cases:
   - `registration: {when: [ConfigField, …], absent: registers_nothing | registers_anyway}` when the kind
     needs a part the install may not have wired.
   - `args:` for every field of the args struct (step 3).
   - `fault: {nil_after_logging: …}` only if the worker logs and returns `nil`. Name the lasting retry
     rule that does the work later.

2. **Run the generator** with `make gen`.
   `backend/tools/gen-jobs` checks the whole contract and writes two files again; never edit either by hand.
   `backend/internal/platform/jobs/specs_gen.go` is the `Spec` table every reader reads.
   `backend/internal/compose/jobkinds_gen.go` holds the closed `declaredJobArgs` union.
   It also holds one role check per kind, run when the code compiles.
   This step fails on a contract that cannot hold.

   For example: a missing timeout, a fan-out to a kind no one declares, or a `cadence` on a worker that
   only runs from the queue. The build does not compile yet: the generated checks name args types you have
   not written.

3. **Write the args type** in the matching `backend/internal/compose/jobs_<concern>.go`.
   Create a new file for a new kind of work.
   A worker for one tenant carries its tenant in a field named `Workspace`.
   Go does not allow a method and a field of the same name, and the wire key is fixed:

   ```go
   type CloseDateWorkspaceArgs struct {
       Workspace ids.UUID `json:"workspace_id"`
   }

   func (CloseDateWorkspaceArgs) Kind() string { return "close_date_workspace" }

   // WorkspaceID binds this pass to its tenant (jobs.WorkspaceScoped).
   func (a CloseDateWorkspaceArgs) WorkspaceID() ids.UUID { return a.Workspace }
   ```

   A dispatcher declares the empty mark instead, and carries no workspace key at all:

   ```go
   func (CloseDateSweepArgs) Kind() string { return "close_date_sweep" }

   // FleetWide marks this a dispatcher: it enumerates and enqueues,
   // and does no tenant work of its own (jobs.FleetWide).
   func (CloseDateSweepArgs) FleetWide() {}
   ```

   Check that `Kind()` returns this kind's string. Say you copy the args struct beside it and keep the
   other kind's string: every other gate on this path still passes. Every args field must be declared in
   step 1, as `id` or as a plain value with the reason it is safe. `river_job` has no workspace column and
   no row-level security, so a job names a row and the worker reads it.

4. **Write `Work`**, a method on your worker type, and nothing else.
   Do not declare `Timeout`, `NextRetry` or `Middleware`.
   The worker reaches River only as `jobs.WorkOnly[T]`, so the declaration answers those.
   Bind the workspace through the shared helper, and return through `jobs.FaultContext`:

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

   A pass that writes names who acts and makes its own `correlation_id`, as above. No HTTP layer stands
   behind a job, and `storekit.Emit` fails without a `correlation_id`
   ([write-backbone.md](../explanation/write-backbone.md#6-correlation--causation-the-trace)).
   `workspaceJobCtx` binds the tenant and only the tenant.

   A `Work` of a dispatcher instead calls one of the fan-out helpers (`dispatchWith`,
   `dispatchOne`, `runPerWorkspace`, `runPerEveryWorkspace`, `runEach`). It makes no tenant write of its own:

   ```go
   func (w *closeDateSweepWorker) Work(ctx context.Context, _ *river.Job[CloseDateSweepArgs]) error {
       return jobs.FaultContext(ctx, runPerWorkspace(ctx, w.pool, w.correctWorkspace))
   }
   ```

5. **Register it in `backend/internal/compose/jobs.go`** through `addDeclaredWorker`.
   Call it from the wiring helper that matches the kind's gate: `addModelLaneJobs`,
   `addDatabaseOnlySweepJobs`, `addCapturePipelineJobs` or `addGmailCaptureJobs`.
   Or use a helper of your own that registers itself and also returns what it schedules:

   ```go
   addDeclaredWorker[CloseDateWorkspaceArgs](reg, &closeDateWorkspaceWorker{corrector: NewCloseDateCorrector(pool, log)})
   ```

   Never call `river.AddWorker`, `AddWorkerArgs` or `AddWorkerSafely`. `forbidigo` blocks all three
   outside the one allowed line in `jobregistry.go`. The `backend/cmd/worker` binary runs every registered
   worker.

   Whether the kind is registered may depend on a new `JobRunnerConfig` field. Then add the field, name it in the
   entry's `registration.when`, and answer it in `configDependencies`
   (`backend/internal/compose/jobcensusconfig.go`). `periodicFor` stops the start with a panic on a path
   that file does not answer.

6. **Place the schedule, or the fan-out.**

   - A dispatcher with a clock gets one line in the `slices.Concat` block of `wireJobs`:
     `periodicFor(cfg, CloseDateSweepArgs{})`. `periodicFor` reads the `cadence`, the rule for when it is registered,
     and whether there is a schedule at all from the declaration. It never reads them from where the call
     sits. Do not build a `river.PeriodicJob` by hand. Do not touch the River `PeriodicJobBundle` either;
     `forbidigo` blocks it.
   - A fan-out over every workspace calls
     `dispatchWith(ctx, workspaces, insert, workspaceSweepOpts(ChildArgs{}.Kind()), argsFor)`. Pass the
     insert of a transaction you already hold when the fan-out must join it. It inserts the whole fan-out as
     one `InsertMany`. A fan-out that is partly inserted, fails and runs again would run the workspaces
     whose child jobs already passed.
   - A pass that runs each workspace in this process calls `runPerWorkspace(ctx, pool, run)`. It tries
     every workspace and joins the failures.
   - A fan-out per connection or per build loops over `dispatchOne(ctx, args, callerOpts)`. The child's
     declared `opts_owner` decides the options. Pass `callerOpts` for `caller` and `nil` for the other two.
     The wrong one stops with a panic, so the rule that keeps a job from running twice is never dropped
     without a sign.

7. **Check** with `make check`.
   The census, the job gates under `backend/gates/` and the start-time check of every kind all run here.
   Add `make test-integration` if the pass touches tenant tables.
   The lane with a real Postgres is what proves the workspace binding.
   Its timeout tests also read each declared time limit back from a live River client.

8. **Commit the contract and the generated files together.**
   Put `api/jobs.yaml`, `specs_gen.go` and `jobkinds_gen.go` in the same commit.
   A missed `make gen` fails the drift gate.
   A pair where only one file is new fails a second check, on the contract hash both files share.

## What each gate is telling you

The failures you meet first, in the order you would meet them:

| Where | Message | What it means |
|---|---|---|
| `make gen` | `kind "x": declares no timeout — an absent one is River's silent 1-minute default, which is what this contract removes` | Choose one of the three `timeout` forms. There is no default |
| `make gen` | `kind "x": is a dispatcher that fans out to nothing` / `fans_out_to "y", whose role is "dispatcher"` | A dispatcher must declare `fans_out_to` and `fan_out_unit`, and the child must be `role: worker` |
| `make gen` | `kind "x": declares a cadence but its role is "worker"` | No clock ever starts a worker that runs from the queue. Move the `cadence` to the dispatcher |
| `make gen` | `kind "x": opts_owner is fan_out but no max_attempts is declared` | The fan-out helper reads that number, and nothing else gives it. Without it, River tries 25 times and says nothing |
| `make gen` | `kind "x": args field "F" is declared a scalar with no reason` | A value that is not an id must say why it is safe in a table that GDPR Article 17 erasure never reaches |
| `go build` | `CloseDateWorkspaceArgs does not satisfy declaredJobArgs` | The kind is not in `api/jobs.yaml`, or you have not run `make gen` since you declared it |
| `golangci-lint` | `register through addDeclaredWorker — a kind absent from api/jobs.yaml is not in declaredJobArgs, and a direct registration also escapes jobs.Govern and the boot-time totality check` | You called the River register API, not `addDeclaredWorker` |
| `golangci-lint` | `a periodic tick is api/jobs.yaml's to declare — give the kind a cadence: and let periodicFor build it` | You reached the River `PeriodicJobBundle` from inside a worker |
| start | `jobs: N kind(s) not declared in api/jobs.yaml: … — add them there and run` `make gen` | A kind passed the compiler: a second name for a kind, a test file path, or a generated union edited by hand |
| start | `compose: a worker is registered under a kind the contract pairs with another args type` | `Kind()` returns the other kind's string. River would work those rows under the other kind's timeout, queue and limit, and your kind would have no worker |
| start | `compose: fanning out to "y", which no declared kind names in fans_out_to` | Declare the fan-out link of the dispatcher. `fans_out_to` is the register of what may take a fan-out at all |
| `jobrole_test.go` | `X is a River job (it declares Kind()) but is not in api/jobs.yaml` | An args type with no declaration. It would run on the River default of one minute and not show on either job page |
| `jobrole_test.go` | `X declares both WorkspaceID() and FleetWide()` | A job does one workspace's work or hands out work, never both |
| `jobwirekey_test.go` | `X.F ships as json:"ws", want json:"workspace_id"` | `args->>'workspace_id'` cannot see a different key. A `null` there reads as a dispatcher, not as tenant work the query cannot see |
| `jobwirekey_test.go` | `X is a dispatcher (it declares FleetWide()) but ships a json:"workspace_id" key` | Put the workspace on the child jobs it adds to the queue. The args of the dispatcher carry none |
| `jobfleetwide_test.go` | `W works FleetWide args X but never fans out` | A dispatcher must call one of the fan-out helpers (`dispatchWith`, `dispatchOne`, `runPerWorkspace`, `runPerEveryWorkspace`, `runEach`). If it does tenant work instead, it is `WorkspaceScoped` |
| `jobfleetwide_test.go` | `W works FleetWide args X and issues a tenant write` | Move the write into the workspace worker, where it can pass or fail as its own row |
| `jobfault_test.go` | `a worker return must be nil, jobs.Fault(...), or a river control return — a raw cause is written verbatim into river_job.errors` | Put the return in `jobs.FaultContext(ctx, err)` |
| `jobfault_test.go` | `W logs an error and returns nil — River will record this job as completed while the work failed` | Return the failure. Or accept it with `fault: {nil_after_logging: …}`, and name the retry rule that does the work later |
| `jobargscontent_test.go` | `X.F is not declared in api/jobs.yaml — say what it carries` | Every args field in the code needs a declaration. Nothing is read from its name |
| `jobargscontent_test.go` | `X.F is declared an id but its name reads like content … and nothing says why` | Give the field a `{reason: …}`, or make it carry an id. The word list cannot decide if it is safe; it only needs a stated reason |
| `jobcensus_test.go` | `job census: api/jobs.yaml and the wiring disagree: …` | The two sides no longer agree. A kind was declared and never wired, a `{derived: …}` value moved, or a queue limit changed in one place only |

## Notes

- **Adding a queue** is a contract change plus a change in `compose`. The census holds the two equal both
  ways. A limit moved in one place only shows operators a number no client runs at. A queue that is
  declared but never built takes fan-out child jobs that no client works.
- **Kind strings are stored state.** A new name leaves every live row that carries the old one with no worker. Correct the
  Go type name if it reads wrong; never the kind.
- `opts_owner: caller` is not checked. The options live where the job is added, so the declared queue only
  writes them down. Use `fan_out` when the file can own the options.
- Some code adds one child of a fan-out once, such as an event that adds the pass for one workspace.
  It uses `oneOffChildOpts`, not `workspaceSweepOpts`, and leaves out the `sweep` tag and the rule that keeps
  one copy live. Such a job is not that workspace's share of a pass over every workspace. If it is merged
  with a running pass, the rows the event was about would be dropped.
- To read the jobs back, see the job gauge values, `GET /v1/admin/job-health`, and the notes on a kind's rows in
  [reference/configuration.md](../reference/configuration.md#reading-the-job-surfaces).
