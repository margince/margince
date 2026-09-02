# The AI activity rail — what the AI is doing for you, while it does it

A rep who asks the product to write a summary should see that it is being
written. Before this existed they saw nothing for however long the model took,
and then the answer appeared — which reads as a product that did nothing and
then guessed.

This page explains the rail: the one projection behind it, who reports into it,
who an occurrence belongs to, and — separately, because it is a different
question with a different owner — which kinds a reader is actually shown.

**The two questions this page keeps apart.** The server's obligation is a
COMPLETE record: every AI task this build can run reports, because a task that
reports nothing is AI work the product performed and then denied. What a reader
is SHOWN is the client's decision, and it is narrower. Conflating them is how
you get either a silent product or 300 strings nobody reads.

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
that reached back into a source's tables would be a second reader of a truth it
is supposed to hold. (Two exceptions, both retention: `PurgeSettledBefore` ages
rows out and `CloseAbandonedRouterRuns` settles the ones whose source will never
settle them. Ageing a read model is not a domain mutation, but it IS a write.)

## Who reports — the router, a carrier, or nobody

`ai.railOwners` (`internal/modules/ai/railowner.go`) answers this for every task
in `api/ai-tasks.yaml`, and it is TOTAL over that table: a task the generator
adds and nobody answers fails the build. Three answers:

| Owner | What it means | What it can say |
|---|---|---|
| `SourceRouter` (`ai_router`) | The default. The router announces on the task's behalf, so a task is wired before its author has thought about the rail. | It learns of a call only once the call is over — plus a `running` line announced just before the call. Never `queued`. |
| A **carrier** (`agent_runner`, `attachment_extraction`) | Work that owns a durable row reports for itself. | `queued`, `running`, and — because a carrier declares a lease — a dead attempt that can be derived as `stalled`. |
| `SourceNoOccurrence` (`none`) | The work is a STEP inside somebody else's occurrence, not an occurrence of its own. | Nothing of its own; it is reported under the unit of work it serves. |

`SourceNoOccurrence` is **not** an exemption from reporting, and every use owes a
reason in `railNoOccurrenceReasons` — checked, because "reported by nobody" is
one keystroke from the silence this registry exists to end. One task uses it
today: `embeddings`, because every embedding call happens in service of a search,
an enrich or a reindex, and that is the occurrence. A reason that is really an
editorial preference ("no rep wants to see it") belongs in the CLIENT, which
decides what to draw.

Where a carrier exists it is the better reporter and **the router stays silent**,
so the two never write one occurrence between them.

## Who an occurrence belongs to

`ResolveActor` (`aiactivity/actor.go`) derives the owner FROM THE ENVELOPE, never
from the payload. An emitter chooses its payload; it cannot choose the
authenticated actor the write shape stamped, so it cannot attribute its work to
somebody else by filling in a field.

| The event's actor | Scope | `actor_user_id` |
|---|---|---|
| human | `personal` | that human |
| non-human **with** `on_behalf_of` | `personal` | the human behind it |
| non-human **without** `on_behalf_of` | `workspace` | NULL |
| human that does not parse | *refused* | — |

The last row is the interesting one: a human actor that does not parse is
REFUSED rather than quietly made workspace-scoped, because quietly widening it is
how one person's work becomes a system sweep nobody can find and nobody notices
is missing.

Stated honestly: on a worker path `OnBehalfOf` is itself derived from the job's
own args, so this is uniform rather than tamper-proof. Uniform is the benefit
worth having — one rule, one place, one failure mode.

**The consequence that decides most of the display census below:** a
workspace-scoped occurrence has a NULL `actor_user_id`, and the read filters
`actor_user_id = $1`. So background work with nobody behind it reaches nobody's
rail — not by editorial choice, but structurally.

## The read

`GET /me/ai-activity` (`aiactivity/read.go`), cookie-authenticated, `human-only`,
read-only — no audit or event row.

**The person is taken from the bound principal and is NOT a parameter, and that
is the whole of the authorization.** A store method that accepted a user id would
let any in-process caller ask for somebody else's feed, and the only thing
standing between that and a leak would be every caller remembering to pass its
own. Here there is nothing to remember: another person's feed cannot be
expressed. No RBAC object gates it, because there is no wider set to withhold.

Four properties worth knowing:

- **One statement, not two.** The transaction is READ COMMITTED, so two
  statements would take two snapshots and an occurrence that settled between them
  would appear in both — the rail saying "reading your document" and "I've read
  your document" about one reading at once. One statement is one snapshot, so the
  window does not exist to be closed.
- **Two arms, each with its own ordering, bound and partial index.** `live` is
  `queued`/`running` ordered by `queued_at`; `settled` is bounded by
  `finished_at`, because "what the AI finished for me today" is a question about
  when it finished — keyed on its start, a run that began 23:50 and ended 00:10
  would fall out of `settled` AND have already left `live`, vanishing entirely.
- **`stalled` is derived at read time and never stored.** A live occurrence past
  the lease its own source declared is reported stalled, unconditionally, in SQL,
  against the DATABASE clock. Nothing writes it, so nothing can forget to — which
  is what stops a worker that died mid-run from being displayed as working
  forever.
- **`recent` is bounded** — since local midnight, at most 10. An unbounded
  per-person history is a per-person activity ledger, which this installation
  deliberately does not keep. `summary` and `degrade_reason` are capped on the
  way to the wire (2000 / 500), because a model's whole output — possibly
  inflated by a prompt injection — otherwise ships to every open tab on every
  poll.

`degrade_reason` is server-authored prose in the SOURCE's own words, never a
provider's or a parser's message: those carry vendor text and can echo credential
material, and this field reaches an ordinary rep. `MarkFailed` takes a typed
`runner.FailureReason` so a raw error **fails to compile**.

`subject_label` is what the occurrence was ABOUT, named the way the product
titles the record elsewhere. Two sources fill it. The document reader resolves
the attachment's title itself. Every router-served task carries whatever its
caller bound with `principal.WithWorkSubject` — the person a reply is drafted
to, the company being read up on, the deal an offer is for, the host of the
website being read — and the router stamps it on both the opening `running`
announcement and the settle (`ai/railstart.go`, `ai/railemit.go`, bounded to
the contract's 120 runes in `railSubject`). The binding sits in the compose
service that already loaded the record under the caller's own visibility gate,
so the name reaches only the person the record was shown to. A caller that
binds nothing leaves the field null, and the client draws its generic sentence.

**`kinds` is how a client that draws part of the record asks for its part**, and
it is applied BEFORE the bounds. Every task reports, so a caller that renders six
kinds and is served the newest ten of twenty-seven can be handed ten it draws
nothing for — the bound would fall on rows the reader never sees and the rail
would go blank while its work was reported correctly. An empty list is a 422, and
so is an unknown name; both would otherwise come back as an empty feed, which is
the TRUE answer for an AI at rest.

The SPA polls every **3s while something is live, 30s at rest**, and refetches
explicitly on tab return — focus refetching is disabled app-wide, and the cached
body is exactly the one that is wrong, because the run it shows as live is the
run that finished while the tab was away.

## What a reader is actually shown

`frontend/src/app/ai-activity-lines.ts` holds `ACTIVITY_LINE`: for each of the
contract's 27 kinds, either a `(state → message key)` table or a written reason
there is none. It is typed `Record`, not `Partial<Record>`, so **a new kind fails
the build** until somebody either writes its copy in every locale or says, in
code, why it is not shown. The reason lives in the source rather than in a review
comment for the same reason: the next author reads the file, not the PR.

Each kind's six keys are built from one stem by `lineSet`, so the orphan guard in
`i18n.test.ts` cannot hold this namespace: it counts a key as rendered when the key
starts with a template stem, and `lineSet`'s template vouches for all of
`agent.activity.` at once. What holds it instead is the copy-set spec in
`ai-activity-lines.test.ts`, which asserts the catalog's `agent.activity.` keys are
exactly the keys the maps name, in both directions, so a retired kind's copy fails
there rather than sitting in three catalogs unflagged.

**Eleven kinds are narrated**, in en/de/vi, total over all six states — and
every kind a person asks for about ONE record also carries a **named** variant
(`NAMED_LINE`), used whenever the feed sent a `subject_label`:

| Kind | Reported by | The line a rep sees |
|---|---|---|
| `morning_brief` | carrier (`agent_runner`) | the scheduled brief |
| `overnight_at_risk_sweep` | carrier (`agent_runner`) | the scheduled sweep |
| `weekly_review` | router | the weekly retrospective |
| `document_extract` | carrier (`attachment_extraction`) | "I'm reading Q3-offer.pdf." |
| `summarize` | router | "I'm pulling together what I know about Zenloop GmbH." |
| `draft_reply` | router | "I'm drafting your reply to Anna Berg." |
| `offer_draft` | router | "I'm drafting your offer for Zenloop renewal." |
| `growth_fit` | router | "I'm judging how well Zenloop GmbH fits what we sell." |
| `corpus_ask` | router | "I'm answering your question from Product docs." |
| `cold_start` | router | "I'm reading zenloop.com." (the Enrich card's website read) |
| `site_extract` | router | "I'm reading through zenloop.com." (the deep read) |

The account-reading kinds used to be withheld as "watched by the asker", on the
argument that the result lands on the surface that asked. They are drawn now
because the orb is the one place a reader looks to learn the AI is at work at
all: a card that fills in forty seconds later tells nobody what was happening
during the forty seconds. Each is personal — the handler runs under the asker's
own principal, or compose binds them as `on_behalf_of` — so each reaches exactly
the feed of the person who asked.

For the router-owned ones, `queued` copy exists and **is unreachable**: the
router announces a call it is ABOUT to serve, never one waiting, and no carrier
owns these tasks. The key exists because the state axis is total and the compiler
requires it — not because a producer is missing.

**Sixteen are not narrated**, and each reason is a different kind of fact:

| Reason | Kinds | Why |
|---|---|---|
| System sweep | `brief_ranking`, `capture_classify`, `capture_confidentiality_verdict`, `capture_counterparty_verdict`, `rate_extract`, `signal_extract`, `propose_roles`, `transcript_propose`, `voice_build` | Background workspace work that belongs to nobody in particular, so it has no personal line to draw. |
| A step of a read already narrated | `site_fact_extract`, `site_triage` | One website read files one occurrence per lane it runs, because the occurrence key is correlation+task and a read's correlation is its `site_read` row id. `site_extract` is the profile lane every human-requested read runs and is the one drawn; `site_fact_extract` is that read's page-parallel fact pass, and `site_triage` fires only for a domain-triage read no human requested (`isDomainTriageRequest`), which is workspace-scoped and reaches nobody's feed anyway. Drawing either would list one read two or three times over. |
| Reaches nobody, and would not be worth showing | `enrich` | Both halves matter. **Reachability:** its one production site is the signature-enrichment pass, which runs under a system principal with no `on_behalf_of` — so every occurrence is workspace-scoped with a NULL `actor_user_id`, and the personal feed selects on `actor_user_id`. **Worth:** it could not be per-person even if it were reachable. The pass mints ONE correlation id for the whole run (`capture_enrich`, up to 100 candidates in series) and the occurrence key is correlation+task, so every candidate collapses into one row — a per-person subject would make that row flap rather than narrate anybody. What a reader wants from it is what it FOUND, which is durable and already drawn as evidence-or-omit provenance on the person record. |
| An operator's measurement | `cert_judge` | The certification lane grading this build's own answers — not a rep's work. |
| Declared, not built | `deal_health`, `nl_search`, `transcript` | Named in `api/ai-tasks.yaml`; no site runs them, so nothing reports them yet. |

`enrich` is the one worth reading twice, because it looks visible and is not: the
ticker's own `enrich` key names DIFFERENT work — a provider run on a person
(`personprovider.tsx`), and the organization page's Enrich card
(`organizations.tsx`), which POSTs `/organizations/{id}/enrich` and therefore
runs `cold_start`, not this task. The deep read rides its own `site-read` ticker
key, not this one.

### Why a run stopped, and what to do about it

`degrade_reason` reaches the reader ONLY through `REASON_LINE`
(`ai-activity-lines.ts`): one translated sentence per value of the router's
closed vocabulary (`classifyError` and the attempt reasons in
`ai/callstore.go`, `ai/logicalcall.go`), each pairing the cause with a repair —
"the provider's quota is used up; an admin can top it up or bind another model
under Settings → AI". The panel lists every run that failed or stopped early
today under **What went wrong today**, the run's own line first and the reason
under it. A value off the vocabulary draws nothing: the scheduled runner's
`FailureReason` sentences are English prose an ordinary rep is not shown, and
a raw token is a fault they cannot read.

### A fault flashes or holds, by who was watching

`useAgentFault` (`agent-fault.ts`) colours the orb for a run that failed or
degraded, and how long it stays coloured depends on whether anybody was there
when it broke:

- **Scheduled and background work** — the brief, the sweep, the review, a
  document reading — fails while nobody is looking and has no other screen to
  land on. Its fault **holds until the panel is opened**, however many hours
  that takes, and opening the panel acknowledges every fault it delivered.
- **Attended work** — the kinds a person asked for and waited on (`ATTENDED`:
  the drafts, the brief, the fit, the answer, the reads) — is reported on the
  screen that asked, the moment it happens. Its fault **flashes** for eight
  seconds and then counts as seen on its own, staying in the panel's
  "What went wrong today" list for the rest of the day. Holding it until the
  panel was opened kept the orb red through the afternoon over a draft the
  reader had already retried and sent.

### Two surfaces, one action, no double narration

The taskbar ticker narrates **this tab's own react-query cache**; the rail
narrates **the server's feed**. The bar renders the ticker line OR the rail line
as one `if/else` on a single span (`agentrail.tsx`), so a reader never sees one
action twice — and where a ticker key and a displayed kind name the same action,
the ticker entry goes (the `COLLISIONS` test in `ai-activity-lines.test.ts`
pins the pairs).

One collision was real and was fixed at the key rather than the table: removing
the ticker's `email` entry to end a two-vocabulary clash also silenced email
SENDS, because four mutations shared that key — two drafts and two sends. The key
is now split (`email-draft` for the rail, `email` for the ticker), so each action
is narrated exactly once by whichever surface actually knows about it. The
ticker's `site-read` entry went the same way once the rail began drawing
`site_extract`: the rail names the site for as long as the AI is reading it,
which is the line the click was standing in for.

## The gates that hold it

All of these live in the ROOT package, because the root is the only place that
can see the task contract, the wire contract, the read's own bounds and the
emitters at once.

| Obligation | Held by |
|---|---|
| Every task names the source that reports it, and every `SourceNoOccurrence` owes a defensible reason | `TestEveryAITaskNamesTheSourceThatReportsIt` (`backend/gates/aitaskrailcensus_test.go`) |
| The registry names no task the contract dropped | `TestTheRailRegistryNamesNoTaskTheContractDropped` |
| A carrier's source literal is one something really emits | `TestEveryCarrierOverrideNamesASourceThatIsReallyEmitted` |
| Every kind something produces is one the contract enum can express | `TestEveryKindSomethingProducesIsOneTheContractCanExpress` (`backend/gates/aiactivitycatalogparity_test.go`) — its failure prints an `align:` line naming the file, the schema and exactly what to add |
| Every contract kind has something that produces it | `TestEveryContractKindHasSomethingThatProducesIt` |
| The read's text caps are the ones the contract publishes | `TestTheReadsTextCapsAreTheOnesTheContractPublishes` |
| Every spec name can be a message-key segment | `TestEverySpecNameCanBeAMessageKeySegment` |
| Every contract kind is displayed or carries a written reason | the TypeScript `Record` type — a **compile error**, not a test |
| Every displayed kind has copy in en/de/vi, for all six states | the `LineSet` type + the i18n catalogs |
| Every kind a person asks for about one record has a named variant, and every router sentinel a reason line | `ai-activity-lines.test.ts` |
| A router occurrence carries the subject its caller bound, at the start and the settle | `TestServingACallAnnouncesItsStartToTheRail`, `TestRailSubjectIsTheBoundedNameOrNothing` (`internal/modules/ai`) |

The census gate is deliberately derived rather than listed: the router announces
for every task the registry leaves to it, and that set grows the moment somebody
declares a task — so a hand-kept list would be one edit behind the contract
forever. That is the exact shape of the defect that left seventeen shipped tasks
reporting nothing at all.

## Known limits

- **A router-owned line begins at the model call, not at the click** ([#2272]).
  The router says `running` the instant it starts serving, and never `queued`;
  a deep read crawls before its profile lane calls a model, so the rail's
  `site_extract` line appears once the crawl has pages to read from, and the
  organization page's own read status covers the crawl. `transcript_propose`,
  `site_fact_extract`, `site_triage` and `voice_build` each have a durable,
  attributable row that could say `queued`, but none is registered as a carrier.
- **One site read files one occurrence per lane it runs** ([#2272]). Its
  correlation id is the `site_read` row id, so the router's
  `correlation_id + task` key produces a separate line per task under one read.
  The client draws `site_extract` and treats the other two as steps of the same
  read, which is what keeps one read from listing three times.
- **A multi-call unit reopens its occurrence once per call** ([#2276]).
  `CompleteStructured` walks the ladder three times, and the lease is announced
  once and cannot be renewed.
- **The projection refuses a write into a live attempt.** `applyStateChangeSQL`
  guards with strict `>` on `(attempt, rank)`, so an equal-tuple event updates
  nothing. A lease REFRESH and per-step progress TICKS are both impossible
  without changing the projection.
- **The contract has no word for two real states.** Site read's `deferred`
  ("waiting on budget, retry at `next_attempt_at`" — not `queued`, since nothing
  will pick it up, and not settled) and `cancelled` (terminal, but `done` and
  `failed` would both lie). Either the enum grows or somebody rules on the
  mapping. Two things already fit exactly: `site_read.stopped_reason` is a closed
  vocabulary that drops straight into `degrade_reason`, and `partial` →
  `degraded`.
- **`subject_type` / `subject_id` are carried and stored but never read.** The
  event envelope has both, `ai_task_run` has both columns, and exactly one
  emitter populates them (`document_extract` → `attachment`). Nothing selects
  them, and the wire contract does not expose them. **This is not a to-do.** A
  subject-scoped read was designed and declined: it would replace an
  authorization that holds by construction — another person's feed cannot be
  expressed — with one that holds because a gate ran, and `auth.EnsureVisible`
  alone is not that gate (it checks no object grant, and for an identity table
  its clause is empty, so it returns success without a query). The one populated
  subject, `attachment`, is not row-scoped at all; its authority is inherited
  from a polymorphic parent. And a single `LIMIT` over a widened predicate lets
  one population evict the other, which is the opposite of what a subject arm is
  for. If it is ever built it copies `ai/feedback.go`, which already spells the
  gate correctly.
- **Per-person `enrich` narration was considered and declined**, not deferred.
  The reason is in the census entry beside the code: the pass mints one
  correlation id for all its candidates, so they share a single occurrence that
  no per-person subject could describe.

[#2272]: https://github.com/margince/margince/issues/2272
[#2276]: https://github.com/margince/margince/issues/2276

## Reference

| Concern | Where |
|---|---|
| The projection + the read | `internal/modules/aiactivity` (owns `ai_task_run`, imports no sibling) |
| Who reports each task | `internal/modules/ai/railowner.go` — `railOwners`, `RailOwner`, `RouterReports` |
| The router's announce/settle pair | `internal/modules/ai/railstart.go`, `railemit.go` |
| The carriers | `internal/modules/agents/runner/activity.go`, `internal/modules/activities/extractionactivity.go` |
| Attribution | `aiactivity/actor.go` — `ResolveActor` |
| The event | `ai_task.state_changed` (`shared/kernel/events/catalog.go`; payload generated into `internalevents_gen.go`) |
| The wire | `AiActivity` / `AiActivityKind` / `AiActivityItem` + `GET /me/ai-activity` (`backend/api/crm.yaml`) |
| What is drawn, and what is not; the named variants; the reason lines | `frontend/src/app/ai-activity-lines.ts` |
| Naming the record a request is about | `principal.WithWorkSubject` (`shared/kernel/principal`), bound in the compose services; stamped by `ai/tracing.go` `newAttemptTrace` and `ai/railstart.go` |
| Which faults flash and which hold | `frontend/src/app/agent-fault.ts` — `ATTENDED` |
| The rail component + poll | `frontend/src/app/agentrail.tsx`, `ai-activity.ts` |
| The census gate | `backend/gates/aiactivitycatalogparity_test.go` |

**Related:** [ai-runtime.md](ai-runtime.md) (the task contract and the Router
gate) · [write-backbone.md](write-backbone.md) (the outbox the facts ride) ·
[job-fleet.md](job-fleet.md) (the scheduled work two carriers report) ·
[frontend-architecture.md](frontend-architecture.md) ·
[how-to/add-an-ai-task.md](../how-to/add-an-ai-task.md).
