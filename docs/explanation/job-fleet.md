<!-- prose:plain -->
# The job fleet: the declaration, the dispatcher, and one row per workspace

Jobs that run outside a request in this backend run on [River](https://riverqueue.com), a job queue that keeps its jobs
in Postgres. A job is a row in the `river_job` table, workers claim rows, and River runs retries, time
limits and the schedule. The contract below sits **on top** of River.

That contract has two parts. First, every job kind is **declared** in `backend/api/jobs.yaml` before it
exists in code, and the running system follows the declaration. So a worker cannot choose its own time
limit, queue or most attempts. Second, every pass over the whole fleet is **two** kinds. A dispatcher
lists the fleet (every workspace on the installation) and queues the work, and a worker carries one
unit's work.

To *add* a job, see [how-to/add-a-job.md](../how-to/add-a-job.md). How an operator reads the same fleet
is in
[reference/configuration.md#reading-the-job-surfaces](../reference/configuration.md#reading-the-job-surfaces).
The write shape every workspace pass commits through is in [write-backbone.md](write-backbone.md).

## The whole shape

```text
DECLARATION                                    RUNTIME
backend/api/jobs.yaml
   │  make gen  (backend/tools/gen-jobs)
   ├──► platform/jobs/specs_gen.go   the Spec table every reader walks
   └──► compose/jobkinds_gen.go      the closed union + the role assertions
                                                │
   river periodic tick ◄── periodicFor(cfg, Args{}) ── cadence: from the declaration
        │  leader-elected, so replicas never double-dispatch
        ▼
   DISPATCHER row            role: dispatcher   ·   jobs.FleetWide
        │  enumerate the fleet — compose/dispatch.go, the ONE scan
        │  dispatchWith | dispatchOne
        ▼  one child per fan-out UNIT, one InsertMany, tagged `sweep`
   WORKER row × N            role: worker       ·   jobs.WorkspaceScoped
        │  workspaceJobCtx binds args.WorkspaceID() onto the context
        │  Work(…) ──► jobs.FaultContext(ctx, err)
        ▼
   river_job: one row per tenant — it succeeds, retries and FAILS on its own
```

**Why two kinds instead of one loop.** Take a single job that loops over every workspace. A failed
workspace then becomes a log line *inside* a row that River records as `completed`. The failure has no
durable place to land. Giving each workspace its own row makes the failure a row. It is retried on its
own, counted on its own, and reported on `GET /v1/admin/job-health` on its own.

---

## 1. `backend/api/jobs.yaml`: the declaration

`jobs.yaml` is the authority for how River runs a job: queues, time limits, most attempts and schedules.
An entry may carry `derives-from:` to name an outside rule whose number it copies. No entry
carries one today.

Every kind River keeps in `river_job.kind` is declared here. Kind strings are **stored state**. Renaming
one leaves every live row that carries it with no worker, so a kind is only ever added, never renamed. If a
Go type name reads wrong, correct the type name, never the kind.

These fields are declared for **every** kind:

| Field | Meaning |
|---|---|
| `role` | `dispatcher` or `worker` (§4). A generated check holds it to the Go marker interface |
| `go_type` | the compose args struct that returns this kind (`^[A-Z][A-Za-z0-9]*Args$`). Carried as data instead of an import, so a gate can check that the kind and the type still match |
| `queue` | must name an entry in the file's own `queues:` block; every queue other than `default` owes a `reason` for being split out of the default pool |
| `timeout` | the time limit for the whole job, by the clock (§3). There is **no default** |
| `opts_owner` | who supplies the insert options River uses: one of three modes, below |

`opts_owner` names *who* decides the queue and the most attempts that River adds a row with:

| Mode | Who owns the options | What the contract does |
|---|---|---|
| `fan_out` | the fan-out helper | **supplied**: the helper reads the declared queue and most attempts and hands them to River |
| `args` | the args type's own `InsertOpts()` | **checked**: the census checks the declaration against what that function returns |
| `caller` | the code that queues the job, in many places | **declared only**: the queue in the file is documentation, and nothing governs it |

Three more fields **depend on what the kind is**, and `make gen` refuses a field that does not belong, both
ways. A field that is owed and missing fails, and so does a field declared where it means nothing:

- **`cadence`**: required on a dispatcher, refused on a queued worker
  (`an enqueued worker is never ticked`). It takes a length of time, or `{setting: key}` naming
  the registered setting where an admin sets the seconds, or `on_demand`. `on_demand` is a
  *declaration*: a human's confirm queues `embed_reindex`, and no clock does. A missing cadence would
  read as a schedule someone did not add. A setting cadence may add `off_at_zero: true`. Its setting then
  accepts `0` to turn the pass off: the workers stay registered and only the tick stops.

  A running worker reads the settings again every 60 seconds and moves a changed schedule
  (`backend/internal/compose/jobschedulebook.go`).
- **`fans_out_to` + `fan_out_unit`**: one declaration, never one without the other. Required on a
  dispatcher (`a dispatcher enumerates and enqueues, so one with no child does no work at all`). It is
  refused on any other kind, and the named child must itself be `role: worker`. The unit is `workspace`, `connection` or `build`,
  and it tells a reader what a child row is. A `gmail_watch_renew_connection` row is the work for one
  *connection*, not one workspace. The unit is declared on the **dispatcher**, beside the edge it names,
  because that is where the fan-out decision is made.
- **`max_attempts`**: required for `opts_owner: fan_out` and refused for every other owner, because
  that is the only case the file governs. Compose's `workspaceSweepOpts` reads this number and nothing
  else does, so a cap declared in any other place would show a number the runtime never reads. Three is
  the number most kinds use. It is small because the real retry for a pass that fans out is the dispatcher's next
  tick.

Three more are declared **by exception**. Leaving one out means the safe default, never a free pass:

- **`registration: {when: [Field, …], absent: registers_nothing | registers_anyway}`**: declared only
  where the kind's wiring depends on something the installation may not have. `when` lists
  `JobRunnerConfig` field paths, and all of them must be set. A kind with no block always
  registers. The two choices for a missing dependency work in two different ways, and neither is
  a default. With `registers_nothing`, a row that nothing could work is never queued.

  With `registers_anyway`, the worker stays, so a row that a worker takes fails with a message that
  says what to do. The row never waits in the queue with no end. The same dependency takes different choices on
  different kinds: `Embedder` takes `registers_nothing` for the embed drift sweep and `registers_anyway`
  for `embed_reindex`. So the choice is per kind, never per field. A choice declared with no `when` fails
  `make gen`. The census holds the wiring to what each kind declares by holding back one dependency at
  a time (§8).
- **`fault: {nil_after_logging: …}`**: this worker logs a failure and returns `nil`. The text names the
  durable retry rule that makes a green River row correct. Two such rules are the `next_sync_at` on the
  connector's own row and a build row's own `deferred` state. Without the block, the worker must return what failed. A
  `fault` block with an empty reason fails `make gen`:
  `an unstated waiver is a swallowed error with a heading`.
- **`args: {Field: id | {scalar: true, reason: …} | {reason: …}}`**: see §5. A field missing from the block is not let off. The **census** checks the declaration against the compiled struct. It is the fitness
  test that puts the compiled wiring beside the contract and fails when the two do not match (§8).

A kind may also carry a free-form `reason:` that says why its numbers are what they are. Nothing
enforces it, and most entries whose numbers are not obvious have one.

---

## 2. `make gen`, and the two files it writes

`make gen` runs `backend/tools/gen-jobs` over the contract and writes two files that must never be
edited by hand:

- **`internal/platform/jobs/specs_gen.go`**: the `Spec` table. Every reader in the tree walks it: the
  fan-out helpers, the `/metrics` list, the `job-health` endpoint, the census.
- **`internal/compose/jobkinds_gen.go`**: the closed union `declaredJobArgs`, the two functions that
  register workers and accept only types in it, and one compile-time check per kind. That check
  matches the kind's args type with its declared role.

Both carry the same `sha256` of `api/jobs.yaml`. So when only one of the two was generated again, it is
visible without checking the two tables (the census checks it:
`bothGeneratedHalvesCameFromOneContract`).

The set is a closed union instead of a marker interface for one reason. A new type can declare a marker
for itself, and the set of kinds must be the file's to state. A kind that is not declared cannot be named at the
place that calls `addDeclaredWorker` at all. The failure is `does not satisfy declaredJobArgs`, on the
line the writer is typing to register it.

---

## 3. Why a worker cannot answer for itself: `jobs.Govern`

River asks a worker four questions: `Work`, `Timeout`, `NextRetry`, `Middleware`. The contract answers
three of them. So a worker written by hand in this tree has only `jobs.WorkOnly[T]`:

```go
type WorkOnly[T river.JobArgs] interface {
    Work(context.Context, *river.Job[T]) error
}

func Govern[T river.JobArgs](w WorkOnly[T], s Spec) river.Worker[T]
```

`Govern` puts the worker inside a type that River reaches **only** through `Work`, so River cannot reach
any option function the worker may carry. The smaller interface makes that an error no one can make.
The type around the worker hides any option function the worker has from an embedded
`WorkerDefaults`. A marker interface and a lint rule would both have missed that case.

Without a declared time limit, River applies a default of 60 seconds. A worker that declares no
`Timeout` embeds `river.WorkerDefaults`, whose `Timeout` returns zero, and River reads zero as 60
seconds. A long pass such as GDPR retention would be stopped before it ends every day and leave a
failing row. From the outside, a job stopped by its limit looks the same as a job that never has
enough time. So no one would see the bug.

So `timeout` has no default, and a missing value is not one of its forms. It takes one of three:

| Form | Meaning |
|---|---|
| `2m` | a fixed time limit |
| `{derived: c, value: 4h, reason: …}` | derived from a Go constant in another place in the tree. `value` is what `Govern` hands River, and the census proves the two still agree, so the declaration follows the constant instead of keeping a copy that never changes. Used only where something other than the lookup table inside the census reads the constant; otherwise the check would test the file against a private copy of itself |
| `{none: true, reason: …}` | declared as none. `TimeoutPolicy.Duration` gives `-1`, which takes the row out of the reach of `JobRescuer` (the part of River that clears jobs that stop moving); the work still waiting limits the pass instead of a clock |

`declaredTimeoutSeconds` never reports zero on `/metrics`, because zero would look like the 60-second
default in River, and the declaration exists to tell the two apart. A kind declared as none reports `-1`.

Two more gates hold the same line at boot. Neither the closed union nor `Govern` can see a generated
file edited by hand, or a test that registers into a test-only `*river.Workers`:

- **`jobs.MustBeTotal`** names every kind this role means to work that the contract does not declare,
  and `NewJobRunner` refuses to boot. A kind that is not declared runs on the default this contract exists
  to remove, and a process that started even so would hide it.
- **`everyKindIsRegisteredWithItsDeclaredType`** catches the other half. The total check says every
  kind is declared, but not that each is worked by the args type its declaration names. Take an
  args struct copied from the one beside it, whose `Kind()` still returns the string of the type it was copied from.
  It passes the total check, runs under that other kind's time limit, queue and most attempts, and leaves
  its own kind with no worker. `Spec.GoType` is carried in the compiled table so this can be checked.

---

## 4. The two roles

```go
type WorkspaceScoped interface {
    river.JobArgs
    WorkspaceID() ids.UUID
}

type FleetWide interface {
    river.JobArgs
    FleetWide()          // a declaration, not behaviour
}
```

Every kind is either a dispatcher or a worker, and there is no third role. A third would change what a
job *is*. Both operator surfaces read `Role` to decide whether a null `args->>'workspace_id'` is
correct or a bug.

**Each role goes both ways.** `role: worker` holds if and only if the args type is a
`jobs.WorkspaceScoped`, and `role: dispatcher` holds if and only if it is a `jobs.FleetWide`.
`make gen` writes one `var _ jobs.FleetWide = XArgs{}` or `var _ jobs.WorkspaceScoped = XArgs{}` line
per kind, so the compiler checks a *declared* kind's role. Gates cover the two cases the generated
checks cannot reach. One is a type the contract does not know
(`TestEveryJobArgsTypeIsDeclaredInTheContract`). The other is a type that is both at once
(`TestNoJobArgsDeclaresBothRoles`: `a job does one workspace's work or dispatches, never both`).

**The args declaration sets the binding.** A workspace pass runs under
`compose.workspaceJobCtx`, and nothing else in the tree binds a workspace inside a `Work` body:

```go
func workspaceJobCtx(ctx context.Context, args jobs.WorkspaceScoped) (context.Context, error) {
    ws := args.WorkspaceID()
    if ws == (ids.UUID{}) {
        return nil, fmt.Errorf("%s: declares WorkspaceScoped but carries no workspace", args.Kind())
    }
    return principal.WithWorkspaceID(ctx, ws), nil
}
```

It cannot live in River middleware. `river.WorkerMiddleware` sees a `rivertype.JobRow`, which is raw
JSON and never the typed args. A middleware could only bind by reading the wire key again. That would
make the role declaration a label *beside* the binding instead of the thing that governs it. Binding
from `WorkspaceID()` keeps the declaration in control: a worker cannot claim one workspace and work in
another.

The **zero id is refused**. No query limits rows by workspace: no table carries the column and no
policy reads one. So a zero id set on the context would not fail by itself. It would take the place of the real
id in an audit row, a blob key or the name of an advisory lock. Someone would see it much later and
far from the job that produced it.

A zero is also what an args type reads back when a queued row is older than a change to its wire key.
Refusing it turns a pass that would touch nothing into a visible failure.

**No role carries a deferred exception today.** `embed_reindex` has both kinds. Its dispatcher's
fan-out fills the set of workspaces the run still has to do and queues its child rows in one
transaction. An `embed_reindex_workspace` child embeds one workspace's search data again. Only its
declared fields set it apart: `cadence: on_demand` (a human's confirm queues it, no clock does) and `max_attempts: 5`
instead of the three most kinds use. With no tick behind it, nothing would queue a workspace that dropped out
again until a human confirms again.

**A dispatcher may read; it may not write.** `TestEveryFleetWideJobOnlyDispatches` holds the
`FleetWide` marker to the code. A dispatcher's `Work` must reach the fleet through one of the helpers
in the gate's closed allowlist, and must make no workspace write. Two of them queue one child per unit
(`dispatchWith`, `dispatchOne`). The other three run the pass for each workspace in this process
(`runPerWorkspace`, `runPerEveryWorkspace`, `runEach`). A direct `river.Insert` is not in the list.

The two queuing helpers build a child's insert options and always add the `sweep` tag. `dispatchWith`
takes the options its caller hands it, and the caller passes the declared queue and most attempts.
`dispatchOne` chooses them by the child's declared `opts_owner`. It uses the declaration for `fan_out`,
the child's own `InsertOpts()` for `args`, and the dispatcher's options for `caller`. A dispatcher that
adds rows around the helpers queues a child that neither sweep gauge can see, carrying whatever numbers
its writer typed.

**One transaction is the reason it is correct.** `dispatchWith` adds the whole fan-out as one
`InsertMany`. Take a loop that adds one workspace at a time and fails after only some of them. It leaves some
child rows queued and then fails the dispatcher. By the time the dispatcher retries, those child rows
may already be `completed`. `activeSweepStates` leaves out `completed`, so `ByArgs` uniqueness does
**not** stop them.

The retry would run those workspaces again, and no one would see it: a second capture pass, backed by
AI, that uses up model budget. One `InsertMany` does not make each job run only once. River can run a
job more than once. Each workspace pass sets the limit on that itself, because it reads its own
waiting work again.

---

## 5. Job args name rows, never carry content

`river_job` has **no workspace column and no RLS**, and River stores `args` into it as is. Take an args
field that holds a message body or an address. It would be a second store of subject data, and an
Article 17 erase would never reach it. It would stay in a table the whole fleet can see, for as long as
the retention River sets keeps the row.

The rule is that a **job names a row** and the worker reads it. That is also what lets an erase reach a
job that is still queued or running. The engine makes it safe by clearing the row the job names
(`comms_outbound` goes to `parked`). When the job runs, it finds nothing to send. This only works while
the job holds an id and not a copy.

There are three declared shapes:

| Declaration | Meaning |
|---|---|
| `Field: id` | a reference to a row: the ordinary case, and the only one that owes nothing |
| `Field: {scalar: true, reason: …}` | the agreed exception: a value that is not an id and could not be one (`Provider: "gmail"`, the embed `Identity` string, the most pages a site scan reads, `MaxPages`). `make gen` refuses a scalar with no reason |
| `Field: {reason: …}` | an **id** that still owes a reason, because its name reads like content (`Body`, `Subject`, `RecipientEmail`) |

The third shape exists because covering every field is not enough by itself.
`TestEveryJobArgsFieldIsAnIdOrAnArguedForScalar` runs two checks that answer different questions:

- **Every field**: this check covers every field on the compiled struct and reads nothing into a
  name. So `Snippet`, `Note` and `Domain` fall under the same rule as `Body`.
- **Name match**: this check tests field names against a list of names that look like content, and
  refuses a flagged name with no reason. Without it, the every-field check would accept `Body: id`
  without comment.

A list of names cannot decide whether a field is safe. It only makes someone say why. A reason on a
name the list does *not* flag is old text and fails the same gate.

The declared reasons count as **exceptions**, and each must meet the rule for every other agreed
exception in the tree. The reason states a cost, and the entry still matches live code.

---

## 6. The failure messages: `jobs.Fault`

River stores `err.Error()` into `river_job.errors` **as is**. That column has no workspace, no RLS,
and a retention that River chooses. So whatever a worker returns is stored where the whole fleet can
see it, for as long as the retry ladder runs. A provider that refuses a message can name the
address it refused, so the raw cause must never go this way.

So every worker returns through `jobs.Fault` / `jobs.FaultContext`. It renders a **fixed operator
message** set by the class of the cause. The real cause can still be reached through `errors.Is`:

```go
type fault struct { sentence string; cause error }
func (f *fault) Error() string { return f.sentence }   // fixed
func (f *fault) Unwrap() error { return f.cause }      // still classifies
```

The message list maps the shared error registry (`internal/shared/apperrors`) to fixed messages. Each one
says what failed **and** what it means for the job. An operator who reads a failure list needs to
know whether to retry, wait, or fix something. Two of them:
`"the record this job names no longer exists"` and
`"the provider refused the credential; reconnect the account"`. A cause with no class logs at `ERROR`
with the context of the caller. It becomes one fixed fallback message that says where to find the full
error.

Two things pass through **as they are**: `river.JobSnoozeError` and `river.JobCancelError`. A snooze
moves the job to a later time, and a `JobCancelError` stops it by choice. Neither is a failure, and
neither carries a cause to show. They are checked *before* the message list. So a stop by choice that
carries a known error stays a stop by choice.

The check cannot live where the job returns. These control returns reach a worker through helpers as
much as directly. Every normal "slow down" from a provider would otherwise log as a failure with no
class.

### A short failure defers the tick instead of failing it

A failure with a class still has to answer a second question that the class alone does not. Does the
tick **fail**, or does it **run again later**? The two are the same Go type, and far apart to an
operator. A failure uses up the child's attempts and becomes failed work on the Maintenance screen. A
deferred tick moves the same row to a later time and shows nobody anything.

A composed unit cannot return `river.JobSnooze` itself. It is a separate module that may import only
the `pkg/extension` surface in the allowlist. So it asks, with the same declared class it would have failed
under:

```go
// An unreachable provider needs nobody, so the tick runs again instead of dying.
return extension.Reschedule(classProviderUnavailable, pollRetryDelay, cause)
```

`jobs.FaultForKind` accepts the request only when the class is one this installation **registered for
the failing kind**. The message follows the same rule, so declaring a class gives both. It keeps the
wait within `[1s, 15m]` before the wait reaches the queue:

- **The floor of `1s`** is needed because River panics on a length of time below zero. River catches
  the panic and fails that attempt, so a unit that derived one from a clock would turn a wait into a failure.
- **The cap of `15m`** keeps a deferred row something you can measure. Both readers count a `scheduled` row as
  waiting, but every "how long has this waited" reading counts only rows with
  `scheduled_at <= now()`. A row deferred by hours would wait without ever showing how long it waited.
  It would stay next to counts that a tick with nothing to do also produces.

A request past the limit logs what it asked for alongside the wait it was set to. So the limit
never hides the error. A deferred tick logs at `WARN` with the cause and the wait. River records no
attempt error for a snooze, so that line and the unit's own row are the whole record.

Both shipped connectors ask for their **dispatcher's own cadence** (120 seconds), and the match is by design.
A deferred child waits in `scheduled`, one of the states the fan-out's uniqueness window covers. While
it waits, the dispatcher's next insert for that workspace merges into it. So the deferred tick *takes
the place of* the tick it would have run beside. The wait runs from the *failure*, not from the
schedule.

While a provider is down, the real gap between ticks is longer than the cadence. It adds the time a tick
takes to find out it cannot reach anyone. That is slower than normal and never the other way. Slower is the safe way
to go against a retention window measured in days.

It is **not a back-off**, because a back-off would risk losing data. For these connectors, asking on
time is about keeping data, not about new data soon. Zalo drops messages from its API after about 9
days, with no webhook and no history to page back through. Asking more slowly while the provider is
down makes it more likely that a connector falls behind for good. It would skip one request every 120
seconds, against a host that is already refusing.

A ladder can be built if a later unit needs one: River keeps a snooze count in the job's own data. The
job's attempt counter is not that count (a snooze *takes one off* the attempt count, so a snooze never
uses up retries). The way the wait moves is what rules out a back-off here; a counter exists.

The **slow down** case is the one where "the provider refuses us already" is not the reason.
`errTransient` covers a 429, and a 429 is a provider we can reach, asking us to slow down. The same
wait is right there because it is the *normal* cadence. A tick that is asked to slow down and defers by 120 seconds
puts no more work on the provider than a tick that works. The River ladder would retry within seconds
and then discard the row; this does not.

Neither connector reads `Retry-After`, so a provider that names a longer wait is answered on our clock
([#1809](https://github.com/margince/margince/issues/1809)). `capture/telegram` already follows the
wait that Telegram names, and is the model to follow.

Only a failure that **needs nobody** may defer itself. Some failures need a human, so each still becomes failed
work. They are a refused credential, a service package that has ended, an API group that is not
registered, and an answer the connector cannot read. A deferred failure while a provider is down is named on the
connector's own settings screen instead, which renders `last_error_class`. The row write is the same.
A deferred tick that skipped it would hide a provider that fails on every tick.

**Never show `river_job.errors` raw to a human.** `jobs.Failure.StoredReason` carries the column
as is, and the caller must check it with `jobs.VettedSentence(s)` before putting it on a wire. A
worker that did not use `Fault` stored its raw cause there, and River writes into the column too.
`JobRescuer` in River writes `"Stuck job rescued by JobRescuer"`, which is not a `Fault` message, and
the check correctly refuses it.

The check matches the whole string, never the start or a part of it. Otherwise a raw cause that holds
a checked message would carry the rest of its text through, because one part matched. The message list
itself stays private to the package. A caller asks whether one string is checked, and never
gets the list to render or match against by hand.

---

## 7. Reading the fleet

Two readers over one table answer two different questions: `/metrics` (is a queue getting longer?) and
`GET /v1/admin/job-health` (whose work failed, and why?). Both live in `internal/platform/jobs`
(`stats.go`, `health.go`) instead of in compose, because `river_job` has no RLS. Every statement over
it carries a **scope written by hand**. Two readers that wrote that scope in two packages could let
the operator surface and the admin surface give different answers about one table.

Some things are written once, for operators, in
[reference/configuration.md#reading-the-job-surfaces](../reference/configuration.md#reading-the-job-surfaces).
They cover what `/metrics` reports and its labels, and the counts that show how much of a sweep has run.
They also cover the list derived from the declaration, and the points to watch for when you read a
kind's rows. They are not copied here.

One point about the shape counts here. The scope that `health.go` sets lets in the rows of the
workspace that calls it. It also lets in the rows with no workspace that belong to the **dispatcher
kinds that the caller declares**. It is closed against that list, and does not let in every null.

"The workspace key is null" is kept in place by tests that read the source, not by a database
constraint. And the app role can read and write that table directly, with no RLS. Otherwise a wrong row, or one added from outside, would land in
the part for every workspace. It would carry its kind, counts and failure class to the admin of every
workspace.

---

## 8. What holds it: the fitness tests

The job gates live under `backend/gates/`, all in package `gates`, all part of `make check`. Every gate
that walks the tree carries a **floor**: the smallest number of things it must have looked at. Most of
them say what must not exist, and a walker that matched nothing would otherwise read green. (The
census carries its own floor, `declaredJobKindFloor`, beside the runner it reads.)

| File | What it catches |
|---|---|
| `jobrole_test.go` | a River job args type (it declares `Kind()`) that `api/jobs.yaml` does not know; and a type declaring `WorkspaceID()` **and** `FleetWide()` at once |
| `jobwirekey_test.go` | a workspace key written as anything but `json:"workspace_id"`, of any type but `ids.UUID`, missing, embedded, or listed twice at one level. The other way, a **dispatcher** that ships a workspace key at all. Both failures look like the safe answer to `args->>'workspace_id'` |
| `jobbinding_test.go` | a `Work` body that binds its own workspace in place instead of through `workspaceJobCtx`. Such a body could declare one field and bind another while the role gate stays green |
| `jobfleetwide_test.go` | a `FleetWide` dispatcher that never fans out, that fans out around the helpers in the allowlist, that makes a workspace write, or that no worker runs at all |
| `jobfleetwideshapes_test.go` | the gate above, tested against itself: it proves every shape the tree uses to queue child rows is **accepted**, and the two shapes it exists to reject are rejected. A gate that blocks a writer doing the right thing gets turned off by the writer it stopped |
| `jobfleetscan_test.go` | a `FROM workspace` read of all workspaces outside the agreed places. Each place must name which of four things it is: a dispatcher listing the fleet, a path that only reads, a boot path, or finding the workspace for an inbound request that has none |
| `jobfault_test.go` | a `Work` return, or a value set on a named error result, that is not `nil`, `jobs.Fault(…)` or a River control return. Also a worker that logs an error and returns `nil` without an agreed `fault:` block |
| `jobargscontent_test.go` | an args field the contract does not declare, a scalar with no reason, and a field whose name looks like content, declared `id` with nothing said about it |
| `jobkindgate_test.go` | the gate on how a worker is registered, tested against itself: the three right ways to register a worker compile, a kind that is not declared does not, and a worker registered under the wrong kind is named. The second half tests the kind that is not declared against the **real** generated union, not a small copy of it |
| `jobregistrationban_test.go` | the forbidigo rules that block registering a worker with River directly, and a change to the schedule at runtime. The test holds them to the River API itself instead of a list of function names someone wrote down. Every public function in package `river` whose first input is `*Workers` counts as an entry point, so a new name in a later River version is added to the list by itself |
| `jobqueuesupplied_test.go` | a `river.InsertOpts` that names a `Queue` where the row is added. The queue is supplied from `api/jobs.yaml`; a queue typed by hand lands rows on a queue the declaration does not size |
| `jobtestonly_test.go` | a production file that sets a value for the River `TestOnly` flag (`jobs.Config.TestOnly`, `compose.JobRunnerConfig.TestOnly`). The flag turns off the slow, step-by-step start of the services River runs for itself, so only test files may set it; the two files that pass it on are let through |
| `jobcensus_test.go` | everything the others cannot see. It builds the real runner as compose does (with no client) and puts it beside the contract. It catches a kind declared and never wired, a `{derived: …}` time limit whose Go constant moved, an args field nobody declared, and a fan-out child that does not write its unit key. Also a kind whose args own the options and that adds rows on the wrong queue, one args type that answers to a second kind, and a declared queue whose size compose does not build |

The census is the only gate that holds **both** ends of the contract at once. Every other gate holds a
single end. The closed union stops a kind that is not declared from compiling. `MustBeTotal` refuses a boot
that has one even so, and `Govern` makes the declared time limit the one River applies. None of them
can see a kind that was declared and never wired.

The census reads a role with **every option turned on**, which is the only way to see the whole
contract. That is also why it needs a second pass for the `registration` choices. With every dependency
supplied, the question that `absent:` answers never comes up. `jobcensusposture.go` holds back one
declared dependency at a time and builds the wiring again. Then it checks what was registered against
what `registers()` says it must be.

Both ways are findings. Take a kind that declares `registers_anyway`, but that no guard registers. Its
row is refused when it is added, with a message about the River worker bundle. Now take a kind that
declares `registers_nothing`, but that a guard still registers. It is a worker that waits
for rows the schedule half will never queue.

---

## Short rules

- **A kind is declared before it is written.** Not in the file means it does not compile, so it cannot
  boot.
- **A worker exposes `Work` and nothing else.** The time limit, the retry rule and the middleware
  belong to the declaration.
- **A dispatcher lists and queues.** A workspace job does the work and owns its failure. A `Work` body
  that loops over the fleet is the shape this layer removed.
- **A fan-out goes through `dispatchWith` / `dispatchOne`**, never a direct River insert. Otherwise the
  child loses the sweep tag and its declared cap.
- **Job args carry ids.** A scalar is an agreed exception with a written reason.
- **Return the failure through `jobs.FaultContext`**, and check anything read back out of
  `river_job.errors` before showing it to a human.
- **A null `args->>'workspace_id'` means a dispatcher**, and nothing else. Every read of the job table
  is built on that, both ways.

## Where the code lives

| | |
|---|---|
| The declaration | `backend/api/jobs.yaml` |
| The `gen-jobs` tool and its rules for checking the file | `backend/tools/gen-jobs/` (`contract.go`, `validate.go`) |
| Compiled `Spec` table (generated) | `internal/platform/jobs/specs_gen.go` |
| `Spec`, roles, fan-out units, time limit and cadence rules | `internal/platform/jobs/spec.go`, `role.go` |
| River client start and stop, and `MustBeTotal` | `internal/platform/jobs/jobs.go` |
| Time limit binding (`WorkOnly`, `Govern`) | `internal/platform/jobs/govern.go` |
| Failure messages (`Fault`, `VettedSentence`) | `internal/platform/jobs/fault.go` |
| Job table readers (`/metrics`, `job-health`) | `internal/platform/jobs/stats.go`, `health.go` |
| Closed union and role checks (generated) | `internal/compose/jobkinds_gen.go` |
| Building the runner, `JobRunnerConfig`, queue set | `internal/compose/jobs.go`, `jobqueues.go` |
| How workers register, and the kind and type matching | `internal/compose/jobregistry.go` |
| The one fleet listing and the fan-out helpers | `internal/compose/dispatch.go` |
| Workspace binding | `internal/compose/workspacejob.go` |
| Schedule from the declared cadence | `internal/compose/jobschedule.go` |
| The census (contract and wiring, both ways) | `internal/compose/jobcensus.go`, `jobcensusconfig.go`, `jobcensusposture.go` |
| Workers and args types, split by subject | `internal/compose/jobs_*.go` |
| The fitness gates | `backend/gates/job*_test.go` |

## Where to go next

- Adding a kind: [how-to/add-a-job.md](../how-to/add-a-job.md).
- Running the fleet (what `/metrics` reports, `job-health`, and the settings that set a cadence):
  [reference/configuration.md](../reference/configuration.md#reading-the-job-surfaces).
- The write shape a workspace pass commits through: [write-backbone.md](write-backbone.md).
- The workflows on a clock that one of these dispatchers runs: [automation.md](automation.md).
- How compose wires seams and edges between modules:
  [composition-layer.md](composition-layer.md).
