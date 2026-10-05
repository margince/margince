# Outbound messaging: the delivery machinery behind a sent message

`internal/modules/comms` is Margince's **outbound message delivery**: the durable record of what was
staged for transmission, the rules that decide whether it may go now, and the dispatcher that hands it
to a provider. It mirrors [capture-connectors.md](capture-connectors.md). Capture is the governed
*ingress* surface; comms is what happens when a rep, or a governed agent, answers.

For the one-paragraph version see [reference/modules.md](../reference/modules.md). To connect a
transport and send something, see [how-to/connect-a-mailbox.md](../how-to/connect-a-mailbox.md) or
[how-to/connect-telegram.md](../how-to/connect-telegram.md). For the write shape every row here
commits through, see [write-backbone.md](write-backbone.md).

## The split: comms owns the machinery, activities owns the message

The user-visible fact of an outbound email is the **activity row**. The `activities` module writes and
owns it; `comms` holds only the state needed to get that message out and to say why it has not. Comms
owns one table: `comms_outbound`.

That split keeps the surface correct across a page reload. The rep's send answers `202 Accepted`, and
what is durable at that moment is the *timeline row*: the message they can see. The delivery row
beside it is bookkeeping: an attempt counter, a status, a reason, a receipt.

```text
POST /activities/{id}/send-email     (or /send-message)
      │
      ├─ gates that run while the rep is still looking at the screen
      │  (authorization → wiring → recipients → consent)
      │
      └─ ONE transaction ────────────────────────────────► 202
           ├─ activity row        the message, on the timeline   (activities)
           ├─ comms_outbound row  status='pending'               (comms)
           └─ comms_send_email    the transmit job               (River)

                          … later, on the worker …

         Dispatcher ── authority → seat → consent → pacing → transmit
                          └─ sent | parked | postponed | retry | skipped
```

Provider I/O lives in whichever connector implements `ports/connector.EmailSender` or
`ports/connector.MessageSender`. comms never speaks to Google or to Telegram.

## The durable staging row, and why a send is accepted instead of transmitted

`Store.StageTx` (mail) and `Store.StageChannelTx` (a messaging channel) both write into
`comms_outbound` **inside the caller's transaction**, so the delivery and the activity it reports on
commit together or not at all. The transmit job is enqueued on that same transaction
(`Runner.EnqueueTx`). "The message is on the timeline" and "something will try to send it" are
therefore one fact.

Accepting asynchronously lets the send be **governed at the moment it leaves**:

- **Consent can be withdrawn between staging and transmission.** The dispatcher's consent call is the
  authoritative one. The request-time check exists to fail fast and keep the response ordering
  correct; it does not replace the later check.
- **A seat can change in that window.** It can be revoked or downgraded, and a staged delivery carries no session of
  its own for a revocation to invalidate.
- **A provider outage never fails the user action.** The delivery rides a bounded retry ladder
  (`sendMaxAttempts = 10`, roughly five hours on River's default backoff). When the ladder is spent it
  parks with a reason, instead of returning an error to the rep at click time.
- **Sends need pacing.** Providers throttle an account that bursts. Pacing our own sends keeps a
  legitimate run of them from costing the user their mailbox's standing.

There is **no in-flight status**. A crash mid-send would strand a row in it, and a guard keyed
on that status would turn River's redelivery into a silent skip. That would disable the connector's
retransmission check in the crash it exists for. Redelivery is safe because of the terminal status:
every transition is guarded on `status = 'pending'`, and a delivery that already finished answers
`ErrTerminal`, which the dispatcher reads as `OutcomeSkipped`.

The row is **mail-shaped or channel-shaped and never half of each** (the `comms_outbound_shape`
constraint). Two Go input types carry that invariant up out of the database. One struct with a mode
flag could name a subject *and* a channel recipient together, and only Postgres would be left to
refuse it, after the caller had already decided to write.

## The dispatcher and its gates

One dispatch attempt is `Dispatcher.DispatchWithWait`, and the sequence is
authority → attachment carriage → attachment integrity → seat → consent → pacing → transmit.
Gates and policies are different mechanisms because they state different facts:

- **A gate says never.** No amount of waiting repairs a revoked grant, so gates are inline, fixed and
  not configurable.
- **A policy says not yet**, so policies are an ordered chain the deployment assembles.

The order matters: **authority must refuse before consent answers**. Otherwise the difference between
"you may not" and "they said no" tells a caller with no rights something about a contact's consent
state.

### Authority: what the provider says about the credential

`gateSendAuthority` reads the scope list the resolver *just* read from the provider, never a copy
stored when the grant was made. `SendScopeFor` answers in three states, because two cannot express a
bot token:

| Capability | Provider | Meaning |
|---|---|---|
| `SendsWithScope` | `gmail` | The grant must carry `.../auth/gmail.send`; a connection without it parks with "reconnect it to enable sending". |
| `SendsWithoutScope` | `telegram` | The credential **is** the whole authority. A bot token carries no OAuth grant, so there is nothing to intersect. |
| `CannotSend` | anything else | Nothing on this installation transmits for it. It is the zero value, so a capability nobody answered refuses instead of sending. |

If "sends without a scope" collapsed into "cannot send", a channel provider would read as
capture-only. Every message it was handed would park, under a reason naming a connector limitation
that does not exist.

> **Gmail is not read-only.** The Gmail connector requests two scopes on one consent: `gmail.readonly`
> for capture and `gmail.send` for the governed outbound path. Google will not add a scope to an
> existing refresh token, so a second grant would mean a second connection for the same mailbox. The
> pair is still least-privilege: no `gmail.modify`, no settings, no delete, and the send scope permits
> transmission only. `comms` cannot import a capture provider. A fitness test in the composition layer
> therefore binds the scope literal the authority gate demands to the connector's own constant. Drift
> there would be silent: a misspelled scope parks every send as ungranted, which reads to an operator
> as a user who declined.

### Attachment carriage: what the provider can carry

`gateAttachmentCarriage` refuses a message whose transport cannot carry the files it was staged with,
and it **parks** instead of stripping them. Stripping is the failure this gate exists to forbid, and
nobody would see it. The sender sees a timeline entry with an attachment chip, because the timeline
records what was **staged**. The recipient sees a message referring to a file that is not there.
Nobody is told, and the record of what was sent is then permanently wrong.

A sending connector declares its capability through `connector.AttachmentCarrier`. The answer is a
descriptor because channels have real per-provider limits. Its fields are `Carries`, `MaxFiles`,
`MaxBytesPerFile`, `MaxBodyWithFiles` and `MaxTotalBytes`. **There is no default:** a connector that does not implement
the seam answers the zero `Carriage`, so an adapter written without attachment support cannot be
mistaken for capable. A zero *bound* means "no limit beyond the contract's own", never "zero
allowed"; only `Carries` says nothing may go.

`MaxBodyWithFiles` is the bound mail does not have. A channel that carries text-with-files as a
**caption** bounds that text far below a text-only message. Such a message can be neither split into
two provider calls (that reintroduces the partial send) nor truncated, so it parks.

`MaxTotalBytes` is the aggregate across every file on one message, which `MaxFiles` and
`MaxBytesPerFile` cannot express between them. A provider that declares none is held to the product's
own budget, `comms.MaxSendBytes` (20 MiB); one that declares a smaller aggregate is held to that.

The whole descriptor is published per transport on `GET /v1/channel-providers` as `attachments`, with
the aggregate as `max_total_bytes`, so the composer can warn **before** a human presses send. A
mismatch discovered at transmission is correct but late.

#### What each core transport declares

| Transport | `MaxFiles` | `MaxBytesPerFile` | `MaxBodyWithFiles` |
|---|---|---|---|
| Gmail | 10 | 25 MiB | 0: mail carries the body as the body |
| Microsoft (Graph) | 10 | 2 MiB | 0: mail carries the body as the body |
| Telegram | 10 | 20 MiB | 1024 characters |

None of these transports declares an aggregate of its own, so each message is held to the 20 MiB
product budget across its files. Read the rows as per-file limits: a message inside every per-file
bound can still be over the total, and the carriage gate parks it on the first attempt with a reason
naming the total. The read path that opens the files applies the same constant as a backstop and
answers `ErrFilesNotCarried`, so it parks too instead of spending the retry ladder.

Telegram's three numbers are **measured against a live bot**, and two of them are lower than the
provider's own ceiling:

- `MaxFiles` is 10 because `sendMediaGroup` proved atomic on validation. A group holding one bad item
  is rejected whole, so there is no partial album to reason about.
- `MaxBytesPerFile` is the *inbound* download cap instead of the higher 50 MB send limit. A file this
  installation cannot receive is a strange thing to promise to send, and a full album at that size
  uploads well inside the send job's timeout.
- `MaxBodyWithFiles` is measured: 1024 characters accepted, 1025 refused.

Microsoft's 2 MiB per file survives Graph's 4 MB request limit after the attachment is base64-encoded
twice (once inside the message, once for the wire). The send path also checks the rendered message.

Telegram sends **every file as a document, an image included.** Telegram refuses an album that mixes
documents and photos outright. Grouping by type would decide per message whether one message becomes
two provider calls, which is the partial send this gate exists to prevent. It also preserves the
bytes: a `photo` is recompressed, and a re-encoded contract scan is a worse record than the file the
rep attached. The visible cost is accepted: an image arrives in the chat as a downloadable file instead
of an inline picture.

### The seat: this installation's answer about the human

`SeatAuthority.ActiveSeat` re-reads the **staging human's live seat at transmit time**. Deactivating a
user revokes their sessions and passports, but a delivery staged before that moment carries no session
of its own. Without this gate, the off-boarded account's staged batch would keep leaving their mailbox
for as long as the maximum age allows. A **downgrade** binds the same way. `seat_type` is the licensing
ceiling every other seam enforces before it lets a principal mutate. So **a rep demoted to a read seat
between staging and transmit is refused**, whatever staged the message.

It **parks** instead of retrying, because an off-boarding and a downgrade are both *answers*: no
amount of waiting restores the authority. A seat authority that could not *answer* is the opposite
case and retries, so an identity-store outage does not destroy every send in flight. The same split
applies on the channel. The credential lookup moves off the human's account (a bot is bound once for
the whole workspace), but the seat check stays the same.

### Authorization: per recipient, per phase, on evidence

The engine is asked about **every subject the delivery reaches**, including Cc. A Cc'd contact is owed
the same answer, and a recipient list that counted only the visible ones would leave a blind copy
unasked.

The engine asks about recipients, not addresses, so one ladder carries both transports. A channel
recipient has no address. A gate that could only be handed addresses would get an empty list for every
channel delivery, and a default-deny gate asked about nobody refuses nobody. The whole channel would
then pass a check that never ran.

It is asked **twice**, and the second time is the one that matters here. `AuthorizeStagingTx` runs in
the transaction that writes the delivery row, so a message that may not go is never queued.
`AuthorizeTransmit` runs immediately before the provider is handed anything. By then the message has
sat in the queue, which is where a withdrawal, an objection, a hard bounce or an edit to the wording
lands. Both write a row per recipient into `communication_decision`.

Default-deny is answered on evidence, never on a caller-supplied purpose key. The engine resolves a
category from what the send is (the thread it answers, the invoice it concerns, the template it
rides) and checks what that category requires. A reply is authorized by this recipient being on the
thread the subject opened. An invoice is authorized by a live invoice reaching them through a current
employment relationship. Marketing still needs consent, and there a `requires_double_opt_in` purpose
needs a confirmed `consent_event`. A send whose category nothing supports is `review`, never a silent
allow.

The engine must distinguish an **answer** (park: a human can act on it) from a **fault** (retry: the
question could not be asked). Getting that backwards silently kills legitimate mail.

The rest of the engine is described in
[privacy-and-consent.md](privacy-and-consent.md#the-authorization-engine-consent): consent
withdrawals read by class, `communication_suppression`, the templates that still reach a restricted
subject (`security_notice`, `privacy_notice`, `optout_confirmation`), and the rollout modes
(`consent.authorization_modes`, `observe`, `warn`). A fixed set of reason codes (`absoluteDenials` in
`commsauthz`) denies in every mode.

A rep can also record the opposite of a stop. `communication_override` is a standing, per-category
statement that a machine-level refusal for lack of evidence may be overruled for one contact. It
reaches only a non-absolute machine reading, so a subject-decided refusal still wins. A vouch names one
category, so it reaches only a reading that resolved one. An `unknown_purpose` refusal is non-absolute
and still unreachable: the request named a key the engine does not know, so no category resolved for a
vouch to name. Resending with a recognised purpose is the remedy there. Only a caller whose authority
may revoke the level the override was recorded at can revoke it. That means a caller above that level,
or an admin taking back another admin's override, which is the one square a stop's stricter rule
refuses. See [privacy-and-consent.md](privacy-and-consent.md) for the full model.

### The three destinations one message offers

A tokenized send derives **three** links from one token, and they are not interchangeable. Collapsing
them would put a POST-only endpoint behind a link recipients click:

| Surface | URL | Who presses it |
|---|---|---|
| `List-Unsubscribe` header | `{base}/v1/public/preferences/{token}/unsubscribe?purpose=` | a mailbox provider, by POST, with no browser |
| Visible "Unsubscribe" | `{base}/#/unsubscribe/{token}/{purpose}?lang=` | the recipient, who gets a page that asks before it acts |
| Visible "Manage preferences" | `{base}/#/preferences/{token}?lang=` | the recipient, who gets every purpose |

`activities.unsubscribeLinksFor` builds all three, so the header, both visible links and the redacted
timeline copy cannot name different tokens, purposes or languages. The two visible ones are hash
routes. The SPA already serves its public surfaces that way, and the token stays out of ordinary
web-server access logs until the page calls the API with it.

The human page never withdraws on arrival. Mail scanners and link prefetchers follow links in a mailbox
with nobody present, which is the same reason RFC 8058 makes the machine endpoint POST-only.

The footer speaks the language of the message it sits under: body, then subject, then the
installation's own language, then English. That is the language of the message. Nothing records the
recipient's preference; the landing pages carry a language switcher, which is the recovery path.

### Confirm-first for agents; a human's own action is its own approval

Both send operations declare `tier: confirmation_required` (🟡) in the contract's `x-mcp-tool`
extension, and both take an `ApprovalToken` parameter. The composition root's autonomy gate reads that
tier off the contract and enforces it **before** the request reaches the handler. An agent caller must
present an approval token, while a human caller's own call arrives as the approval it is. That is the
same 🟡 machinery the approvals module runs for every other confirm-first verb; see
[agent-surface.md](agent-surface.md).

### Pacing: the policies that postpone

`MailboxRatePolicy` keys on the **mailbox**, because a per-message key would give every send its own
window and pace nothing. It *peeks* the limiter instead of spending a slot, because a slot stands for a
message that reached the provider. It is told about a real send only after the receipt is durable
(`SendRecorder.Recorded`), because a limiter counting checks instead of sends paces nothing. A
permanently saturated policy would defer a delivery forever with no signal, so past the configured
maximum age the delivery parks with a reason instead.

### The five outcomes

`OutcomeSent`, `OutcomeSkipped`, `OutcomePostponed`, `OutcomeParked`, `OutcomeRetry` are the caller's
whole instruction. A job runner maps them to done / snooze / back off without re-deriving anything
from the row. Park reasons are written for the human who has to decide what happens next. The
recipient-unreachable reason names the recipient *and* says which two remedies are wasted; the
unknown-outcome reason says the message will not be retried and to check the conversation.

## Receipt before bookkeeping: key on the identity the provider stamped

Gmail rewrites `Message-ID`. A message staged under the identity this system minted goes out under
Google's, so bookkeeping keyed on the id we *requested* loses the receipt. Three things key on that
string: the echo collapse (the captured copy of our own sent mail folding onto the same activity
instead of duplicating it), the reply join and the threading headers. The wire never carried it.

The sent row carries the one mail identity (`connector.EmailSourceSystem`) instead of the mailbox it
left through. An echo read back by a *different* connector than the one that sent (a Gmail send seen
again over a colleague's IMAP) then folds onto the send instead of landing beside it. The rewrite
below is still needed, because it handles the id changing, whichever adapter observed it.

`Store.RecordSent` therefore does two things in **two transactions**, and which fact is in which is the
safety property:

1. **`commitReceipt`** writes the receipt alone (`status='sent'`, `provider_message_id`, `sent_at`,
   `inflight_at=NULL`) in a transaction carrying no bookkeeping it could fail with. It returns only
   once that is durable.
2. **`reconcileIdentity`** then moves the delivery and its timeline row onto the identity the provider
   stamped (`RFC822MessageID`), in a transaction of its own, best-effort, reporting nothing.

**The rule: receipt before bookkeeping.** By the time `RecordSent` is called the provider has accepted
the message, so an obligation exists that nothing afterwards may revoke. Leaving the delivery pending
sends it back to River. The connector's prior-send lookup cannot see an identity the provider
discarded, so it finds nothing and **transmits again**. A single transaction with the re-key under a
savepoint does not give the same guarantee. A savepoint isolates one refused statement. It does not
survive a failed RELEASE, a dropped connection, or a panic raised outside the guarded call. Any of
those leaves the receipt as an uncommitted UPDATE in a transaction that then fails to commit, and the
double-send is back.

So the whole reconcile is defensive by construction. It runs inside a `recover` boundary covering the
transaction plumbing *and* the fault report, because a panic escaping it would unwind the dispatch
attempt and let the redelivery re-send. A provider identity that is not a shape a message could carry
is **recorded, never adopted**. Every failure degrades to one outcome: *receipt recorded, one duplicate
timeline row*, with a `comms_identity_reconcile_failed` breadcrumb in `system_log` for the operator.
`thread_key` moves only when it equalled the message's own identity. A conversation **root** re-roots
onto the identity the world will reply to, while a **reply**'s thread key belongs to the conversation
it joined.

Both writes run under a context **detached from the caller's** with a deadline of its own: cancelling
the job cannot un-send the mail, so it must not be able to un-record it either.

### The seam that cannot look back: at-most-once

Mail can discover a prior send: the RFC822 identity is searchable at the provider, so a mail delivery
rides the retry ladder. Telegram's `sendMessage` has **neither an idempotency key nor a prior-send
lookup**, so no later attempt could ever tell. `sendSeam.detectsPriorSend` is the one flag that
distinguishes them, and it turns on two behaviours:

- **`MarkInFlight` before the provider call.** Marked afterwards, a worker that died mid-send would
  leave a row that looks untried, and the redelivery would deliver a second copy with nothing able to
  notice. A delivery that *already* carries the marker parks under the unknown-outcome reason.
  `ClearInFlight` retracts it only on a **definite** answer from the provider, which proves nothing was
  transmitted. It is shape-blind, and a no-op on mail rows where the column is always NULL.
- **`ParkTransmitted` instead of the ladder** when the receipt itself fails to write. For a seam with no
  prior-send lookup, returning to the ladder is a *loss*. The next attempt reads the marker, learns
  nothing, and parks the delivery as an outcome nobody knows, while the customer is holding the
  message. Parking here states what is definitely true and **keeps the provider's message id**, which
  after a failed receipt is the only handle left on that message.

Connector-side, `sendOutcome` maps Telegram's errors onto the shared vocabulary:

- a transport failure becomes `connector.ErrSendOutcomeUnknown` (never retried);
- a 403 becomes `connector.ErrRecipientUnreachable` (definite, permanent, parks at once);
- a 401/404 becomes `connector.ErrAuthRejected`;
- a 429 passes through with Telegram's own stated interval, so a backoff of our own invention does not
  earn a harder limit.

## The channel twin: the reply that can only reach the human who wrote

`POST /activities/{id}/send-message` is `send_email`'s sibling, and `resolveSeam` is the **one** branch
on provider class in the whole path. Past it, the gates, the pacing chain, the ladder and the five
outcomes are one code path for both transports. A second branch downstream would let the channel path,
exercised far less, drift from the rules the mail path keeps.

Only the vocabulary of the transport differs:

- **The activity's `kind` names the medium.** Capture files a Telegram update under `kind='telegram'`,
  and the reply transmits through the provider of that same name. `IsChannelKind` lists only what this
  installation can transmit through. `whatsapp` is a kind the contract reserves with no connector
  behind it, and admitting it would accept a reply that could only park.
- **The caller never names the recipient.** `SendMessageRequest` carries the body, the
  attachments and the context the engine is asked about, and no recipient. A channel identity is an
  opaque third-party account id. A caller able to name one could message a human this conversation is
  not with, and the reply surface has no legitimate use for that. The server reads the anchor's
  `activity_link` rows and asks the contacts module which of those contacts are **reachable** on the
  provider. It refuses unless the answer is one contact.
- **Reachability replaces address validity.** `ReachableChannelIdentities` returns live identities with
  `blocked_at IS NULL`. It returns a **list**, because the unique key binds an account to one contact
  and not a contact to one account. Handing back the first row would reply to whichever account the
  planner returned.

Three refusals, all `422`, all before anything is staged:

| Case | Code |
|---|---|
| A contact with no live channel identity: they never messaged the workspace's bot, or they blocked it | `contact_unreachable` |
| The conversation reaches more than one contact | `ambiguous_channel_recipient` |
| No live bot is bound for the provider at all | `channel_not_send_capable` |

The outbound activity carries **no subject and no natural key**. A channel has no subject line, and a
bot files no echo of the sent message back into capture for a `(source_system, source_id)` key to
collapse onto. It does carry the anchor's `thread_key`: the reply must be filed on the conversation it
answers, or reply detection will never see it. The delivery itself is staged **unanchored**. The chat
*is* the conversation, and anchoring to a specific message would mean guessing at the capture
provider's natural-key format.

Reachability is checked at request time and not again inside the write transaction. That gap is
accepted and bounded. A block landing in between leaves a staged message the provider itself refuses
with a definite error, so the delivery fails visibly instead of arriving. The consent gate makes the
same fail-fast split.

## Reply detection: the channel is resolved, not assumed

An **inbound** message in a thread where we already wrote **outbound** is a reply, and
`engagement.reply` is what the engagement scoring feeds on. The formula keys on nothing but
`thread_key` and `direction`, so it holds for any threaded medium. The event still has to name the
channel it arrived on, because an automation answering a reply routes on that value alone.

`replyOriginOf` resolves **both halves in one switch** over `counterpartyShapeOf`, the single place
capture asks how a record names its human:

- **`shapeMail`** → channel `"email"`, and the contact resolved from `contact_email`. It names the
  medium instead of the source system: `gmail`, `imap` and `graph` are three ways of reaching one
  inbox, and a consumer routing a reply back has the same job for all three.
- **`shapeChannel`** → channel = the identity's own **provider** (`telegram`), and the contact resolved
  from `contact_channel_identity`. The provider *is* the medium there.

A missing contact is not a fault. The ensure that creates contacts runs after the capture transaction
commits, so a first-ever sender has no contact yet on either medium. The malformed shapes return their
sentinels instead of a silent miss: a record naming its human both by an address and by a channel
identity, or half a channel identity. The Sink refuses both at the edge, so reaching them here means
that guard was bypassed, and the reply path reports the invariant break instead of absorbing it.

The prior-outbound scan matches **within one medium** (`kind = $2`), and that predicate is a security
control. `thread_key` is a single flat namespace holding both a mail thread root and a channel's
`<provider>:<bot>:<chat>` key. The mail half is attacker-supplied: it is the message's own
`References` root, chosen verbatim by the sender. Without the predicate, a forged `References` header
could name a Telegram conversation. Both parts of that key are discoverable (a bot id is public, and a
private chat's id is the user's own), so the forgery would manufacture a reply fact against a
conversation that sender was never in. A reply is answered on the medium it arrived on, so a
cross-medium match could never have been actionable anyway.

## Voice-bound drafts: how a draft binds to its send

Margince learns each rep's writing voice from what they send. When the AI drafts an email for a human,
the send is the moment we find out whether they sent that draft as-is or reworded it. That judgement
is captured on the send path, because nowhere else can see both texts.

Drafting hands the caller an **opaque draft reference** for the text a model served. The send that
carries that reference back says whether the human sent that text or reworded it first. That is the
only evidence a later corpus decision has that the profile is drafting in its owner's voice.

The binding is the `DraftRef` on the mail send's input. `Store.recordDraftOutcome` runs
`RecordSendOutcomeTx` **inside the send's own transaction**, so the judgment commits with the message
or not at all. The outcome is classified by comparing the served text against what went out:
`accepted` when the tokens are identical, `edited_sent` otherwise. The similarity metric is **pinned**:
a normalized token-level Levenshtein ratio over NFC-normalized, case-folded, whitespace-collapsed text.
It is pinned because a later corpus is built retroactively from these rows, and a definition that
drifted would poison every decision made from the ones already stored.

Two asymmetries carry the whole design:

- **`recorded=false` with a nil error** is every learning-domain answer. It covers a reference this
  installation never issued, one whose served text an erasure already removed, one another user owns,
  one a previous send already decided, and a sender who is not human. None of them blocks the send. A
  message that legitimately went out must never be refused over a learning signal. Answering "nothing
  to record" for a row the caller may not touch also keeps a foreign reference indistinguishable from
  an unknown one.
- **A non-nil error fails the send**, because it is a real fault. It arrives inside the transaction that
  already holds the activity and the delivery, and half of that write shape must never commit.

**An agent's send carries no draft reference.** A voice outcome is the *owner's* judgment of the
machine's draft, so an agent's edit is not the owner's authored text. The recorder refuses a non-human
principal anyway; naming a reference on the agent path would only make that refusal look like an
accident of wiring. The channel reply carries none either: `SendMessageInput` has no such field.

The signal row keeps **no** `final_text`, and carries no contact, activity or subject linkage. Art. 17
erasure could not find it, so persisting the sent correspondence there would keep an erased contact's
mail alive for the retention window.

## Provider seams

Comms depends on interfaces and never on a provider:

| Seam | What it does |
|---|---|
| `connector.EmailSender` | `SendEmail(auth, EmailMessage)`: the mail transmission, with the prior-send lookup that makes a retry safe. |
| `connector.MessageSender` | `SendMessage(auth, ChannelMessage)`: the optional channel seam. A capture-only connector does not implement it, and the resolver reports `ErrCannotSend` instead of treating it as absent. |
| `connector.AttachmentCarrier` | `Carriage()`: what this provider can carry. Read through `connector.CarriageOf`, which answers the zero descriptor for a connector that never declared any. There is no default, so nothing is mistaken for capable. |
| `ConnectionResolver.Resolve` | Resolves **one human's** mailbox: the send seam, its unsealed credential, and the scopes the provider says the grant holds. |
| `ConnectionResolver.ResolveChannel` | Resolves the **workspace's** channel binding: seam + credential, no user id (a bot is bound once for the whole workspace) and no scope list (there is nothing to intersect). |
| `MessageIdentityReconciler` | Re-keys the timeline row when the provider stamped a different identity. A required constructor parameter, because a role that transmits without one files every sent message under an identity that exists nowhere on the wire. |
| `SeatAuthority` / `ConsentGate` | The two authority answers, each obliged to distinguish an answer from a fault. The consent seam is the authorization engine: it answers per recipient, on evidence, and writes the decision it took. |
| `SendPolicy` (+ optional `SendRecorder`) | The ordered pacing chain; adding a policy is a registration, with no change to the dispatch sequence. |

Three deployment facts are the only resolver errors that park a delivery. They are `ErrNoMailbox`,
`ErrCannotSend` and `ErrProviderNotConfigured`. **Every other error is transient.** A keyvault blip or a
database timeout is a failure to *get an answer*, and parking on one would permanently destroy a
legitimate send that nothing is wrong with. `ErrProviderNotConfigured` is a sentinel of its own
because reading it as transient would leave the row pending forever, looking live and never sending.

## Rules of thumb

- **The activity is the message**; `comms_outbound` is the machinery. If a fact is user-visible, it
  belongs on the timeline row.
- **A gate says never**; a policy says not yet. Gates are fixed and inline; policies are a configured
  chain. Never mix the two.
- **Authority refuses before consent answers.** A caller with no rights learns nothing about a contact's
  consent state.
- **The seat is re-read at transmit time**, on both transports.
- **Authorization is default-deny and per recipient.** It covers every subject the delivery reaches,
  Cc included, and is answered on the evidence the resolved category requires. A purpose key the
  caller chose does not decide it.
- **It is decided twice and recorded both times.** Staging refuses what may never go; transmit
  catches what changed while the message waited: a withdrawal, a suppression, a hard bounce, an edit
  to the wording.
- **Receipt before bookkeeping.** A message the provider accepted may never end up recorded as unsent.
- **Park only on an answer**, never on a failure to get one.
- **A seam that cannot detect a prior send** marks in-flight first and never retries an unknown
  outcome.
- **comms never speaks to a provider.** Everything provider-shaped is behind the connector seams.

## Where the code lives

| | |
|---|---|
| The module contract (what comms owns, and what it does not) | `backend/internal/modules/comms/doc.go` |
| The staging row, `Load`, the transitions, the receipt + re-key ordering | `backend/internal/modules/comms/store.go` |
| The channel-shaped staging + the at-most-once marker + `ParkTransmitted` | `backend/internal/modules/comms/storechannel.go` |
| The dispatch sequence and the five outcomes | `backend/internal/modules/comms/dispatcher.go` |
| The carriage gate and the aggregate send budget | `backend/internal/modules/comms/gates.go` |
| The one branch on provider class + the at-most-once guard | `backend/internal/modules/comms/sendseam.go` |
| The seams, the send-capability table, recipient derivation | `backend/internal/modules/comms/seams.go` |
| The identity reconcile and everything that keeps it from costing a second email | `backend/internal/modules/comms/identityreconcile.go` |
| The pacing chain | `backend/internal/modules/comms/policy.go` |
| The mail send: gate order, addressee derivation, the draft-outcome hook | `backend/internal/modules/activities/email.go`, `draftoutcome.go` |
| The channel reply: recipient resolution, reachability, the outbound row | `backend/internal/modules/activities/channelsend.go`, `handlers_channelsend.go` |
| The send-capability refusals shared by both transports | `backend/internal/modules/activities/sendauthority.go` |
| Reply detection and the reply origin switch | `backend/internal/modules/capture/sinkreply.go` |
| Channel reachability (`blocked_at`) | `backend/internal/modules/contacts/channelidentity.go` |
| The consent gate the dispatcher calls | `backend/internal/modules/consent/gate.go` |
| The voice learning loop's send half | `backend/internal/modules/ai/voice_sendoutcome.go` |
| Composition: the stager, the transmit job, the resolver, the Gmail scope pair | `backend/internal/compose/commsjobs.go`, `comms.go`, `capture.go` |
| The tables | `comms_outbound` (`backend/migrations/core/0001_baseline.up.sql`, with the `comms_outbound_shape` constraint and the in-flight marker) |
| The REST contract | `backend/api/crm.yaml` (`/activities/{id}/send-email`, `/activities/{id}/send-message`) |
| The job declaration | `backend/api/jobs.yaml` (`comms_send_email`) |

## Where to go next

- The inbound mirror image (the connector seam, the one Sink, the three ingestion modes):
  [capture-connectors.md](capture-connectors.md).
- Binding the channel this path replies on: [how-to/connect-telegram.md](../how-to/connect-telegram.md).
- Connecting a mailbox to send from: [how-to/connect-a-mailbox.md](../how-to/connect-a-mailbox.md).
- The consent model, the purposes, and Art. 17 erasure: [privacy-and-consent.md](privacy-and-consent.md).
- The 🟡 confirm-first machinery both send verbs declare: [agent-surface.md](agent-surface.md).
- The write shape and the outbox bus every row here commits through: [write-backbone.md](write-backbone.md).
- The other governed egress surface, outbound webhooks: [outbound-webhooks.md](outbound-webhooks.md).
- What every module owns, `comms` included: [reference/modules.md](../reference/modules.md).
