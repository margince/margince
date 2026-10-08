<!-- prose:plain -->
# The ingress gate, auto-capture, and the verdict engine

What happens to a message **after** a connector pulled it from an outside provider, and before it
shows up in the CRM.

The path is the same for every source: Gmail, IMAP, Microsoft Graph, Telegram, or an extension unit
like `openchannel`. How a connector works with its provider is that connector's own business, and
[capture-connectors.md](capture-connectors.md) covers it.

A record has one of two **shapes**, and the shape decides which steps run:

- **Mail shape**: an address names the counterparty. This is the default path on this page.
- **Channel shape**: a *channel identity* names the counterparty, which is the account they hold at a
  message provider (Telegram, a transport a unit supplies). This is what lets a user reply to the
  message, and it skips the mail ladder (section 3).

A channel record may also carry an address. The identity still names the human; see "Channel records skip the
ladder" in section 3.

There are four stages:

```text
 1. a connector      2. the ingress gate    3. auto-capture        4. the verdict engine
    picks and    ──▶    allows or      ──▶    writes the      ──▶    decides who an
    normalizes          rejects it            message once           unknown sender is
    one message
```

Each stage answers one question:

| Stage | Question |
|---|---|
| Connector | Which messages does the CRM need? |
| Ingress gate | May this call go on, and whose permissions does it run under? |
| Auto-capture | What gets written to the database? |
| Verdict engine | Does this sender become a contact? |

Three rules apply in every stage:

- **Connectors do not write to the database.** They turn a provider's message into a normal record
  and hand it over. All permission checks, audit rows and events happen in one place.
- **A second copy writes nothing.** If the same message comes twice, the second time writes nothing, so
  connectors can send a message again without risk.
- **Nothing fails in silence.** Every "no" either returns an error or writes a log row that says
  why.

## Which parts use AI

| Stage | How it works | AI task |
|---|---|---|
| Ingress gate | Plain code | none |
| Auto-capture | Plain code | none |
| Tier ladder (T0–T4) | Plain code | none |
| Verdict engine: claiming, writing, hiding, sweeping | Plain code | none |
| Verdict engine: **deciding about one sender** | **AI** | `capture_counterparty_verdict` |
| Privacy engine: **deciding about one thread** | **AI** | `capture_confidentiality_verdict` |
| Does this domain need a company record? | **AI** | `site_triage` |
| Labels that mark captured mail as needing a look (a separate feature) | **AI** | `capture_classify` |

Two model decisions answer different questions.

*What kind of sender is this?* The model gets this question once per **sender**, not per message,
and only for senders that plain code could not place. The answer decides whether a **contact** is
created, and whether that contact belongs to the whole workspace or to the importing seat alone.

*Is this thread ordinary business?* The model gets this question once per **thread per seat**, on the
first message. The answer decides who may **read** the message, never whether it is kept. A thread it
clears is open to colleagues. Any other thread stays with the seats whose mailboxes it comes from, and
the kind the model gave is the reason.

Both run on a local model by default, and neither sends message text off the machine unless an admin
binds a cloud one. The generated table of what leaves the machine
([ai-egress.md](../reference/ai-egress.md)) says which, and a gate fails when the two disagree.

**Neither model can open access by being down.** With no model set up, capture works as normal:
records stay with their owner, threads stay on hold, and nothing is erased. From the outside, a
model that is down looks like a model that says no: mail stays with the seats whose mailboxes it
comes from. That is the safe way for a model to fail.

A sender does not stay without a decision. The answers that come from the address and the
ledger alone (a decision the owner made, a role mailbox) still land. A sender that needs a model goes
to a human instead. A row that stays open would never be closed, because nothing else moves it on. It would
also look the same as a row whose turn has not come.

---

## 1. The ingress gate

The gate checks the call, then either rejects it or passes it to auto-capture. It writes nothing
itself.

The checks with the smallest cost run first:

| # | Check | Error | Reason |
|---|---|---|---|
| 1 | Is this source declared in the unit's manifest? | `ErrIngressNotDeclared` | A wrong name is refused, instead of making a new source name that nobody knows about |
| 2 | May the source stand behind the identity keys this record carries? | `ErrInvalid` | An address offered as evidence for a match is a claim about who someone is. Only a source that declared the `email` merge key may make it |
| 3 | Is this a job the system runs, not a user request? | `ErrAttendedIngest` | A user request would put the permissions of two callers into one call |
| 4 | Is the caller inside its own transaction already? | `ErrNestedIngest` | Capture opens its own. Two connections from a small pool do not fail with an error; they wait with no end |
| 5 | Is the record within the size limits? | `ErrInvalid` | Limits what an outside provider can make us store |
| 6 | Does the kind match the transport? | `ErrInvalid` | A message must name a channel the unit declared. No other kind may name one, because the reply path would answer on it |
| 7 | Is the channel identity of the counterparty on a provider this unit supplies? | `ErrInvalid` | The reply path reads that binding to find *who* a reply goes to. An account with a `telegram` binding would take a user's next reply |
| 8 | Did this installation wire up a capture pipeline? | named error | A clear error wins over a half-wired pipeline that stores messages but no contacts |
| 9 | Has this user stored a credential with this unit? | `ErrForbidden` | Storing the credential is how a user grants the unit the right to work for them |
| 10 | What may this user do right now? | none | Looked up again on every call |

**Size limits (check 5):** the raw payload may hold 256 KB, the subject 500 characters and the body
32,768 characters. There may be at most 64 addresses (none empty), each at most 320 bytes. The thread
key may hold 512 bytes, the record's `Key` 256 bytes, the channel account ID 256 bytes and the sender
name 200 characters. `OccurredAt` must be set.

A record may list no addresses at all, but only if its counterparty carries no address either. A chat
message many times names none. A counterparty address with an empty address list is refused, because that
shape turns off the internal-only check (section 2) without any error.

**The merge-key declaration (check 2)** is `IngressSource.Merges` on the unit's source. It is empty by
default and grants nothing, and an operator can read it in `manifest.generated.json` before turning
the unit on. The check is small: it only asks about an address that comes with a channel identity. A mail-shaped record's address *is* its identity, so it needs no declaration.

The rule is checked in two places. The gate refuses with the unit named, so a unit's writer sees which
declaration is missing. Capture's own check on the way in holds the same rule for every other caller. Those callers never
pass through the gate: a core connector, a test setup, a job that fills in old data.

The gate does two more things:

- **It fills in the source and `captured_by` itself**, from the calling unit. The record type has no
  field for these, so a connector cannot claim someone else captured a message.
- **It turns errors into a kind.** Database errors hold table names and SQL state, so only the kind
  of error reaches connector code.

The gate returns one of two results, and **both mean "move your cursor on"**:

| Result | Meaning |
|---|---|
| `accepted` | The message is in the CRM |
| `skipped` | The core kept nothing by design, and logged why |

`skipped` is a good result. As an error, it would make the connector retry the same message on every
pull, with no end.

## 2. Auto-capture: the single write

Everything below happens in **one database transaction**. It keys on `(source_system, source_id)`,
the provider's own ID for the message, so a second copy writes nothing.

Steps, in order:

1. **Erase check.** If the message names an account that was erased, reject it.
2. **Internal-only check.** If *every* address on the message belongs to the company's own mail
   domains, this is colleagues writing to each other. Write a short log row and drop the message.
   This runs **before** step 3, so a message between colleagues alone is never stored at all.
3. **Store the raw payload.** It is written once and never written over, so the original stays
   available.
4. **Write the activity row**, plus links, files, the parties, an `audit_log` row and an
   `event_outbox` event. This is the normal write path ([write-backbone.md](write-backbone.md)). The audit row
   stores data about the message only, never the subject or body.
5. **Run the tier ladder** (section 3) inside a savepoint. If it fails, only the contact decision fails;
   the message itself is still stored.

**Why `Addresses` must list every party.** Step 2 asks "are all parties internal?". Over an empty
list the answer is "no", so the message is kept. An empty address list so *turns off* the check, and
internal chat gets stored. The field `Counterparty.Domain` works the same way. If it is missing, the
rules below that hold a message back read the message as "keep".

The list names humans, and leaves out things. The calendar connector leaves out a place or a machine
on a calendar event (`…@resource.calendar.google.com`, or an entry Google flags as one). It is
on nobody's own domain. So counting it would turn every meeting of colleagues in such a place into a
customer touch.

## 3. The tier ladder: create a contact or not?

This is the **mail** path. It runs inside the same transaction as the message, so a message always
has a decision with it. All of it is plain SQL and Go. The only job of T4 is to record that a model
must get the question later. Channel records skip it; see the end of this section.

| Tier | Question | Result |
|---|---|---|
| **T0** | Is the sender a colleague? | Decide about the outside contact on the message instead. If every party is internal, create nothing |
| **T1** | Have we and this address sent mail to each other? | **Create the contact.** Wins over every rule below |
| **T2** | Is this a mail system (DocuSign, SendGrid…)? | Keep the message, create no contact and no company |
| **T2.5** | Did we already decide about this address? | Use that decision again. No new question, no model call |
| **T3** | Is it a personal mail domain (`gmail.com`…)? | Create the contact, but no company |
| **T4** | Nobody knows who this is | Create nothing yet. Write a row for the verdict engine |

Notes on some tiers:

- **T0 switches target instead of skipping.** If a colleague emails a client and copies you, the
  message is about the client. The ladder decides about the client, and the colleague is never
  recorded as the contact.
- **T1 only trusts proof that *we* sent the mail.** It does not look at the `From` header, which
  someone can forge. An outbound message whose text says no (`not interested`, `unsubscribe`,
  `kein Interesse`) is no evidence of intent at all.
- **T1 needs two-way mail**, and a single send is not that. Writing to someone shows intent, and intent
  many times gets no answer. The owner of a new company mails 40 companies about an event, and 6
  write back. So the ladder creates a contact at once only in two cases. They wrote back on a thread we
  wrote on, or we wrote to them on two separate threads, which nobody does without meaning to. A single send
  is not refused: it falls to T4, and the verdict engine reads the message and decides on what it
  says.
- **Mail to a list is no reply.** An address we wrote to once that then sends a newsletter has
  added us to a list; it has not answered us.
- **T1 also wins over an old verdict.** A reply to a sender we once marked as noise returns
  them in full.
- **T2.5 stops a second cost.** Without it, every new message from a decided sender asks the model
  again, and offers again a decision a human already made. A live contact that holds the address
  counts as a decided `real`. The exception is an address that a channel connector stands behind:
  that names a contact, but it is not two-way mail.

**After the transaction commits,** the contacts module creates the contact. This happens outside the
transaction, so a failure there cannot lose the message. Failures are logged for the daily repair
job.

Creating a contact does **not** create their company. That is a separate question, and the answer
comes from reading the web site of the domain (AI task `site_triage`).

### Channel records skip the ladder

A channel record (a direct message on Telegram or on a transport a unit supplies) skips every tier
above. All four mail gates (internal domains, personal mail domains, the registry of mail systems,
the queue of deferred senders) key off a mail domain. The skip keys off the **shape**; an empty
address does not set it off.

There is also nothing to defer. Someone who opens a conversation with the workspace's own bot has
already shows the intent that T1 looks for. Nobody messages a company's bot without meaning to, and
no one sends a bot mail it never asked for. So the contact is created at once, as a **contact only, never a company**.
It has **no owner**, because a workspace bot has no human who granted it and could own the record.

Capture then binds the account to that contact. A reply is routed on that binding; a record with no
identity lands a message nobody can answer.

**If the record carries an address too**, the identity still names the human, and the address only
backs it up. Only a source that declared the `email` merge key may send one. The match ladder matches
on the address and **uses** the contact already captured from mail. Capture binds the account to
their existing record, while it holds the same guard that a merge or an erase takes. Without this,
that human becomes a second contact.

An address that a connector stands behind is stored as *not* two-way mail
(`contact_email.from_correspondence = false`). It names the contact, but proves nothing about mail.
Otherwise one direct message from someone new would mark their address as a known counterparty for
good. Every later mail it sends to a list would create a contact on its own. The noise sweep would also be
switched off for it for good.

The erase check runs on **both** keys before the record lands. A request to erase that keys on an
address must still hold after the subject's next direct message. That message names them by an
account the erase list never named.

## 4. The verdict engine

The rows that T4 created are processed every hour, per workspace.

**How the work is claimed.** Workers claim rows 8 at a time with a token and a time limit. So many workers
can run at once, and a worker that fails before it ends leaves no row claimed for good. Each decision
commits on its own, so a failure keeps whatever was already decided. The decision and what it does
share a transaction, so a row can
never say `real` without the contact it promised.

**The AI call: `capture_counterparty_verdict`.** One sender per call, never a group. Only the text
of that sender is in the prompt, so an attack message cannot answer for anyone else. SQL first limits
the subject, body and sender name (300 / 1200 / 300 characters), and the prompt marks them off as
data.

The reply must be correct JSON. It holds the ID that was asked for, a kind from a fixed list,
and a score for how much the model trusts its answer. Anything else is rejected.

The model returns one of these kinds:

| Kind | What happens |
|---|---|
| `contact` | Create the contact. Queue the domain for `site_triage` |
| `role_mailbox` (such as `support@`, `cs6@`) | Keep the mail visible. Create no contact, because there is no human to record |
| `company_sender` | Same as above |
| `newsletter` | Hide the mail, and mark the domain as "not a company" |
| `transactional` | Same as above |
| `spam` | Same as above |
| `advisor` | Create the contact, visible to the mailbox owner alone |
| `personal` | Create no contact, and remove one already made. The mail stays with its owner |

A support mailbox is a `role_mailbox` whether or not it has a number. It stays one when an agent
signs with a first name, because someone else writes the next reply. The same kind of mailbox that
handles the private business of its owner (where they live, their money) is `personal` instead. Whose
business it is decides the kind; the kind of company does not.

### Private mail leaves no contact behind

The verdict comes after the contact most of the time. Capture creates a record on commit, and the
model reads the conversation some time later, up to hours. So refusing to create one is only half an
answer. The contacts on a personal thread most of the time exist before the verdict about it.
Both verdicts so *remove* the record, and also refuse a new one.

Two answers reach a contact, and they ask different questions:

- The **sender** verdict answers about an address. `personal` removes the records that address holds
  in the deciding seat's own mailbox.
- The **thread** verdict answers about one conversation. `personal` removes the record of the counterparty
  when no other conversation with that seat is business.

Two-way mail does not keep a record from either. That rule exists so that one newsletter put in the
wrong kind does not cost a real counterparty their record. It is wrong here: the owner writing back is
what private mail looks like. Applied to `personal`, it would keep every record the kind is about in a
CRM the whole company reads. One such record holds test results of the owner.

What still keeps a record safe, in every case:

- **A human opened or edited it.** Opening means making it visible to the workspace. A machine doing
  that does not count. The sender model opens a contact to the workspace when it decides the contact is a counterparty with
  nobody behind it. If being visible to the whole `workspace` kept a record safe, the earlier of two
  machine answers would win over the later one.
- **The owner marked the address `business`.** A human decision that stands wins over every model.
- **Another conversation with that seat is business.** This needs evidence: a thread that has a
  decision and is not personal, or mail already open to the whole workspace. A thread with no
  decision is unknown. A question still open is a reason to wait; when the answer comes back
  personal, the remove step runs again.

Removing a record archives it like any other archive, on the record's own audit history. A record
that was visible to the whole workspace is first made visible to its owner alone. Anyone who asks to
see archived contacts can still list one.

A `contact` verdict does not always open the contact to the workspace. Two cases keep the record visible to the
mailbox owner alone:

- **The owner wrote first, with no reply.** The address has never answered. Writing to someone
  shows intent, but there is no business between them yet. Opening the record would tell every
  colleague which companies this user is going after. The record opens up on its own the first time that address replies.
- **The thread is on a privacy hold.** A contact visible to the whole workspace, created from mail on
  hold, would show the counterparty that the hold exists to keep private.

Marking the domain is a separate step from hiding the mail. A newsletter company has a real web
site. Without the mark, the company check would create it the next time someone from that company
writes from that domain. A domain an admin approved by hand is never written over, and personal mail
domains are skipped.

**A small score is safe.** Below 0.7 the model gets the sender once more, on its own. If the score
is still below, the row becomes `unsure`. A small score never makes a row `noise`, which is the only
verdict that hides anything. It costs one more question, never a wrong delete.

**`unsure` goes to a human.** A proposal shows up in the review queue and points at the *message*,
since the sender has no record yet. **Accept** creates the contact. **Reject** does nothing at all:
the mail stays where it is. Proposals can only add, so an old proposal, or one rejected in error,
can never delete anything.

**Noise: hide first, delete later.** The engine hides the mail at once. After the undo window it erases
the content and keeps the activity row. A model verdict alone only hides: the content is erased only
when the message also carries its own `List-Unsubscribe` header.

The scope is small. It covers only
inbound mail that links to no contact, except a contact who was only copied on it. The mail must come
from an address we have never written to, which no live contact holds as two-way mail. An address that a channel connector
only stands behind does not keep it safe.

**What runs each hour, in order:**

1. **AI**: decide about senders whose turn has come. Skipped if no model is set up.
2. End rows that used up their retries, and end proposals a human rejected.
3. Create review proposals for `unsure` rows.
4. End proposals that stayed open too long.
5. Hide new mail from senders already marked as noise.
6. Erase the content of noise whose undo window has passed.

Steps 2–6 run even with AI switched off. Turning off AI does not mean keeping the content of messages
the workspace already decided were noise.

### Mail that reached an address before we learned it was yours

A message is imported for a seat when one of that seat's own addresses is on it. Most addresses are
known from the start: declared, reported by the connection, or used to sign in. An address that passes mail on
is not. It may be a past job's address, a role address that sends its mail on, or a personal domain
pointed at a work mailbox. None of those is ever the From of anything the mailbox sends. The only way to learn one
is from the mail that comes to it, and two separate messages must agree before the CRM trusts it.

So there is always mail older than the claim, and for that mail the seat made no claim. It has no
import row, so no privacy question was opened. Under a mailbox that has the privacy model on, it
would stay on hold with nothing scheduled to decide about it.

When an address becomes a seat's own, learned or declared, the seat takes over that mail. Taking it
over writes the import row the message was owed, and opens the question its thread was owed.
A decided thread keeps its answer instead of getting the question again. A `shared` mailbox gets the
row and no question. A question written but never asked reads as on hold, so a question for a shared
mailbox would hold its mail for good.

A daily job does the same for mail missed earlier. It derives its work from the state, not from a
list. The state is an address the seat owns, a message their own connector captured, and no import row. So it
also catches mail missed by any other route to the same state.

The evidence of delivery is a whole address that the server which let in the mail stated. It is
never a domain, and never a `Delivered-To` header that the sender wrote. A forged header claims no mail, because it
never becomes an address in the first place.

## 5. What a dropped message stores

"Dropped" means four different things, and only one of them stores nothing.

| Kind of drop | Activity row | Raw payload | What is stored |
|---|---|---|---|
| Connector skipped it (a like on a message, a bot) | no | no | Nothing. It never reaches the core |
| Connector could not build a correct record | no | no | The unit's own log row and a `record_dropped` event |
| **Internal-only** (section 2, step 2) | **no** | **no** | One `system_log` row: `capture_internal_dropped`, reason `internal_only`, plus the message's provider ID. **No address, subject or body** |
| T2 / T4 / `noise` verdict | **kept** | kept | The message, plus a decision row and a log row that says why no contact was made |

The internal-only check is the only one that drops message content. It runs before the raw payload
is stored, so no copy exists at all. Storing an address in the log would expose what dropping the
message was there to keep out.

The other three keep the **message** and only hold back the **contact**. A `noise` verdict is the only
path that later deletes content, and only after the undo window.

## 6. Settings and limits

**You can change these at runtime, per workspace.** A change applies from the next message on:

| Setting | Where | Controls |
|---|---|---|
| Own mail domains | Capture settings (admins write, every user reads) | Which messages count as internal, and so are dropped |
| Personal mail domains | `POST /v1/capture/consumer-mail-domains`: added domains and exceptions on top of a built-in list of ~8,700 domains | T3: create a contact but no company |
| `auto_enrich` | `PATCH /v1/capture/settings` | Whether new companies get more data on their own |
| Approved sender domains | Contacts settings | Lets an admin approve a domain a verdict blocked |

**You change these in `margince.yaml`**, and the server must start again:

| Key | What it does |
|---|---|
| `capture.transactional_extra` | More mail system domains for T2 |
| `capture.transactional_never` | Domains that must never count as a mail system. Wins over everything |
| `capture.freemail_extra` / `freemail_never` | **Accepted, but not used.** The API above owns this list; the server writes a log line at boot to say so |

**These are fixed in code.** Changing one means a code change:

| Limit | Value | Meaning |
|---|---|---|
| `PendingDeferralCap` | 500 | Most open questions per workspace. Past this, messages still come in but get no decision |
| `PendingDeferralDomainCap` | 50 | Most open questions from one sender domain, so one domain cannot use up all 500 |
| `verdictConfidenceFloor` | 0.7 | Below this: ask again, then give up and ask a human |
| `PendingMaxAttempts` | 2 | Verdict retries before the row becomes `unsure` |
| `NoiseUndoWindow` | 7 days | How long mail the engine hides can still come back before it is erased |
| `UnsureReviewWindow` | 30 days | How long a review proposal stands |
| `noiseVerdictReach` | 14 days | How far back a `noise` decision applies to later mail |
| Claim time / claim size / cap | `45m` / 8 / 200 | How long a claim holds, rows per claim, senders decided per run |
| Message size limits | section 1 | What one message may store |
| Schedule | 1 hour | Declared in `backend/api/jobs.yaml`. Changing it is a contract change plus a new generate run |
| Capture activity window | 24 hours | How long the trace keeps rows; the sweep runs every hour. The read and the sweep share one constant, so the API cannot show rows the sweep has decided to delete |

When either cap is reached, a `capture_deferral_capped` log row records **which** one. So "the queue
is full" and "one domain is filling it" are never read as the same thing.

## 7. Where to see it: Capture activity

Every decision above can be read, per user, in **Settings → Capture activity**.

The tab holds two things. First, **Keep out of capture**: the addresses and domains whose mail never
comes into the CRM. It lives here instead of under the admin's Capture settings. Blocking a sender is
how the reader answers what this page reports. It is no setting for the whole
installation.

Then the last 24 hours of the connections the reader owns: a count per result, and a section titled
**Messages** that the reader opens. In it is a row per message that says when it reached the CRM, which
connector carried it and what the pipeline decided. Where the decision needs it, the row adds the
reason that changes what the decision means. A deferred message shows what later happened to its
sender, read from the ledger of decisions.

The log is closed by default. It answers a question about one message. Someone who is fixing the pipeline
asks that, but a user who opens the page is there for something else. Every seat can still reach it; it
does not move behind the `capture_trace` grant. A user's own trace rows answer to their owner, and
no grant opens them to more users. So a gate here would make up a rule the API does not have.

That last column is **for mail rows only**. The ledger belongs to the mail ladder and keys on an
address. Without the guard, a direct message would take on whatever verdict is open for the same
human's mail. It would tell a user that a conversation they already answered is "waiting on a
verdict". A channel record has no ladder verdict, which is not the same as having one that is open.

| | |
|---|---|
| Who sees what | Your own connections need no grant, since it is your own data. Rows from a channel binding the workspace owns (a Telegram bot, a Zalo OA) belong to no user, and holders of the `capture_trace` object see them. A grant never reaches a colleague's mailbox |
| Where the numbers start | At the ingress gate. What a connector skipped before the gate is not the same from connector to connector, so it is not counted, and the page says so |
| Scope | Captured messages. Lead capture has its own results, which this tab does not list |
| Retention | 24 hours, deleted by a sweep every hour |

The trace **names the sender** and keeps a short subject: one address and one subject line, kept to
320 and 300 characters, never a body. It covers mail dropped as internal, which the CRM otherwise
stores nothing about. That is what someone looks for when a message is missing.

This is on by default. A trace of decisions that names nobody cannot answer the question the page
exists for: it cannot tell a user what the pipeline dropped. The risk has three limits:

- the payload is an address and a subject, never the message;
- the hourly sweep deletes it with its row inside a day;
- a user reads only rows from their own connections, so this is their own mail, returned to them.

`capture.trace_payloads: false` in `margince.yaml` turns it off, for an installation whose works
agreement requires that. The trace then keeps a record of every decision and names nobody. One note
above the list says so, once, about the installation; a note on each row would read as a fact about
that message.

You can **set it only in that file**, with no API and no switch in the app. So
neither choice is one user's to change for their colleagues. The address of an erased subject is never
written, whatever the setting says. A request to erase that comes inside the window reaches what is
already there.

Operators also get `margince_capture_outcomes_total` on `/metrics`, counted per result since the
process started. Take a change like "every message from that mailbox was dropped as internal since someone
registered a domain". It shows up as a turn in the line, which an operator can watch for.

## 8. Cases

Say `acme.com` is a registered own domain, and the client is `dana@client.io`.

| Case | What happens |
|---|---|
| A colleague messages you | **Dropped** at section 2, step 2 (internal-only). Only a log row is written |
| A colleague sends you notes on a client meeting, and the client is not on the message | **Also dropped.** Every party on the message is internal. Notes *about* a client are not mail *with* one |
| A colleague emails the client and copies you | **Kept.** T0 switches to `dana@client.io` and decides about the client |
| The client replies, and we have emailed them before | **T1**: contact created at once |
| Someone new writes for the first time | Message kept, **T4**: no contact until the verdict engine answers |
| …verdict `contact` | Contact created, owned by the user whose connection captured it. Domain queued for `site_triage` |
| …verdict `spam` | The engine hides the mail now, domain marked as not a company, content erased after 7 days |
| …score below 0.7 twice | `unsure`: a proposal goes to the review queue. Rejecting it changes nothing |
| A DocuSign mail comes in | Message kept, **T2**: no contact, and `eu.docusign.net` never becomes a company |
| A newsletter from someone you have emailed | **T1 keeps them.** A known contact is not a mail system |
| A first-time sender at `gmail.com` | **T3**: contact created, no company |
| Someone mails you from 60 new addresses on one domain | The first 50 get queued. The rest come in with no decision, and a `capture_deferral_capped` log row |
| The same message is read twice | Nothing happens the second time |
| A user's permissions were made smaller after they connected | Their next pull runs with the smaller permissions |
| Someone new sends a direct message on a message channel | Message kept, **no tier runs**. A contact with no owner is created at once and the account linked to it, so a user can reply to the message |
| …and the connector also knows their address, and its source declared the `email` merge key | The CRM **uses** the contact already captured from that address. Capture binds the account to them instead of to a second contact |
| …but the source declared no merge key | Refused at the gate, which names the missing declaration. The address is never dropped in silence |
| A direct message from someone whose mail verdict is still open | The trace shows the direct message's own result, never the open mail verdict |

## 9. What a connector must send

| Field | What it must hold | If you get it wrong |
|---|---|---|
| `Key` | The provider's own ID, the same every time it is read | A duplicate is created on every pull, and **nothing reports an error** |
| `Addresses` | Every party on the message, your own user too. No empty entries. May be empty only when the counterparty carries no address | The internal-only check stops working |
| `Domain` | Mail domain in small letters | The rules that hold a message back read a missing value as "keep" |
| `ThreadKey` | Starts with the provider name | Two providers can use the same key and merge conversations that have nothing to do with each other |
| `OccurredAt` | The provider's own time | The timeline is in the order we pulled, not the order things happened |
| `Raw` | The original payload | You lose the original record |
| `Counterparty.ChannelIdentity` | On a channel record: both parts (provider and account ID), or neither | Half an identity is refused. An empty one lands a message with no way to reply |
| `Counterparty.Email` on a channel record | Send it every time the provider knows it, and declare `email` in the source's `Merges` | No declaration, no record. No address, and a human already captured from mail becomes a second contact |
| `Activity.ChannelProvider` | On a `message` and nothing else, naming a channel the unit declared | A record that is not a message but names a transport is a note the reply path would answer on |

The gate can check all of these but `Key`. Getting `Key` right is the connector's job.

The last three follow one rule: a unit **sends every field** its provider gives it, and decides nothing
about identity. Which of those fields the ladder may match on is the core's call, read from the
source's declaration. Dropping an address you hold, to match a shape, drops evidence the ladder
has a right to.

## References

### Code

| Subject | File |
|---|---|
| Record type, size limits, results | `backend/pkg/extension/ingress.go` |
| The merge keys a source may declare | `backend/pkg/extension/mergekey.go` |
| A channel a unit supplies, and its transport | `backend/pkg/extension/channel.go`, `internal/compose/extchannelsend.go` |
| The ingress gate | `backend/internal/compose/extingress.go` |
| Auto-capture and the internal-only check | `backend/internal/modules/capture/sink.go`, `sinkmailgates.go` |
| The tier ladder | `backend/internal/modules/capture/sinkensure.go` |
| The channel path: shape, the check on the way in, erase guard | `backend/internal/modules/capture/sinkchannel.go` |
| Creating or taking over the human behind a channel account | `backend/internal/modules/contacts/ensurechannel.go`, `ensurechanneladopt.go` |
| Address as identity against address as two-way mail | `contact_email.from_correspondence`, created in `backend/migrations/core/0001_baseline.up.sql` and carried to its current table by `1789170001_the_record_is_called_a_contact.up.sql` |
| The ledger of decisions and its caps | `backend/internal/modules/capture/pending.go`, `pendingcap.go` |
| Review queue and sweeps | `backend/internal/modules/capture/pendingreview.go`, `pendingsweeps.go` |
| Own domains, personal mail list, settings | `owndomainstore.go`, `freemaildomain.go`, `baselinelist.go`, `settings.go` |
| T2 registry and its config keys | `capture/transactional.go`, `platform/deployconfig/capture.go` |
| Verdict engine, prompt, sweeps, accept | `backend/internal/compose/captureverdict{,ask,sweeps,accept}.go` |
| AI task names and routing | `internal/modules/ai/tasks_gen.go` (generated), `internal/compose/brain.go` |
| What starts the company check | `backend/internal/compose/capturedomaintriage.go` |
| Job schedules and time limits | `backend/api/jobs.yaml` |
| A connector to learn from | `extensions/openchannel/` |

### Other pages

- [capture-connectors.md](capture-connectors.md)
- [extensibility.md](extensibility.md)
- [write-backbone.md](write-backbone.md)
- [ai-runtime.md](ai-runtime.md)
- [privacy-and-consent.md](privacy-and-consent.md)
