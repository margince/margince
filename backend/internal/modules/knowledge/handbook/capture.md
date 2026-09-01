# Capture: how conversations get into Margince

Nobody should have to copy an email into a CRM by hand. **Capture** is the part
of Margince that connects to your mailbox, your calendar and your chat, and
files what arrives against the right people, companies, deals and projects.

This page explains what connects, what happens to a message, how it finds its
place, and — just as important — what capture refuses to store.

## What you can connect

You connect these yourself, at **Settings → Connections → Connected inboxes**.

| Connection | What it brings | Can send? |
|---|---|---|
| **Gmail** | "The mail you send and receive, from Google — and the only connection Margince can send from." | Yes |
| **Google Calendar** | "Your Google calendar. It connects separately from Gmail." | No |
| **Microsoft** | "Mail and calendar on a Microsoft work account, over the Graph API. Capture only." | No |
| **IMAP mailbox** | "Any other mail host, with an app password. Capture only." | No |
| **Telegram** | One bot for the whole organization. An administrator connects it, not you. | Yes |

A mailbox connected before sending existed **cannot be upgraded in place**. The
provider only grants sending on a fresh connection, so you have to reconnect.
The app tells you this rather than letting a send fail mysteriously.

### What each connection shows you

- **Capturing** — working
- **Pending — not yet confirmed live**
- **Needs reconnect** — "The provider rejected our credentials."
- **Sync error** — with a plain reason: being throttled, unreachable, or a
  history window that expired. Most of these say "nothing is lost" and keep
  retrying.
- **Disconnected**

You also see "Last synced", "Next check ~", and whether it is polled on a
schedule or has a push subscription.

### Disconnecting

> This will delete the credential we stored for this mailbox. Capture stops
> immediately; everything already captured stays in your CRM, and reconnecting
> will ask for permission again.

Note the honest footnote: Google or Microsoft may still list Margince under your
account's third-party access. Remove it there too if you want it fully revoked.

### An agent can never connect a mailbox for you

Connecting is a human-only action. An agent granting itself read access to a
person's personal mail is exactly what this product does not allow.

A connection also cannot exceed the person who made it. If your permissions are
reduced later, the connection's reach is reduced with them on its next poll.

## Importing your mail history

When you connect a mailbox you are offered a one-time backward scan. You choose
a window: **3 months, 6 months, 12 months, 2 years or 5 years** — or skip it.
Six months is the default.

Before it runs, it counts. The count reads message ids only, not bodies, and
gives you the number of messages in the window and an estimated AI cost. It
counts exactly up to 20,000 messages; a larger mailbox is reported as a floor
rather than a made-up estimate.

You can stop it: "Stopped. Everything captured so far is kept."

The window can only be **widened** later, never narrowed. And when the progress
bar fills, the import is finished but the AI work is not — classification runs
hourly and enrichment daily afterwards.

**Everything it brings in is held to begin with.** A new mailbox is *Held until
classified*, so a backfill of five years of mail does not put five years of mail
in front of your colleagues: each thread stays with the people who were on it
until a classifier judges it ordinary business. That is also true of the records
the import mints — a contact created from a thread nothing has judged yet is
yours alone until the verdict clears it.

Which means a backfill is worth watching in two places once it finishes:
**Senders**, for what was concluded about each address it saw, and your own
timeline, for the threads still held. Both are under Settings → Connections.

## What happens to one message

Four stages, in order.

**1. The connector fetches it.** Some things never leave the connector — a chat
reaction, a message your own mail rules filtered. Those never reach Margince at
all.

**2. The admission check.** Ten checks, and it writes nothing itself. It
confirms the message is well-formed, within size limits, from a source that
vouches for the identities on it, and — checked fresh every single time — that
the member it belongs to is still allowed to do this.

There are only two outcomes, and **both mean the connector moves on**:
*accepted*, or *skipped* with the reason logged. A skip is a success. Treated
as a failure, the connector would retry the same message forever.

**3. Filing it.** One transaction: check the address was not erased, check it is
not internal-only, store the raw message once, then write the timeline entry
with its links, attachments and participants — plus the audit record and the
event. **The audit record stores metadata only, never a subject or a body.**

**4. Deciding about the sender.** See the ladder below.

You can watch all of this. **Settings → Capture activity** shows "What happened
to your messages in the last 24 hours", and any single message can be opened to
see **"How this message was handled"** — every step in the order it met them,
each marked Done, Skipped, Waiting, Failed, Did not apply, or Cannot tell.

## Who the message belongs to

Plain rules, checked in order. No model is involved in any of this.

**Is the sender a colleague?** Then judge the external person on the message
instead. If *everyone* on it is internal, create nothing.

**Have we provably sent mail to this address before?** Then create the contact.
This beats every rule below it, including an old decision that the sender was
noise. Only proof that *we* sent counts — never the `From` header, which anyone
can forge. One outbound message counts unless its text was a refusal ("not
interested", "unsubscribe"); two or more always count.

**Is it mail infrastructure?** (DocuSign, a mail relay.) Keep the message.
Create no contact and no company.

**Have we already decided about this address?** Reuse that decision. No model
call.

**Is it a personal mail domain?** Margince ships with a long list of known
consumer-mail domains, and the Capture settings screen prints the current count.
If it is one of those, create the person, but no company.

**Nobody knows who this is.** Create nothing yet. Write down the question.

### Capture never creates a company

Worth saying on its own, because it is a deliberate change. Creating a contact
does not create their employer.

It used to. Deriving a company from every mail domain manufactured companies
named after people — `sebastian@kestner.example` became a company called
"Kestner". So now capture records an open question, and a separate step that
actually reads the domain's website answers it.

## When nothing matches

An unknown sender goes into a queue. Once an hour, the system works through it.

**A model is asked exactly one question: what kind of sender is this?** It is
asked once per *sender*, never per message, and only for senders the plain rules
could not classify. Six answers are possible:

- **A person** → create the contact, and go check whether the domain deserves a
  company.
- **A role mailbox** or **a company sender** → keep the mail visible, create no
  contact.
- **A newsletter**, **something transactional**, or **spam** → hide the mail and
  mark the domain as not a company.

**This model never decides whether a message is kept.** It only decides whether
a *contact* is created. If your installation has no model configured at all,
capture works normally — only this judging step is skipped.

### Low confidence never becomes a deletion

Below a confidence of 0.7 the sender is asked about once more on its own. If it
is still below, it becomes **unsure** and goes to a person. The rule the product
states: a low score costs an extra question, never a wrong deletion.

An unsure sender reaches you as an approval card called **"Add someone from your
mail"**. Accept creates the contact. **Reject does nothing at all** — the mail
stays exactly where it is. These proposals can only ever add, so a wrongly
rejected one can never delete anything.

### Two safety limits

Margince holds at most **500 open sender questions** per organization, and at
most **50 from any one sender domain**. When either limit is hit, the log
records *which* one — so "the queue is full" and "one domain is flooding it" are
never confused with each other.

## How a message finds its project

If you use projects, a captured message is filed against one by three exact
rules, first match wins. No model, in any of them.

**1. The conversation.** A reply to a thread already filed goes under the same
project. Matched within one medium only, so a forged mail header cannot file
email onto a chat conversation.

**2. The deal.** A message linked to a deal is filed under that deal's project.

**3. The key in the subject.** Every project gets a short key. A subject
carrying `[ERP-27]` is filed under that project. **The brackets matter** — a bare
`ERP-27` is not a reference. Without that rule, a project keyed `RE` would
swallow every reply in your mailbox.

Ambiguity cancels. Two different keys in one subject, or two deals rolling up to
two different projects, both settle nothing and the ladder moves on.

If nothing matches, nothing happens. No link, and no question raised. Filing by
guesswork is exactly what the product will not do — every rule above is either
exact, or confirmed by a person.

A message that arrives before its project exists is not re-filed later on its
own. Relink it, or let the next message in the thread carry the filing.

### Filing under a project is permanent

Read this before you file anything.

Under the German rules pack, an email linked to a project is business
correspondence and must be kept **six years from the end of the calendar year in
which it was sent or received** — its own date, not the day you filed it.

The mark is written the moment the link is made, by any route. **Moving the
email off the project does not remove it.** An erasure request will then hold
that message under a restriction rather than deleting it, and it appears on the
Restricted records page with the project's name as the reason.

## Fixing a mistake: Relink

Any message on any timeline has a **Relink** action. The dialog searches people,
companies, deals, leads and projects, and offers two choices:

- **Move instead of also-link** — "Replaces the existing link of the same type
  rather than adding another."
- **Also move the rest of this conversation** — "Every message in this thread
  you can edit moves with it, in one step."

Relinking *to a project* asks for confirmation first, because of the retention
rule above.

## What capture refuses to do

These refusals are the product's character. They are not gaps.

**Colleagues talking to each other is never stored.** If every address on a
message belongs to your own domains, capture writes a one-line log row and drops
it. The app is blunt: *"When colleagues write to each other, that message is not
stored. Not even for you."*

The check runs **before** the message is stored, on purpose, so no copy exists
anywhere. The log row does not record the address or the subject — recording
them would leak exactly what dropping the message was meant to prevent.

This extends further than people expect: a colleague's recap *about* a client is
still internal mail. A recap about a client is not correspondence with one.

**Capture cannot send anything.** It is read-only. Sending is a completely
separate path, staged as its own record and re-checked against the sender's live
permissions at the moment of transmission.

**No credential is ever stored in the clear.** If the credential vault is not
configured, the connect screen refuses the connection rather than falling back
to storing the secret somewhere else.

**An IMAP connection cannot point at a private or loopback address.** Checked
against the real IP after DNS, so a rebind cannot get around it. This is a
security guard, not a bug.

**Telegram captures private chats only.** Group messages are refused before
anything is stored. Attachments are named — `[photo]`, `[document]` — not
downloaded. There is no history import for Telegram, ever. And there is exactly
one bot: a second one would not add a channel, it would remove the ability to
reply on either.

**A reply that could only fail is not offered.** If Margince cannot reach a
person on a channel, or the person has blocked the bot, there is no reply
button. And a send without consent for that purpose is blocked outright, with a
**Review consent** link rather than a silent failure.

**Sender and subject are not kept in the capture trace** unless an operator
explicitly turns that on in the deployment file. There is no in-app switch,
because it retains correspondence the CRM otherwise refuses to store. When it is
off, the trace says so: "content not stored".

## Things you control yourself

**Own email domains** (Settings → Capture, admin) — the domains that belong to
your company. This is what makes internal mail internal. Changing it is
irreversible in one direction: mail skipped while a domain was registered is
never offered again by any mailbox.

**Keep out of capture** — addresses and domains whose messages never enter the
CRM. Your own rules bind only the mailboxes you connected; the organization's
rules bind everyone. Takes effect from the next message; messages already
captured stay.

**Consumer mail domains** — which domains count as personal mailboxes. "Mail
from a consumer mailbox still creates the person — it just never creates a
company."

**Refused domains** — which domains this installation refuses a company, and
what decided each one: a model verdict, a heuristic, or a person. Letting a
domain back in re-opens the company question rather than merely clearing a flag.

**Who may read mail from this inbox** (Settings → Connections, under each
mailbox) — the posture that mailbox asks for. Three answers:

- **Held until classified** — the default for every new mailbox. A message stays
  with the people who were on it until a classifier judges the thread ordinary
  business; only then can colleagues read it. Nothing is shared before a
  decision, so a classifier that is down or out of budget leaves mail held
  rather than open.
- **Always held** — the same, minus the classifier. You share a thread yourself,
  one at a time, from its row on the record timeline.
- **Shared with the team** — a captured message is readable the moment it lands.
  Off unless an admin allows it for the organization, because reading an
  employee's mailbox into a shared CRM is what a works-council agreement covers
  in Germany and Austria. Margince does not verify that one exists.

Changing the posture governs mail captured afterwards. The same dialog offers to
narrow what is already captured; it only ever narrows, because re-opening what
was captured under a stricter answer is a separate decision.

**Senders** (Settings → Connections) — every address your mailbox brought in and
what the classifier concluded about each: a person, a role mailbox, an automated
tool, a newsletter, an advisor, personal. You can overrule any of them. "This is
business" readmits a sender; "keep out" destroys what they brought into your
mailbox and stops the next message. A decision you make is never overwritten by
a later verdict.

**Private correspondence** (a contact's or company's page) — keep your mail with
one party to the people on it, without deciding message by message. A domain
hold covers the whole firm, which is usually what you want for a lawyer or an
accountant. It binds mail from then on, and lifting it re-opens nothing.

**Email sharing** (Settings → Capture, admin) — the organization-wide floor. On
by default; turned off, every message captured from then on is held to its
participants whatever any mailbox asks for. The app warns you honestly that
doing so "will make usage of the CRM difficult."

## Whose capture activity you can see

Your own connections need no permission from anyone — it is your own mail.

Messages that arrived through an organization-wide connection, like the Telegram
bot, belong to nobody in particular and are shown to people granted that access.

**No permission grant ever reaches a colleague's mailbox.** The organization-wide
view never returns a member's personal rows.

A held message is not hidden — it is visible as a row with its date and kind,
and its content withheld. You learn that a conversation happened and nothing
about it, including why it is held: the reason describes what the message is
about, so it is withheld with the content. An admin is no exception, and neither
the audit log nor an export names a held message's subject or attachments.

When a message reached two mailboxes, each owner contributes what their own
mailbox asks for and the message ends at the strictest of those. Sharing
releases your own hold only; if a colleague is still holding it, the response
says how many other seats are, and never who.
