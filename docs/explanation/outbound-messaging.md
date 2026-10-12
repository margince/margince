<!-- prose:plain -->
# Outbound messaging: the delivery work behind a sent message

`internal/modules/comms` is how Margince sends a message out. It holds the durable record of what was
staged to go. It holds the rules that decide whether it may go now, and the dispatcher that hands it to
a provider. Read it beside [capture-connectors.md](capture-connectors.md): capture is the governed way
in, and comms is what happens when a rep, or a governed agent, answers.

For the short version see [reference/modules.md](../reference/modules.md). To connect a transport and
send something, see [how-to/connect-a-mailbox.md](../how-to/connect-a-mailbox.md) or
[how-to/connect-telegram.md](../how-to/connect-telegram.md). For the write shape every row here
commits through, see [write-backbone.md](write-backbone.md).

## The split: comms owns the delivery work, activities owns the message

What the user sees of an outbound email is the **activity row**. The `activities` module writes and
owns it. `comms` holds only the state it needs to get that message out, and to say why it has not been
sent yet. It owns one table: `comms_outbound`.

That split keeps the screen correct when the page is opened again. The rep's send answers `202 Accepted`,
and what is durable at that moment is the *timeline row*: the message they can see. The delivery row
beside it is a ledger: a count of tries, a status, a reason, a receipt.

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

The calls to a provider live in the connector behind `ports/connector.EmailSender` or
`ports/connector.MessageSender`. comms never calls Google or Telegram itself.

## The durable staging row, and why a send is accepted instead of sent at once

`Store.StageTx` (mail) and `Store.StageChannelTx` (a message channel) both write into
`comms_outbound` **inside the transaction of the caller**. So the delivery and the activity it reports
on commit together or not at all. The send job goes into the queue in that same transaction
(`Runner.EnqueueTx`). "The message is on the timeline" and "something will try to send it" are one
fact.

Accepting now and sending later lets the send be **governed at the moment it leaves**:

- **Consent can be revoked between staging and sending.** The consent call in the dispatcher is the
  one that counts. The check at request time exists to fail early and keep the response in the right
  order. It does not take the place of the later check.
- **A seat can change in that window.** It can be revoked or moved down to a smaller seat type. A
  staged delivery carries no session of its own for the revoke to end.
- **A provider that is down never fails users.** The delivery uses a capped retry ladder
  (`sendMaxAttempts = 10`, about 5 hours on the default wait of River between tries). When the ladder
  runs out, the delivery parks with a reason, instead of returning an error to the rep at click time.
- **Sends need a pace.** Providers slow down an account that sends many at once. Pacing our own sends
  keeps a run of real mail from costing the user the good name of their mailbox.

There is **no "sending now" status**. A worker that stopped half way through a send would leave a row
in it for good. A guard keyed on that status would then turn the second delivery of the job by River into
a silent skip. That would turn off the connector's check for a past send in the one case it exists
for. A second delivery of the job is safe because of the end statuses: every change of status is
guarded on `status = 'pending'`. A delivery that already ended answers `ErrTerminal`, which the
dispatcher reads as `OutcomeSkipped`.

The row is **mail-shaped or channel-shaped and never half of each** (the `comms_outbound_shape`
constraint). Two Go input types carry that rule up out of the database. One `struct` with a mode flag
could name a subject *and* a channel recipient together. Only Postgres would then refuse
it, after the caller has already decided to write.

## The dispatcher and its gates

One dispatcher try is `Dispatcher.DispatchWithWait`, and the order is authority → what files the
provider can carry → file check → seat → consent → pace → send. Gates and policies are different
tools because they state different facts:

- **A gate says never.** Waiting never repairs a revoked grant, so gates are written into the code,
  fixed, and not something you set.
- **A policy says not yet**, so policies are an ordered list that the deploy puts together.

The order counts: **authority must refuse before consent answers**. Otherwise the gap between "you
may not" and "they said no" tells a caller with no rights something about a contact's consent state.

### Authority: what the provider says about the credential

`gateSendAuthority` reads the scope list the resolver has only now read from the provider, never a copy
stored when the grant was made. `SendScopeFor` answers in three states, because two cannot cover a bot
token:

| Value | Provider | Meaning |
|---|---|---|
| `SendsWithScope` | `gmail` | The grant must carry `.../auth/gmail.send`; a connection without it parks with `"reconnect it to enable sending"`. |
| `SendsWithoutScope` | `telegram` | The credential **is** the whole authority. A bot token carries no OAuth grant, so there is nothing to check it against. |
| `CannotSend` | anything else | Nothing on this installation sends for it. It is the zero value, so a provider nobody answered for refuses instead of sending. |

If "sends without a scope" turned into "cannot send", a channel provider would look capture-only.
Every message handed to it would park, under a reason naming a connector limit that does not exist.

> **Gmail asks for more than read access.** The Gmail connector asks for two scopes on one consent:
> `gmail.readonly` for capture and `gmail.send` for the governed outbound path. Google will not add a
> scope to a token it already gave, so a second grant would mean a second connection for the same
> mailbox. The two scopes still ask for no more than they need: no `gmail.modify`, no settings, no
> delete, and the send scope allows sending only.

> `comms` cannot import a capture provider. So a test in the composition layer binds the scope text
> the authority gate asks for to the connector's own constant. Drift there would be silent. A scope
> with a typing error parks every send as not granted, which an operator reads as a user who said no.

### What files the provider can carry

`gateAttachmentCarriage` refuses a message whose transport cannot carry the files it was staged with.
It **parks** the message instead of dropping the files. Dropping them is the failure this gate exists
to stop, and nobody would see it. The sender sees a timeline entry with a file on it, because the
timeline records what was **staged**. The recipient sees a message that names a file that is
not there. Nobody learns of it, and the record of what was sent is then wrong for good.

A sending connector declares what it can carry through `connector.AttachmentCarrier`. The answer is a
set of fields, because channels have real limits that are different for each provider. The fields are
`Carries`, `MaxFiles`, `MaxBytesPerFile`, `MaxBodyWithFiles` and `MaxTotalBytes`. **There is no
default:** a connector that does not have the seam answers the zero `Carriage`. So a connector written
without file support cannot pass for one that has it. A zero *limit* means "no limit beyond the
contract's own", never "zero allowed"; only `Carries` says nothing may go.

`MaxBodyWithFiles` is the limit mail does not have. Some channels send text with files as one short
text under the files. Such a channel caps that text far below the size of a text-only message. Such a message
cannot be split into two provider calls (that would make a half send again). It cannot be made short
either, so it parks.

`MaxTotalBytes` is the total over every file on one message, which `MaxFiles` and `MaxBytesPerFile`
cannot cover between them. A provider that declares none is capped at the product's own budget,
`comms.MaxSendBytes` (20 MiB). One that declares a smaller total is capped at that.

The whole set of fields is public per transport on `GET /v1/channel-providers` as `attachments`, with
the total as `max_total_bytes`. So the composer can tell the user **before** a human clicks send. A
check at send time that the files are too large is correct, but it comes after the fact.

#### What each core transport declares

| Transport | `MaxFiles` | `MaxBytesPerFile` | `MaxBodyWithFiles` |
|---|---|---|---|
| Gmail | 10 | 25 MiB | 0: mail carries the body as the body |
| Microsoft (Graph) | 10 | 2 MiB | 0: mail carries the body as the body |
| Telegram | 10 | 20 MiB | 1024 characters |

None of these transports declares a total of its own, so each message is capped at the 20 MiB product
budget over all its files. Read the rows as limits per file: a message inside every per-file limit
can still be over the total. The gate then parks it on the first try, with a reason that names the
total. The read path that opens the files applies the same constant as a second guard and answers
`ErrFilesNotCarried`. So it parks too, instead of using up the retry ladder.

The three numbers for Telegram were **measured against a live bot**, and two of them are below the
provider's own limit:

- `MaxFiles` is 10 because `sendMediaGroup` proved to be all or nothing on its checks. A group that
  holds one file it refuses is rejected whole, so there is never a half-sent group to think about.
- `MaxBytesPerFile` is the *inbound* cap on files this installation takes in, instead of the 50 MB
  send limit, which is more. A file this installation could not take in is not something to promise as a
  send. And a full group of that size is sent long before the send job's time limit.
- `MaxBodyWithFiles` is measured: 1024 characters accepted, 1025 refused.

The 2 MiB per file for Microsoft stays inside the 4 MB request limit of Graph after the file is
turned into `base64` twice. That happens once inside the message, and once for the wire. The send path
also checks the message as it was rendered.

Telegram sends **every file as a document, an image included.** Telegram refuses a group that puts
documents and images together. Grouping by type would decide, per message, whether one message
becomes two provider calls, which is the half send this gate exists to stop. It also keeps the file as
it was. A `photo` is made smaller again, and a contract scan made smaller is not the record the rep
added. The cost a user sees is accepted: an image comes into the chat as a file, instead of an image
in the message body.

### The seat: this installation's answer about the human

`SeatAuthority.ActiveSeat` reads the **staging human's live seat again** at send time. Turning
off a user revokes their sessions and passports. But a delivery staged before that moment carries no
session of its own. Without this gate, the staged mail of a user who was turned off would keep leaving.
It would go out from their mailbox for as long as the most wait allows.

A move **down a seat** binds the same way. `seat_type` is
the license limit every other seam enforces before it lets a caller change data. So **a rep moved to
a read seat between staging and send is refused**, whatever staged the message.

It **parks** instead of trying again, because a user turned off and a smaller seat are both
*answers*. No wait gives the authority back. A seat authority that could not *answer* is the other
case, and it tries again. So an identity store that is down does not lose every send that is on its
way. The same split applies on the channel. The credential lookup moves off the human's account (one
bot serves the whole workspace), but the seat check stays the same.

### Permission: per recipient, per step, on evidence

The engine is asked about **every subject the delivery reaches**, Cc included. A contact on Cc is owed
the same answer. A recipient list that counted only the visible ones would leave a Bcc recipient out of
the check.

The engine asks about recipients, not addresses, so one ladder carries both transports. A channel
recipient has no address. A gate that could only be handed addresses would get an empty list for
every channel delivery. And a gate that refuses by default, when asked about nobody, refuses nobody.
The whole channel would then pass a check that never happened.

It is asked **twice**, and the second time is the one that counts here. `AuthorizeStagingTx` runs in
the transaction that writes the delivery row, so a message that may not go never goes into the queue.
`AuthorizeTransmit` runs right before the provider is handed anything. By then the message has waited
in the queue. That is where a revoked consent, a contact who objects, a bounce for good or an edit to
the text lands. Both write a row per recipient into `communication_decision`.

The answer is no by default, and an allow rests on evidence, never on a purpose key the caller
supplies. The engine resolves a kind from what the send is: the thread it answers, the invoice it is
about, the template it uses. It then checks what that kind requires. A reply is allowed because this
recipient is on the thread the subject opened. An invoice is allowed because a live invoice reaches
them through the company they work for now.

A marketing mail still needs consent, and there a `requires_double_opt_in` purpose needs a confirmed
`consent_event`. A send whose kind nothing supports is `review`, never a silent pass.

The engine must tell an **answer** (park: a human can do something about it) from a **fault** (retry:
the question could not be asked). Reading one as the other silently loses good mail.

The rest of the engine is in
[privacy-and-consent.md](privacy-and-consent.md#the-authorization-engine-consent). It covers consent
revoked by kind, and `communication_suppression`. It covers the messages that still reach a subject
whose data use is limited (`security_notice`, `privacy_notice`, `optout_confirmation`). And it covers
the modes for turning it on step by step (`consent.authorization_modes`, `observe`, `warn`). A fixed
set of reason codes (`absoluteDenials` in `commsauthz`) refuses in every mode.

A rep can also record the counter to a stop. `communication_override` is a standing exception, per
kind, that says a machine "no", made for missing evidence, may be turned off for one contact. It reaches only a
machine answer outside the fixed set, so a "no" the subject decided still wins. An exception names one
kind, so it reaches only an answer that resolved one. An `unknown_purpose` "no" is outside the fixed
set and still out of reach.

The request named a key the engine does not know, so no kind resolved for
an exception to name. Sending again with a known purpose fixes that case.

Only a caller whose authority may revoke the level the exception was recorded at can revoke it. That
means a caller above that level, or an admin taking back another admin's exception. The second is the
one case where the rule for a stop, which asks for more, refuses. See
[privacy-and-consent.md](privacy-and-consent.md) for the full model.

### The three places one message points to

A send with a token derives **three** links from one token, and one cannot stand in for another.
Merging them into one would put an endpoint that takes only POST behind a link recipients click:

| Where | URL | Who clicks it |
|---|---|---|
| `List-Unsubscribe` header | `{base}/v1/public/preferences/{token}/unsubscribe?purpose=` | a mailbox provider, by POST, with no human |
| Visible `Unsubscribe` link | `{base}/#/unsubscribe/{token}/{purpose}?lang=` | the recipient, who gets a page that asks before it does anything |
| Visible `Manage preferences` link | `{base}/#/preferences/{token}?lang=` | the recipient, who gets every purpose |

`activities.unsubscribeLinksFor` builds all three. So the header, both visible links, and the copy on
the timeline with the token removed cannot name different tokens, purposes or languages. The two
visible links are routes after a `#`. The SPA already serves its public pages that way. And the token
stays out of ordinary web server access logs until the page calls the API with it.

The human page never revokes consent when it opens. Mail scanners and link checkers follow links in a
mailbox with nobody there. That is the same reason RFC 8058 makes the machine endpoint take only POST.

The text at the end of the mail uses the language of the message above it. That is the language of
the body, then the subject, then the installation's own language, then English. Nothing records the
recipient's own choice. The pages a link opens carry a language switch, which is the way back.

### Confirm first for agents; a human's own action is its own approval

Both send operations declare `tier: confirmation_required` (🟡) in the contract's `x-mcp-tool`
extension, and both take an `ApprovalToken` input. The composition layer's gate for agent actions
reads that tier off the contract and enforces it **before** the request reaches the handler. An agent
caller must show an approval token, while the own call of a human caller comes in as the approval it is.
That is the same 🟡 machine the approvals module runs for every other action that asks first; see
[agent-surface.md](agent-surface.md).

### Pace: the policies that wait

`MailboxRatePolicy` keys on the **mailbox**, because a key per message would give every send its own
window and slow down nothing. It *looks at* the limiter instead of using up a place in it, because a
place stands for a message that reached the provider. It learns of a real send only once the
receipt is durable (`SendRecorder.Recorded`). A limiter that counts checks instead of sends slows down
nothing. A policy that stays full for good would defer a delivery with no end and no signal. So past
the most wait the config allows, the delivery parks with a reason instead.

### The 5 results

`OutcomeSent`, `OutcomeSkipped`, `OutcomePostponed`, `OutcomeParked`, `OutcomeRetry` are the whole
order to the caller. A job runner maps them to end / wait / try later, without deriving anything
from the row again. Park reasons are written for the human who has to decide what happens next. The
reason for a recipient out of reach names the recipient *and* says which two fixes would be wasted.
The reason for an unknown result says the message will not be tried again, and to check the
conversation.

## Receipt before ledger: key on the identity the provider set

Gmail changes the `Message-ID`. A message staged under the identity this system made goes out under
one Google makes. So a ledger keyed on the id we *asked for* loses the receipt. 

Three things key on
that string. One is the copy merge: the captured copy of our own sent mail merging onto the same
activity, instead of a duplicate. The others are the reply match and the thread headers. The wire
never carried our id.

The sent row carries the one mail identity (`connector.EmailSourceSystem`) instead of the mailbox it
was sent through. Say a *different* connector from the one that sent reads the copy back, such as a
Gmail send read again over a colleague's IMAP. The copy then merges onto the send instead of landing
beside it. The step below that writes the new id is still needed, because it handles the id changing, whatever connector
read it.

So `Store.RecordSent` does two things in **two transactions**, and which fact sits in which one is
the rule that keeps it safe:

1. **`commitReceipt`** writes the receipt alone (`status='sent'`, `provider_message_id`, `sent_at`,
   `inflight_at=NULL`). Nothing else in that transaction could fail it, and it returns once durable.
2. **`reconcileIdentity`** then moves the delivery and its timeline row onto the identity the provider
   set (`RFC822MessageID`). It runs in a transaction of its own, may fail, and reports nothing.

**The rule: receipt before ledger.** By the time `RecordSent` is called, the provider has accepted the
message. So a promise exists that nothing after it may revoke. Leaving the delivery at `pending` sends
it back to River. The connector's lookup of past sends cannot see an identity the provider discarded,
so it finds nothing and **sends again**. One transaction with the new key under a savepoint does not
give the same promise.

A savepoint keeps one refused statement apart from the rest. But a savepoint does not live through a failed RELEASE, a dropped connection, or a `panic` outside the
guarded call. Any of those leaves the receipt as an UPDATE that never commits, in a transaction that
then fails. The second send is back.

So the whole identity fix is built to expect failure. It runs inside a `recover` guard that covers the
transaction work *and* the fault report. A `panic` it did not catch would end the dispatcher try and let
the second delivery of the job send again. A provider identity that does not have a shape a message
could carry is **recorded, never used**. Every failure comes down to one result: *receipt recorded,
one duplicate timeline row*, with a `comms_identity_reconcile_failed` note in `system_log` for the
operator.

`thread_key` moves only when it was the same as the message's own identity. The first message of a
conversation moves onto the identity others will reply to. The thread key of a **reply** belongs to the
conversation it is part of.

Both writes run under a context **apart from the one of the caller**, with a time limit of its own. Stopping the
job cannot take back the mail, so it must not take back the record either.

### The seam that cannot look back: at most once

Mail can find a past send: the RFC822 identity can be searched at the provider, so a mail delivery
uses the retry ladder. The `sendMessage` call of Telegram has **neither a key that makes a second send
safe nor a lookup of past sends**. So no later try could ever tell. `sendSeam.detectsPriorSend` is the
one flag that tells them apart, and it turns on two things:

- **`MarkInFlight` before the provider call.** Marked after the call, a worker that stopped half way
  through a send would leave a row that looks never tried. The second delivery of the job would then
  send a second copy, and nothing could see it. A delivery that *already* carries the mark parks
  under the reason for an unknown result. `ClearInFlight` takes the mark off only on a **clear** answer
  from the provider, which proves nothing was sent. It works on any shape, and does nothing on mail
  rows, where the column is always `NULL`.
- **`ParkTransmitted` instead of the ladder** when the receipt itself fails to write. For a seam with
  no lookup of past sends, going back to the ladder is a *loss*. The next try reads the mark, learns
  nothing, and parks the delivery as a result nobody knows, while the customer already has the
  message. Parking here states the plain fact and **keeps the provider's message id**. After a failed
  receipt, that id is the only handle we still have on that message.

In the connector, `sendOutcome` maps the errors of Telegram onto the shared error values:

- a transport failure becomes `connector.ErrSendOutcomeUnknown` (never tried again);
- a 403 becomes `connector.ErrRecipientUnreachable` (clear, for good, parks at once);
- a 401/404 becomes `connector.ErrAuthRejected`;
- a 429 passes through with the wait Telegram itself stated, so a wait we made up does not make
  Telegram block us for longer.

## The channel version: the reply that can only reach the human who wrote

`POST /activities/{id}/send-message` is the channel version of `send_email`, and `resolveSeam` is the
**one** branch on the kind of provider in the whole path. Past it, the gates, the pace list, the ladder
and the 5 results are one code path for both transports. A second branch later in the path would
let the channel path drift from the rules the mail path keeps. The channel path carries a small
share of the sends, so a drift there could pass with nobody seeing it.

Only the names of the transport are different:

- **The activity's `kind` names the channel kind.** Capture files a Telegram update under
  `kind='telegram'`, and the reply goes out through the provider of that same name. `IsChannelKind`
  lists only what this installation can send through. `whatsapp` is a kind the contract keeps for
  later, with no connector behind it. Letting it in would accept a reply that could only park.
- **The caller never names the recipient.** `SendMessageRequest` carries the body, the files and the
  context the engine is asked about, and no recipient. A channel identity is an account id at a third
  party that tells us nothing. A caller who could name one could message a human this conversation is
  not with, and the reply screen has no good use for that. The server reads the `activity_link` rows
  of the activity being answered. It asks the contacts module which of those contacts it can
  **reach** on the provider, and refuses unless the answer is one contact.
- **Reach takes the place of a good address.** `ReachableChannelIdentities` returns live identities
  with `blocked_at IS NULL`. It returns a **list**, because the table's key binds an account to one
  contact, not a contact to one account. Handing back the first row would reply to whatever account the
  query planner returned first.

Three ways it refuses, all `422`, all before anything is staged:

| Case | Code |
|---|---|
| A contact with no live channel identity: they never sent a message to the workspace's bot, or they blocked it | `contact_unreachable` |
| The conversation reaches more than one contact | `ambiguous_channel_recipient` |
| No live bot is set up for the provider at all | `channel_not_send_capable` |

The outbound activity carries **no subject and no source key**. A channel has no subject line. A bot
also files no copy of the sent message back into capture for a `(source_system, source_id)` key to
merge onto. It does carry the `thread_key` of the activity it answers. The reply must be filed on the
conversation it answers, or the reply check will never see it.

The delivery itself is staged **on no single
message**. The chat *is* the conversation, and staging it on one message would need us to know
the form of source key the capture provider uses.

Reach is checked at request time, and not again inside the write transaction. That gap is accepted and
capped. A block that lands in between leaves a staged message that the provider itself refuses with a
clear error. So the delivery fails where you can see it, instead of reaching them. The consent gate
makes the same fail-early split.

## Finding replies: resolve the channel, never trust it

An **inbound** message in a thread where we already wrote **outbound** is a reply, and
`engagement.reply` is what the contact score uses. The rule keys on nothing but `thread_key` and
`direction`, so it holds for any channel with threads. The event still has to name the channel it reached
us on, because a workflow that answers a reply routes on that value alone.

`replyOriginOf` resolves **both parts in one switch** over `counterpartyShapeOf`, the single place
capture asks how a record names its human:

- **`shapeMail`** → channel `"email"`, and the contact resolved from `contact_email`. It names the
  channel kind instead of the source system. `gmail`, `imap` and `graph` are three ways to reach one
  mailbox, and a reader routing a reply back has the same job for all three.
- **`shapeChannel`** → channel = the identity's own **provider** (`telegram`), and the contact resolved
  from `contact_channel_identity`. The provider *is* the channel kind there.

A missing contact is not a fault. The step that creates contacts runs after the capture transaction
commits. So a sender who writes for the first time has no contact yet, on either channel kind.

The
wrong shapes return their own error values instead of a silent miss. One is a record that names its
human both by an address and by a channel identity; the other is half a channel identity. The Sink
refuses both at the edge. So reaching them here means that guard was skipped, and the reply path
reports the break in the rule instead of letting it pass.

The scan for a past outbound message matches **within one channel kind** (`kind = $2`), and that check
is a security control. `thread_key` is one shared set of keys that holds both the first message id of a mail thread and a
channel's `<provider>:<bot>:<chat>` key. The mail half comes from the attacker: it is the message's own
first `References` id, set by the sender as is. Without the check, a forged `References` header could name
a Telegram conversation.

Anyone can learn both parts of that key: a bot id is public, and a private chat's id is the user's own.
So the forged key would make up a reply fact against a conversation that sender was never in. A reply
is answered on the channel it reached us on, so a match between channel kinds could never have been of use.

## Voice drafts: how a draft binds to its send

Margince learns the writing voice of each rep from what they send. When the AI drafts an email for a
human, the send is the moment we find out what they did with it. Either they sent that draft as it
was, or they changed the text.
That decision is captured on the send path, because no other place can see both texts.

Drafting hands the caller a **draft reference** that tells nothing about itself, for the text a model
served. The send that carries that reference back says whether the human sent that text or changed it
first. That is the only evidence a later corpus decision has that the profile is drafting in the voice
of its owner.

The binding is the `DraftRef` on the input of the mail send. `Store.recordDraftOutcome` runs
`RecordSendOutcomeTx` **inside the send's own transaction**, so the decision commits with the message
or not at all. The result comes from checking the served text against what was sent: `accepted` when
the tokens are the same, `edited_sent` otherwise.

The way it measures how much the two texts agree is **pinned**. It is a Levenshtein ratio over tokens.
The text is NFC normalized and case folded, and each run of whitespace becomes one space. It is pinned because a later
corpus is built after the fact from these rows. A meaning that drifted would break every decision made
from the rows already stored.

Two places where the answers are different carry the whole design:

- **`recorded=false` with a `nil` error** is every answer about learning. It covers a reference this
  installation never gave out, and one whose served text an erase already removed. It also covers one
  another user owns, one a past send already decided, and a sender who is not human. None of them
  blocks the send. A message that was sent for good reason must never be refused over a learning
  signal. Answering "nothing to record" for a row the caller may not touch also keeps another user's
  reference looking the same as an unknown one.
- **Any error at all fails the send**, because it is a real fault. It comes inside the
  transaction that already holds the activity and the delivery, and half of that write shape must
  never commit.

**An agent's send carries no draft reference.** A voice result is the decision of the *owner* on the
machine's draft, so an agent's edit is not text the owner wrote. The recorder already refuses a caller that
is not human. Naming a reference on the agent path would only make that "no" look like an error
in the wiring. The channel reply carries none either: `SendMessageInput` has no such field.

The signal row keeps **no** `final_text`, and carries no link to a contact, activity or subject. An
erase under GDPR article 17 could not find it. So keeping the sent mail there would keep the mail of an
erased contact for the whole retention window.

## Provider seams

The comms module depends on seams and never on a provider:

| Seam | What it does |
|---|---|
| `connector.EmailSender` | `SendEmail(auth, EmailMessage)`: the mail send, with the lookup of past sends that makes a retry safe. |
| `connector.MessageSender` | `SendMessage(auth, ChannelMessage)`: the channel seam a connector may leave out. A capture-only connector does not have it, and the resolver reports `ErrCannotSend` instead of reading it as missing. |
| `connector.AttachmentCarrier` | `Carriage()`: what this provider can carry. Read through `connector.CarriageOf`, which answers the zero value for a connector that never declared any. There is no default, so nothing passes for one that can carry files. |
| `ConnectionResolver.Resolve` | Resolves **one human's** mailbox: the send seam, its opened credential, and the scopes the provider says the grant holds. |
| `ConnectionResolver.ResolveChannel` | Resolves the **workspace's** channel binding: seam + credential, no user id (one bot serves the whole workspace) and no scope list (there is nothing to check it against). |
| `MessageIdentityReconciler` | Moves the timeline row to a new key when the provider set a different identity. A required input when building the dispatcher, because a role that sends without one files every sent message under an identity that exists in no place on the wire. |
| `SeatAuthority` / `ConsentGate` | The two authority answers, each one must tell an answer from a fault. The consent seam is the permission engine: it answers per recipient, on evidence, and writes the decision it made. |
| `SendPolicy` (+ `SendRecorder`, if any) | The ordered pace list; adding a policy means you register it, with no change to the dispatcher order. |

Three setup facts are the only resolver errors that park a delivery. They are `ErrNoMailbox`,
`ErrCannotSend` and `ErrProviderNotConfigured`. **Every other error is short-lived.** A short break in
the key store or a database time-out is a failure to *get an answer*. Parking on one would lose for
good a send that nothing is wrong with.

`ErrProviderNotConfigured` has an error value of its own. Reading it as short-lived would leave the row at `pending` with no end, looking live and never
sending.

## Short rules

- **The activity is the message**; `comms_outbound` is the delivery work. If the user sees a fact, it
  belongs on the timeline row.
- **A gate says never**; a policy says not yet. Gates are fixed and in the code; policies are a
  list in the config. Never use one for the other.
- **Authority refuses before consent answers.** A caller with no rights learns nothing about a
  contact's consent state.
- **The seat is read again at send time**, on both transports.
- **Permission says no by default**, and asks per recipient. It covers every subject the delivery
  reaches, Cc included, and answers on the evidence the resolved kind requires. A purpose key the
  caller sets does not decide it.
- **It is decided twice and recorded both times.** Staging refuses what may never go. The send step
  catches what changed while the message waited: revoked consent, a block, a bounce for good, an edit
  to the text.
- **Receipt before ledger.** A message the provider accepted may never end up recorded as not sent.
- **Park only on an answer**, never on a failure to get one.
- **A seam that cannot find a past send** marks the row as sending first, and never tries an unknown
  result again.
- **comms never calls a provider itself.** Everything shaped by a provider is behind the connector
  seams.

## Where the code lives

| | |
|---|---|
| The module contract (what comms owns, and what it does not) | `backend/internal/modules/comms/doc.go` |
| The staging row, `Load`, the status changes, the receipt + new key order | `backend/internal/modules/comms/store.go` |
| The channel-shaped staging + the at-most-once mark + `ParkTransmitted` | `backend/internal/modules/comms/storechannel.go` |
| The dispatcher order and the 5 results | `backend/internal/modules/comms/dispatcher.go` |
| The gate on what files can go, and the total send budget | `backend/internal/modules/comms/gates.go` |
| The one branch on the kind of provider + the at-most-once guard | `backend/internal/modules/comms/sendseam.go` |
| The seams, the table of what can send, how recipients are derived | `backend/internal/modules/comms/seams.go` |
| The identity fix, and everything that keeps it from costing a second email | `backend/internal/modules/comms/identityreconcile.go` |
| The pace list | `backend/internal/modules/comms/policy.go` |
| The mail send: gate order, how addresses are derived, the hook for the draft result | `backend/internal/modules/activities/email.go`, `draftoutcome.go` |
| The channel reply: resolving the recipient, reach, the outbound row | `backend/internal/modules/activities/channelsend.go`, `handlers_channelsend.go` |
| The ways it refuses on what can send, shared by both transports | `backend/internal/modules/activities/sendauthority.go` |
| Finding replies, and the switch for the source of a reply | `backend/internal/modules/capture/sinkreply.go` |
| Channel reach (`blocked_at`) | `backend/internal/modules/contacts/channelidentity.go` |
| The consent gate the dispatcher calls | `backend/internal/modules/consent/gate.go` |
| The send half of the voice learning work | `backend/internal/modules/ai/voice_sendoutcome.go` |
| Composition: the stager, the send job, the resolver, the two Gmail scopes | `backend/internal/compose/commsjobs.go`, `comms.go`, `capture.go` |
| The tables | `comms_outbound` (`backend/migrations/core/0001_baseline.up.sql`, with the `comms_outbound_shape` constraint and the "sending now" mark) |
| The REST contract | `backend/api/crm.yaml` (`/activities/{id}/send-email`, `/activities/{id}/send-message`) |
| The job declaration | `backend/api/jobs.yaml` (`comms_send_email`) |

## Where to go next

- The way in (the connector seam, the one Sink, the three ways mail comes in):
  [capture-connectors.md](capture-connectors.md).
- Binding the channel this path replies on: [how-to/connect-telegram.md](../how-to/connect-telegram.md).
- Connecting a mailbox to send from: [how-to/connect-a-mailbox.md](../how-to/connect-a-mailbox.md).
- The consent model, the purposes, and erase under GDPR article 17:
  [privacy-and-consent.md](privacy-and-consent.md).
- The 🟡 confirm-first machine both send actions declare: [agent-surface.md](agent-surface.md).
- The write shape and the outbox bus every row here commits through: [write-backbone.md](write-backbone.md).
- The other governed way out, outbound webhooks: [outbound-webhooks.md](outbound-webhooks.md).
- What every module owns, `comms` included: [reference/modules.md](../reference/modules.md).
