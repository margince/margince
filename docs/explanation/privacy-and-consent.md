# Privacy, consent & the GDPR engines

How Margince meets data-subject obligations: the **authorization engine** that decides — and
records — whether each outbound message may go, and the **privacy engines** (erasure,
subject-access, retention) that a fulfilled request executes. The product refuses scraping-based enrichment in the first place; the legal
position behind that, for the EU and Vietnam, is a whitepaper kept with the company's business
material rather than in this tree. Two modules cooperate — `consent` owns the engine and the case queue, `privacy` owns
the machinery — and they are stitched together at the composition root, never by a sibling import.

## The authorization engine (`consent`)

`consent` owns two things that are easy to confuse. **Consent** is a subject's answer to a question
about a purpose — the catalog, each contact's current state, an **append-only proof log**.
**Authorization** is whether one particular message may go, which is a different question and usually
has a different answer: most legitimate mail is not sent on consent at all, but on a contract, a
reply the subject started, or a legal duty.

The engine answers the second. It resolves a **category** from what the send actually is, checks the
**evidence** that category requires, and records a per-recipient **decision** saying why:

- **The category is server-resolved**, not caller-named. A closed vocabulary
  (`reply_to_inbound`, `invoice_or_payment`, `marketing`, `security_notice`, … in
  `internal/shared/ports/commsauthz`) is derived from the message's own origin — its anchor thread,
  the records it links, the template it rides. A caller can propose one; it cannot invent one, and
  the send doors refuse a claim to any of the five **subject-serving** categories — a security or
  privacy notice, an opt-out, consent or record confirmation — so only the installation itself
  writes to somebody on its own behalf.
- **Basis is evidence, not consent.** A reply is authorized by the thread the subject opened — this
  recipient on that thread, not merely the message being a reply. An invoice is authorized by a live
  invoice reaching this recipient through a current employment relationship, which is a real bar:
  a finance contact nobody linked to the customer record is `review`, not a refusal. Marketing is
  the case that needs consent, and it stays purpose-specific.
- **The decision is taken twice and recorded before any provider I/O** — once as the message is
  staged, once immediately before the provider is handed anything — into `communication_decision`,
  one row per distinct recipient. The row says the category, the verdict, the reason, the basis and
  a fingerprint of the wording, so "why did this message go" is a query rather than a
  reconstruction. (The evidence itself lands in `communication_basis`, not on the decision row.)
  A message that reaches a provider always has both rows; the second is what catches a withdrawal,
  a bounce or an edit to the wording landing between the two.
- **A withdrawal and a suppression are different records, and both bind hard.** Unsubscribing
  withdraws consent, which the engine reads **by class** — "did they stop this kind of message" —
  because a category resolved from evidence may carry no purpose key to match.
  `communication_suppression` records the other stops: an Art. 21 objection, a statutory
  restriction, a subject's request to stop, a hard bounce. Neither expires on its own, and no
  rollout mode softens either.
- **A rep may vouch for a send the engine refused for lack of evidence, and that vouch is not
  consent.** `communication_override` (`consent.Allow`) records a standing, per-category statement
  that a machine-level refusal may be overruled for one contact — the category the engine resolved
  the send to, and only that category; a vouch for `marketing` says nothing about
  `customer_service`. It flips nothing absolute: `Decision.CanBeOverruled` only asks the question
  for a non-absolute machine reading, so the nine `absoluteDenials` above and any subject-decided
  refusal are unreachable through this door regardless of who is vouching — a subject stop still
  wins. The reason is required — unlike a suppression, which may relay a bare phone call, an
  override is the rep's own judgement call and the record must say why. It is revocable
  (`consent.RevokeOverride`) only by a caller whose authority level `CanRevoke` the level it was
  recorded at — `CanOverrule` plus one square, because admin is the top human authority and an
  admin-recorded vouch would otherwise have no seat able to take it back; `lift.go` keeps the
  stricter `CanOverrule` for a stop, where erring toward not-sending is the safe direction. It
  survives a merge onto the
  surviving contact (`consent.CarryOverridesTx`) with its original `decided_by_level` and reason
  intact, so a merge cannot launder a vouch down to a lower authority. It is carried with the rest
  of a contact's consent record through Art. 17 erasure and Art. 15 subject access
  (`privacy.AssembleSAR`'s `communication_overrides`).
- **A restriction is not total, and that is deliberate.** Three categories still reach a restricted
  subject through a registered template — `security_notice`, `privacy_notice` and
  `optout_confirmation` — because a contact is not better off for being unable to hear that their
  account was breached or that their opt-out was recorded. A hard bounce stops even those: no
  template makes a dead address deliverable.
- **Every category ships enforcing, and an omitted one enforces too.**
  `consent.authorization_modes` can move one to `observe` or `warn`, which records the engine's
  answer without binding — an operator's rollback lever, not the shipped posture. A category the
  stored map does not mention enforces rather than observing, and a NON-EMPTY map that omits any
  category is refused at the door naming the ones it missed. An empty map is accepted and every
  category enforces, which is the same answer as storing nothing at all. Absent used to mean
  observe, which turned a half-written setting into mail nobody checked. The older purpose-key gate decides only where **no** recipient's category is
  enforced, so flipping one category buys less than it looks. Nine reason codes are absolute
  (`absoluteDenials` in `commsauthz`) and deny in every mode whatever the setting says: the four
  above, an unconfirmed double opt-in, a recipient that resolves to no single subject, a consent
  withdrawal, a jurisdiction's advertising frequency cap, and a request whose claimed category
  contradicts what the record resolves it to.

Marketing consent still works the way it always did, and the round trip is what proves it: a
double-opt-in purpose needs a confirmed `consent_event`, completed **only by the data subject**, by
spending a single-use link mailed to their own live primary address. There is no operator-held
token, because a token an operator can read and hand back proves nothing about the mailbox it was
supposed to reach.

A refusal names only the address — it discloses nothing new. The engine is spelled once
(`consent.NewGate`) and **injected into the send path** (activities) at the composition root, so
consent never becomes an import edge between siblings. Every consent *state* write also appends a
proof row (Art. 7(1) demonstrability) — a fitness test (`consentproof_test.go`) fails any state write
that skips its proof.

## What a refusal leaves behind

A refusal used to be an error string. The send failed, the sender read a
sentence, and nothing on the system knew there was work outstanding — so the
message was either abandoned or retried by hand until it went. Two records now
survive a refusal, and they answer different questions.

**A review is the refused message, still resumable.** `communication_review`
holds one send attempt that stopped. Where the attempt had a mail payload and the hold
succeeded it is bound to the held `scheduled_send` that froze it, and resuming
works from there. Two cases leave a review with nothing to resume: a refused
channel reply, because `scheduled_send` carries mail, and a hold that itself
failed — which is swallowed on purpose, since the rep is owed the refusal they
can act on rather than an operator's problem they cannot.

The refusal keeps the status and code it already had — a consent refusal stays
`409 consent_not_granted` — and gains the review id beside them. That is
deliberate: the status is what every client and test already recognises, and
what the reference adds is a handle a machine can act on, so an agent given a
refusal can hand the question to a human instead of only reporting a sentence.
States name what is NEEDED, not who is blocked — `needs_context` is a fact about
the message and stays true whoever is looking at it, where "waiting for Anna"
stops being true when Anna leaves:

| State | What it means |
|---|---|
| `needs_context` | More evidence could answer this. The engine found no ground for the message to stand on. |
| `needs_repair` | No evidence can answer it. Something about the message or the address is wrong. |
| `awaiting_decision` | Handed to somebody who may override it. No longer the sender's work. |
| `resolved` | Somebody finished it. |
| `superseded` | Replaced by a fresher attempt at the same message. |
| `cancelled` | Nobody intends to send it. |

Resuming reuses the held intent, so a resumed review stages one delivery and not
a second copy of the message. A review is readable by the human who initiated it
or a human holding `communication_exception:read`; an unrelated human gets 404
rather than 403, because the existence of a refused message about a named
subject is itself a disclosure. A non-human principal is refused before the
query runs.

**An instruction is a named human deciding one refused message goes out
anyway.** Sometimes the installation has a ground the engine cannot see — a
contract clause, a legal obligation, a subject who asked in a room nobody
logged. `communication_instruction` records that decision.

It is not a consent grant and is never recorded as one. The refusal stays
exactly where it is and the instruction sits beside it, naming who overrode it
and what they said their reason was. A subject asking later why they received a
message must be shown the refusal AND the decision, not a grant nobody made.
The refused recipient's decision row keeps the verdict the engine gave it —
`deny`, or `review` where the refusal was a machine reading — and carries
`execution_authority='instruction'` beside it. So a query can tell a message
that was never refused from one that was refused and sent anyway.

**A known gap: the advertising frequency cap does not count the refused
recipients.** `advertisingMessagesReceived` counts transmit decisions with
`verdict='allow'`, and the row for a recipient who was directed through is not
one. An allowed recipient sharing the same envelope still counts, so a mixed
send is counted for some of its recipients and not others. An installation under
a jurisdiction's ceiling can therefore exceed it through exceptional sends
without the count moving. The column the query would need is already on the row;
nothing reads it yet.

Four things consumption has to be sure of, each a different way the record could
end up describing something that did not happen:

- **The decision is still live.** Revoked, expired and already spent are three
  different reasons the same row authorizes nothing now.
- **It is this message.** An instruction is given against one review, and that
  review holds one message.
- **The message has not changed**, where there is a fingerprint to compare. The
  human acknowledged a warning about a specific subject and body, and an edit
  after that is a message nobody signed for. An instruction written before the
  column existed, or against a review whose held message could not be read,
  carries none — and parking those would refuse a send for a reason nobody can
  act on.
- **It is spent exactly once.** The row moves to `consumed` and names its
  delivery inside the same transaction that stages the message, so a retry, a
  double click or a redelivered job cannot spend it twice.

Directing a send answers to `communication_exception`, its own RBAC object
rather than a corner of consent's settings. The two are different authorities:
consent's settings are who may change the RULES, and this is who may act against
the answer those rules produced about one contact. An installation that
delegated the first has not thereby delegated the second. The check is
`auth.RequireHuman` as well as the grant, so a passport inheriting an admin's
grants cannot mint one.

## Reaching the subject without the message that carried the link

Three records exist so a subject's own acts do not depend on a mailbox, a
session or a token that has since rotated.

- **A withdrawal credential outlives the mail it rode on.**
  `withdrawal_credential` is separate from `preference_token` because the two
  authorize different things: reading and editing a preference profile rotates,
  and withdrawing must survive that rotation. An old link and an RFC 8058
  one-click POST still withdraw after the read token has rotated, with no
  session and no expanded read authority. The credential itself lives 24 months, on the
  reasoning that one outliving every copy of the message it rode on protects
  nobody: there is no longer a link for a subject to press, only a working
  credential for whoever finds one. Leads get one too, which is what gives a lead-only recipient an
  opt-out. It works for the broad all-marketing scope; a lead pressing a
  NAMED-PURPOSE credential records no stop at all, because the purpose-scoped
  writer resolves contacts and a lead is not one.
- **A stop says who said it and how far it reaches.** An Art. 21 objection to
  direct marketing and a request to stop contact entirely are different legal
  acts with different reach, and the objection binds marketing alone while the
  request binds everything but the three categories above.
  `communication_suppression` records which kind, its `source`, the seat that
  captured it, and the authority level it was decided at, and `lift.go` refuses
  any lift the lifter's own level cannot overrule. The two kinds differ there. A
  relayed **marketing objection** is stamped `decided_by_level='subject'`,
  because Art. 21 is the subject's own act and no seat should be able to undo
  it. A relayed **subject request** keeps the seat's level: it is a colleague's
  report of a conversation with no article behind it, so an admin correcting a
  misheard "stop everything" does not need the subject back on the phone.
- **A public correction or erasure proposal is a case with a clock.** A subject
  typing into a confirm link opens a `data_subject_request` in the same
  transaction, with a receipt reference, rather than leaving a row nobody works.

## What the engine reports about itself

Two things, and they answer different questions.

**A counter says what the engine is deciding right now.**
`margince_communication_authz_decisions_total` counts transmit decisions since
process start, labelled by verdict, category and mode. The labels are those
three and nothing else: the recipient is not a field of the count and must never
become one, because a metrics endpoint is the last place a subject's address
should appear.

**A report says how far the engine and the older purpose-key gate disagree.**
`DisagreementReportSince` reads that per category from the verdicts already
recorded on every transmit row, swept daily rather than left as a number
somebody must think to go and fetch. It decides nothing and writes no domain
row: enforcement is a setting a human changes after reading it, and a pass that
flipped a category itself would be a second authority over what may be sent.

The report outlived the rollout it was written for. Every category now ships
enforcing, so it is no longer "is it safe to turn this on" — it is how somebody
notices a category refusing mail the old gate would have allowed.

## The privacy engines (`privacy`)

`privacy` owns the GDPR machinery a fulfilled request runs. The DSR **case queue** lives in `consent`
(the `data_subject_request` rows + their HTTP surface); the composition root injects privacy's engines
into consent's handlers.

- **Art. 17 erasure** (`Eraser.EraseContact`) — anonymize the normalized rows in place, purge raw
  capture, embeddings, and attachment bytes, hash the identifiers onto a **suppression list** so
  re-capture can't resurrect the subject, and prove it with a **PII-free audit tombstone** — all in
  **one transaction per record**. Atomicity *is* the guarantee. It refuses a subject under `legal_hold`.
- **Art. 15 subject access** (`AssembleSAR`) — one *privileged* read (needs the `contact.delete` grant
  **and** an unbounded row scope) gathers everything held about a contact — channels, deals, leads,
  activities, attachments, consent + its proof log, raw capture, field origins — into one export
  package, itself audited (`action=export`).
- **The nightly retention evaluator** (`RetentionService.EvaluateInstallation`, run as one River job per workspace off
  the `privacy_retention` dispatcher in `cmd/worker`, default every 24h) — evaluates **one**
  workspace's enabled policies and applies the policy's single action to over-age records, **one
  audited transaction per record**, and a tenant whose pass fails fails its own job row.
  `legal_hold` rows are never auto-acted, and an activity is held transitively when any linked
  contact/company/deal is held. A policy whose scope the engine doesn't understand is
  **skipped loudly**, never half-applied.

## The single-transaction cross-store exception

`privacy` owns exactly one table (`erasure_suppression`) — yet erasure and retention deliberately
**write tables they do not own**: `contact`, `contact_email`/`_phone`/`_social`,
`contact_channel_identity`, `lead`, `activity`, `activity_participant`, `graph_interaction_edge`,
`linkedin_connection`, `comms_outbound`, `deal`, `attachment`, `embedding`, `raw_capture`,
`field_provenance`, `preference_token`, `capture_pending_counterparty`, `voice_learning_signal`,
`ai_call` and `ai_call_payload`. The ratified list is the cross-writer map in
`backend/gates/tableownership_test.go`, which gates it — that file, not this page, is the authority.

Four of those are worth naming for *why* nothing else can reach them. A **channel identity** is the key
an inbound message would re-bind the subject by, so it must die in the same commit that hashes it onto
the suppression list. A **LinkedIn ghost** holds the subject's name, employer and address, imported
from a colleague's export without the subject ever being asked, and is invisible to every contact-keyed
clause because a ghost is not a contact row. The **interaction edge** would otherwise be left to a bus
consumer, and an Art. 17 obligation discharged by an event is one that fails silently when the bus is
behind. And a participant's **address arm** exists precisely for a party who never became a record, so
it survives the `contact_email` purge and would keep the erased address re-matchable.

That is by design: a data-subject
obligation must reach **every** store that holds the subject, in **one transaction per record** —
routing each purge through the owning module would trade away the atomicity that is the guarantee.

This is the one sanctioned exception to "a module writes only its own tables." Every such write is
**ratified per table** in `backend/gates/tableownership_test.go` with a self-contained rationale; a reasonless
or stale waiver fails the test. See
[reference/modules.md](../reference/modules.md) for the ownership map and
[write-backbone.md](write-backbone.md) for the write shape these purges still ride.

## What an erasure reaches in the AI telemetry, and what it does not

With payload capture enabled (`ai.capture_payloads`, opt-in), `ai_call_payload` holds the request and
response of every model call. For a reading of a meeting transcript that request **is** the transcript,
which makes it the largest copy of somebody's words this product holds.

An erasure reaches that table two ways, and the difference is worth knowing before you rely on either.

**By citation.** A call that said which record it was about carries that record on `ai_call`
(`subject_type`, `subject_id`), and the erasure deletes the payloads of every call that named the
subject, an activity of theirs, or a lead wiped with them. This is the lane that reaches a transcript:
a transcript names its speakers rather than addressing them, so it can hold a whole conversation
without spelling one address.

**By content.** Any payload whose text names one of the subject's addresses, matched crudely and on
purpose — over-deleting captured telemetry is recoverable, under-deleting personal data is not.

**The citation is an optimisation for the reachable half, not the boundary.** A call whose input spans
several records names none, deliberately: a list that is right half the time is a citation nobody can
trust for a purge. Those calls are reached by the content match or not at all, exactly as they were
before the column existed, and their guaranteed end is the `ai_call_payload` retention window an
installation configures. `backend/gates/aicallsubject_test.go` is the census that keeps the covered
half honest: every model call in the tree either names its subject, says why it is about no single
record, or is listed as owing one.

## Jurisdiction retention floors

A destructive retention action must not violate a statutory floor. Country packs register through the
`ports/jurisdiction` seam and **compile into the binary by a blank import** — core code never names a
jurisdiction. The **`de`** (German) pack declares GoBD retention classes; the retention evaluator takes
the strictest compiled-in **commercial-correspondence** floor and shields external business
correspondence (a *Handelsbrief*) from destruction below it — while an internal note or task, which is
not correspondence, carries no floor. A fitness test pins that boundary (a 400-day email survives; a
same-age note is erased).

**What qualifies correspondence as a *Handelsbrief*.** Two things, and both are recorded on the record
itself the moment they happen rather than re-derived later:

- **A deal it is filed under concludes** — won, or carrying an offer past draft.
- **It is filed under a project.** A project is a commercial engagement from the moment it exists, so
  its correspondence documents an actual transaction whether or not a deal on it has closed. This is
  what reaches mail from a negotiation that was lost and from delivery work years after the deal that
  started it — both of which the deal rule alone misses.

**The mark is permanent, and moving the record does not remove it.** Relinking an activity away from
the project, archiving the project, or closing it all leave the classification standing. The evidence
behind it is frozen too: the project's name is copied at the moment it qualifies, so a later rename
does not rewrite what the record says. Removing a mark takes a named contact giving a written reason,
through the controller's release path. The asymmetry is deliberate — over-retention is an argument to
have with a supervisory authority, and destruction is irreversible.

## Where the code lives

| | |
|---|---|
| The authorization engine | `internal/modules/consent/authorize*.go` (`AuthorizeStagingTx`, `AuthorizeTransmit`) |
| The shared vocabulary | `internal/shared/ports/commsauthz/` (category, basis, phase, verdict, mode) |
| Per-recipient decisions | `communication_decision`, `communication_basis`, `communication_suppression` |
| Standing rep overrides | `internal/modules/consent/override.go`, `overridecarry.go` (`Allow`, `RevokeOverride`, `CarryOverridesTx`, `communication_override`) |
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
