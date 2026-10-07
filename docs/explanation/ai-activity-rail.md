# The AI activity rail: what the AI is doing for you, while it does it

A rep who asks for a summary should see that it is being written. The AI
activity rail shows AI work while it runs and after it finishes. Without it, the
answer appears after a silent wait, which reads as a product that did nothing
and then guessed.

The sections below cover the one projection behind the rail, who reports into
it, and who an occurrence belongs to. Which kinds a reader is shown is a
separate question with a different owner, so it gets its own section.

The server must record every AI task: a task that reports nothing is AI work the
product performed and then denied. The client decides which kinds to show, and it
shows fewer. Merging the two decisions produces either a silent product or 300
strings nobody reads.

## The shape at a glance

```
  a writer                     the bus                the projection            the read              the rail
  ────────                     ───────                ──────────────            ────────              ────────
  ai.Router          ──┐
  agent runner       ──┼──▶  ai_task.state_changed ──▶  aiactivity      ──▶  GET /me/ai-activity ──▶  AgentRail
  attachment extract ──┘         (outbox)                handler              (one statement,          (locale
                                                          │                    two arms)                decides
                                                          ▼                                             the words)
                                                     ai_task_run
```

Nothing outside `internal/modules/aiactivity` writes `ai_task_run`, and no
statement in it invents a fact the bus did not carry. That is why the module
imports no sibling: the facts it needs arrive in the envelope, and a projection
that read a source's tables would be a second reader of a truth it is supposed
to hold. Two exceptions, both retention: `PurgeSettledBefore` ages rows out and
`CloseAbandonedRouterRuns` settles the ones whose source will never settle them.
Ageing a read model is not a domain mutation, but it is a write.

## Who reports: the router, a carrier, or nobody

`ai.railOwners` (`internal/modules/ai/railowner.go`) answers this for every task
in `api/ai-tasks.yaml`, and it is total over that table: a task the generator
adds and nobody answers fails the build. The possible answers:

| Owner | What it means | What it can say |
|---|---|---|
| `SourceRouter` (`ai_router`) | The default. The router announces on the task's behalf, so a task is wired before its author has thought about the rail. | It learns of a call only once the call is over, plus a `running` line announced just before the call. Never `queued`. |
| A **carrier** (`agent_runner`, `attachment_extraction`, `account_scan`, `transcript_read`, `voice_build`, `site_read`) | Work that owns a durable row reports for itself. | `queued`, `running`, and (because a carrier declares a lease) a dead attempt that can be derived as `stalled`. |
| `SourceNoOccurrence` (`none`) | The work is a step inside somebody else's occurrence and has no occurrence of its own. | Nothing of its own; it is reported under the unit of work it serves. |

`SourceNoOccurrence` does not exempt a task from reporting, and every use owes a
reason in `railNoOccurrenceReasons`. The reasons are checked, because "reported
by nobody" is one keystroke from the silence this registry exists to end. Two
groups use it today. `embeddings` does, because every embedding call serves a
search, an enrich or a reindex, and that is the occurrence. The three website-read
passes do, because the read is the occurrence. A reason that is really an
editorial preference ("no rep wants to see it") belongs in the client, which
decides what to draw.

Where a carrier exists it is the better reporter and the router stays silent,
so the two never write one occurrence between them.

The website read is the one carrier that is not a task. A deep read
(`contacts/sitereadactivity.go`, `source=site_read`, kind `site_read`) crawls up
to a dozen pages and makes several model calls. The router does not report those
calls; the read reports itself as one `site_read` occurrence, keyed on its own
id. The dossier row can say `queued` when a rep presses "read the site" and
`running` while the crawl is in flight. The three `site_*` kinds (`site_triage`,
`site_extract`, `site_fact_extract`) stay in the enum only so older rows remain
filterable.

## Who an occurrence belongs to

`ResolveActor` (`aiactivity/actor.go`) derives the owner from the envelope, never
from the payload. An emitter chooses its payload; it cannot choose the
authenticated actor the write shape stamped, so it cannot attribute its work to
somebody else by filling in a field.

| The event's actor | Scope | `actor_user_id` |
|---|---|---|
| human | `personal` | that human |
| non-human **with** `on_behalf_of` | `personal` | the human behind it |
| non-human **without** `on_behalf_of` | `workspace` | NULL |
| human that does not parse | *refused* | (none) |

A human actor that does not parse is refused rather than made workspace-scoped
without a trace. Widening it would turn one user's work into a system sweep that
nobody can find and nobody notices is missing.

On a worker path, `OnBehalfOf` comes from the job's own args, so this rule is
uniform but not tamper-proof: one rule, one place, one failure mode.

This decides most of the display census below. A workspace-scoped occurrence has
a NULL `actor_user_id`, and the read filters `actor_user_id = $1`, so background
work with nobody behind it reaches nobody's rail by construction.

## The read

`GET /me/ai-activity` (`aiactivity/read.go`) is cookie-authenticated,
`human-only` and read-only, with no audit or event row.

The user is taken from the bound principal and is not a parameter, and that is
all the authorization there is. A store method that accepted a user id would let
any in-process caller ask for somebody else's feed. The only guard against a
leak would then be every caller remembering to pass its own. Here another user's
feed cannot be expressed. No RBAC object gates it, because there is no wider set
to withhold.

Four properties to know:

- **One statement.** The transaction is READ COMMITTED, so two statements would
  take two snapshots. An occurrence that settled between them would appear in
  both, with the rail saying "reading your document" and "I've read your
  document" about one reading at once. One statement is one snapshot.
- **Two arms.** Each has its own ordering, bound and partial index. `live` is
  `queued`/`running` ordered by `queued_at`. `settled` is bounded by
  `finished_at`, because "what the AI finished for me today" is a question about
  when it finished. Keyed on its start, a run that began 23:50 and ended 00:10
  would fall out of `settled` after having left `live`, and vanish.
- **Derived `stalled`.** `stalled` is computed at read time and never stored. A live occurrence past
  the lease its own source declared is reported stalled, unconditionally, in SQL,
  against the database clock. Nothing writes it, so nothing can forget to. That
  stops a worker that died mid-run from being displayed as working forever.
- **`recent` is bounded**: since local midnight, at most 10. An unbounded
  per-user history is a per-user activity ledger, which this installation does
  not keep. `summary` and `degrade_reason` are capped on the way to the wire
  (2000 / 500). Otherwise a model's whole output, possibly inflated by a prompt
  injection, ships to every open tab on every poll.

`degrade_reason` is server-authored prose in the source's own words, never a
provider's or a parser's message: those carry vendor text and can echo credential
material, and this field reaches an ordinary rep. `MarkFailed` takes a typed
`runner.FailureReason` so a raw error fails to compile.

`kinds` is how a client that draws part of the record asks for its part, and it
is applied before the bounds. Every task reports, so a caller that renders a few
kinds and is served the newest ten of all kinds can get ten it draws nothing
for. The bound would fall on rows the reader never sees, and the rail would go
blank while its work was reported correctly. An empty list is a 422, and so is
an unknown name. Both would otherwise come back as an empty feed, which is the
true answer for an AI at rest.

The SPA polls every **3s while something is live, 30s at rest**, and refetches
on tab return. Focus refetching is disabled app-wide, and the cached body is the
one that is wrong: the run it shows as live is the run that finished while the
tab was away.

## What a reader is shown

`frontend/src/app/ai-activity-lines.ts` holds `ACTIVITY_LINE`: for each kind in
the contract's `AiActivityKind` enum, either a `(state → message key)` table or a
written reason there is none. It is typed `Record`, not `Partial<Record>`, so a
new kind fails the build until somebody writes its copy in every locale or says,
in code, why it is not shown. The reason lives in the source because the next
author reads the file and not the PR.

Copy is by literal key, never `t(\`agent.activity.${kind}.${state}\`)`. The
orphan guard in `i18n/orphan-keys.test.ts` counts a key as rendered when it
starts with a template stem. An interpolated key would vouch for the whole
namespace forever, and a retired kind's copy would sit in three catalogs with
nothing to flag it.

**Narrated kinds**, in en/de/vi, total over all six states:

| Kind | Reported by | The line a rep sees |
|---|---|---|
| `morning_brief` | carrier (`agent_runner`) | the scheduled brief |
| `overnight_at_risk_sweep` | carrier (`agent_runner`) | the scheduled sweep |
| `document_extract` | carrier (`attachment_extraction`) | reading a document you attached |
| `account_scan` | carrier (`account_scan`, the `company_scan` row) | "I'm reading Brandt Automotive's exchanges and deals." Named for the account, so a reader who opened three accounts and moved on knows which is ready |
| `site_read` | carrier (`site_read`) | reading a company's website, named for the company |
| `transcript_propose` | carrier (`transcript_read`) | reading a meeting transcript for its next steps |
| `voice_build` | carrier (`voice_build`) | learning the reader's own writing voice |
| `weekly_review` | router | the weekly retrospective, under the rep's own principal |
| `weekly_learnings` | router | the rep's own week, under their own principal |
| `summarize` | router | "I'm writing your summary." |
| `draft_reply` | router | "I'm drafting your reply." |
| `offer_draft` | router | "I'm drafting your offer." |

For `summarize`, `draft_reply` and `offer_draft`, `queued` copy exists and is
unreachable. The router announces a call it is about to serve, never one
waiting, and no carrier owns these tasks. The key exists because the state axis
is total and the compiler requires it.

**Kinds not narrated**, each with its own kind of reason:

| Reason | Kinds | Why |
|---|---|---|
| Watched by the asker | `growth_fit`, `cold_start`, `corpus_ask` | Shown on the surface that asked. |
| System sweep | `brief_ranking`, `capture_classify`, `capture_confidentiality_verdict`, `capture_counterparty_verdict`, `owed_verdict`, `propose_roles`, `rate_extract`, `request_settlement`, `signal_extract`, `stage_evidence_extract` | Background workspace work that belongs to nobody in particular. |
| The read narrates itself | `site_extract`, `site_fact_extract`, `site_triage` | Retired; `site_read` is the occurrence. |
| Reaches nobody, and would not be worth showing | `enrich` | No personal owner, and one row for a whole pass. |
| An operator's measurement | `cert_judge` | Grades this build's answers; not a rep's work. |
| Answered on screen | `nl_search` | The filter answers in the builder the reader pressed it from. |
| Declared, not built | `deal_health`, `transcript` | Named in `api/ai-tasks.yaml`; no site runs them. |

`growth_fit` renders the band it returns on the panel that asked. `cold_start`
runs behind two product surfaces. One is onboarding, whose screen has no rail:
`onboarding` is a member of `RAIL_LESS_SCREENS` in `nav.ts`, which `shell.tsx`
reads to drop the chrome. The other is the company page's Enrich card
(`cmd/api/modelwiring.go` wires `WithScrape` with the cold-start brain), where a
rail exists and the card itself renders the proposal. It also declares four
invocation *sites* in `aitaskregistry.go`, which is a different count from the
two surfaces.

The three `site_*` kinds are the individual model calls a website read makes.
`site_read` is one occurrence for the whole crawl, announced by the dossier from
queued to settled, so a line per call would tell one reading several times over.
Attribution belongs to the read: a human-requested read carries that human as
`on_behalf_of` and is personal to them, while a domain-triage or auto-enrich read
names no human and is workspace-scoped.

`enrich` fails on reachability and on worth. Its one production site is the
signature-enrichment pass, which runs under a system principal with no
`on_behalf_of`. Every occurrence is therefore workspace-scoped with a NULL
`actor_user_id`, and the personal feed selects on `actor_user_id`. It also could
not be per-contact if it were reachable. The pass mints one correlation id for
the whole run (`capture_enrich`, up to 100 candidates in series), and the
occurrence key is correlation+task. Every candidate collapses into one row, so a
per-contact subject would make that row flap without narrating anybody. What a
reader wants from it is what it found, which is durable and already drawn as
evidence-or-omit provenance on the contact record.

`enrich` looks visible and is not: the ticker's own `enrich` key names different
work. That key covers a provider run on a contact (`contactprovider.tsx`) and
the company page's Enrich card (`companies.tsx`), which POSTs
`/companies/{id}/enrich` and so runs `cold_start` instead of this task. The deep
read rides its own `site-read` ticker key.

### Where a narrated line lands

Three places, and each answers a different question.

- **The panel's running section** lists what is live, and only that.
- **The panel's recap** ("What it has done") lists what settled today, newest
  first, at most five. It uses the same copy table, so one occurrence is told in
  one vocabulary from start to finish: "I'm reading the Acme website" while it
  runs and "I've read the Acme website" once it is done. The record's name is a
  link, which is the half of the row a reader can act on. The recap reads this
  feed instead of the model-call trace: `ai_call` is telemetry and carries no
  subject (`Call.Subject` reaches the occurrence and never the trace). A recap
  read from there could report that something happened and nothing about what
  it happened to. The trace is still one click away, behind "Full log".
- **The card's resting line** rotates the newest settled occurrence among the
  agent's other standing facts, because the rail is 235px wide and carries one
  line.

A kind with no copy draws nothing in any of the three. The recap says "nothing
has finished today" only once the feed has answered. An unread feed draws no
sentence at all, because "nothing finished" is a claim about a day somebody
looked at.

### The ask: what this tab knows before the feed does

The feed arrives on a poll, so between a rep pressing "Draft with AI" and the
next read there is a live model call nothing on screen reports. The client
closes that window from its own end (`frontend/src/api/model-inflight.ts`). It
counts every request it is holding open to a route whose handler calls a model
and waits, and the rail treats a non-zero count as `working`. That state has no
kind, no state and no sentence, because the client knows none of those. It ranks
below every occurrence the feed carries, so the feed names the work as soon as it
can. The count also drops the poll to its live cadence and refetches on both
edges of the request, so the feed's own line follows within seconds.

The contract says which routes count. An operation whose handler holds the
request open on a model carries `x-waits-on-model: always` (a draft, the meeting
brief: generated on every call) or `x-waits-on-model: on-miss`. The `on-miss`
routes (the dossier, the contact brief, the deal status, the morning brief) are
served from a stored reading and generate only when there is none. The client's
`MODEL_ROUTES` table (`api/client.ts`) is a declared mirror of the marked set. It
is keyed by method and path, because the dossier is read and refreshed at one
path and only the refresh generates every time. `backend/gates/modelroutes_test.go`
fails when the two disagree in either direction or on the value.

An `on-miss` route answers from the store in well under a second and from the
model in many, and nothing the client can see when the request leaves tells the
two apart. So it is counted only once the request has outlived
`CACHE_ANSWER_GRACE_MS` (one second). A stored answer never lights the orb,
since that is the reader's own click, and a generation lights it a second late.

A route that enqueues model work and answers 202 is unmarked: it holds nothing
open, and its occurrence reaches the rail the way every background run does,
through its carrier and the feed. A surface that starts one calls
`watchStartedAiRun` instead, which is the other bridge.

### Two surfaces, one action, no double narration

The taskbar ticker narrates **this tab's own react-query cache**; the rail
narrates **the server's feed**. Three ticker entries describe work the rail also
covers, and they do not collide: the bar renders the ticker line or the rail
line as one `if/else` on a single span (`agentrail.tsx`), so a reader never sees
one action twice.

The ticker keys for email are split: `email-draft` for the rail and `email` for
the ticker. Four mutations (two drafts and two sends) once shared one key, and
the split keeps each action narrated once, by whichever surface knows about it.

## The gates that hold it

All of these live in the root package, because only the root can see the task
contract, the wire contract, the read's own bounds and the emitters at once.

| Obligation | Held by |
|---|---|
| Every task names the source that reports it, and every `SourceNoOccurrence` owes a defensible reason | `TestEveryAITaskNamesTheSourceThatReportsIt` (`backend/gates/aitaskrailcensus_test.go`) |
| The registry names no task the contract dropped | `TestTheRailRegistryNamesNoTaskTheContractDropped` |
| A carrier's source literal is one something really emits | `TestEveryCarrierOverrideNamesASourceThatIsReallyEmitted` |
| Every kind something produces is one the contract enum can express | `TestEveryKindSomethingProducesIsOneTheContractCanExpress` (`backend/gates/aiactivitycatalogparity_test.go`); its failure prints an `align:` line naming the file, the schema and what to add |
| Every contract kind has something that produces it | `TestEveryContractKindHasSomethingThatProducesIt` |
| The read's text caps are the ones the contract publishes | `TestTheReadsTextCapsAreTheOnesTheContractPublishes` |
| Every spec name can be a message-key segment | `TestEverySpecNameCanBeAMessageKeySegment` |
| The client's table of routes that hold a model call open is the contract's `x-waits-on-model` set, in both directions and on the value | `TestTheClientsModelRouteTableIsTheContracts` (`backend/gates/modelroutes_test.go`) |
| Every contract kind is displayed or carries a written reason | the TypeScript `Record` type: a **compile error**, not a test |
| Every displayed kind has copy in en/de/vi, for all six states | the `LineSet` type + the i18n catalogs |

The census gate derives its subjects from the registry. The router announces for
every task the registry leaves to it, and that set grows the moment somebody
declares a task. A hand-kept list would always be one edit behind the contract
and could miss a task that reports nothing.

## Known limits

- **Multi-call reopening** ([#2276]). A multi-call unit reopens its occurrence once per call.
  A task whose unit of work spans several logical calls under one correlation
  id settles after each and reopens at the next call's attempt. Within one
  logical call the occurrence is stable: `CompleteStructured` walks the ladder
  up to three times under one start, and the lease is renewed before every
  model call after the first.
- **Lease renewal.** The projection admits one write into a live attempt: a
  longer lease.
  `applyStateChangeSQL` guards with strict `>` on `(attempt, rank)`. Beside that,
  it takes an equal-tuple event in the same live state whose `stale_after` is
  later than the row's. That is the lease renewal the router makes per model
  call, and it lets each lease be sized for one call (`CallCeiling` plus the
  flush), not for the whole logical call. A redelivery carries an equal instant
  and a late renewal an earlier one, so the branch refuses both. A settled row
  has no `stale_after`, so it refuses those too. Per-step progress ticks remain
  impossible without a further change.
- **Two unnamed states.** The contract has no word for two real states. Site read's `deferred`
  ("waiting on budget, retry at `next_attempt_at`") is neither `queued`, since
  nothing will pick it up, nor settled. Its `cancelled` is terminal, but `done`
  and `failed` would both misstate it. Either the enum grows or somebody rules on
  the mapping. Two mappings already fit: `site_read.stopped_reason` is a closed
  vocabulary that drops straight into `degrade_reason`, and `partial` →
  `degraded`.
- **Subjects are forwarded, never filtered on.** The event envelope has
  `subject_type` and `subject_id`, and `ai_task_run` has both columns. The feed
  ships them beside `subject_label` so the rail can make the name a link to the record (a
  company or a contact; a document or a meeting has no page and stays text).
  They travel on the same ground as the label: the source emitted them only
  where the actor is the user the record was already shown to. The read stays
  keyed on the user alone.
- **A subject-scoped read was designed and declined.** It would replace an
  authorization that holds by construction (another user's feed cannot be
  expressed) with one that holds because a gate ran. `auth.EnsureVisible` alone
  is not that gate: it checks no object grant, and for an identity table its
  clause is empty, so it returns success without a query. The one populated
  subject, `attachment`, is not row-scoped at all; its authority is inherited
  from a polymorphic parent. A single `LIMIT` over a widened predicate would also
  let one population evict the other. If it is ever built it copies
  `ai/feedback.go`, which already spells the gate correctly.
- **Per-contact `enrich` narration was considered and declined.** The reason is
  in the census entry beside the code: the pass mints one correlation id for all
  its candidates, so they share a single occurrence that no per-contact subject
  could describe.

[#2276]: https://github.com/margince/margince/issues/2276

## Reference

| Concern | Where |
|---|---|
| The projection + the read | `internal/modules/aiactivity` (owns `ai_task_run`, imports no sibling) |
| Who reports each task | `internal/modules/ai/railowner.go`: `railOwners`, `RailOwner`, `RouterReports` |
| The router's announce/settle pair | `internal/modules/ai/railstart.go`, `railemit.go` |
| The carriers | `internal/modules/agents/runner/activity.go`, `internal/modules/activities/extractionactivity.go`, `internal/modules/activities/transcriptactivity.go`, `internal/modules/ai/voicebuildactivity.go`, `internal/modules/contacts/sitereadactivity.go`, `internal/compose/companyscan/` |
| Attribution | `aiactivity/actor.go`: `ResolveActor` |
| The event | `ai_task.state_changed` (`shared/kernel/events/catalog.go`; payload generated into `internalevents_gen.go`) |
| The wire | `AiActivity` / `AiActivityKind` / `AiActivityItem` + `GET /me/ai-activity` (`backend/api/crm.yaml`) |
| What is drawn, and what is not | `frontend/src/app/ai-activity-lines.ts` |
| The rail component + poll | `frontend/src/app/agentrail.tsx`, `ai-activity.ts` |
| The ask: which routes count, and the count | `x-waits-on-model` in `backend/api/crm.yaml`, `MODEL_ROUTES` in `frontend/src/api/client.ts`, `frontend/src/api/model-inflight.ts` |
| The census gate | `backend/gates/aiactivitycatalogparity_test.go` |

**Related:** [ai-runtime.md](ai-runtime.md) (the task contract and the Router
gate) · [write-backbone.md](write-backbone.md) (the outbox the facts ride) ·
[job-fleet.md](job-fleet.md) (the scheduled work two carriers report) ·
[frontend-architecture.md](frontend-architecture.md) ·
[how-to/add-an-ai-task.md](../how-to/add-an-ai-task.md).
