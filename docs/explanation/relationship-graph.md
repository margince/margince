<!-- prose:plain -->
# The relationship graph: participants, the interaction edge & deal coverage

Margince answers the question *is this account cold* from the real parties to each conversation. It
does not guess from the last time someone touched a record. The table **`activity_participant`**
records the parties. The table **`graph_interaction_edge`** folds them into one row per (team member,
contact). The
coverage read turns that into named risk findings on a deal, and a user can open each one to see why.

The data comes from two places. Captured mail and calls logged by hand make *participants*; see
[capture-connectors.md](capture-connectors.md) for the capture side. A member's own LinkedIn export
makes *ghosts*, a second tier that never becomes a contact; see
[how-to/import-your-linkedin-network.md](../how-to/import-your-linkedin-network.md). What follows is
about the first: the graph built from recorded contact.

## The question it answers

A sales user about to write into a new account wants one thing before they draft. Does someone here
already know these contacts, and how well? A cold account and a warm one look the same on a company
page (same fields, same logo, same empty pipeline). What makes them different is who on the team has a
real exchange on file.

Two surfaces answer it, and both read the same `graph_interaction_edge` projection:

- `GET /contacts/{id}/network`: *who on our team knows this contact*, warmest first
  (`EdgesForContact`, one contact).
- `GET /deals/{id}/coverage`: *who covers this deal, and what is wrong with how it is covered*
  (`CoverageFor` → `EdgesForContacts`, every stakeholder at once). That is what ranks the team member
  on our side of each relationship.

One test inside coverage does **not** use the projection. Whether a stakeholder counts as *engaged*
is `deals.EngagedStakeholders`, which walks `activity_link` directly. The engaged test is a question
about a deal's own conversations, not a ranking over a contact's history.

The agent surface asks the same questions through the same seams. `who_knows`, `company_coverage`,
`intro_path_to` and `at_risk_relationships` are all 🟢 read tools under `ScopeRead`. They reach
records only through the row-scoped reads the HTTP surface uses. So a governed tool can never see
more than the human who uses it ([agent-surface.md](agent-surface.md)).

## The whole shape

Two write sources fill one participant table; one projection folds it; two surfaces read the fold.

```text
captured mail / calendar          hand-logged call or meeting
 (capture, in the ingest tx)            (activities)
        │                                   │
        ▼                                   ▼
   activity_participant  ── one row per party per role
        │   user_id = ours · contact_id = a contact · address = never became one
        │   (contacts promotes address → contact_id at the link chokepoint)
        ▼
   graph_interaction_edge  ── the PROJECTION: one row per (user, contact)
        │   counts + exact moments; no id, no audit, no event —
        │   throw it away and rebuild at any time
        │   maintained two ways, converging on one fold:
        │     the cg:graph-edge consumer (incremental) · the nightly reconcile
        ▼
   read-time score = recency × frequency × reciprocity  (never stored)
        │
        ├── GET /contacts/{id}/network    who on our team knows this contact
        │      EdgesForContact — one contact
        └── GET /deals/{id}/coverage    who covers this deal, and what's wrong
               EdgesForContacts — every stakeholder at once
               + deals.EngagedStakeholders, which walks activity_link DIRECTLY:
                 "engaged" is about this deal's own conversations
```

## Participants: who is in the conversation

`activity_link` records which **records** an activity is about. It has no user arm, so it cannot say
which of *our* team members is in the conversation. `activity_participant` is a row per party, with
three identity arms that do different jobs:

| Arm | What it means |
|---|---|
| `user_id` | **Our** side: the arm the interaction edge is keyed on. |
| `contact_id` | A known counterparty, already a contact. |
| `address` | A party who is not a record. Kept, not dropped, because an attendee nobody matched is a fact about the meeting. |

A row must name someone (`activity_participant_identity` CHECK), and the set of roles is closed in the
database (`from`, `to`, `cc`, `attendee`, `organizer`). An index allows one row per
`(activity_id, role, user_id, contact_id, address)` and counts the NULL arms as the same value. So the
rule also binds rows with only an address, and every write does nothing, at no cost, on
replay.

### Why capture must write it in the ingest transaction

The mailbox owner is known **only from the connector principal**. `capture_connection` is one per
user per provider, so the registry stamps the human who granted it on the principal that runs the
sync. By the time any other module sees the activity, its `captured_by` reads `connector:gmail`. The
human behind it can no longer be known. That is the agreed reason `capture` writes a table
`activities` owns. `backend/gates/tableownership_test.go` carries it word for word, and a waiver with
no reason, or an out-of-date one, fails the gate.

Capture writes the counterparty as an **address**, not a contact. The tier gate that creates contacts
runs *after* that transaction commits. For a sender on the stop list it never runs at all. Direction
decides the roles and nothing else: our user is `from` on outbound and `to` on inbound. That lets the
fold tell a real exchange from many sends nobody answered.

### Which module writes which arm

Four modules touch the table, and each agreement in the table-owner gate states its own reason:

| Module | What it writes | Why it, and not the owner |
|---|---|---|
| `activities` | **Owner.** The path for calls logged by hand: the human who logged it plus the contacts they linked, with the same roles from direction that capture stamps. | — |
| `capture` | Both arms of a captured message, in the ingest transaction. | The connector principal is the only place the mailbox owner is known. |
| `contacts` | Turns the address arm into a `contact_id` at `linkActivityToContact`. | That is the one chokepoint every `ensure` path reaches, **and** the one that has already checked the contact against a merge. Naming the party is the same write, on the same row, in the same transaction as the link. |
| `privacy` | Erasure deletes rows whose only identity is the subject. It sets the subject's arms to NULL on rows that also name one of our users. | The address arm exists for a party who is not a record. So it lives through the `contact_email` delete and would keep an erased address open to read and to a new match. |

One party gets one row. `nameContactAmongParticipants` **updates** the address row and does not add a
second. It leaves the row alone if a contact row for the same `(activity, role)` already exists. In
that case it is a second address for a party already recorded, not a second party.

### Getting history back

Messages captured before participant rows existed have none. So a workspace with years of mail would
read empty. To the user who looks at it, that looks the same as a feature that does not work.
`BackfillParticipantsBatch` gets these classes back, and class 1 comes before class 2:

- **Class 1**: `captured_by` reads `human:<uuid>`. Exact, with no guessing.
- **Class 2a**: `captured_by` reads `connector:<provider>:<user>`. Exact; every row captured since
  that source label shipped.
- **Class `2b`**: older rows stamped `connector:<provider>` alone. These can be put to a user **only**
  when the workspace has a single connection for that provider. With two, the row stays with no user,
  and is not put on a guess. A wrong edge tells someone to ask a team member who does not know the
  contact.

It carries no cursor. The test is "an activity with no participant rows, from which at least one
participant can be worked out". Every activity it chooses gets a row. So the set left over only
gets smaller. The caller runs it until it returns zero, and a batch that fails half-committed is
part of the next run.

The `participant_backfill` dispatcher runs per workspace every `24h`. One run writes 25 transactions
of 500 activities and then stops. A workspace with nothing left stops at the first empty batch.

**What it does not get back:** reading raw `From`/`To`/attendee headers out of the stored originals. That pass
reads message bodies and needs its own rules to match addresses. When it fails, it puts a meeting on
the wrong user, and nobody sees it.

## The projection: one fold, two paths into it

`graph_interaction_edge` is one row per `(workspace, user, contact)` holding counted facts and exact
times: `last_at`, `last_inbound_at`, `last_outbound_at`, the 90-day counts, and the all-time count.
It is a **projection**. It holds no fact of its own and carries no id, no version, no `audit_log` row
and no `event_outbox` row. It can be deleted and built again at any time. That rebuild *is* the
fix for wrong data, which is why the table can carry no audit history.

**The 0–100 strength is not stored.** It is a pure function of `(row, now)`, worked out at read time
by `relstrength.Compute`. A score that drops as time passes is wrong as soon as the clock moves. To
store it would mean either an old number, or a daily job that writes every row again, with values any
reader can work out.

The rule to keep it right is **work it out again, never add one**. The bus delivers at least once, so
adding one counts twice when a message comes again. Merge, archive and erasure all fix history after
the fact, which adding one cannot say at all. To work a pair out again from the source tables gives the
same answer each time, by design.

Two paths keep it true. They meet on the same statement.

- **Step by step**: the `cg:graph-edge` consumer. `activity.captured` / `.updated` / `.archived` and
  `retention.applied` fold again the pairs the activity's participants point to. Those pairs come from
  the participant rows. So a **relink** also folds again the old pair of the activity, which the event could not have named. `contact.merged` drops the source's edges and folds
  the contact that is kept. `contact.archived` / `.restored` / `.updated` / `.created` fold that
  contact again.

  `user.deactivated` does nothing at all. Reads filter through the live-member join, so a user who
  leaves drops out, and no row is written again.
- **Daily reconcile**: the `graph_edge_reconcile` dispatcher (`24h`) runs `graph_edge_workspace` per
  workspace. It empties and fills the whole projection in **one transaction**, so a reader never sees
  an empty graph. It runs daily for a reason no event can give: the 90-day window counts go out of
  date by time alone. The migration states that limit. A count may hold up to `24h` too much, while
  recency, which counts most in the score, is exact.

A fixture keeps the two paths in line: a rebuild and a stream of step-by-step updates over the same
history must agree.

To delete counts as much as to write. Say the last interaction that counts for a pair is
archived: the pair's row is deleted. An edge that lives longer than its evidence would point to a team
member for an introduction they can no longer make.

Counting is over **activities, each once**, never over join rows. One message makes a participant row
per party per role. So a contact who is both a `to` and a `cc` would else count that single message
twice. That would count frequency, and so the score, too much on the busy threads a relationship score
should read.

## Every participant role makes an edge, cc included

The set of roles that count is **every role there is**:

```go
const interactionRoles = `('from','to','cc','attendee','organizer')`
```

Most CRM products leave out `cc`: being copied, they say, is not a relationship. Margince counts it.
In the accounts this product serves, the team member always in copy is most of the time the one who
knows the customer. Think of the account lead copied on their team's mail, or the partner copied on
every exchange. To drop `cc` would remove those team members from the answer to "who here knows
them".

The real test sits in the **reciprocity part**, not in a role filter. A team member who is only
copied has messages in one direction, so reciprocity puts them well below someone in a real exchange.
They show up, ranked at their real level, and do not drop out.

**What does not make an edge**, and why:

- **Anything outside `email`, `call`, `meeting`** (`relstrength.IsInteractionKind`, written into SQL
  from the same list so the writers cannot go out of line). A task is a *plan*, and a note is a
  record of *thinking*. No such record means a real exchange happened. To count them would let a user's own
  to-do list score as a relationship. If the two paths used different lists, a captured note would
  count as an interaction while the same note logged by hand would not.
- **An archived activity.** The fold joins `activity … AND a.archived_at IS NULL`, and the prune step
  removes a pair that has lost all its evidence.
- **A seat on a deal.** A stakeholder row is a statement about *who should take part*. Only a recorded
  interaction is evidence of *contact*. That is why a deal can carry 5 seats and still be
  single-threaded.
- **A note with no link.** The path for activities logged by hand writes nothing for an activity with no
  contact link. Such a note is for the whole workspace, not a conversation with a contact.

## Warm is per user, and `none` is not zero

How warm a contact is on these surfaces is the **relationship strength per user**. It is the same
recency × frequency × reciprocity formula as the contact's score for the whole workspace, over only
the interactions *that team member* is in. The contract states what follows: the two **cannot be
added together and are never merged**.

A contact can be warm to the company while the team member beside them has one short exchange with them.
That gap *is* the answer to "who should make the introduction".

**The reader is ranked, never offered as introduction.** Nothing in the route read leaves out the user who
reads. On a contact they write to on their own, the warmest way in *is* them. That is a fact the surface
should report, and never an introduction to ask for. Both writers refuse it
(`introductions.Store.Create` and the account draft in `compose/company360`, each with
`ErrInvalidArgument`). Both surfaces call the reader "you", and do not show the name of the reader back
to them as a third party.

The words on screen are not the same either. The bands per user are `none / weak / moderate / strong`,
and the bands on the card for the whole workspace are `dormant / weak / warm / strong`. So nobody is
asked to set two such numbers side by side.

**A `none` band carries no number at all**: no strength and no 90-day count. *"We have no history with
them"* and *"we have history, and it is cold now"* are different facts about an account. A zero
shows them the same way. The API type lets `strength` be left out and leaves it out, and the card
leaves out the count to match.

Ranking cannot move into SQL. The score is a function of `(row, now)`, so the database would have to
write the formula again. The `relstrength` package exists to keep it single.

`EdgesForContact` returns **last-contact** order. A caller who promises "warmest first" fetches
more (100), calls `SortByStrength`, and only then cuts the list (10). To cut in SQL would cut by
recency. A one-line reply would then push out the team member who has worked the account for a year.

Two gates guard every edge read, and both are needed:

- **The contact gate.** `auth.Require("contact", read)` plus `auth.EnsureVisibleLive`. `EnsureVisible`
  alone returns early, without checking, for a caller with no limit. Either gap would let a known id
  return a contact's team members and interaction counts after the record itself can no longer be
  read.
- **The live-member join.** `app_user.status = 'active' AND archived_at IS NULL`. Both parts count.
  Deactivation sets `status` and leaves `archived_at` NULL. So a filter on `archived_at` alone keeps
  offering a team member who left as a way in. The surface exists to name someone who can *act*.

## Deal coverage and its risk rules

`CoverageFor` reads inside **one transaction at one point in time**. Say the stakeholder list and the
engaged test come from different snapshots. Then a view could report a deal as single-threaded while
it lists three engaged contacts. It then folds the facts it read with a pure function, against a
clock that is injected.

**Engaged means a real exchange both ways**, not a seat on a list. It needs both an inbound *and* an
outbound interaction that counts, inside a 90-day window (`deals.EngagementWindowDays`). A target we
only send to, who never writes back, is not engaged, for any number of messages we send. (The engaged test
walks the linked activities of the deal's stakeholders directly; it does not read the interaction
projection.)

Coverage answers this with `deals.EngagedStakeholders`, and the deal-health composite
(`healthActivityEvidence` in `deals/health.go`) calls the same function. One rule serves both screens,
so they agree about a deal.

### The coverage view needs the edge grant, and says when it does not get it

Every seat on a deal is a `deal_stakeholder` **edge**. So to read one needs `relationship:read` on top
of the deal grant. To know a deal does not give the right to learn who is on it. `CoverageFor` checks
that grant **first, before any statement**. A caller it refuses gets a payload that names
`stakeholders`, `our_side` and `risks` in `sections_omitted`, not a 403 or an empty risk list.

All three sections go together: all three are shown, or none is. `our_side` comes from the seats, and
every risk rule but `going_cold` reads them. A named section is **empty, never half full**.
`going_cold` needs no edge and could have stayed. But think of a findings list with one item, under a name
that says the list is held back. It leaves a client no way to say whether the list is complete.

That channel is why the gate could be added at all. Without it, a restricted caller sees an empty
`risks` list, which every surface shows as *"Nothing flagged — this deal passes every coverage
check"*. That would be a **wrong verdict on deal risk**, which does more harm than the pair it stopped
showing. The same duty reaches the agent surface. `company_coverage` adds a `section_withheld`
notice, and the at-risk sweep sets `coverage_withheld` on a report whose gaps would else read as deals
with no risk.

The health composite has no such channel and needs none. Its engaged part is a count of edges against
a target number. So a refused caller gets **no score**, not a smaller one.

Every rule is a **pipeline** rule: a coverage view whose deal is not `open` folds to no findings at
all. To tell a user that business they already closed is single-threaded would make them stop reading
the flag.

| Kind | The rule, as coded |
|---|---|
| `single_threaded_theirs` | Under two engaged contacts (`reportThreadingFloor = 2`). Their side: one contact stands for the customer. |
| `single_threaded_ours` | One team member holds ≥ `ourSideDominanceShare` (0.8) of at least `ourSideMinInteractions` (5) 90-day interactions. A separate rule from `single_threaded_theirs`, with its own kind. |
| `coverage_gap` | Seats on the deal, but no *engaged* champion. Not the same as single-threading: three engaged contacts and no champion in that group is a deal nobody inside is pushing for. |
| `champion_left` | The built-in `champion` seat has left the account. |
| `stakeholder_left` | Another seat has left. Two kinds, not one, because they are different sentences to a user. To fold them into one would handle a small case and a serious one the same way. |
| `going_cold` | No captured touch for `goingColdDays` (30) days, with the real day count beside it. One limit ships. The 60-day view is the same finding filtered on `days_since_touch`, and a second kind would let a deal at 61 days show on one surface and not the other. |

The floor of 5 on `single_threaded_ours` counts as much as the share. Without it, say one team member
sent the only two messages in a deal's whole history. That deal would flag as one-sided, when it is
only new.

The coverage read refuses to guess at two facts.

- **Leaving needs evidence.** A contact counts as having left only when *both* parts hold. There is a
  job at this account with an end date that has **passed**, and no live job there now. An **archived**
  job is not evidence of leaving. To archive takes back a statement (someone recorded the job by
  error), while an end date records a real fact.

  Say a team member fixes a typing error, and the product reports that someone left their job. That
  wrong flag makes a user stop reading flags. "Still has a job there" is written the same way here and
  in the two places that ask whether someone has left.
- **A zero `LastTouchAt` means "do not judge".** The gap between *we have not looked* and *nobody has
  any history* is the whole finding. To read the first as the second would flag every deal in a
  fixture that never set one. On a real read, the last touch goes back to the day someone created
  the deal. A deal nobody has touched counts from the day someone wrote it down.

## Every risk carries the ids behind it

A flag a human cannot open to see why gives them nothing to act on. Each finding carries `contact_ids`
and `user_ids` as **ids, not names**: the stakeholder who is not engaged, and the team member who
carries the thread. So the caller shows them under its own row scope.

Only `going_cold` carries `days_since_touch`. To send a zero on the others would read as "touched
today". That is wrong on a finding about someone leaving, which says nothing about recency at all.

The server also owns the **words**. `summary` is the rule's own explanation, so the same flag reads
the same on the deal card and in the AI chat. The client does not change the order or the words.
Either would be a second copy of the score formula or the rule text, and the two would disagree as
soon as either changed.

## Privacy: dropped in the transaction, not by a consumer

The graph structures exist to hold a party who *never became a record*: that is what the address arm
of a participant row and a LinkedIn ghost both are. A contact-keyed sweep alone leaves the subject
named, reachable and re-matchable. So every clause in `privacy/erasure_graph.go` reaches the subject
by **identifier** as well as by contact id, inside the same Art. 17 transaction as the rest of the
cascade:

- **Participants**: delete rows whose only identity is the subject (a participant row must name
  somebody, so it cannot be blanked). Null the subject's arms on rows that also name one of our
  users, because the colleague was in that conversation and that is not the subject's data to erase.
- **Ghosts**: delete on *suggestion-grade* evidence as well as a confirmed match. Matching errs toward
  caution because a wrong link attaches a stranger to a customer record. Deletion errs the other way:
  deleting one ghost too many costs a re-import of a file the colleague still has, while keeping one
  too few leaves a named individual's data behind after we certified it destroyed.
- **Edges**: `DELETE FROM graph_interaction_edge WHERE contact_id = $1`, **here** rather than in the
  `cg:graph-edge` consumer.

Erasure deletes edges in its own transaction, because an Art. 17 obligation discharged by an event
fails silently when the bus is behind. The ownership gate records this as the ratification for
`privacy` writing `search`'s table. The projection holds who corresponded with the subject, how often
and how recently.

Two other rules follow the same shape:

- **Deactivation** deletes the leaving member's imported LinkedIn network in the single deactivation
  transaction. It is all one transaction with the revoke of their sessions and passports. The rows are
  deleted, not turned into a tombstone, because a tombstone still holds the names.
- **Retention** uses the *one* fold again and does not write a second statement. The retention sweep
  archives and erases under `retention.applied`, which the `cg:graph-edge` consumer handles by name. A
  separate delete statement would copy the formula. It would also leave a pair that lives on with old
  counts, each time the activity is not its last evidence.

Both tables are reached only through `database.WithWorkspaceTx`, as every other module statement is.
See [authorization.md](authorization.md) and [privacy-and-consent.md](privacy-and-consent.md).

## Limits we know of

- **The 90-day counts may be out of date**, within a limit the contract sets: up to `24h` too much
  between daily reconcile runs. The migration states it; recency is exact and counts most in the score.
- **Calendar attendees from history are not filled in.** The history pass gets back the mailbox owner
  and the linked counterparty. Reading attendees out of stored originals is separate work.
- **`CompanyLinkedInReach` is wired to nothing.** The ghost count per team member, per account,
  exists in `contacts`, and tests run it. But no HTTP surface, agent tool or screen reads it today.
  The shipped answer at account level is the member's own
  [`/me/linkedin-reach`](../how-to/import-your-linkedin-network.md).

## Short rules

- **Work it out again, never add one.** The bus delivers at least once, and history is fixed after the
  fact.
- **The projection holds no fact of its own.** To delete it and build it again is always safe,
  and is the fix for wrong data.
- **The score is worked out at read time**, never stored, because a number that drops over time is
  wrong as soon as the clock moves.
- **Every role makes an edge, cc included.** The real test is reciprocity, not a role filter.
- **Anything that returns a record is a read**, and carries the contact gate plus `EnsureVisibleLive`.
- **The live-member join hides members who left.** No row is written again.
- **Erasure runs in its own transaction**, never through an event.

## Where the code lives

| | |
|---|---|
| Participant rows for captured mail (both arms, ingest transaction) | `internal/modules/capture/participant.go` |
| Participant rows for a call or meeting logged by hand | `internal/modules/activities/participantlog.go` |
| Address → contact at the link chokepoint | `internal/modules/contacts/participant.go` |
| Getting participant history back (class 1 / `2a` / `2b`) | `internal/modules/activities/participantbackfill.go`, job in `internal/compose/participantbackfilljob.go` |
| The interaction projection: fold, prune, rebuild, reads | `internal/modules/search/graphedge.go` |
| The `cg:graph-edge` consumer + the set of events it acts on | `internal/modules/search/graphedgegen.go` |
| The score (recency × frequency × reciprocity, bands, the 90-day window) | `internal/shared/kernel/relstrength/` |
| Coverage read (deal facts, who has left) and the pure risk fold | `internal/compose/network/coveragefacts.go`, `risk.go` |
| The network/coverage HTTP surface | `internal/compose/network/handlers.go` |
| Engaged stakeholders, used by coverage and deal health | `internal/modules/deals/engagement.go` |
| Agent-tool seams (`who_knows`, `company_coverage`, `intro_path_to`, `at_risk_relationships`) | `internal/compose/networkseams.go`, `introseams.go`; `internal/modules/agents/tools_network.go` |
| Erasure of participants, ghosts and edges (one transaction) | `internal/modules/privacy/erasure_graph.go`, `retention_graph.go` |
| Deactivation deleting a leaving member's network | `internal/modules/identity/users.go` |
| Cross-store agreements for every table above | `backend/gates/tableownership_test.go` |
| The tables | `backend/migrations/core/0001_baseline.up.sql` (`activity_participant`, `graph_interaction_edge`, `linkedin_connection`, `linkedin_account`) |
| The REST contract | `backend/api/crm.yaml` (`getContactNetwork`, `getDealCoverage`) |
| The job contract (schedule, runs per workspace, batch sizes) | `backend/api/jobs.yaml` (`graph_edge_reconcile`, `participant_backfill`, `linkedin_rematch`) |
| The contact's **Network** tab | `frontend/src/screens/contactnetwork/`, reading `getContactGraph` through `frontend/src/screens/contactgraph.tsx`; `getContactNetwork` has no screen of its own |

## Where to go next

- How to import a personal network as the second evidence tier beside this one, with a clear label:
  [how-to/import-your-linkedin-network.md](../how-to/import-your-linkedin-network.md).
- Where the participant rows come from (the connector seam, the one Sink, the three capture modes):
  [capture-connectors.md](capture-connectors.md).
- The write shape every change to a source table commits through, and the outbox both consumers read from:
  [write-backbone.md](write-backbone.md).
- Why the erasure writes tables it does not own, and how each write is agreed:
  [privacy-and-consent.md](privacy-and-consent.md).
- Where the cross-module edges above are injected: [composition-layer.md](composition-layer.md).
- What every module owns, including these tables: [reference/modules.md](../reference/modules.md).
