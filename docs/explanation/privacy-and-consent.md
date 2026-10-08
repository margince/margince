<!-- prose:plain -->
# Privacy, consent & the GDPR engines

How Margince meets its duties to a data subject. The **authorization engine** decides whether each
outbound message may go, and keeps a record of why. The **privacy engines** (erasure, subject
access, retention) do the work a granted request asks for. The product refuses to fill records by
scraping in the first place. The legal reasons behind that, for the EU and Vietnam, are in a
document kept with the company's business files, not in this tree.

Two modules work together: `consent` owns the engine and the case queue, and `privacy` owns the
code that does the work. They are connected at the composition root, never by a sibling import.

## The authorization engine (`consent`)

`consent` owns two things a reader can take for one. **Consent** is a subject's answer to a question
about a purpose: the catalog, each contact's current state, and an **add-only proof log**.
**Authorization** is whether one message may go. That is a different question, and it mostly has a different answer. Most mail goes out on a contract, on a reply to a thread the subject
started, or on a legal duty, not on consent.

The engine answers the second question. It works out a **category** from what the send is, checks
the **proof** that category needs, and records a **decision** per recipient saying why:

- **The server works out the category.** The caller does not name it. A closed set of names
  (`reply_to_inbound`, `invoice_or_payment`, `marketing`, `security_notice`, … in
  `internal/shared/ports/commsauthz`) comes from the message's own source. That source is its
  thread, the records it links and the template it uses. A caller can offer a category but cannot
  make one up.

  The send paths refuse a claim to any **subject-serving** category: a security or privacy
  notice, an opt-out, a note that confirms consent or a record. So only the installation
  itself writes to someone in its own name.
- **The basis is proof.** A reply is allowed by the thread the subject opened, and only for this
  recipient on that thread; being a reply is not enough. An invoice is allowed by a live invoice that
  reaches this recipient through a current job at the customer. That is a real test. Say a contact handles money for the customer, but nobody linked them to the
  customer record: they get `review`, not a refusal. Marketing is the case that needs
  consent, and that consent holds for one purpose only.
- **The decision is recorded twice, before sending.** It is made once when the message is staged.
  It is made again right before the provider gets anything, into `communication_decision`, one row per
  recipient. The row holds the category, the verdict, the reason, the basis and a fingerprint of the
  words. So "why could this message go" is one query, not a search through old mail.

A refusal names only the address and discloses nothing new. The engine is spelled once
(`consent.NewGate`) and **injected into the send path** (activities) at the composition root, so
consent never becomes an import edge between siblings. Every consent *state* write also appends a
proof row (Art. 7(1) demonstrability), and a fitness test (`consentproof_test.go`) fails any state
write that skips its proof.

## What a refusal leaves behind

A refusal leaves two records behind, and they answer different questions.

**A review is the refused message.** It can still go on. `communication_review` holds one send
attempt that stopped. Where the attempt carried a mail payload and the hold worked, the review points
at the held `scheduled_send` that holds it. Resuming works from there.

Two cases leave a review
with nothing to resume. One is a refused channel reply, because `scheduled_send` carries mail. The
other is a hold that itself failed. That error is not passed on: the user has a right to the refusal
they can act on, not an operator's problem they cannot.

The refusal keeps the status and code it already holds (a consent refusal stays
`409 consent_not_granted`) and adds the review ID beside them. Every client and test already knows
the status. The ID adds a handle a machine can act on. So an agent that gets a refusal can hand the
question to a human, not only report text.

States name what is needed, not who is blocked.
`needs_context` is a fact about the message and stays true for every reader. "Waiting for
Anna" stops being true when Anna leaves:

| State | What it means |
|---|---|
| `needs_context` | More proof could answer this. The engine has no reason for the message to stand on. |
| `needs_repair` | No proof can answer it. Something about the message or the address is wrong. |
| `awaiting_decision` | Handed to someone who may override it. No longer the work of the sender. |
| `resolved` | Someone closed it. |
| `superseded` | A newer attempt at the same message stands in its place. |
| `cancelled` | Nobody plans to send it. |

Resuming uses the held message again, so a resumed review stages one delivery and not a second copy
of the message. A review can be read by the user who started it, or by a user who holds
`communication_exception:read`. Any other user gets 404, not 403. The fact that a refused message
about a named subject exists is itself something to keep private. A principal that is not human is
refused before the query runs.

**An instruction overrides one refusal, by name.** It is a named user sending one refused message all
the same. At times the installation has a reason the engine cannot see. It may be a contract clause, a legal
duty, or a subject who asked in a meeting nobody logged. `communication_instruction` records that decision.

An instruction is never recorded as a consent grant. The refusal stays where it is, and the
instruction sits beside it, naming who overrides it and the reason they gave. A subject may ask later
why a message reached them. They must be shown the refusal and the decision; a grant nobody made would
tell them something wrong.

The refused recipient's decision row keeps the verdict the engine gave it.
That is `deny`, or `review` where the refusal is a reading by the machine. The row carries
`execution_authority='instruction'` beside it. So a query can tell a message that is never refused
from one that is refused and sent all the same.

**Open problem: the cap misses directed recipients.** `advertisingMessagesReceived` counts transmit
decisions with `verdict='allow'`, and the row for a recipient a user directed through does not have
that verdict. An allowed recipient on the same envelope still counts. So a send to both kinds is
counted for some of its recipients and not for others. So an installation under a legal cap can
go over it through such sends without the count moving. The column the query would need is
already on the row; nothing reads it yet.

Using an instruction checks four things. Each is a different way the record could end up telling of
something that never happened:

- **The decision is still live.** Revoked, out of date and already used are three different reasons
  the same row allows nothing now.
- **It is this message.** An instruction is made against one review, and that review holds one
  message.
- **The message has not changed**, where there is a fingerprint to check. The sender confirmed they would send
  that subject and body. An edit after that is a message nobody signed for. Some
  instructions are not held back: one with no fingerprint, or one against a review whose held message
  could not be read. Holding it back would refuse a send for a reason nobody can act on.
- **It is used once.** The row moves to `consumed` and names its delivery inside the same transaction
  that stages the message. So a retry, a double click or a job delivered twice cannot use it twice.

Directing a send answers to `communication_exception`, its own RBAC object, not a part of
consent's settings. The two are different authorities. Consent's settings decide who may change the
rules. This decides who may act against the answer those rules gave about one contact. An
installation that handed over the first has not handed over the second with it. The check is
`auth.RequireHuman` plus the grant, so a passport that holds an admin's grants cannot make one.

## Reaching the subject without the message that carried the link

Three records exist so a subject's own acts do not rest on a mailbox, a session or a token that has
since rotated.

- **A withdrawal credential lives longer than its mail.** `withdrawal_credential` is separate from
  `preference_token` because the two allow different things. The token to read and edit the profile
  rotates, and a withdrawal must live through that change. An old link and an RFC 8058 one-click
  POST still work after the read token has rotated, with no session and no more read authority.

  The credential itself lives two years. One that lived longer than every copy of the message that carried it
  would keep nobody safe. There would be no link left for a subject to click, only a working credential
  for the next user who finds one.

  Leads get a withdrawal credential too, which is what gives a recipient who is only a lead an
  opt-out. A lead who clicks the all-marketing scope records an objection to every marketing message.
  A lead who clicks a credential for one named purpose records a stop for that one purpose, on
  `communication_suppression.purpose_id`. The engine binds it only to a marketing send with the same
  purpose. It leaves the subject's other marketing purposes running, which is what "unsubscribe from
  this list" means.

  As with an all-marketing objection, it leaves the other categories alone: an objection of
  either kind says nothing about an invoice. A marketing message is allowed only once it has a
  purpose. So no marketing send without a purpose exists for a narrow stop to miss.
- **A stop records its maker and its reach.** An Article 21 objection to direct marketing and a
  request to stop all contact are different legal acts with different reach. The objection binds
  marketing alone; the request binds everything but the three categories above.
  `communication_suppression` records which kind, its `source`, the seat that captured it, and the
  authority level that decided it. `lift.go` refuses any lift that the level of the user who lifts it cannot override.

  The two kinds are not the same there. A passed-on **marketing objection** is marked
  `decided_by_level='subject'`, because Article 21 is the subject's own act and no seat should undo
  it. A passed-on **subject request** keeps the seat's level. It is a user's report of a call with no
  article behind it. So an admin who fixes a wrong "stop everything" note does not need the subject
  back on the phone.
- **A public fix or erasure opens a case.** A subject who types into a confirm link opens a
  `data_subject_request` in the same transaction. The case gets a reference number and the last day to answer it.
  No row is left behind for nobody to work.

## What the engine reports about itself

**A counter shows what the engine decides now.** `margince_communication_authz_decisions_total`
counts transmit decisions since the process started, labelled by verdict, category and mode. The
labels are those three and nothing else. The recipient is not a field of the count and must never
become one. A metrics endpoint is the last place a subject's address should show.

**A report shows where engine and gate disagree.** `DisagreementReportSince` reads that per
category from the verdicts already recorded on every transmit row. A daily sweep runs it, so nobody
has to ask for it. It decides nothing and writes no domain row.

To enforce is a setting a human
changes after reading the report. A pass that turned a category itself would be a second authority
over what may be sent. Since every category ships enforcing, the report is how someone notices a
category refusing mail the old gate would have allowed.

## The privacy engines (`privacy`)

`privacy` owns the GDPR code a granted request runs. The DSR **case queue** lives in `consent` (the
`data_subject_request` rows and their HTTP surface). The composition root injects privacy's engines
into consent's handlers.

- **Article 17 erasure** (`Eraser.EraseContact`) removes the personal values from the normal rows in
  place. It deletes raw capture, embeddings and attachment bytes. It hashes each address and handle into a
  **suppression list**, so a later capture cannot add the subject again. It proves the work with a
  **audit tombstone with no PII**, all in **one transaction per record**. That all-or-nothing
  transaction *is* the promise. It refuses a subject under `legal_hold`.
- **Article 15 subject access** (`AssembleSAR`) is one read that needs more rights. It needs the
  `contact.delete` grant **and** a row scope with no limit. It puts everything held about a contact
  into one export package: channels, deals, leads, activities, attachments, consent and its proof log,
  raw capture, field sources. The export is itself audited (`action=export`).
- **The daily retention check** (`RetentionService.EvaluateInstallation`) runs as one River job per
  workspace. The `privacy_retention` dispatcher in `cmd/worker` starts it, by default every `24h`. It
  checks **one** workspace's live policies. It takes each policy's single action on records past
  their time, in **one audited transaction per record**.

  A tenant whose pass fails fails its own job
  row. The machine never acts on `legal_hold` rows. An activity is held too when any linked contact,
  company or deal is held. A policy whose scope the engine does not understand is **skipped, with an
  error in the log**, never run in part.

## The single-transaction cross-store exception

`privacy` owns one table (`erasure_suppression`), yet erasure and retention **write tables they do
not own**. The list: `contact`, `contact_email`/`_phone`/`_social`, `contact_channel_identity`,
`lead`, `activity`, `activity_participant`, `graph_interaction_edge`, `linkedin_connection`,
`comms_outbound`, `deal`, `attachment`, `embedding`, `raw_capture`, `field_provenance`,
`preference_token`, `capture_pending_counterparty`, `voice_learning_signal`, `ai_call` and
`ai_call_payload`. The agreed list is the cross-writer map in `backend/gates/tableownership_test.go`,
which gates it. That file, not this page, is the authority.

Four of those show *why* nothing else can reach them:

- A **channel identity** is the key an inbound message would bind the subject by again. So it must
  be deleted in the same commit that hashes it into the suppression list.
- A **LinkedIn ghost** holds the subject's name, company and address, imported from a user's export.
  The subject is never asked. No contact-keyed clause can see it, because a ghost is not a
  contact row.
- The **interaction edge** would be left to a bus consumer without this. An Article 17 duty carried out
  by an event fails, and nobody sees it, when the bus is behind.
- A participant's **address arm** exists for a party who is not a record. So it lives through
  the `contact_email` delete and would keep the erased address open to a new match.

A data-subject duty must reach **every** store that holds the subject, in that one transaction per
record. To route each delete through the module that owns the table would give up the all-or-nothing
promise.

Privacy's deletes are one of the agreed cross-writer exceptions to "a module writes only its own
tables." Every such write is **agreed per table** in `backend/gates/tableownership_test.go`, with a
reason that stands alone. A waiver with no reason, or an out-of-date one, fails the test. See
[reference/modules.md](../reference/modules.md) for the map of who owns what and
[write-backbone.md](write-backbone.md) for the write shape these deletes still follow.

## What an erasure reaches in the AI telemetry, and what it does not

With payload capture turned on (`ai.capture_payloads`, opt-in), `ai_call_payload` holds the request
and response of every model call. For a reading of a meeting transcript, that request **is** the
transcript. That makes it the longest copy of someone's words this product holds.

An erasure reaches that table two ways, and they cover different calls.

**By the named subject.** A call that said which record it is about carries that record on `ai_call`
(`subject_type`, `subject_id`). The erasure deletes the payloads of every call that named the subject,
an activity of theirs, or a lead erased with them. This is the path that reaches a transcript. A
transcript gives names, not addresses, so it can hold a whole meeting without one address in it.

**By content.** Any payload whose text names one of the subject's addresses is deleted. The match deletes too much by design: to delete too much captured telemetry can be put right, while to delete not enough
personal data cannot.

**The named subject covers only calls carrying one.** A call whose input covers more than one record names
none. A list that is right half the time is a name nobody can trust for a delete. Those calls are
reached by the content match or not at all.

What reaches them in the end is the `ai_call_payload` retention window
an installation sets. `backend/gates/aicallsubject_test.go` is the census that keeps the covered half
complete. Every model call in the tree either names its subject, says why it is about no single
record, or is listed as owing one.

## Jurisdiction retention floors

A retention action that destroys data must not break a legal floor. Packs for each jurisdiction are extensions
(`extensions/de`), built in through the `ports/jurisdiction` seam; see
[extensibility.md](extensibility.md). Core code never names a jurisdiction. The **`de`** (German) pack
declares GoBD retention classes.

The retention check takes the longest built-in floor for business
correspondence. It keeps outside business correspondence (a *Handelsbrief*) from being destroyed
before that floor. An internal note or task is not correspondence and carries no floor. A fitness test
pins that line (a 400-day email lives on; a note as old as that is erased).

**What makes correspondence a *Handelsbrief*.** Either of two things, each recorded on the record
itself when it happens, not worked out again later:

- **A deal it is filed under goes through**: `won`, or carrying an offer past draft.
- **It is filed under a project.** A project is business work from its first day. So its
  correspondence records a real transaction whether or not a deal on it has closed. This reaches mail
  from a deal the customer turned down, and from delivery work years after the deal that started it. The deal
  rule alone misses both.

**Only Undo filing removes the mark.** To take the activity off the project, to archive the
project, or to end it all leave the class standing. The proof behind it cannot change either. The
project's name is copied when the activity first counts, so a later rename does not change what the
record says. The one way out is **Undo filing** (`POST /activities/{id}/project-filing/undo`). It is
a decision only a human makes: a named user who holds `activity.update`, with a written reason.

Over-retention is an argument to have with a supervisory authority, while destruction cannot be
reversed. So the undo is narrow, and the data layer refuses what the writer refuses.

## Where the code lives

| | |
|---|---|
| The authorization engine | `internal/modules/consent/authorize*.go` (`AuthorizeStagingTx`, `AuthorizeTransmit`) |
| The shared names | `internal/shared/ports/commsauthz/` (category, basis, phase, verdict, mode) |
| Decisions per recipient | `communication_decision`, `communication_basis`, `communication_suppression` |
| Standing user overrides | `internal/modules/consent/override.go` and `internal/modules/consent/overridecarry.go` (`Allow`, `RevokeOverride`, `CarryOverridesTx`, `communication_override`), with `internal/modules/consent/overridechain.go` holding the path a revoke takes across the copies a merge made, and the lock that keeps a merge from running before it; the merge reaches the carry through `internal/modules/contacts/overridecarry.go`, which owns the port and not the table |
| Consent state + proof log | `internal/modules/consent/` (`consent_purpose`, `contact_consent`, `consent_event`) |
| Article 17 erasure | `internal/modules/privacy/eraser.go` (`NewEraser`, `EraseContact`) |
| Article 15 SAR | `internal/modules/privacy/sar.go` (`AssembleSAR`) |
| Retention check | `internal/modules/privacy/retention.go` (`RetentionService.EvaluateInstallation`), run per workspace by `internal/compose/jobs_privacyretention.go` in `cmd/worker` |
| Refused sends and who directed one | `internal/modules/consent/review.go`, `instruction.go`, `instructionconsume.go` (`communication_review`, `communication_instruction`) |
| Withdrawal credentials | `internal/modules/consent/withdrawalcredential.go` (`withdrawal_credential`) |
| Duties to tell | `internal/modules/consent/noticecase.go` (`privacy_notice_case`) |
| Decision counters | `internal/modules/consent/decisioncounter.go`, exported by `internal/compose/authzmetrics.go` |
| The engine-and-gate report | `internal/modules/consent/authorizedisagreement.go`, run daily by `internal/compose/authzdisagreement.go` |
| Cross-store agreement | `backend/gates/tableownership_test.go` |
| Jurisdiction packs | `internal/shared/ports/jurisdiction/`, `extensions/de/` |
