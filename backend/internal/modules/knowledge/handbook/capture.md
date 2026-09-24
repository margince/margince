# Capture: how conversations get into Margince

Nobody should have to copy an email into a CRM by hand. **Capture** is the part
of Margince that connects to your mailbox, your calendar and your chat, and
files what arrives against the right contacts, their companies and projects.

This page explains what connects, what happens to a message, how it finds its
place, and — just as important — what capture refuses to store.

Connecting a mailbox or calendar, and importing your older mail, is its own
page: [Connecting your mailbox and calendar](connecting-mail-and-calendars.md).

### How does email get into Margince?
Email gets into Margince through a connected mailbox: once you connect Gmail, Outlook or an IMAP mailbox at **Settings → Connections**, capture reads new mail on each sync and files it on the timelines of the contacts on it.
There is no forwarding address and no copy-paste step. Mail between colleagues is never stored, and senders judged to be newsletters, automated tools or spam create no contact.
To bring in older mail, use **Import mailbox history**. To see what happened to one message, open **Settings → Capture activity**.
Also called: email logging, sync email to CRM, auto-log emails.

## What happens to one message

A captured message passes four stages, in order.

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

You can watch all of this. **Settings → Capture activity** shows "What the last
24 hours of your mail produced." Under **Messages**, any single message can be
opened again with **Show every processing step for this message** to see **How
this message was handled** — every step in the order it met them, each marked
Done, Skipped, Waiting, Failed, Not applicable, or Unknown.

### Why was my email not captured?
To find out why an email is missing from Margince, open **Settings → Capture activity**, find it under **Messages**, and press **Show every processing step for this message**.
**How this message was handled** marks each step Done, Skipped, Waiting, Failed, Not applicable or Unknown, with the reason.
Common causes: every address was on your own domains, so it was never stored; the sender is on your **Capture exclusions**; the sender was judged a newsletter or automated tool; or the mailbox shows **Needs reconnect**.
The log keeps 24 hours and shows only your own connections.
Also called: missing email, email did not sync, email not logged.

## Who the message belongs to

The sender ladder is plain rules, checked in order. No model is involved in any
of this.

**Is the sender a colleague?** Then judge the external contact on the message
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

**Is it a consumer mail domain?** Margince ships with a long list of known
consumer-mail domains, and the Capture rules screen prints the current count.
If it is one of those, create the contact, but no company.

**Nobody knows who this is.** Create nothing yet. Write down the question.

### Capture never creates a company

Capture creating a contact does not create their employer. Worth saying on its
own, because it is a deliberate change.

It used to. Deriving a company from every mail domain manufactured companies
named after individuals — `sebastian@kestner.example` became a company called
"Kestner". So now capture records an open question, and a separate step that
actually reads the domain's website answers it.

## When nothing matches

An unknown sender goes into a queue. Every ten minutes, the system works through it.

**A model is asked exactly one question: what kind of sender is this?** It is
asked once per *sender*, never per message, and only for senders the plain rules
could not classify. Six answers are possible:

- **A contact** → create the contact, and go check whether the domain deserves a
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
is still below, it becomes **unsure** and goes to a human. The rule the product
states: a low score costs an extra question, never a wrong deletion.

An unsure sender reaches you as an approval card, **Add contact from mail**.
Accept creates the contact. **Reject does nothing at all** — the mail stays
exactly where it is. These proposals can only ever add, so a wrongly rejected
one can never delete anything.

### Two safety limits

Margince holds at most **500 open sender questions** per company, and at
most **50 from any one sender domain**. When either limit is hit, the log
records *which* one — so "the queue is full" and "one domain is flooding it" are
never confused with each other.

### Why was my email not linked to a deal?
Margince does not link captured email to a deal on its own: capture files a message under the contacts on it, their company and, where a rule matches, a project. To put an email on a deal, use **Relink** on the message.
1. Open the contact or company timeline and find the email.
2. Press **Relink**.
3. Search for the deal under "Search contacts, companies, deals, leads or projects" and pick it.
4. Leave **Replace existing link** off to add the deal beside the current links. Tick **Move rest of thread** to bring the whole thread.
5. Press **Relink**.
Also called: email missing from opportunity, attach email to deal, associate email.

## How a message finds its project

If you use projects, a captured message is filed against one by three exact
rules, first match wins. No model, in any of them.

**1. The conversation.** A reply to a thread already filed goes under the same
project. Matched within one medium only, so a forged mail header cannot file
email onto a chat conversation.

**2. The deal.** A message already linked to a deal is filed under that deal's
project.

**3. The key in the subject.** Every project gets a short key. A subject
carrying `[ERP-27]` is filed under that project. **The brackets matter** — a bare
`ERP-27` is not a reference. Without that rule, a project keyed `RE` would
swallow every reply in your mailbox.

Ambiguity cancels. Two different keys in one subject, or two deals rolling up to
two different projects, both settle nothing and the ladder moves on.

If nothing matches, nothing happens. No link, and no question raised. Filing by
guesswork is exactly what the product will not do — every rule above is either
exact, or confirmed by a human.

A message that arrives before its project exists is not re-filed later on its
own. Relink it, or let the next message in the thread carry the filing.

### Filing under a project is permanent

Filing an email under a project starts a retention clock. Read this before you
file anything.

Under the German rules pack, an email linked to a project is business
correspondence and must be kept **six years from the end of the calendar year in
which it was sent or received** — its own date, not the day you filed it.

The mark is written the moment the link is made, by any route. **Moving the
email off the project does not remove it.** An erasure request will then hold
that message under a restriction rather than deleting it, and it appears on the
Restricted records page with the project's name as the reason.

## Fixing a mistake: Relink

Any message on any timeline has a **Relink** action. The **Relink this activity**
dialog searches contacts, companies, deals, leads and projects, and offers two
choices:

- **Replace existing link** — "Replaces the existing link of the same type
  instead of adding one."
- **Move rest of thread** — "Every message in this thread that you can edit
  moves in one step."

Relinking *to a project* asks for confirmation first, because of the retention
rule above.

### How do I see all emails with a contact?
To see every email with a contact in Margince, open the contact and choose its **History** tab. There is no separate Mail tab.
1. Under **Timeline filter**, choose **Threads** to see the email conversations, or **All** for everything.
2. Or set **Activity kind** to **Email**, and use **Search this timeline** to find one message.
You see only the messages whose audience includes you; a held message stays with those who were on it. See [Who can see an email](who-can-see-an-email.md).
Also called: email history, correspondence, all mails with a customer.

### How do I move an email to the right contact or deal?
To move a captured email that was filed on the wrong record, choose **Relink** on its row in the timeline, search for the right contact, company, deal, lead or project, tick **Replace existing link** to swap it rather than add a second link, and choose **Relink**. Tick **Move rest of thread** to move the whole conversation.
Also called: email on the wrong deal, refile an email, change which deal an email belongs to.

## What capture refuses to do

These refusals are the product's character. They are not gaps.

**Spam, junk, trash and drafts are never captured.** Whichever provider your
mailbox is on:

| Provider | What capture reads | What it never reads |
|---|---|---|
| Gmail | Your mail, and your sent mail | Spam and Trash — the listing asks Gmail to leave them out, and any message that carries the Spam or Trash label when it is read is refused even so. Drafts, by the same label rule. |
| Microsoft 365 / Outlook | Inbox and Sent Items | Junk Email, Deleted Items and Drafts, which are simply never followed. |
| IMAP | The folder you configured (usually INBOX), and your Sent folder | Junk and Trash folders, which are never opened. Other folders are not read either. |

The Gmail row is two rules rather than one because listing the mail and reading
it are separate calls, and a message can be moved to Spam or Trash in between.
The listing narrows what is offered; the label check refuses what turns up
anyway.

Note what this does **not** cover: everything else in the folder capture does
read. Newsletters, personal mail and Gmail's Promotions and Social categories
all arrive, and are dealt with after capture rather than before it — see *What
happens to one message*.

**Colleagues talking to each other is never stored.** If every address on a
message belongs to your own domains, capture writes a one-line log row and drops
it. The app is blunt: "Messages between colleagues are not stored for anyone,
including you."

The check runs **before** the message is stored, on purpose, so no copy exists
anywhere. The log row does not record the address or the subject — recording
them would leak exactly what dropping the message was meant to prevent.

This extends further than readers expect: a colleague's recap *about* a client is
still internal mail. A recap about a client is not correspondence with one.

**Capture cannot send anything.** It is read-only. Sending is a completely
separate path, staged as its own record and re-checked against the sender's live
permissions at the moment of transmission. See [Writing and sending
mail](sending-mail.md).

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
contact on a channel, or they have blocked the bot, there is no reply button.
And a send without consent for that purpose is blocked outright, with a
**Review consent** link rather than a silent failure.

**The capture trace names the sender and keeps a bounded subject** — one address
and one subject line, never a body, deleted with the row after 24 hours. That is
what lets the log answer why a message did not arrive; without it the page is a
list of decisions naming nobody. You see only rows from your own connections.

An operator can turn this off in the deployment file (`capture.trace_payloads:
false`) where a works agreement requires it. There is no in-app switch either
way, so neither posture is one member's to change for their colleagues. When it
is off, the log says so once, above its rows, as a fact about the installation
rather than about any one message.

### How do I stop capturing emails from a domain or address?
To stop Margince capturing mail from a domain or address, open **Settings → Capture activity**, press **New exclusion** in **Capture exclusions**, enter the address or domain, and press **Exclude**.
1. Open **Settings**, then **Capture activity**.
2. In **Capture exclusions**, press **New exclusion**.
3. Choose **Applies to** (**Your mailboxes** or **Whole company**) and the **Kind** (**Address**, **Domain**, or **Label, folder or mailbox**).
4. Enter the value and press **Exclude**.
It applies from the next message; captured mail stays. **Whole company** rules are an administrator's. Undo with **Resume capture of {value}**.
Also called: block a sender, ignore a domain, blacklist, do not log.

### How do I tell Margince a sender is business, or exclude one?
To correct what Margince concluded about a sender, open **Settings → Connections**, find the address in **Senders**, and press **Business** to readmit it or **Exclude** to keep it out.
1. Open **Settings**, then **Connections**, and scroll to **Senders**.
2. Find the address; the **Decision** column shows what was concluded.
3. Press **Business**, or press **Exclude** and confirm **Exclude and destroy**.
**Exclude** creates no contact and destroys the mail that sender brought into your mailbox; copies a colleague imported stay theirs. **Undo** withdraws your answer. A decision you make is never overwritten by a later verdict.
Also called: mark as not spam, whitelist a sender.

## Things you control yourself

**Own email domains** (Settings → Capture rules) — the domains that belong to
your company. Adding one is open to a sales seat; changing or removing an entry
that is already there is Admin's and Ops's. This is what makes internal mail
internal. Changing it is irreversible in one direction: "Mail skipped while
registered is never offered again by any mailbox."

**Your other addresses** (Settings → Connections) — a send-as alias, a private
domain you read, an address you forward from. Mail between them is not captured
and never creates a contact. Private to you.

**Capture exclusions** (Settings → Capture activity) — addresses and domains
whose messages never enter the CRM. Your own rules bind only the mailboxes you
connected; the company's rules bind everyone, and only an administrator may
change one of those. "Applies from the next message. Messages already captured
stay."

**Consumer mail domains** (Settings → Capture rules) — which domains count as
personal mailboxes. "Mail from a consumer mailbox creates the contact but never
a company."

**Refused domains** (Settings → Capture rules) — which domains this installation
refuses a company, and what decided each one: a model result, a heuristic, or a
human. Allowing a domain back in reopens the company question rather than merely
clearing a flag.

**Mail visibility** (Settings → Connections, under each mailbox) — who may read
mail from this inbox. Three answers:

- **Held until classified** — the default for every new mailbox. A message stays
  with whoever was on it until a classifier judges the thread ordinary
  business; only then can colleagues read it. Nothing is shared before a
  decision, so a classifier that is down or out of budget leaves mail held
  rather than open.
- **Always held** — the same, minus the classifier. You share a thread yourself,
  one at a time, from the message or from **Held threads**.
- **Shared with the team** — a captured message is readable the moment it lands.
  Off unless an admin allows it for the company, because reading an
  employee's mailbox into a shared CRM is what a works-council agreement covers
  in Germany and Austria. Margince does not verify that one exists.

Changing the posture governs mail captured afterwards. The same dialog offers to
narrow what is already captured; it only ever narrows, because re-opening what
was captured under a stricter answer is a separate decision.

**Senders** (Settings → Connections) — every address your mailbox brought in and
what the classifier concluded about each: a contact, a role mailbox, an automated
tool, a newsletter, an advisor, personal. You can overrule any of them.
**Business** readmits a sender; **Exclude** destroys what they brought into your
mailbox and stops the next message. A decision you make is never overwritten by
a later verdict.

**Held threads** (Settings → Connections) — the threads your mailbox is
withholding right now, with the reason. **Share with the team** releases one.

**Private correspondence** (a contact's or company's page) — keep your mail with
one party to the participants on it, without deciding message by message. A domain
hold covers the whole firm, which is usually what you want for a lawyer or an
accountant. It binds mail from then on, and lifting it re-opens nothing.

**Email sharing** (Settings → Capture rules, admin) — the company-wide floor. On
by default; turned off, every message captured from then on is held to its
participants whatever any mailbox asks for. The app warns you honestly: "With
email sharing off, the CRM is hard to use." See [Who can see an
email](who-can-see-an-email.md).

## Whose capture activity you can see

Your own connections need no permission from anyone — it is your own mail.

Messages that arrived through a company-wide connection, like the Telegram
bot, belong to nobody in particular and are shown to seats granted that access.

**No permission grant ever reaches a colleague's mailbox.** The company-wide
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
