# Capture: how conversations get into Margince

Nobody should have to copy an email into a CRM by hand. **Capture** is the part
of Margince that connects to your mailbox, your calendar and your chat, and
files what arrives against the right contacts, their companies and projects.

Connecting a mailbox or calendar, and importing your older mail, is its own
page: [Connecting your mailbox and calendar](connecting-mail-and-calendars.md).

### How does email get into Margince?
Email gets into Margince through a connected mailbox: once you connect Gmail, Outlook or an IMAP mailbox at **Settings → Connections**, capture reads new mail on each sync and files it on the timelines of the contacts on it.
There is no forwarding address and no copy-paste step. Mail between colleagues is never stored, and senders judged to be newsletters, automated tools or spam create no contact.
To bring in older mail, use **Import mailbox history**. To see what happened to one message, open **Settings → Capture activity**.
Also called: email logging, sync email to CRM, auto-log emails.

### How do I see emails I sent from Outlook or Gmail in Margince?
Emails you send from Outlook or Gmail itself are captured into Margince too. A connected Gmail or Outlook mailbox is read in your inbox and your sent mail, so each sent message lands on the recipients' contact timelines marked **Sent**.
1. Connect the mailbox at **Settings → Connections**; mail sent before that comes in with **Import mailbox history**.
2. Open the contact's **History** tab and pick **Threads** or **All** to see what you sent beside their replies.
Mail sent only to colleagues is never stored. An IMAP mailbox reads only the one **Mailbox** it was set up with.
Also called: sent items, outgoing mail, outbox, log sent emails.

## What happens to one message

A captured message passes four stages, in order.

**1. The connector fetches it.** Some things never leave the connector, such as
a chat reaction or a message your own mail rules filtered. Those never reach
Margince at all.

**2. The admission check.** Margince confirms the message is well-formed, within
size limits, and from a source that vouches for the identities on it. It also
checks, every time, that the member it belongs to is still allowed to capture.
The result is either *accepted*, or *skipped* with the reason logged. A skipped
message is not retried.

**3. Filing it.** Margince checks that the address was not erased and that the
message is not internal-only. It then stores the message and adds it to the
timelines of the contacts on it, with its links and attachments. The audit log
records that this happened, never the subject or the body.

**4. Deciding about the sender.** See the ladder below.

You can watch all of this in **Settings → Capture activity**, under "What the
last 24 hours of your mail produced". Under **Messages**, open any single
message with **Show every processing step for this message**. **How this
message was handled** lists every step in order, each marked Done, Skipped,
Waiting, Failed, Not applicable, or Unknown.

### Why was my email not captured?
To find out why an email is missing from Margince, open **Settings → Capture activity** and find it under **Messages**. Then press **Show every processing step for this message**.
**How this message was handled** marks each step Done, Skipped, Waiting, Failed, Not applicable or Unknown, with the reason.
Common causes: every address was on your own domains, so it was never stored; the sender is on your **Capture exclusions**; the sender was judged a newsletter or automated tool; or the mailbox shows **Needs reconnect**.
The log keeps 24 hours and shows only your own connections.
Also called: missing email, email did not sync, email not logged.

## Who the message belongs to

Margince decides about the sender with plain rules, checked in order. No AI
model is involved in any of them.

**Is the sender a colleague?** Then judge the external contact on the message
instead. If *everyone* on it is internal, create nothing.

**Have we sent mail to this address before?** If provably so, create the contact.
This beats every rule below it, including an old decision that the sender was
noise. Only proof that *we* sent counts, never the `From` header, which anyone
can forge. One outbound message counts unless its text was a refusal ("not
interested", "unsubscribe"); two or more always count.

**Is it mail infrastructure?** (DocuSign, a mail relay.) Keep the message.
Create no contact and no company.

**Have we already decided about this address?** Reuse that decision.

**Is it a consumer mail domain?** Margince ships with a long list of known
consumer-mail domains, and the Capture rules screen prints the current count.
If it is one of those, create the contact, but no company.

**Nobody knows who this is.** Create nothing yet, and note the open question.

### Capture never creates a company

Capture creates the contact but not their company. A separate step reads the
domain's website and proposes the company, so a personal domain such as
`sebastian@kestner.example` does not become a company called "Kestner".

## When nothing matches

An unknown sender goes into a queue, which Margince works through every ten
minutes.

**An AI model is asked one question:** what kind of sender is this? It is asked
once per *sender*, never per message, and only for senders the plain rules could
not classify. The possible answers:

- **A contact**: create the contact, and check whether the domain deserves a
  company.
- **A role mailbox** or **a company sender**: keep the mail visible, create no
  contact.
- **A newsletter**, **something transactional**, or **spam**: hide the mail and
  mark the domain as not a company.

**The model never decides what is kept.** It only decides whether
a *contact* is created. If your installation has no model configured at all,
capture works normally and only this judging step is skipped.

### Low confidence never becomes a deletion

Below a confidence of 0.7 the sender is asked about once more on its own. If it
is still below, it becomes **unsure** and goes to a human. A low score costs an
extra question, never a wrong deletion.

An unsure sender reaches you as an approval card, **Add contact from mail**.
Accept creates the contact. **Reject does nothing at all**: the mail stays where
it is. These proposals can only add, so rejecting one by mistake deletes
nothing.

### Two safety limits

Margince holds at most **500 open sender questions** per company, and at
most **50 from any one sender domain**. When either limit is hit, the log
records *which* one, so you can tell "the queue is full" from "one domain is
flooding it".

### Why was my email not linked to a deal?
Margince does not link captured email to a deal on its own: capture files a message under the contacts on it, their company and, where a rule matches, a project. To put an email on a deal, use **Relink** on the message.
1. Open the contact or company timeline and find the email.
2. Press **Relink**.
3. Search for the deal under "Search contacts, companies, deals, leads or projects" and pick it.
4. Leave **Replace existing link** off to add the deal beside the current links. Tick **Move rest of thread** to bring the whole thread.
5. Press **Relink**.
Also called: email missing from opportunity, attach email to deal, associate email.

## How a message finds its project

If you use projects, a captured message is filed against one by three fixed
rules, first match wins. No AI model is used in any of them.

**1. The conversation.** A reply to a thread already filed goes under the same
project. Matching stays within one channel, so an email cannot be filed onto a
chat conversation.

**2. The deal.** A message already linked to a deal is filed under that deal's
project.

**3. The key in the subject.** Every project gets a short key. A subject
carrying `[ERP-27]` is filed under that project. The brackets are required: a
bare `ERP-27` is not a reference, so a project keyed `RE` does not catch every
reply in your mailbox.

If a message points two ways (two different keys in one subject, or two deals
in two different projects), that rule files nothing and the next one is tried.

If nothing matches, nothing happens: no link, and no question raised. Margince
does not file by guesswork. Every rule above is either an exact match or
confirmed by a person.

A message that arrives before its project exists is not re-filed later on its
own. Relink it, or let the next message in the thread carry the filing.

### Filing under a project starts a retention clock

Under the German rules pack, an email linked to a project is business
correspondence and must be kept **six years from the end of the calendar year in
which it was sent or received**. The clock runs from the email's own date, not
the day you filed it.

The mark is set the moment the link is made, by any route. Moving the email
off the project **does not remove the mark**. An erasure request will then hold that
message under a restriction instead of deleting it, and it appears on the
Restricted records page with the project's name as the reason.

To take a mistaken filing back, use **Undo filing** on the project's timeline
and give a written reason. Undo is refused while something else keeps the
message: a won deal or sent offer, a controller's pin, a legal hold, an erasure
request or a statutory hold. An assistant cannot undo a filing.

## Fixing a mistake: Relink

Any message on any timeline has a **Relink** action. The **Relink this activity**
dialog searches contacts, companies, deals, leads and projects, and offers two
choices:

- **Replace existing link**: "Replaces the existing link of the same type
  instead of adding one."
- **Move rest of thread**: "Every message in this thread that you can edit
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
To move a captured email that was filed on the wrong record, choose **Relink** on it in the timeline and search for the right contact, company, deal, lead or project. Tick **Replace existing link** to swap it instead of adding a second link, and choose **Relink**. Tick **Move rest of thread** to move the whole conversation.
Also called: email on the wrong deal, refile an email, change which deal an email belongs to.

## What capture refuses to do

**Spam, junk, trash and drafts are never captured.** Whichever provider your
mailbox is on:

| Provider | What capture reads | What it never reads |
|---|---|---|
| Gmail | Your mail, and your sent mail | Spam and Trash. Gmail is asked to leave them out, and a message that carries the Spam or Trash label when it is read is refused anyway. Drafts, by the same label rule. |
| Microsoft 365 / Outlook | Inbox and Sent Items | Junk Email, Deleted Items and Drafts, which are never opened. |
| IMAP | The one folder you configured (usually INBOX); point it at your Sent folder and its mail counts as sent by you | Every other folder, Junk and Trash included, is never opened. |

Everything else in the folder capture reads does arrive: newsletters, personal
mail, and Gmail's Promotions and Social categories. They are dealt with after
capture, as described in *What happens to one message*.

**Deleting in your own mailbox sometimes reaches Margince.** If you
delete a captured message in Gmail or Outlook, Margince notices and acts on it.
What happens then depends on who else has it:

| Situation | What happens |
|---|---|
| Nobody else imported it | Deleted here too: text, the provider's original, attachments, and everything derived from it |
| A colleague also imported it | Kept. Their copy is theirs, and your tidying does not reach their timeline |
| It is commercial correspondence inside its retention window, or an erasure request is still open about it | Kept, and the reason is recorded |

The last row matters most. A business letter (Handelsbrief) cannot be deleted
this way while the law requires keeping it, however you delete it at the
provider. The record says why it was kept.

**Mail between colleagues is never stored.** If every address on a message
belongs to your own domains, capture logs one line and drops it. The app says:
"Messages between colleagues are not stored for anyone, including you."

The check runs before the message is stored, so no copy exists anywhere. The
log line does not record the address or the subject, because those would reveal
what dropping the message keeps private.

This covers more than readers expect. A colleague's recap *about* a client is
still internal mail, because it is not correspondence with the client.

**Capture cannot send anything.** It only reads. Sending is separate: each
message is saved as its own record and checked against the sender's current
permissions at the moment it goes out. See [Writing and sending
mail](sending-mail.md).

**Passwords are never stored in plain text.** This covers app passwords and mailbox access keys. If secure storage
for them is not set up, the connect screen refuses the connection.

**IMAP cannot reach your internal network.** An IMAP connection cannot point at
an address inside your own network. This is a security guard.

**Telegram captures private chats only.** Group messages are refused before
anything is stored. Attachments are named (`[photo]`, `[document]`), not
downloaded. Telegram has no history import. The company has one Telegram bot,
because a second bot would stop replies working on both.

**No reply button where a reply would fail.** If Margince cannot reach a
contact on a channel, or they have blocked the bot, there is no reply button.
A send without consent for that purpose is blocked, with a **Review consent**
link.

**The capture log names the sender.** It keeps one address and one shortened
subject line, never a body, and deletes them after 24 hours. That is how the log
can tell you why a message did not arrive. You see only entries from your own
connections.

Whoever runs your installation can turn this off, for example where a works
agreement requires it. There is no switch in the app, so no member can change it
for their colleagues. When it is off, the log says so once, above its entries.

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

**Own email domains** (Settings → Capture rules): the domains that belong to
your company. A sales seat can add one; changing or removing an existing entry
is for Admin and Ops. This is what makes internal mail internal. Adding one
cannot be undone for mail already skipped: "Mail skipped while registered is
never offered again by any mailbox."

**Your other addresses** (Settings → Connections): a send-as alias, a private
domain you read, an address you forward from. Mail between them is not captured
and never creates a contact. Private to you.

**Capture exclusions** (Settings → Capture activity): addresses and domains
whose messages never enter the CRM. Your own rules cover only the mailboxes you
connected. The company's rules cover everyone, and only an administrator may
change one of those. "Applies from the next message. Messages already captured
stay."

**Consumer mail domains** (Settings → Capture rules): which domains count as
personal mailboxes. "Mail from a consumer mailbox creates the contact but never
a company."

**Refused domains** (Settings → Capture rules): the domains this installation
will not create a company for, and what decided each one: an AI model, a fixed
rule, or a human. Allowing a domain back in asks the company question again.

**Senders** (Settings → Connections): every address your mailbox brought in and
what Margince concluded about each: a contact, a role mailbox, an automated
tool, a newsletter, an advisor, personal. You can overrule any of them.
**Business** readmits a sender; **Exclude** destroys what they brought into your
mailbox and stops the next message. A decision you make is never overwritten by
a later verdict. **Waiting on a decision**, under the list, names the contacts
only you see until their sender is decided. An administrator sees only how many
there are.

**Held threads** (Settings → Connections): the threads your mailbox is
withholding right now, with the reason. **Share with the team** releases one.

**Mail visibility** (Settings → Connections, under each mailbox),
**Private correspondence** (a contact's or company's page) and **Email sharing**
(Settings → Capture rules, administrators only) decide who may read captured
mail. They are explained in [Who can see an email](who-can-see-an-email.md),
together with whose capture activity you can see.
