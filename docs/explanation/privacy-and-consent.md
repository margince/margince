# Privacy, consent & the GDPR engines

How Margince meets data-subject obligations: the **authorization engine** that decides, and
records, whether each outbound message may go, and the **privacy engines** (erasure,
subject-access, retention) that a fulfilled request executes. The product refuses scraping-based
enrichment in the first place; the legal position behind that, for the EU and Vietnam, is a
whitepaper kept with the company's business material rather than in this tree. Two modules
cooperate: `consent` owns the engine and the case queue, and `privacy` owns the machinery. They are
connected at the composition root, never by a sibling import.

## The authorization engine (`consent`)

`consent` owns two things that are easy to confuse. **Consent** is a subject's answer to a question
about a purpose: the catalog, each contact's current state, and an **append-only proof log**.
**Authorization** is whether one particular message may go. That is a different question and usually
has a different answer, because most legitimate mail is sent on a contract, a reply the subject
started, or a legal duty rather than on consent.

The engine answers the second. It resolves a **category** from what the send actually is, checks the
**evidence** that category requires, and records a per-recipient **decision** saying why:

- **The server resolves the category.** The caller does not name it. A closed vocabulary
  (`reply_to_inbound`, `invoice_or_payment`, `marketing`, `security_notice`, … in
  `internal/shared/ports/commsauthz`) is derived from the message's own origin: its anchor thread,
  the records it links, the template it uses. A caller can propose a category but cannot invent one.
  The send doors refuse a claim to any of the five **subject-serving** categories (a security or
  privacy notice, an opt-out, consent or record confirmation), so only the installation itself
  writes to somebody on its own behalf.
- **The basis is evidence.** A reply is authorized by the thread the subject opened, and only for
  this recipient on that thread; being a reply is not enough. An invoice is authorized by a live
  invoice reaching this recipient through a current employment relationship. That is a real bar:
  a finance contact nobody linked to the customer record gets `review` rather than a refusal.
  Marketing is the case that needs consent, and it stays purpose-specific.
- **The decision is recorded twice, before sending.** It is taken once as the message is
  staged and once immediately before the provider is handed anything, into
  `communication_decision`, one row per distinct recipient. The row says the category, the verdict,
  the reason, the basis and a fingerprint of the wording, so "why did this message go" is a query
  rather than a reconstruction. (The evidence itself lands in `communication_basis`, not on the
  decision row.) A message that reaches a provider always has both rows; the second catches a
  withdrawal, a bounce or an edit to the wording landing between the two.
- **Withdrawals and suppressions are separate, and both bind.** Unsubscribing withdraws consent,
  which the engine reads **by class** ("did they stop this kind of message"), because a category
  resolved from evidence may carry no purpose key to match. `communication_suppression` records the
  other stops: an Art. 21 objection, a statutory restriction, a subject's request to stop, a hard
  bounce. Neither expires on its own, and no rollout mode softens either.
- **A rep may vouch for a refused send.** The vouch stands apart from consent.
  `communication_override` (`consent.Allow`) records a standing, per-category statement that a
  machine-level refusal for lack of evidence may be overruled for one contact. It covers the
  category the engine resolved the send to and only that one, so a vouch for `marketing` says
  nothing about `customer_service`. It flips nothing absolute. `Decision.CanBeOverruledByCategory`
  asks the question only for a non-absolute machine reading that resolved a category. The
  `absoluteDenials` below and any subject-decided refusal stay out of reach whoever is vouching, so
  a subject stop still wins. An `unknown_purpose` refusal is out of reach too, for a different
  reason. It is non-absolute, but the request named a purpose key the engine does not know, so no
  category resolved and a per-category vouch has nothing to answer. Resending with a recognised
  purpose is the remedy there. The reason is required: a suppression may relay a bare phone call,
  but an override is the rep's own judgement call and the record must say why. Only a caller whose
  authority level `CanRevoke` the level it was recorded at may revoke it (`consent.RevokeOverride`).
  That is `CanOverrule` plus one square, because admin is the top human authority and an
  admin-recorded vouch would otherwise have no seat able to take it back. `lift.go` keeps the
  stricter `CanOverrule` for a stop, where erring toward not sending is the safe direction. A vouch
  survives a merge onto the surviving contact (`consent.CarryOverridesTx`) with its original
  `decided_by_level` and reason intact, so a merge cannot launder it down to a lower authority.
  Revoking one therefore reaches every copy a merge made of it, on the pre-merge id the caller was
  given. Each copy is announced lifted on the stream that heard it recorded, because a vouch left
  standing under an id its author never saw goes on allowing the send. Art. 17 erasure and the
  retention sweep delete it outright with the rest of the contact's consent record: a standing
  "write to them anyway" must not outlive its contact. Art. 15 subject access exports it
  (`privacy.AssembleSAR`'s `communication_overrides`). A subject asking what is held about them is
  owed the record that a human decided to write to them, and why.
- **A restriction does not block everything.** Three categories still reach a restricted subject
  through a registered template: `security_notice`, `privacy_notice` and `optout_confirmation`.
  A subject is not better off for being unable to hear that their account was breached or that
  their opt-out was recorded. A hard bounce stops even those, because no template makes a dead
  address deliverable.
- **Every category ships enforcing, even when omitted.**
  `consent.authorization_modes` can move one to `observe` or `warn`, which records the engine's
  answer without binding. That is an operator's rollback lever; the shipped posture is enforcing.
  A category the stored map does not mention enforces. A non-empty map that omits any category is
  refused at the door, naming the ones it missed, so a half-written setting cannot leave mail
  unchecked. An empty map is accepted and every category enforces, the same answer as storing
  nothing at all. The older purpose-key gate decides only where **no** recipient's category is
  enforced, so flipping one category changes less than it looks. Some reason codes are absolute
  (`absoluteDenials` in `commsauthz`) and deny in every mode whatever the setting says. They are
  the four above, an unconfirmed double opt-in, a recipient that resolves to no single subject, a
  consent withdrawal, a jurisdiction's advertising frequency cap, and a request whose claimed
  category contradicts the one the engine resolved.

Marketing consent rests on a round trip. A double-opt-in purpose needs a confirmed `consent_event`,
completed **only by the data subject**, by spending a single-use link mailed to their own live
primary address. There is no operator-held token, because a token an operator can read and hand
back proves nothing about the mailbox it was supposed to reach.

A refusal names only the address and discloses nothing new. The engine is spelled once
(`consent.NewGate`) and **injected into the send path** (activities) at the composition root, so
consent never becomes an import edge between siblings. Every consent *state* write also appends a
proof row (Art. 7(1) demonstrability), and a fitness test (`consentproof_test.go`) fails any state
write that skips its proof.

## What a refusal leaves behind

A refusal leaves two records behind, and they answer different questions.

**A review is the refused message, still resumable.** `communication_review`
holds one send attempt that stopped. Where the attempt had a mail payload and the hold
succeeded, it is bound to the held `scheduled_send` that froze it, and resuming
works from there. Two cases leave a review with nothing to resume. One is a refused
channel reply, because `scheduled_send` carries mail. The other is a hold that itself
failed; that failure is swallowed, because the rep is owed the refusal they can act
on rather than an operator's problem they cannot.

The refusal keeps the status and code it already had (a consent refusal stays
`409 consent_not_granted`) and gains the review id beside them. Every client and
test already recognises the status; the id adds a handle a machine can act on, so
an agent given a refusal can hand the question to a human instead of only
reporting a sentence. States name what is needed rather than who is blocked.
`needs_context` is a fact about the message and stays true whoever is looking at
it, where "waiting for Anna" stops being true when Anna leaves:

| State | What it means |
|---|---|
| `needs_context` | More evidence could answer this. The engine found no ground for the message to stand on. |
| `needs_repair` | No evidence can answer it. Something about the message or the address is wrong. |
| `awaiting_decision` | Handed to somebody who may override it. No longer the sender's work. |
| `resolved` | Somebody finished it. |
| `superseded` | Replaced by a fresher attempt at the same message. |
| `cancelled` | Nobody intends to send it. |

Resuming reuses the held intent, so a resumed review stages one delivery and not
a second copy of the message. A review is readable by the user who initiated it
or a user holding `communication_exception:read`. An unrelated user gets 404
rather than 403, because the existence of a refused message about a named
subject is itself a disclosure. A non-human principal is refused before the
query runs.

**An instruction overrides one refusal, by name.** It is a named user sending one refused message anyway.
Sometimes the installation has a ground the engine cannot see: a contract
clause, a legal obligation, a subject who asked in a room nobody logged.
`communication_instruction` records that decision.

An instruction is never recorded as a consent grant. The refusal stays where it
is and the instruction sits beside it, naming who overrode it and the reason they
gave. A subject asking later why they received a message must be shown the
refusal and the decision; a grant nobody made would misstate what happened.
The refused recipient's decision row keeps the verdict the engine gave it
(`deny`, or `review` where the refusal was a machine reading) and carries
`execution_authority='instruction'` beside it. So a query can tell a message
that was never refused from one that was refused and sent anyway.

**Known gap: the cap misses directed recipients.**
`advertisingMessagesReceived` counts transmit decisions with
`verdict='allow'`, and the row for a recipient who was directed through is not
one. An allowed recipient sharing the same envelope still counts, so a mixed
send is counted for some of its recipients and not others. An installation under
a jurisdiction's ceiling can therefore exceed it through exceptional sends
without the count moving. The column the query would need is already on the row;
nothing reads it yet.

Consuming an instruction checks four things. Each is a different way the record
could end up describing something that did not happen:

- **The decision is still live.** Revoked, expired and already spent are three
  different reasons the same row authorizes nothing now.
- **It is this message.** An instruction is given against one review, and that
  review holds one message.
- **The message has not changed**, where there is a fingerprint to compare. The
  sender acknowledged a warning about a specific subject and body, and an edit
  after that is a message nobody signed for. An instruction with no fingerprint,
  or one against a review whose held message could not be read, is not parked,
  because parking it would refuse a send for a reason nobody can act on.
- **It is spent once.** The row moves to `consumed` and names its delivery inside
  the same transaction that stages the message, so a retry, a double click or a
  redelivered job cannot spend it twice.

Directing a send answers to `communication_exception`, its own RBAC object
rather than a corner of consent's settings. The two are different authorities:
consent's settings decide who may change the rules, and this decides who may act
against the answer those rules produced about one contact. An installation that
delegated the first has not thereby delegated the second. The check is
`auth.RequireHuman` as well as the grant, so a passport inheriting an admin's
grants cannot mint one.

## Reaching the subject without the message that carried the link

Three records exist so a subject's own acts do not depend on a mailbox, a
session or a token that has since rotated.

- **A withdrawal credential outlives its mail.**
  `withdrawal_credential` is separate from `preference_token` because the two
  authorize different things: reading and editing a preference profile rotates,
  and withdrawing must survive that rotation. An old link and an RFC 8058
  one-click POST still withdraw after the read token has rotated, with no
  session and no expanded read authority. The credential itself lives 24 months.
  One that outlived every copy of the message it rode on would protect nobody:
  there would be no link left for a subject to press, only a working credential
  for whoever finds one.

  Leads get a withdrawal credential too, which is what gives a lead-only
  recipient an opt-out. A lead pressing the broad all-marketing scope records an
  objection to every marketing message. A lead pressing a named-purpose
  credential records a stop narrowed to that one purpose, on
  `communication_suppression.purpose_id`. The engine binds it only to a
  marketing send that resolves to the same purpose. It leaves the subject's other
  marketing purposes running, which is what "unsubscribe from this list" means.
  Like a broad objection, it leaves the other categories alone: an objection of
  either width says nothing about an invoice. A marketing message is only ever
  allowed once it has resolved through a purpose, so there is no unscoped
  marketing send for a narrow stop to miss.
- **A stop records its author and reach.** An Art. 21 objection to
  direct marketing and a request to stop contact entirely are different legal
  acts with different reach. The objection binds marketing alone; the request
  binds everything but the three categories above.
  `communication_suppression` records which kind, its `source`, the seat that
  captured it, and the authority level it was decided at, and `lift.go` refuses
  any lift the lifter's own level cannot overrule. The two kinds differ there. A
  relayed **marketing objection** is stamped `decided_by_level='subject'`,
  because Art. 21 is the subject's own act and no seat should be able to undo
  it. A relayed **subject request** keeps the seat's level: it is a colleague's
  report of a conversation with no article behind it, so an admin correcting a
  misheard "stop everything" does not need the subject back on the phone.
- **A public correction or erasure opens a case.** A subject
  typing into a confirm link opens a `data_subject_request` in the same
  transaction, with a receipt reference and a deadline, rather than leaving a
  row nobody works.

## What the engine reports about itself

**A counter shows what the engine decides now.**
`margince_communication_authz_decisions_total` counts transmit decisions since
process start, labelled by verdict, category and mode. The labels are those
three and nothing else. The recipient is not a field of the count and must never
become one, because a metrics endpoint is the last place a subject's address
should appear.

**A report shows engine and gate disagreement.**
`DisagreementReportSince` reads that per category from the verdicts already
recorded on every transmit row, swept daily so nobody has to remember to fetch
it. It decides nothing and writes no domain row. Enforcement is a setting a
human changes after reading the report; a pass that flipped a category itself
would be a second authority over what may be sent. Since every category ships
enforcing, the report is how somebody notices a category refusing mail the old
gate would have allowed.

## The privacy engines (`privacy`)

`privacy` owns the GDPR machinery a fulfilled request runs. The DSR **case queue** lives in `consent`
(the `data_subject_request` rows and their HTTP surface); the composition root injects privacy's
engines into consent's handlers.

- **Art. 17 erasure** (`Eraser.EraseContact`) anonymizes the normalized rows in place and purges raw
  capture, embeddings and attachment bytes. It hashes the identifiers onto a **suppression list** so
  re-capture can't resurrect the subject, and proves it with a **PII-free audit tombstone**, all in
  **one transaction per record**. Atomicity *is* the guarantee. It refuses a subject under `legal_hold`.
- **Art. 15 subject access** (`AssembleSAR`) is one *privileged* read (it needs the `contact.delete`
  grant **and** an unbounded row scope). It gathers everything held about a contact into one export
  package: channels, deals, leads, activities, attachments, consent and its proof log, raw capture,
  field origins. The export is itself audited (`action=export`).
- **The nightly retention evaluator** (`RetentionService.EvaluateInstallation`) runs as one River
  job per workspace off the `privacy_retention` dispatcher in `cmd/worker`, by default every 24h.
  It evaluates **one** workspace's enabled policies and applies the policy's single action to
  over-age records, **one audited transaction per record**. A tenant whose pass fails fails its own
  job row. `legal_hold` rows are never auto-acted, and an activity is held transitively when any
  linked contact/company/deal is held. A policy whose scope the engine doesn't understand is
  **skipped loudly**, never half-applied.

## The single-transaction cross-store exception

`privacy` owns one table (`erasure_suppression`), yet erasure and retention
**write tables they do not own**: `contact`, `contact_email`/`_phone`/`_social`,
`contact_channel_identity`, `lead`, `activity`, `activity_participant`, `graph_interaction_edge`,
`linkedin_connection`, `comms_outbound`, `deal`, `attachment`, `embedding`, `raw_capture`,
`field_provenance`, `preference_token`, `capture_pending_counterparty`, `voice_learning_signal`,
`ai_call` and `ai_call_payload`. The ratified list is the cross-writer map in
`backend/gates/tableownership_test.go`, which gates it; that file, not this page, is the authority.

Four of those show *why* nothing else can reach them. A **channel identity** is the key an inbound
message would re-bind the subject by, so it must be deleted in the same commit that hashes it onto
the suppression list. A **LinkedIn ghost** holds the subject's name, employer and address, imported
from a colleague's export without the subject ever being asked. It is invisible to every
contact-keyed clause because a ghost is not a contact row. The **interaction edge** would otherwise
be left to a bus consumer, and an Art. 17 obligation discharged by an event fails silently when the
bus is behind. A participant's **address arm** exists for a party who never became a record, so it
survives the `contact_email` purge and would keep the erased address re-matchable.

A data-subject obligation must reach **every** store that holds the subject in that one transaction
per record; routing each purge through the owning module would give up the atomicity.

Privacy's purges are one of the ratified cross-writer exceptions to "a module writes only its own
tables." Every such write is **ratified per table** in `backend/gates/tableownership_test.go` with a
self-contained rationale; a reasonless or stale waiver fails the test. See
[reference/modules.md](../reference/modules.md) for the ownership map and
[write-backbone.md](write-backbone.md) for the write shape these purges still follow.

## What an erasure reaches in the AI telemetry, and what it does not

With payload capture enabled (`ai.capture_payloads`, opt-in), `ai_call_payload` holds the request and
response of every model call. For a reading of a meeting transcript that request **is** the transcript,
which makes it the largest copy of somebody's words this product holds.

An erasure reaches that table two ways, and they cover different calls.

**By citation.** A call that said which record it was about carries that record on `ai_call`
(`subject_type`, `subject_id`). The erasure deletes the payloads of every call that named the
subject, an activity of theirs, or a lead wiped with them. This is the path that reaches a transcript:
a transcript names its speakers rather than addressing them, so it can hold a whole conversation
without spelling one address.

**By content.** Any payload whose text names one of the subject's addresses is deleted. The match is
crude by design: over-deleting captured telemetry is recoverable, under-deleting personal data is not.

**The citation covers only calls carrying one.** A call whose input spans several records names
none, because a list that is right half the time is a citation nobody can trust for a purge. Those
calls are reached by the content match or not at all, and their guaranteed end is the
`ai_call_payload` retention window an installation configures. `backend/gates/aicallsubject_test.go`
is the census that keeps the covered half complete: every model call in the tree either names its
subject, says why it is about no single record, or is listed as owing one.

## Jurisdiction retention floors

A destructive retention action must not violate a statutory floor. Country packs are extension units
(`extensions/de`), composed in at build time through the `ports/jurisdiction` seam; see
[extensibility.md](extensibility.md). Core code never names a jurisdiction. The **`de`** (German)
pack declares GoBD retention classes. The retention evaluator takes the strictest compiled-in
**commercial-correspondence** floor and shields external business correspondence (a *Handelsbrief*)
from destruction below it. An internal note or task is not correspondence and carries no floor. A
fitness test pins that boundary (a 400-day email survives; a same-age note is erased).

**What makes correspondence a *Handelsbrief*.** Either of two things, each recorded on the
record itself the moment it happens rather than re-derived later:

- **A deal it is filed under concludes**: won, or carrying an offer past draft.
- **It is filed under a project.** A project is a commercial engagement from the moment it exists, so
  its correspondence documents an actual transaction whether or not a deal on it has closed. This
  reaches mail from a negotiation that was lost and from delivery work years after the deal that
  started it, both of which the deal rule alone misses.

**Only Undo filing removes the mark.** Relinking an activity away from the
project, archiving the project, or closing it all leave the classification standing. The evidence
behind it is frozen too: the project's name is copied at the moment it qualifies, so a later rename
does not rewrite what the record says. The one way out is **Undo filing**
(`POST /activities/{id}/project-filing/undo`). It is a human-only decision by a named user holding
`activity.update`, with a written reason:

- It removes the activity from its project and withdraws the class together with the project filing's
  evidence. One transaction carries that change, the audit entry (the reason and the decider's name)
  and an `activity.updated` event carrying `project_filing_undone`.
- It is allowed only when the project filing is the **sole** basis. A won deal, a sent offer, a
  controller's pin or a deal link that still qualifies keeps the class (`409 other_basis_remains` /
  `qualifying_deal`). An activity a statutory hold has already restricted never loses it
  (`409 restricted`). A legal hold on any record it is linked to, the project included, outranks the
  undo (`409 legal_hold`). A project the user cannot see still holds the activity
  (`409 hidden_project`); the read shows such a project unnamed and a decision about it as a bare
  moment. `GET /activities/{id}/project-filing` answers the same judgement, plus the decisions already
  taken, so the screen and the write cannot disagree.
- The database enforces the same rule underneath. The class may clear only inside a transaction that
  declares the undo for that one activity, from an unrestricted row with no evidence and no project
  link left. The declaration may delete project-filing evidence and nothing else. Every other change
  to the class or its timestamp is still refused.
- An agent never decides it, even holding an administrator's passport. It can stage the relink that
  files an activity under a project, and release it on an attended call, but only while the undo
  could still take the filing back. A member releases the relink in the CRM when the activity is
  restricted, held through a link or covered by an open erasure request, because that filing would be
  permanent.

Over-retention is an argument to have with a supervisory authority, while destruction cannot be
reversed. So the undo is narrow, and the data layer refuses what the writer refuses.

## Where the code lives

| | |
|---|---|
| The authorization engine | `internal/modules/consent/authorize*.go` (`AuthorizeStagingTx`, `AuthorizeTransmit`) |
| The shared vocabulary | `internal/shared/ports/commsauthz/` (category, basis, phase, verdict, mode) |
| Per-recipient decisions | `communication_decision`, `communication_basis`, `communication_suppression` |
| Standing rep overrides | `internal/modules/consent/override.go` and `internal/modules/consent/overridecarry.go` (`Allow`, `RevokeOverride`, `CarryOverridesTx`, `communication_override`), with `internal/modules/consent/overridechain.go` holding the walk a revoke takes across the copies a merge made and the lock that keeps a merge from outrunning it; the merge reaches the carry through `internal/modules/contacts/overridecarry.go`, which owns the port and not the table |
| Consent state + proof log | `internal/modules/consent/` (`consent_purpose`, `contact_consent`, `consent_event`) |
| Art. 17 erasure | `internal/modules/privacy/eraser.go` (`NewEraser`, `EraseContact`) |
| Art. 15 SAR | `internal/modules/privacy/sar.go` (`AssembleSAR`) |
| Retention evaluator | `internal/modules/privacy/retention.go` (`RetentionService.EvaluateInstallation`), fanned out per workspace by `internal/compose/jobs_privacyretention.go` in `cmd/worker` |
| Refused sends and who directed one | `internal/modules/consent/review.go`, `instruction.go`, `instructionconsume.go` (`communication_review`, `communication_instruction`) |
| Withdrawal credentials | `internal/modules/consent/withdrawalcredential.go` (`withdrawal_credential`) |
| Disclosure duties | `internal/modules/consent/noticecase.go` (`privacy_notice_case`) |
| Decision counters | `internal/modules/consent/decisioncounter.go`, exported by `internal/compose/authzmetrics.go` |
| The engine-vs-gate reading | `internal/modules/consent/authorizedisagreement.go`, swept by `internal/compose/authzdisagreement.go` |
| Cross-store ratification | `backend/gates/tableownership_test.go` |
| Jurisdiction packs | `internal/shared/ports/jurisdiction/`, `extensions/de/` |
