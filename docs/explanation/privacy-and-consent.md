# Privacy, consent & the GDPR engines

How Margince meets data-subject obligations: the **authorization engine** that decides — and
records — whether each outbound message may go, and the **privacy engines** (erasure,
subject-access, retention) that a fulfilled request executes. The product refuses scraping-based enrichment in the first place; the legal
position behind that, for the EU and Vietnam, is a whitepaper kept with the company's business
material rather than in this tree. Two modules cooperate — `consent` owns the engine and the case queue, `privacy` owns
the machinery — and they are stitched together at the composition root, never by a sibling import.

## The authorization engine (`consent`)

`consent` owns two things that are easy to confuse. **Consent** is a subject's answer to a question
about a purpose — the catalog, each person's current state, an **append-only proof log**.
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
- **A restriction is not total, and that is deliberate.** Three categories still reach a restricted
  subject through a registered template — `security_notice`, `privacy_notice` and
  `optout_confirmation` — because a person is not better off for being unable to hear that their
  account was breached or that their opt-out was recorded. A hard bounce stops even those: no
  template makes a dead address deliverable.
- **Every category ships enforcing.** `consent.authorization_modes` can move one to `observe` or
  `warn`, which records the engine's answer without binding — an operator's rollback lever, not the
  shipped posture. The older purpose-key gate decides only where **no** recipient's category is
  enforced, so flipping one category buys less than it looks. Eight reason codes are absolute
  (`absoluteDenials` in `commsauthz`) and deny in every mode whatever the setting says: the four
  above, an unconfirmed double opt-in, a recipient that resolves to no single subject, a consent
  withdrawal, and a jurisdiction's advertising frequency cap.

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

## The privacy engines (`privacy`)

`privacy` owns the GDPR machinery a fulfilled request runs. The DSR **case queue** lives in `consent`
(the `data_subject_request` rows + their HTTP surface); the composition root injects privacy's engines
into consent's handlers.

- **Art. 17 erasure** (`Eraser.ErasePerson`) — anonymize the normalized rows in place, purge raw
  capture, embeddings, and attachment bytes, hash the identifiers onto a **suppression list** so
  re-capture can't resurrect the subject, and prove it with a **PII-free audit tombstone** — all in
  **one transaction per record**. Atomicity *is* the guarantee. It refuses a subject under `legal_hold`.
- **Art. 15 subject access** (`AssembleSAR`) — one *privileged* read (needs the `person.delete` grant
  **and** an unbounded row scope) gathers everything held about a person — channels, deals, leads,
  activities, attachments, consent + its proof log, raw capture, field origins — into one export
  package, itself audited (`action=export`).
- **The nightly retention evaluator** (`EvaluateWorkspace`, run as one River job per workspace off
  the `privacy_retention` dispatcher in `cmd/worker`, default every 24h) — evaluates **one**
  workspace's enabled policies and applies the policy's single action to over-age records, **one
  audited transaction per record**, and a tenant whose pass fails fails its own job row.
  `legal_hold` rows are never auto-acted, and an activity is held transitively when any linked
  person/company/deal is held. A policy whose scope the engine doesn't understand is
  **skipped loudly**, never half-applied.

## The single-transaction cross-store exception

`privacy` owns exactly one table (`erasure_suppression`) — yet erasure and retention deliberately
**write tables they do not own**: `person`, `person_email`/`_phone`/`_social`,
`person_channel_identity`, `lead`, `activity`, `activity_participant`, `graph_interaction_edge`,
`linkedin_connection`, `comms_outbound`, `deal`, `attachment`, `embedding`, `raw_capture`,
`field_provenance`, `preference_token`, `capture_pending_counterparty`, `voice_learning_signal`,
`ai_call` and `ai_call_payload`. The ratified list is the cross-writer map in
`backend/gates/tableownership_test.go`, which gates it — that file, not this page, is the authority.

Four of those are worth naming for *why* nothing else can reach them. A **channel identity** is the key
an inbound message would re-bind the subject by, so it must die in the same commit that hashes it onto
the suppression list. A **LinkedIn ghost** holds the subject's name, employer and address, imported
from a colleague's export without the subject ever being asked, and is invisible to every person-keyed
clause because a ghost is not a person row. The **interaction edge** would otherwise be left to a bus
consumer, and an Art. 17 obligation discharged by an event is one that fails silently when the bus is
behind. And a participant's **address arm** exists precisely for a party who never became a record, so
it survives the `person_email` purge and would keep the erased address re-matchable.

That is by design: a data-subject
obligation must reach **every** store that holds the subject, in **one transaction per record** —
routing each purge through the owning module would trade away the atomicity that is the guarantee.

This is the one sanctioned exception to "a module writes only its own tables." Every such write is
**ratified per table** in `backend/gates/tableownership_test.go` with a self-contained rationale; a reasonless
or stale waiver fails the test. See
[reference/modules.md](../reference/modules.md) for the ownership map and
[write-backbone.md](write-backbone.md) for the write shape these purges still ride.

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
does not rewrite what the record says. Removing a mark takes a named person giving a written reason,
through the controller's release path. The asymmetry is deliberate — over-retention is an argument to
have with a supervisory authority, and destruction is irreversible.

## Where the code lives

| | |
|---|---|
| The authorization engine | `internal/modules/consent/authorize*.go` (`AuthorizeStagingTx`, `AuthorizeTransmit`) |
| The shared vocabulary | `internal/shared/ports/commsauthz/` (category, basis, phase, verdict, mode) |
| Per-recipient decisions | `communication_decision`, `communication_basis`, `communication_suppression` |
| Consent state + proof log | `internal/modules/consent/` (`consent_purpose`, `person_consent`, `consent_event`) |
| Art. 17 erasure | `internal/modules/privacy/erasure.go` (`NewEraser`, `ErasePerson`) |
| Art. 15 SAR | `internal/modules/privacy/sar.go` (`AssembleSAR`) |
| Retention evaluator | `internal/modules/privacy/retention.go` (`EvaluateWorkspace`), fanned out per workspace by `internal/compose/jobs_privacyretention.go` in `cmd/worker` |
| Cross-store ratification | `backend/gates/tableownership_test.go` |
| Jurisdiction packs | `internal/shared/ports/jurisdiction/`, `extensions/de/` |
