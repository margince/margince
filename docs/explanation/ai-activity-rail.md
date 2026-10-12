<!-- prose:plain -->
# The AI activity rail: what the AI is doing for you, while it does it

A rep who asks for a summary should see that it is being written. The AI activity rail shows AI work
while it runs and after it ends. Without it, the answer shows up after a silent wait, which looks like a
product that did nothing and then made up an answer.

The sections below cover the one projection behind the rail, who reports into it, and who an occurrence
belongs to. Which kinds a reader sees is a separate question with a different owner, so it gets its own
section.

The server must record every AI task: a task that reports nothing is AI work the product did and then
kept from the user. The client decides which kinds to show, and it shows fewer. If one place made both choices, the
result would be either a silent product or 300 lines of text nobody reads.

## The shape

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

Nothing outside `internal/modules/aiactivity` writes `ai_task_run`, and no statement in it makes up a fact
the bus did not bring. That is why the module imports no sibling: the facts it needs come in the envelope.
A projection that read the tables of a source would be a second reader of a truth it is meant to hold.
There are two exceptions, both for retention: `PurgeSettledBefore` removes old rows, and
`CloseAbandonedRouterRuns` settles the ones whose source will never settle them. Removing old rows from a
read model does not change domain data, but it is still a write.

## Who reports: the router, a carrier, or nobody

`ai.railOwners` (`internal/modules/ai/railowner.go`) answers this for every task in `api/ai-tasks.yaml`.
It covers that whole table: a task the generator adds and nobody answers fails the build. The answers it
can give:

| Owner | What it means | What it can say |
|---|---|---|
| `SourceRouter` (`ai_router`) | The default. The router reports for the task, so a task is wired before its author has looked at the rail. | It learns of a call only once the call is over, plus a `running` line sent just before the call. Never `queued`. |
| A **carrier** (`agent_runner`, `attachment_extraction`, `account_scan`, `transcript_read`, `voice_build`, `site_read`) | Work that owns a lasting row reports for itself. | `queued`, `running`, and (because a carrier declares a lease) a dead attempt that the read can show as `stalled`. |
| `SourceNoOccurrence` (`none`) | The work is a step inside someone else's occurrence and has no occurrence of its own. | Nothing of its own; it is reported under the unit of work it serves. |

`SourceNoOccurrence` does not free a task from reporting, and every use must give a reason in
`railNoOccurrenceReasons`. The reasons are checked, because "reported by nobody" is one key press away
from the silence this registry is there to end. Two groups use it today. `embeddings` does, because every
embedding call serves a search, an enrich or a reindex, and that is the occurrence. The three website read
passes do, because the read is the occurrence. A reason that is a matter of what looks good ("no rep wants to see
it") belongs in the client, which decides what to draw.

Where a carrier exists, it is the better reporter and the router stays silent. So the two never write one
occurrence between them.

The website read is the one carrier that is not a task. A deep read (`contacts/sitereadactivity.go`,
`source=site_read`, kind `site_read`) reads up to 12 pages and makes more than one model call. The
router does not report those calls. The read reports itself as one `site_read` occurrence, keyed on its
own id. The dossier row can say `queued` when a rep presses "read the site", and `running` while the read
is under way. The three `site_*` kinds (`site_triage`, `site_extract`, `site_fact_extract`) stay in the
enum only so a filter can still find old rows.

## Who an occurrence belongs to

`ResolveActor` (`aiactivity/actor.go`) finds the owner from the envelope, never from the payload. A
sender chooses its payload. It cannot choose the signed-in actor that the write shape put on the event.
So it cannot claim its work for someone else by filling in a field.

| The event's actor | Scope | `actor_user_id` |
|---|---|---|
| human | `personal` | that human |
| not human, **with** `on_behalf_of` | `personal` | the human behind it |
| not human, **without** `on_behalf_of` | `workspace` | NULL |
| human that does not parse | *refused* | (none) |

A human actor that does not parse is refused; it is not given workspace scope without a trace. Making it
wider would turn one user's work into a system sweep that nobody can find, and nobody would see that it
is missing.

On a worker path, `OnBehalfOf` comes from the job's own input. So this rule is the same everywhere but
does not stop a forged value: one rule, one place, one way to fail.

This decides most of the display census below. A workspace occurrence has a NULL `actor_user_id`, and
the read filters on `actor_user_id = $1`. So background work with nobody behind it reaches nobody's rail,
by the way it is built.

## The read

`GET /me/ai-activity` (`aiactivity/read.go`) needs a cookie, is `human-only`, and only reads. It writes
no audit or event row.

The user comes from the bound principal and is not a parameter, and that is all the authorization there
is. A store method with a user id parameter would let any caller in the process ask for someone else's feed.
The only guard against a leak would then be that no caller forgets to pass its own. Here there is
no way to ask for another user's feed. No RBAC object gates it, because there is no wider set to keep
back.

Four things to know:

- **One statement.** The transaction is READ COMMITTED, so two statements would take two snapshots. An
  occurrence that settled between them would show up in both. The rail would then say
  `reading your document` and `I've read your document` about one reading at once.
 One statement is one snapshot.
- **Two parts.** Each has its own order, bound and partial index. `live` is `queued`/`running`, in order
  of `queued_at`. `settled` is bounded by `finished_at`, because "what the AI finished for this user today" is a
  question about when it finished. If it were keyed on the start, a run from 23:50 to 00:10 would drop
  out of `settled` after it had left `live`, and never show.
- **`stalled` is worked out.** The read works out `stalled` and never stores it. A live occurrence past
  the lease its own source declared is reported as `stalled`, always, in SQL, against the database clock.
  Nothing writes it, so nothing can forget to. That stops a worker that died during a run from showing
  as working forever.
- **`recent` is bounded**: since the start of the local day, at most 10. A per-user history with no limit would be a
  per-user activity log, which this installation does not keep. `summary` and `degrade_reason` are
  cut to a limit on the way to the wire (2000 / 500). If they were not, a model's whole output, which a
  prompt injection could make longer, would ship to every open tab on every poll.

`degrade_reason` is text the server writes in the source's own words, never a message from a provider or
a parser. Those messages hold vendor text and can hold credential data, and this field reaches an
ordinary rep. `MarkFailed` takes a typed `runner.FailureReason`, so a raw error does not compile.

`kinds` is how a client that draws part of the record asks for its part, and the read applies it before
the bounds. Every task reports. So a caller that shows a few kinds, and gets the newest ten of all kinds,
can get ten it draws nothing for. The bound would fall on rows the reader never sees, and the rail would
go blank while its work was reported correctly. An empty list is a 422, and so is an unknown name. If not,
both would come back as an empty feed, which is the true answer for an AI at rest.

The SPA polls every **`3s` while something is live, `30s` at rest**, and fetches again when the user
comes back to the tab. Fetching again on focus is turned off across the app. Here the cached body is the
wrong one: the run it shows as live is the run that finished while the tab was away.

## What a reader is shown

`frontend/src/app/ai-activity-lines.ts` holds `ACTIVITY_LINE`. For each kind in the contract's
`AiActivityKind` enum, it has either a `(state → message key)` table or a written reason there is none. It
is typed `Record`, not `Partial<Record>`. So a new kind fails the build until someone writes its text in
every locale, or says in code why it is not shown. The reason sits in the source because the next author
reads the file, not the PR.

The text uses a literal key, never `t(\`agent.activity.${kind}.${state}\`)`. The guard for keys no code uses, in

`i18n/orphan-keys.test.ts` counts a key as in use when it starts with the fixed part of a template. A
key built from parts would count for the whole namespace forever. The text of a removed kind would then
sit in three catalogs with nothing to flag it.

**Kinds the rail tells**, in `en`, `de` and `vi`, over all six states:

| Kind | Reported by | The line a rep sees |
|---|---|---|
| `morning_brief` | carrier (`agent_runner`) | the scheduled brief |
| `overnight_at_risk_sweep` | carrier (`agent_runner`) | the scheduled sweep |
| `document_extract` | carrier (`attachment_extraction`) | reading a document you attached |
| `account_scan` | carrier (`account_scan`, the `company_scan` row) | `I'm reading Brandt Automotive's exchanges and deals.` Named for the account, so a reader who opened three accounts and moved on knows which is ready |
| `site_read` | carrier (`site_read`) | reading a company's website, named for the company |
| `transcript_propose` | carrier (`transcript_read`) | reading a meeting transcript for its next steps |
| `voice_build` | carrier (`voice_build`) | learning the reader's own writing voice |
| `weekly_review` | router | the review of the week, under the rep's own principal |
| `weekly_learnings` | router | the rep's own week, under their own principal |
| `summarize` | router | `I'm writing your summary.` |
| `draft_reply` | router | `I'm drafting your reply.` |
| `offer_draft` | router | `I'm drafting your offer.` |

For `summarize`, `draft_reply` and `offer_draft`, `queued` text exists and is never reached. The router
reports a call it is about to serve, never one that waits, and no carrier owns these tasks. The key exists
because the set of states must be whole, and the compiler needs it.

**Kinds the rail does not tell**, each with its own kind of reason:

| Reason | Kinds | Why |
|---|---|---|
| Watched by the asker | `growth_fit`, `cold_start`, `corpus_ask` | Shown on the surface that asked. |
| System sweep | `brief_ranking`, `capture_classify`, `capture_confidentiality_verdict`, `capture_counterparty_verdict`, `owed_verdict`, `propose_roles`, `rate_extract`, `request_settlement`, `signal_extract`, `stage_evidence_extract` | Background workspace work that belongs to no single user. |
| The read tells itself | `site_extract`, `site_fact_extract`, `site_triage` | Removed; `site_read` is the occurrence. |
| Reaches nobody, and would not be worth showing | `enrich` | No personal owner, and one row for a whole pass. |
| A measure for operators | `cert_judge` | Grades this build's answers; not a rep's work. |
| Answered on screen | `nl_search` | The filter answers in the builder the reader pressed it from. |
| Declared, not built | `deal_health`, `transcript` | Named in `api/ai-tasks.yaml`; no site runs them. |

`growth_fit` shows the band it returns on the panel that asked. `cold_start` runs behind two product
surfaces. One is onboarding, whose screen has no rail: `onboarding` is in `RAIL_LESS_SCREENS` in `nav.ts`,
which `shell.tsx` reads to drop the frame. The other is the Enrich card on the company page
(`cmd/api/modelwiring.go` wires `WithScrape` with the `cold_start` brain). There a rail exists, and the
card itself shows the proposal. It also declares four call *sites* in `aitaskregistry.go`, which is a
different count from the two surfaces.

The three `site_*` kinds are the single model calls a website read makes. `site_read` is one occurrence
for the whole read, reported by the dossier from queued to settled. A line per call would tell one reading
more than once. The read owns the work: a read a human asked for holds that human as `on_behalf_of` and
is personal to them. A domain triage read, or one that an enrich run started on its own, names no human and has
workspace scope.

`enrich` fails on reach and on worth. Its one production site is the signature enrich pass, which runs
under a system principal with no `on_behalf_of`. So every occurrence has workspace scope with a NULL
`actor_user_id`, and the personal feed selects on `actor_user_id`. It also could not be per contact if it
were reached.

The pass makes one correlation id for the whole run (`capture_enrich`, up to 100 candidates
in a row), and the occurrence key is correlation plus task. Every candidate falls into one row. So a
subject per contact would make that row flip again and again without telling anything. What a reader
wants from it is what it found, and that lasts. It is already on the contact record as provenance: shown
with its evidence, or left out.

`enrich` looks visible and is not: the `enrich` key of the ticker names different work. That key covers a
provider run on a contact (`contactprovider.tsx`) and the Enrich card on the company page
(`companies.tsx`). The card sends a POST to `/companies/{id}/enrich` and so runs `cold_start`, not this
task. The deep read has its own `site-read` ticker key.

### Where a told line shows up

Three places, and each answers a different question.

- **The panel's running section** lists what is live, and only that.
- **The panel's recap** ("What it has done") lists what settled today, newest first, at most five. It
  uses the same text table, so one occurrence is told in one vocabulary from start to end. It says
  `I'm reading the Acme website` while it runs, and `I've read the Acme website` once it is done. The record's name is
  a link, which is the half of the row a reader can act on.
- The recap reads this feed, not the trace of model calls. `ai_call` is telemetry and holds no subject
  (`Call.Subject` reaches the occurrence and never the trace). A recap read from there could report that
  something happened, and nothing about what it happened to. The trace is still one click away, behind
  `Full log`.
- **The card's resting line** turns through the newest settled occurrence among the agent's other facts,
  because the rail is `235px` wide and holds one line.

A kind with no text draws nothing in any of the three. The recap says "nothing has finished today" only
once the feed has answered. A feed not yet read draws no sentence at all, because "nothing finished" is a
claim about a day someone looked at.

### The ask: what this tab knows before the feed does

The feed comes on a poll. So between a rep pressing "Draft with AI" and the next read, there is a live
model call that nothing on screen reports. The client closes that window from its own end
(`frontend/src/api/model-inflight.ts`). It counts every request it holds open to a route whose handler
calls a model and waits. The rail treats a count above zero as `working`.

That state has no kind, no state and no sentence, because the client knows none of those. It comes below
every occurrence the feed holds, so the feed names the work as soon as it can.

The count also moves the poll to its live speed, and fetches again at the start and end of the request.
So the feed's own line follows within seconds.

The contract says which routes count. An operation whose handler holds the request open on a model is
marked `x-waits-on-model: always` (a draft, the meeting brief: made on every call) or
`x-waits-on-model: on-miss`. The `on-miss` routes answer from a stored reading, and generate only when there is
none. They are the dossier, the contact brief, the deal status and the morning brief.

The client's
`MODEL_ROUTES` table (`api/client.ts`) is a declared mirror of the marked set. It is keyed by method and
path, because the dossier is read and refreshed at one path, and only the refresh generates every time.
`backend/gates/modelroutes_test.go` fails when the two disagree in either direction or on the value.

An `on-miss` route answers from the store in well under a second, and from the model in many. When the request leaves, nothing the client can see tells which of the two it is. So it is counted only once the request has
lived past `CACHE_ANSWER_GRACE_MS` (one second). A stored answer never lights the orb, since that is the
reader's own click, and a generated one lights it a second late.

A route that queues model work and answers 202 is not marked. It holds nothing open, and its occurrence
reaches the rail the way every background run does, through its carrier and the feed. A surface that
starts one calls `watchStartedAiRun`, which is the other way the rail learns of it.

### Two surfaces, one action, told once

The taskbar ticker tells **this tab's own react-query cache**; the rail tells **the server's feed**.
Three ticker lines describe work the rail also covers, and they do not both show. The bar shows the
ticker line or the rail line as one `if/else` on a single span (`agentrail.tsx`). So a reader never sees
one action twice.

The ticker keys for email are split: `email-draft` for the rail and `email` for the ticker. Four
changes (two drafts and two sends) once shared one key. The split keeps each action told once, by the
surface that knows about it.

## The gates that hold it

All of these are in the root package. Only the root can see the task contract, the wire contract, the
read's own bounds and the senders at once.

| Rule | Gate |
|---|---|
| Every task names the source that reports it, and every `SourceNoOccurrence` gives a reason that holds up | `TestEveryAITaskNamesTheSourceThatReportsIt` (`backend/gates/aitaskrailcensus_test.go`) |
| The registry names no task the contract dropped | `TestTheRailRegistryNamesNoTaskTheContractDropped` |
| A carrier's source literal is one that something really sends | `TestEveryCarrierOverrideNamesASourceThatIsReallyEmitted` |
| Every kind something makes is one the contract enum can express | `TestEveryKindSomethingProducesIsOneTheContractCanExpress` (`backend/gates/aiactivitycatalogparity_test.go`); its failure prints an `align:` line naming the file, the schema and what to add |
| Every contract kind has something that makes it | `TestEveryContractKindHasSomethingThatProducesIt` |
| The read's text limits are the ones the contract publishes | `TestTheReadsTextCapsAreTheOnesTheContractPublishes` |
| Every spec name can be part of a message key | `TestEverySpecNameCanBeAMessageKeySegment` |
| The client's table of routes that hold a model call open is the contract's `x-waits-on-model` set, in both directions and on the value | `TestTheClientsModelRouteTableIsTheContracts` (`backend/gates/modelroutes_test.go`) |
| Every contract kind is shown or holds a written reason | the TypeScript `Record` type: a **compile error**, not a test |
| Every shown kind has text in `en`, `de` and `vi`, for all six states | the `LineSet` type + the `i18n` catalogs |

The census gate takes its subjects from the registry. The router reports for every task the registry
leaves to it, and that set grows the moment someone declares a task. A list kept by hand would always be
one edit behind the contract, and could miss a task that reports nothing.

## Limits we know of

- **A unit of many calls opens again** ([#2276]). Such a unit opens its occurrence again once
  per call.
  - Take a task whose unit of work spans more than one model call under one correlation id. It settles
  after each call, and opens again at the next call's attempt.
  - Within one such call, the occurrence stays the same. `CompleteStructured` walks the ladder up to three times under one start. The lease is made
  longer before every model call after the first.
- **Lease renewal.** The projection admits one write into a live attempt: a longer lease.
  `applyStateChangeSQL` guards with a strict `>` on `(attempt, rank)`. Beside that, it takes an event
  with an equal tuple in the same live state whose `stale_after` is later than the row's.
  - That is the lease renewal the router makes per model call. It lets each lease be sized for one call (`CallCeiling`
  plus the flush), not for the whole unit of work. A second delivery holds an equal time and a late
  renewal an earlier one, so the branch refuses both. A settled row has no `stale_after`, so it refuses
  those too. A line per step to show how far a run has come stays out of reach without a further change.
- **Two states with no name.** The contract has no word for two real states. The site read's `deferred`
  ("waiting on budget, retry at `next_attempt_at`") is not `queued`, since nothing will pick it up, and
  not settled. Its `cancelled` is final, but `done` and `failed` would both state it wrong. Either the
  enum grows or someone decides the mapping.
  - Two mappings already fit: `site_read.stopped_reason` is a closed vocabulary that drops right into
    `degrade_reason`, and `partial` → `degraded`.
- **Subjects are passed on, never filtered on.** The event envelope has `subject_type` and `subject_id`,
  and `ai_task_run` has both columns. The feed ships them beside `subject_label`, so the rail can make the
  name a link to the record. That works for a company or a contact; a document or a meeting has no page
  and stays text.
  - They travel on the same ground as the label. The source sent them only where the actor
  is the user who was already shown the record. The read stays keyed on the user alone.
- **A read by subject was turned down.** Today another user's feed cannot be asked for, so the
  authorization holds by the way it is built. The new read would hold only because a gate has run.
  `auth.EnsureVisible` alone is not that gate. It checks no object grant, and for an identity table its
  clause is empty, so it returns success without a query.
- The one subject in use, `attachment`, has no row scope at all. Its rights come from a parent record that
  can be of more than one type. A single `LIMIT` over a wider filter would also let one group of rows push
  out the other. If it is ever built, it copies `ai/feedback.go`, which already spells the gate correctly.
- **`enrich` per contact was turned down.** The reason is in the census entry beside
  the code. The pass makes one correlation id for all its candidates, so they share a single occurrence
  that no subject per contact could describe.

[#2276]: https://github.com/margince/margince/issues/2276

## Reference

| What | Where |
|---|---|
| The projection + the read | `internal/modules/aiactivity` (owns `ai_task_run`, imports no sibling) |
| Who reports each task | `internal/modules/ai/railowner.go`: `railOwners`, `RailOwner`, `RouterReports` |
| The router's start and settle pair | `internal/modules/ai/railstart.go`, `railemit.go` |
| The carriers | `internal/modules/agents/runner/activity.go`, `internal/modules/activities/extractionactivity.go`, `internal/modules/activities/transcriptactivity.go`, `internal/modules/ai/voicebuildactivity.go`, `internal/modules/contacts/sitereadactivity.go`, `internal/compose/companyscan/` |
| Who owns the work | `aiactivity/actor.go`: `ResolveActor` |
| The event | `ai_task.state_changed` (`shared/kernel/events/catalog.go`; payload generated into `internalevents_gen.go`) |
| The wire | `AiActivity` / `AiActivityKind` / `AiActivityItem` + `GET /me/ai-activity` (`backend/api/crm.yaml`) |
| What is shown, and what is not | `frontend/src/app/ai-activity-lines.ts` |
| The rail part + poll | `frontend/src/app/agentrail.tsx`, `ai-activity.ts` |
| The ask: which routes count, and the count | `x-waits-on-model` in `backend/api/crm.yaml`, `MODEL_ROUTES` in `frontend/src/api/client.ts`, `frontend/src/api/model-inflight.ts` |
| The census gate | `backend/gates/aiactivitycatalogparity_test.go` |

**See also:**

- [ai-runtime.md](ai-runtime.md): the task contract and the Router gate.
- [write-backbone.md](write-backbone.md): the outbox the facts travel on.
- [job-fleet.md](job-fleet.md): the scheduled work two carriers report.
- [frontend-architecture.md](frontend-architecture.md) and [how-to/add-an-ai-task.md](../how-to/add-an-ai-task.md).

