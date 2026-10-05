# The job fleet: the declaration, the dispatcher, and one row per tenant

Background work in this backend runs on [River](https://riverqueue.com), a Postgres-backed job queue:
a job is a row in the `river_job` table, workers claim rows, and River runs retries, timeouts and
scheduling. The contract below sits **on top** of River.

That contract has two halves. Every job kind is **declared** in `backend/api/jobs.yaml` before it
exists in code, and the running system obeys the declaration, so a worker cannot choose its own
timeout, queue or attempt cap. And every fleet-wide pass is **two** kinds: a dispatcher that
enumerates the fleet (every workspace on the installation) and enqueues, plus a worker that carries
one unit's work.

To *add* a job, see [how-to/add-a-job.md](../how-to/add-a-job.md). The operator's reading of the
same fleet is in
[reference/configuration.md → Reading the job surfaces](../reference/configuration.md#reading-the-job-surfaces).
The write shape every workspace pass commits through is in [write-backbone.md](write-backbone.md).

## The shape at a glance

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

**Why two kinds instead of one loop.** A single job that loops over every workspace turns a failed
tenant into a log line *inside* a row River records as `completed`. The failure has no durable place
to land. Giving each workspace its own row makes the failure a row: it is retried on its own, counted
on its own, and reported on `GET /v1/admin/job-health` on its own.

---

## 1. `backend/api/jobs.yaml`: the declaration

`jobs.yaml` is the authority for River mechanics: queues, timeouts, attempt caps and cadences. An
entry may carry `derives-from:` to name an external obligation its number restates; no entry carries
one today.

Every kind River persists in `river_job.kind` is declared here. Kind strings are **persisted state**:
renaming one strands every live row that carries it, so they are append-only in practice. Correct a
Go type name that reads wrong, never the kind.

These fields are declared for **every** kind:

| Field | Meaning |
|---|---|
| `role` | `dispatcher` or `worker` (§4). Held to the Go marker interface by a generated assertion |
| `go_type` | the compose args struct that returns this kind (`^[A-Z][A-Za-z0-9]*Args$`). Carried as data instead of an import, so a gate can assert the kind↔type pairing still holds |
| `queue` | must name an entry in the file's own `queues:` block; every non-`default` queue owes a `reason` for having been split out of the default pool |
| `timeout` | the whole-job wall clock (§3). There is **no default** |
| `opts_owner` | who supplies River's insert options: one of three modes, below |

`opts_owner` names *who* decides the queue and attempt cap River inserts a row with:

| Mode | Who owns the options | What the contract does |
|---|---|---|
| `fan_out` | the fan-out helper | **supplied**: the helper reads the declared queue and cap and hands them to River |
| `args` | the args type's own `InsertOpts()` | **checked**: the census compares the declaration against what that method returns |
| `caller` | scattered enqueue sites | **declared only**: the queue in the file is documentation, and nothing governs it |

Three more are **conditional on what the kind is**, and generation refuses the mismatch in both
directions. A field owed and absent fails, and so does a field declared where it means nothing:

- **`cadence`**: required on a dispatcher, refused on an enqueued worker (*"an enqueued worker is
  enqueued by its dispatcher, never ticked"*). It takes one of a duration, `{operator: Field}` naming
  the `JobRunnerConfig` dial the number comes from, or `on_demand`. `on_demand` is a *declaration*:
  `embed_reindex` is enqueued by a human's confirm and by no clock, and an absent cadence would read
  as a schedule somebody forgot. An optional `schedule_when_positive: Field` is a third posture: the
  workers stay registered and only the tick goes away.
- **`fans_out_to` + `fan_out_unit`**: one declaration, never one without the other. Required on a
  dispatcher (*"a dispatcher that fans out to nothing … does no work at all"*), refused elsewhere,
  and the named child must itself be `role: worker`. The unit is `workspace`, `connection` or
  `build`, and it makes a child row readable: a `gmail_watch_renew_connection` row is one
  *connection's* renewal, not one tenant's. The unit is declared on the **dispatcher**, beside the
  edge it names, because that is where the fan-out decision is made.
- **`max_attempts`**: required for `opts_owner: fan_out` and refused for every other owner, because
  that is the only case the file governs. Compose's `workspaceSweepOpts` reads this number and
  nothing else does, so a cap declared elsewhere would publish a number the runtime ignores. Three is
  the house number, and it is small because a fanned-out pass's real retry cadence is the
  dispatcher's next tick.

Three more are declared **by exception**. An omission means the strict posture, never a licence:

- **`registration: {when: [Field, …], absent: registers_nothing | registers_anyway}`**: declared
  only where the kind's wiring depends on something the deployment may not have. `when` is a
  **conjunction** of `JobRunnerConfig` field paths, and an omitted block registers unconditionally.
  The two absence postures are opposites and neither is a default. With *registers nothing*, a row
  that nothing could work is never queued. With *registers anyway*, the worker stays, so a picked-up row
  fails with an actionable message instead of sitting queued forever. The same dependency takes
  different postures on different kinds: `Embedder` registers nothing for the embed drift sweep and
  anyway for a reindex. So the posture is per kind, never per field. A posture declared with no
  condition fails generation. The census holds the wiring to what each kind declares by withholding
  one dependency at a time (§8).
- **`fault: {nil_after_logging: …}`**: this worker logs a failure and returns `nil`. The text names
  the durable retry policy that makes a green River row truthful (the connector sidecar's
  `next_sync_at`, a build row's own `deferred` state). Omitted, the worker must return what went
  wrong. A `fault` block with an empty rationale fails generation: *"an unstated waiver is a
  swallowed error with a heading"*.
- **`args: {Field: id | {scalar: true, reason: …} | {reason: …}}`**: see §5. An omitted field is not
  waived. The **census** compares the declaration against the compiled struct; it is the fitness test
  that lays the compiled wiring beside the contract and fails on any disagreement (§8).

A kind may also carry a free-form `reason:` stating why its numbers are what they are. Nothing
enforces it, and most non-obvious entries have one.

---

## 2. Generation, and the two halves it writes

`make gen` runs `backend/tools/gen-jobs` over the contract and writes two files that must never be
hand-edited:

- **`internal/platform/jobs/specs_gen.go`**: the `Spec` table. Every reader in the tree walks it:
  the fan-out helpers, the metrics catalogue, the health endpoint, the census.
- **`internal/compose/jobkinds_gen.go`**: the closed union `declaredJobArgs`, the two registration
  functions constrained to it, and one compile-time assertion per kind pairing its args type with its
  declared role.

Both carry the same `sha256` of `api/jobs.yaml`, so a half-regenerated pair is visible without
diffing the two tables (the census checks it: `bothGeneratedHalvesCameFromOneContract`).

The set is a union instead of a marker interface because a new type can declare a marker for itself,
and the set of kinds must be the file's to state. An undeclared kind cannot be named at
`addDeclaredWorker`'s call site at all. The failure is `does not satisfy declaredJobArgs`, on the
registration line the author is writing.

---

## 3. Why a worker cannot answer for itself: `jobs.Govern`

River asks a worker four questions: `Work`, `Timeout`, `NextRetry`, `Middleware`. The contract
answers three of them. A hand-written worker in this tree therefore satisfies only
`jobs.WorkOnly[T]`:

```go
type WorkOnly[T river.JobArgs] interface {
    Work(context.Context, *river.Job[T]) error
}

func Govern[T river.JobArgs](w WorkOnly[T], s Spec) river.Worker[T]
```

`Govern` wraps the worker in a type River reaches **only** through `Work`, so any option method the
worker happens to carry is unreachable. Narrowing the interface makes that impossible: an embedded
`WorkerDefaults` override is shadowed by the outer type, which a marker interface and a linter rule
would both have missed.

Without a declared timeout, River applies a one-minute default. A worker that declares no `Timeout`
embeds `river.WorkerDefaults`, whose `Timeout` returns zero, and River reads zero as one minute. A
long pass such as GDPR retention would be cancelled mid-run every night and leave a failing row. A
cancelled job and a job that never had enough time look identical from the outside, so the defect
would go unnoticed.

So `timeout` has no default, and absence is not one of its forms. It takes one of three:

| Form | Meaning |
|---|---|
| `2m` | a literal wall clock |
| `{derived: c, value: 4h, reason: …}` | computed from a Go constant elsewhere in the tree. `value` is what `Govern` hands River, and the census proves the two still agree, so the declaration tracks the constant instead of freezing a copy. Used only where something other than the census's own lookup table reads the constant; otherwise the check would compare the file against a private copy of itself |
| `{none: true, reason: …}` | a declared absence. `TimeoutPolicy.Duration` yields `-1`, which takes the row out of River's rescuer (its stuck-job reaper); a backlog bounds the pass instead of a wall clock |

`declaredTimeoutSeconds` never publishes zero on `/metrics`, because zero would look like River's
one-minute default, and the declaration exists to tell the two apart. A declared absence is `-1`.

Two more gates hold the same line at boot, because neither the union nor `Govern` can see a
hand-edited generated file or a fixture registering into a throwaway `*river.Workers`:

- **`jobs.MustBeTotal`** names every kind this role intends to work that the contract does not
  declare, and `NewJobRunner` refuses to boot. An undeclared kind runs on the default this contract
  exists to remove, and a process that started anyway would hide it.
- **`everyKindIsRegisteredWithItsDeclaredType`** catches the other half: totality says every kind is
  declared, but not that each is worked by the args type its declaration names. Picture an args
  struct copied from the one beside it, whose `Kind()` still returns the neighbour's string. It
  passes totality, runs under the neighbour's timeout, queue and attempt cap, and leaves its own
  kind with no worker. `Spec.GoType` is carried in the compiled table so this can be asked.

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

Every kind is either a dispatcher or a worker, and there is no third role. A third would change what
a job *is*: both operational surfaces read `Role` to decide whether a null `args->>'workspace_id'` is
correct or a defect.

**The biconditional.** `role: worker` ⟺ the args type implements `jobs.WorkspaceScoped`, and
`role: dispatcher` ⟺ it implements `jobs.FleetWide`. Generation emits one `var _ jobs.FleetWide =
XArgs{}` / `var _ jobs.WorkspaceScoped = XArgs{}` line per kind, so the compiler checks a *declared*
kind's role. Gates cover the two cases the generated assertions cannot reach. One is a type the
contract has never heard of (`TestEveryJobArgsTypeIsDeclaredInTheContract`). The other is a type
that implements both at once (`TestNoJobArgsDeclaresBothRoles`: *"a job does one workspace's work or
dispatches, never both"*).

**Binding comes from the args' own declaration.** A workspace pass runs under
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
JSON and never the typed args. A middleware could only bind by re-reading the wire key, which would
make the role declaration a label *beside* the binding instead of the thing that governs it. Binding
from `WorkspaceID()` keeps the declaration in control: a worker cannot claim one workspace and work
in another.

The **zero id is refused**. No query narrows by workspace (no table carries the column and no
policy reads one), so a zero id bound onto the context would not fail on its own. It would become
the value an audit entity id, a blob key or an advisory-lock name carries instead of the real one,
found much later and far from the job that produced it. A zero is also what an args type decodes to
when a queued row predates a change to its wire key. Refusing it turns a pass that would touch
nothing into a visible failure.

**No role carries a deferred exception today.** `embed_reindex` is a full pair: a dispatcher whose
fan-out seeds the run's pending set and enqueues its children in one transaction, and an
`embed_reindex_workspace` child that re-embeds one tenant's corpus. Its only unusual properties are
declared ones: `cadence: on_demand` (a human's confirm enqueues it, no clock does) and
`max_attempts: 5` instead of the house three. With no tick behind it, nothing would re-enqueue a lost
workspace until a human confirms again.

**A dispatcher may read; it may not write.** `TestEveryFleetWideJobOnlyDispatches` holds the
`FleetWide` marker to the code. A dispatcher's `Work` must reach the fleet through one of the
helpers in the gate's closed allowlist, and must issue no tenant write. Two of them enqueue one child
per unit (`dispatchWith`, `dispatchOne`); the other three run the pass for each workspace in this
process (`runPerWorkspace`, `runPerEveryWorkspace`, `runEach`). A direct `river.Insert` is not in
the list. The two enqueuing helpers build a child's insert options and always stamp the `sweep`
tag. `dispatchWith` takes the options its caller hands it, and the caller passes the declared
queue and attempt cap. `dispatchOne` picks them by the child's declared `opts_owner`: the
declaration for `fan_out`, the child's own `InsertOpts()` for `args`, and the dispatcher's options
for `caller`. A dispatcher inserting around the helpers enqueues a child invisible to both sweep
gauges, carrying whatever numbers its author typed.

**Atomicity is the correctness argument.** `dispatchWith` inserts the whole fan-out as one
`InsertMany`. A per-workspace loop of single inserts that fails partway leaves some children queued
and then fails the dispatcher. By the time it retries, those children may already be `completed`.
`activeSweepStates` excludes `completed`, so `ByArgs` uniqueness does **not** suppress them. The
retry would re-run those workspaces without notice: a second AI-backed capture pass spending model
budget. One `InsertMany` does not make delivery once-only. River is at-least-once, and the workspace
passes themselves bound that, each re-reading its own backlog.

---

## 5. Args name rows, never carry content

`river_job` has **no workspace column and no RLS**, and River persists `args` verbatim into it. An
args field holding a message body or an address would therefore be a second store of subject data
that Art. 17 erasure never reaches. It would sit in a fleet-visible table for as long as River's
retention keeps the row.

The rule is that a **job names a row** and the worker reads it. That is also what makes erasure reach an
in-flight job at all: the engine neutralizes it by scrubbing the row the job names (`comms_outbound`
goes to `parked`, and the waking job finds nothing to send). It only works while the job holds an id
and not a copy.

There are three declared shapes:

| Declaration | Meaning |
|---|---|
| `Field: id` | a reference to a row: the ordinary case, and the only one that owes nothing |
| `Field: {scalar: true, reason: …}` | the ratified exception: a value that is not an id and could not be one (`Provider: "gmail"`, the embed `Identity` string, a crawl's `MaxPages`). Generation refuses a scalar with no reason |
| `Field: {reason: …}` | an **id** that still owes an argument, because its name reads like content (`Body`, `Subject`, `RecipientEmail`) |

The third shape exists because coverage alone is not enough. `TestEveryJobArgsFieldIsAnIdOrAnArguedForScalar`
runs two checks that answer different questions:

- **Coverage** is total over the fields that exist on the compiled struct and infers nothing from a
  name, so `Snippet`, `Note` and `Domain` fall under the same rule as `Body`.
- **Suspicion** matches field names against a word list and refuses a flagged name with no
  rationale. Without it, coverage would accept `Body: id` without comment.

A word list cannot decide whether a field is safe. It only forces someone to state why. A reason on a
name the list does *not* flag is stale prose and fails the same gate.

The declared reasons are read as **waivers**, held to the same bar as every other ratified exception
in the tree: a reason that states a cost, and an entry that still describes live code.

---

## 6. The failure vocabulary: `jobs.Fault`

River persists `err.Error()` into `river_job.errors` **verbatim**. That column has no workspace, no
RLS, and a retention River chooses, so whatever a worker returns is stored fleet-visible for as long
as the ladder runs. A provider refusing a message routinely names the address it refused, so the raw
cause must never travel this way.

So every worker returns through `jobs.Fault` / `jobs.FaultContext`, which renders a **fixed operator
sentence** chosen by the cause's class, and keeps the real cause reachable through `errors.Is`:

```go
type fault struct { sentence string; cause error }
func (f *fault) Error() string { return f.sentence }   // fixed
func (f *fault) Unwrap() error { return f.cause }      // still classifies
```

The vocabulary maps the shared sentinel registry (`internal/shared/apperrors`) to fixed sentences.
Each says what went wrong **and** what it means for the job. An operator reading a failure list
needs to know whether to retry, wait, or fix something (`"the record this job names no longer
exists"`, `"the provider refused the credential; reconnect the account"`). An unclassified cause logs
at ERROR with the caller's context and becomes one fixed fallback sentence that says where the
diagnosis went.

Two things pass through **untouched**: `river.JobSnoozeError` and `river.JobCancelError`. A snooze
reschedules and a cancel stops by choice; neither is a failure and neither carries a cause to
publish. They are checked *before* the vocabulary, so a cancel carrying a known sentinel stays a
cancel. The check cannot live at the call sites: control returns reach a worker through helpers as
often as directly, and every routine provider throttle would otherwise log as an unclassified
failure.

### A transient failure postpones the tick instead of failing it

A classified failure still has to answer a second question the class alone does not: does the tick
**fail**, or does it **run again later**? The two are the same Go type and very different to an
operator. A failure spends the child's attempts and becomes dead work on the Maintenance screen. A
postponement reschedules the same row and shows nobody anything.

A composed unit cannot return `river.JobSnooze` itself: it is a separate module that may import only
the allowlisted `pkg/extension` surface. So it asks, with the same declared class it would have failed
under:

```go
// An unreachable provider needs nobody, so the tick runs again instead of dying.
return extension.Reschedule(classProviderUnavailable, pollRetryDelay, cause)
```

`jobs.FaultForKind` honours the request only when the class is one this installation **registered for
the failing kind**. The sentence follows the same rule, so declaring a class buys both. It clamps the
delay to `[1s, 15m]` before it reaches the queue:

- **The 1-second floor** matters because River *panics* on a negative duration. A unit that computed
  one from a clock would take the worker process down instead of failing a tick.
- **The 15-minute ceiling** keeps a postponed row measurable. Both readers count a `scheduled` row as
  waiting, but every "how long has this waited" reading counts only rows with
  `scheduled_at <= now()`. A row postponed far into the future would wait without ever showing an
  age, next to counts a healthy idle tick also produces.

A clamped request logs what it asked for alongside what it got, so the clamp never hides the mistake.
A postponement logs at WARN with the cause and the delay. River records no attempt error for a
snooze, so that line and the unit's own row are the whole trail.

Both shipped connectors ask for their **dispatcher's own cadence** (120s), and the match is by
design. A postponed child sits in `scheduled`, one of the states the fan-out's uniqueness window
covers. While it waits, the dispatcher's next insert for that workspace collapses into it, so the
postponement *replaces* the tick it would have raced. The delay runs from the *failure*, not from the
schedule. During an outage the effective interval is the cadence plus however long a tick spends
discovering it cannot reach anybody. That is slower than in health, never faster, which is the safe
direction against a retention window measured in days.

It is **not a backoff**, because a backoff would risk losing data. For these connectors poll liveness
is a *data-integrity* concern, not a freshness one. Zalo drops messages from its API after roughly
nine days, with no webhook and no depth to page back to. Polling less during an outage widens the
window by which a connector can permanently fall behind, to save one request every two minutes
against a host that is already refusing. A ladder is buildable if a later unit wants one: River keeps
a snooze count in the job's own metadata. The job's attempt counter is not that count (a snooze
*decrements* attempt, so snoozes never exhaust retries). The direction is what rules out a backoff
here; a counter exists.

The **throttle** arm is the one case where "the provider is refusing anyway" is not the argument.
`errTransient` covers a 429, and a 429 is a reachable provider asking for less traffic. The same
delay is right there because it is the *healthy* cadence. A throttled tick postponing to 120s puts no
more load on the provider than a successful one. It is also gentler than River's ladder, which
retried within seconds and then discarded the row. Neither connector reads `Retry-After`, so a
provider naming a longer wait is answered on our clock
([#1809](https://github.com/margince/margince/issues/1809)). `capture/telegram` already honours the
interval Telegram names, and is the pattern to follow.

Only a failure that **needs nobody** may postpone itself. A refused credential, a lapsed service
package, an unregistered API group and an answer the connector cannot read all still become dead work,
because each of them needs a human. A postponed outage is named on the connector's own settings
screen instead, which renders `last_error_class`. The row write is unchanged; a postponement that
skipped it would hide a noisy outage.

**`river_job.errors` is never shown to a human raw.** `jobs.Failure.StoredReason` carries the column
verbatim, and the caller must vet it with `jobs.VettedSentence(s)` before putting it on a wire. A
worker that bypassed `Fault` stored its raw cause there, and River writes into the column too. Its
rescuer's `"Stuck job rescued by JobRescuer"` is not a `Fault` sentence and is correctly refused. The
comparison is a full-string match, never a prefix or a contains. A raw cause that embeds a vetted
sentence would otherwise carry the rest of its text through on the strength of the part that matched.
The vocabulary itself stays unexported: a caller asks whether one string is vetted, and never gets the
list to render or match against by hand.

---

## 7. Reading the fleet

Two readers over one table answer two different questions: `/metrics` (is a queue growing?) and
`GET /v1/admin/job-health` (whose work died, and why?). Both live in `internal/platform/jobs`
(`stats.go`, `health.go`) instead of in compose, because `river_job` has no RLS. Every statement
over it is a **hand-imposed scope**, and two readers spelling that scope in two packages would let the
operational and the admin surface give different answers about one table.

The gauge families, their labels, the sweep-coverage pairs, the declaration-derived catalogue, and
the caveats that apply when you read a kind's rows are documented once, for operators, in
[reference/configuration.md → Reading the job surfaces](../reference/configuration.md#reading-the-job-surfaces).
They are not repeated here.

One structural point matters here. The scope `health.go` imposes admits the caller's own workspace
rows plus the untenanted rows of the **caller's declared dispatcher kinds**. It is closed against
that list and does not admit every null. "The workspace key is null" is held by source-shape tests,
not by a database constraint, and the app role holds direct CRUD on an RLS-less table. A malformed or
externally inserted row would otherwise land in a global arm and carry its kind, counts and failure
class to every workspace's admin.

---

## 8. What holds it: the fitness tests

The job gates live under `backend/gates/`, all in package `gates`, all part of `make check`. Every
gate that walks the tree carries a **floor**, a minimum number of things it must have inspected.
Most of them are prohibitions, and a walker that matched nothing would otherwise read green. (The
census carries its own, `declaredJobKindFloor`, beside the assembly it reads.)

| File | What it catches |
|---|---|
| `jobrole_test.go` | a River job args type (it declares `Kind()`) that `api/jobs.yaml` has never heard of; and a type declaring `WorkspaceID()` **and** `FleetWide()` at once |
| `jobwirekey_test.go` | a workspace key spelled anything but `json:"workspace_id"`, of any type but `ids.UUID`, absent, embedded, or duplicated at one depth. In the other direction, a **dispatcher** shipping a workspace key at all. Both failures look like the reassuring answer to `args->>'workspace_id'` |
| `jobbinding_test.go` | a `Work` body that binds its own workspace inline instead of through `workspaceJobCtx`, which could declare one field and bind another with the role gate still green |
| `jobfleetwide_test.go` | a `FleetWide` dispatcher that never fans out, that fans out around the three chokepoints, that issues a tenant write, or that no worker runs at all |
| `jobfleetwideshapes_test.go` | the gate above, falsified: every dispatch shape the tree uses proven **accepted**, and the two shapes it exists to reject proven rejected. A gate that blocks a legitimate author gets weakened by the author it stopped |
| `jobfleetscan_test.go` | a `FROM workspace` collection read outside the ratified sites. Each site must name which of four things it is: a dispatcher's enumeration, a pure read, a boot path, or tenant resolution for an untenanted inbound request |
| `jobfault_test.go` | a `Work` return, or an assignment to a named error result, that is not `nil`, `jobs.Fault(…)` or a River control return. Also a worker that logs an error and returns `nil` without a ratified `fault:` waiver |
| `jobargscontent_test.go` | an args field the contract does not declare, a scalar with no rationale, and a content-sounding field name declared `id` with nothing said about it |
| `jobkindgate_test.go` | the registration gate falsified: the three legitimate authoring shapes compile, an undeclared kind does not, and a worker registered under the wrong kind is named. The second half puts the undeclared registration in front of the **real** generated union, not a miniature of it |
| `jobregistrationban_test.go` | the forbidigo rules that ban a direct River registration and a runtime schedule mutation, held to River's own API instead of a remembered list of spellings. Every exported function in package `river` whose first parameter is `*Workers` is derived as an entry point, so a new spelling in a future upgrade enrols itself |
| `jobqueuesupplied_test.go` | a `river.InsertOpts` that names a `Queue` at the insert site. The queue is supplied from `api/jobs.yaml`; a hand-written one lands rows on a queue the declaration does not size |
| `jobtestonly_test.go` | a production file that originates a value for River's `TestOnly` flag (`jobs.Config.TestOnly`, `compose.JobRunnerConfig.TestOnly`). The flag turns off the maintenance services' staggered startup, so only test files may set it; the two forwarding files are exempt |
| `jobcensus_test.go` | everything the others cannot see, by building a real (client-less) runner assembly and laying it beside the contract. It catches a kind declared and never wired, a `{derived: …}` timeout whose Go constant moved, an args field nobody declared, and a fan-out child not writing its unit key. Also an args-owned kind inserting on the wrong queue, one args type answering to a second kind, and a declared queue whose bound compose does not build |

The census is the only gate that holds **both** ends of the contract at once. Every other one holds a
single end. The union stops an undeclared kind compiling, `MustBeTotal` refuses a boot that got one
in anyway, and `Govern` makes the declared timeout the one River applies. None of them can see a kind
that was declared and never wired.

It reads a **maximally-configured** role, which is the only way to see the contract's full extent.
That is also why it needs a second pass for the registration postures: with every dependency
supplied, the question `absent:` answers never comes up. `jobcensusposture.go` withholds one declared
dependency at a time, rebuilds the wiring, and holds what got registered to what `registers()` says
should have. Both directions are findings. A kind declaring *registers anyway* that no guard
registers is a row refused at insert with a message about River's worker bundle. A kind declaring
*registers nothing* that a guard registers anyway is a worker waiting for rows the schedule half will
never enqueue.

---

## Rules of thumb

- **A kind is declared before it is written.** Not in the file ⇒ does not compile ⇒ cannot boot.
- **A worker exposes `Work` and nothing else.** Timeout, retry policy and middleware belong to the
  declaration.
- **A dispatcher enumerates and enqueues.** A workspace job does the work and owns its failure. A
  `Work` body that loops the fleet is the shape this layer removed.
- **A fan-out goes through `dispatchWith` / `dispatchOne`**, never a direct
  River insert, or the child loses the sweep tag and its declared cap.
- **Args carry ids.** A scalar is a ratified exception with a written reason.
- **Return the failure through `jobs.FaultContext`**, and vet anything read back out of
  `river_job.errors` before showing it to a human.
- **A null `args->>'workspace_id'` means a dispatcher**, and nothing else. Every read of the job table
  is built on that, in both directions.

## Where the code lives

| | |
|---|---|
| The declaration | `backend/api/jobs.yaml` |
| The generator + its validation rules | `backend/tools/gen-jobs/` (`contract.go`, `validate.go`) |
| Compiled Spec table (generated) | `internal/platform/jobs/specs_gen.go` |
| Spec, roles, fan-out units, timeout/cadence policies | `internal/platform/jobs/spec.go`, `role.go` |
| River client lifecycle + `MustBeTotal` | `internal/platform/jobs/jobs.go` |
| Timeout binding (`WorkOnly`, `Govern`) | `internal/platform/jobs/govern.go` |
| Failure vocabulary (`Fault`, `VettedSentence`) | `internal/platform/jobs/fault.go` |
| Job-table readers (`/metrics`, `job-health`) | `internal/platform/jobs/stats.go`, `health.go` |
| Closed union + role assertions (generated) | `internal/compose/jobkinds_gen.go` |
| Runner assembly, `JobRunnerConfig`, queue set | `internal/compose/jobs.go`, `jobqueues.go` |
| Registration path + kind↔type pairing | `internal/compose/jobregistry.go` |
| The one fleet enumeration + the three fan-out helpers | `internal/compose/dispatch.go` |
| Workspace binding | `internal/compose/workspacejob.go` |
| Schedule resolution from the declared cadence | `internal/compose/jobschedule.go` |
| The census (contract ⟷ wiring, both directions) | `internal/compose/jobcensus.go`, `jobcensusconfig.go`, `jobcensusposture.go` |
| Per-concern workers and args types | `internal/compose/jobs_*.go` |
| The fitness gates | `backend/gates/job*_test.go` |

## Where to go next

- Adding a kind: [how-to/add-a-job.md](../how-to/add-a-job.md).
- Operating the fleet (gauges, `job-health`, and the dials that set a cadence):
  [reference/configuration.md](../reference/configuration.md#reading-the-job-surfaces).
- The write shape a workspace pass commits through: [write-backbone.md](write-backbone.md).
- The clock-triggered automations one of these dispatchers drives: [automation.md](automation.md).
- How compose wires seams and cross-module edges generally:
  [composition-layer.md](composition-layer.md).
