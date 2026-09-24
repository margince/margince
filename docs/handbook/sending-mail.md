# Writing and sending mail

[Capture](capture.md) is how mail comes **in**. This page is how it goes **out**:
the composer, what is checked before a send lands, and scheduling one for later.

Who may then read what you sent is a separate question —
[Who can see an email](who-can-see-an-email.md).

### How do I send an email to a contact?
To send an email to a contact in Margince, open the contact's page and choose **Email** in its header, fill in the composer and press **Send**.
1. Open the contact. The button may read **Write**, or **Message on {transport}** when chat is the only channel.
2. Check **To** (at least one recipient), add **Cc** or **Bcc** if you need them.
3. Fill **Subject** and the message **Body**.
4. Pick a **Reason for contact**.
5. Press **Send**, review the **Send email** dialog and press **Send** again.
"No address and no thread to reply to." means the contact has no address to write to.
Also called: write an email, email a customer, message a client.

### Where can I start writing an email?
To start an email in Margince, use the contact header's **Email** button, **Write email** or **Draft reply** in a record's **Email** panel, **Send email** on a deal, **New email** or **Write email** on Home, or **Reply** on any message in a timeline.
Every entry point opens the same composer, so the fields, checks and scheduling are identical wherever you begin.
A company page opens the composer with the company's contacts to choose from; with none on file it says "No contacts at this company yet. Write the message manually, or add a contact first."
Also called: compose, new message, reply to an email.

### What do the composer's error messages mean?
The composer in Margince refuses to send until four things are filled in, and names each one under its field.
- "Add at least one recipient." — **To** is empty.
- "Enter a subject." — **Subject** is empty.
- "Enter a message before sending." — the **Body** is empty.
- "Select the reason for contact." — no **Reason for contact** is chosen. A reply to their own message needs none.
A channel reply, such as Telegram, has no subject, so only the body and the reason are checked.
Also called: validation error, why is Send not working.

### Can I use an email template?
Margince has no email templates, snippets or canned replies in the composer. To start from something ready-made, press **Draft with AI**, which drafts in your own writing voice, or keep a message with **Save as draft** and return to it. Your **Email signature** is added for you.
Offer templates are a different thing: they shape an offer, not an email. See [Offers and the rate card](offers-and-products.md).
Also called: canned response, saved reply, mail template, snippet.

## The composer

The Margince composer is reached from a contact, a company, a deal, a lead, or
the reply action on any message.

To and Cc stand on the form; **Bcc is a button until you ask for it**, and says
what it means: "Other recipients cannot see these addresses or that anyone else
was copied."

A channel reply — Telegram — drops the subject and Cc, because the channel has
neither.

### How do I attach a file to an email?
To attach a file to an email in Margince, press the paperclip (**Attach**) in the composer and pick a file from **On this record** or choose **Upload file**.
1. In the composer, press **Attach**.
2. Pick a file under **On this record**, or use **Upload file** ("Drop a file here, or choose one").
3. The file shows in the message; remove it with its **Remove** button.
One message carries at most 10 files: "A message can include at most 10 files. Send the rest in a second message."
Also called: add an attachment, send a document, attach a PDF.

### Attaching files

Composer attachments come from two sources, both behind the paperclip.

**On this record** offers the files already filed there — a shelf, not a file
browser, so it shows the most recent 25.

**Upload file** uploads one, and files it on the record *first*: "The file is
stored on this record first, so the history keeps every attachment sent."

**Ten files is the limit for one message.** At the cap: "A message can include
at most 10 files. Send the rest in a second message."

A reply does not offer back the files that came in with the message you are
answering.

### How do I use AI to draft an email?
To have AI draft an email in Margince, open the composer, describe what the email is for in **Purpose of the email**, and press **Draft with AI**.
1. Open the composer from a contact, company or deal.
2. On a company, choose the contact under **Draft to**, and optionally the deal under **Related to**.
3. Type the purpose, for example "follow up on last week's quote".
4. Press **Draft with AI** (**Draft reply with AI** when replying). It shows **Drafting…** while it writes.
5. Read and edit the draft, then send it as usual.
With no model configured, AI drafting is unavailable and you write the email yourself.
Also called: AI writer, generate an email, write it for me.

### Drafting with AI

**Draft with AI** writes an email from the record's own context, in your voice.
It tells you what it read — "Based on: {inputs}" — and **Why this draft?** opens
the reasoning.

Every AI draft is marked **AI-assisted draft**: "This draft was written by AI.
Review and edit it before sending."

Your voice profile carries a version, shown as "Built from your writing samples ·
v{n}". A profile still being built is marked **Provisional voice**, and the
product is careful to say what that does *not* mean: "Your Voice DNA is still
being built. It shapes this draft the same way a finished one would."

If the profile could not be loaded, it refuses to pretend:

> **This draft is not in your voice.** Your voice profile could not be loaded,
> so this draft does not use your voice. Draft again, or edit before sending.

**Discard draft**: "Marks this draft as a miss for your Voice DNA. The generated
text is never kept."

**Save as draft** keeps an unfinished message to come back to; it does not send
anything.

### The four rewrites

The AI draft offers four rewrites, under **Rewrite**, over text the model wrote
and you have not edited yet: **Shorter**, **Warmer**, **More formal**, **Add a
deadline**.

They stop being offered the moment you type, because they rewrite the model's
text rather than yours.

### Why are you writing?
The **Reason for contact** field says why you are writing:

> **The record determines what is allowed; the reason lets the send be checked
> against it.**

Eight answers: Follow-up they requested · Active deal · Quote or proposal they
requested · Support for a purchase · Invoice or payment · Their contract · Their
customer relationship · Marketing.

**You are not asked at all when you are replying** to their own message: "This
replies to the recipient's own message, so no reason is needed."

The reason drives the consent check, which runs *before* you send and shows one
of three answers: **Ready to send**, **No recorded reason to contact this
recipient**, or **Message cannot be sent**. An unanswered check is never read as
permission: "The send check did not complete. Sending runs the check again."

Note what is deliberately missing from that list. Several purposes the system
recognises are not offered here, because a sender who could claim one could dress
marketing up as a security notice.

### Why can't I send an email to this contact?
When Margince will not send an email to a contact, the composer names the reason above the **Send** button and, where there is one, the fix.
- **Send blocked: no consent** — no consent for this purpose. Use **Review consent**, or **Request review** where offered.
- **No recorded reason to contact this recipient** — press **Record reason for contact** and say why, under your name.
- **Message cannot be sent** — a reason nobody can lift, such as "The recipient objected to marketing."
- A capture-only mailbox — press **Reconnect your mailbox** and approve sending.
Also called: email blocked, cannot email customer, send refused.

## What a send refuses, and what it merely warns about

A send without consent is a refusal:

> **Send blocked: no consent** — "A recipient has not granted consent for this
> purpose, so the send was blocked (denied by default)."

The reason is specific, and several of them say plainly that nobody here can
overrule them — "The recipient objected to marketing. No one here can lift this,
including an administrator." Others name the actual fix: "This address does not
accept mail. Correct the address; an override does not apply.", and "Several
records share this address, so the recipient cannot be identified. Merge the
records to resolve this."

**A bouncing address is a warning, not a block.** "Mail to {addresses} is
bouncing: the last delivery was refused and none has succeeded since. Send
anyway, or use another address."

**A colleague's mailbox** is a note, not a block: "Your reply is sent from your
own mailbox, under your name."

**A mailbox that cannot send** is refused with the remedy: "Your mailbox is
connected for capture only, without permission to send. Reconnect it and approve
sending; a mailbox connected before sending existed cannot be upgraded in place."
The **Reconnect your mailbox** link opens Settings → Connections.

**A message carrying an unsubscribe link reaches one addressee at a time**,
because that link is the recipient's own consent record. Send it once per
recipient, with no Cc.

**Channel limits are explained rather than hit.** Telegram cannot carry files at
all, carries a limited number, caps each file's size, and truncates a caption —
and each of those is its own sentence naming the number.

### How do I set my email signature?
To set your email signature in Margince, open the account menu, choose **Settings**, then **Account**, and use **Edit signature** under **Email signature**.
1. Open the account menu at the top right and choose **Settings**.
2. Open **Account**.
3. Under **Email signature**, press **Edit signature**, type plain text and save.
Leave it empty to send unsigned. The AI never writes a sign-off; this signature is the one that goes out.
Also called: sign-off, email footer, sender signature.

## Your signature

The email signature is set at **Settings → Account**. Plain text, appended below
every message you send and above the unsubscribe footer. Leave it empty to send
unsigned.

> **The AI never writes a sign-off — this is the one that goes out.**

An agent signs nothing at all.

The order in what actually leaves: your message, your signature, any required
disclosure, then the unsubscribe footer.

The copy kept on your timeline has the footer's **token redacted** — the shape is
there, the live link is not, because that link is the recipient's own credential.

### How do I schedule an email to send later?
To schedule an email in Margince, open the caret beside **Send** (**Other ways to send**), choose **Schedule send**, pick a time, and confirm.
1. Write the email as usual.
2. Press the caret beside **Send** and choose **Schedule send**.
3. Pick **Tomorrow morning** (08:00), **Tomorrow afternoon** (13:00) or **Monday morning** (08:00), or **Select date and time** and pick a **Date** and one of 08:00, 09:00, 13:00 or 17:00. **Send now** goes back to sending immediately.
4. Press **Schedule send** in the **Schedule email** dialog.
You see "Email scheduled. It has not been sent yet." with a link to **Scheduled messages**.
Also called: send later, delayed send, timed email.

### How do I cancel or reschedule a scheduled email?
To cancel or move a scheduled email in Margince, open **Scheduled messages** from the command palette (⌘K or Ctrl+K) and use **Reschedule** or **Withdraw** on the message.
1. Press ⌘K (Mac) or Ctrl+K, type "Scheduled messages" and open it. The toast shown after scheduling links there too.
2. To move it, press **Reschedule**, pick the new time and press **Reschedule**. Only the time can change.
3. To cancel it, press **Withdraw**, then **Withdraw message** in "Withdraw this message?".
Scheduled messages has no sidebar row. To change the text, withdraw it and write it again.
Also called: unschedule, delete a scheduled email, change send time.

## Scheduling a send

A scheduled send is set behind the caret on Send: **Schedule send**.

Three presets — **Tomorrow morning**, **Tomorrow afternoon**, **Monday morning**
— or pick a date and one of four times.

The confirmation is explicit about what has and has not happened:

> The email is sent at the selected time, and the consent and mailbox checks run
> again then. Until then it can be moved or canceled from Scheduled messages.

That re-check is the point. A scheduled send is not a decision made today and
executed blindly later.

### The scheduled list

The **Scheduled messages** page: "Your scheduled messages that have not been
sent yet. Only you can see them." It is your own list, never the company's.

It is not in the sidebar — reach it from the command palette, or from the toast
after you schedule something.

Three groups: **Held for your action** · **Scheduled** · **Sent or withdrawn**.

You can **Reschedule** — the time only. Content is what the checks were made
against, so changing that means withdrawing and writing again.

**Withdraw** is the other verb, and it is called that rather than "delete":
"“{subject}” will not be sent, and nothing reaches the timeline. Sending it later
means writing it again."

### Held

A scheduled message is **Held** when the moment came and a check refused it. The
reason says which, and what to do:

- **A recipient withdrew their consent** after you scheduled it.
- **Your seat or mailbox changed**, so it cannot be sent as you.
- **Its send time passed while the system was not running** — "it is now too
  late to send. Reschedule or withdraw it."
- **The send job ran out of attempts.** Reschedule it to retry.
- **A check blocked it when it was due.** Nothing was sent.

A held message also raises an approval card, and it is **the one card that
never expires**. The message is being held and nothing else will reap it, so the
card waits as long as it takes. See [Approvals](approvals.md).

## What the recipient controls

Every marketing message carries a footer with two destinations: unsubscribe, and
manage preferences. Both are the recipient's own private links.

**The preference centre** — "Each purpose is separate. Transactional messages
cannot be switched off here because you need them; all other purposes are yours
to control."

It shows three states per purpose, not two: on because they asked, on because
they have not objected, or off because they asked you to stop. **Only an explicit
objection reads as off.**

**Security and service messages are locked on**, and the page says why: "They
are needed for something you requested, such as a password reset or a
confirmation."

**Stop all marketing** switches off every marketing row at once, and is careful
about what it does not touch: "Switches off every marketing purpose above.
Replies to your own inquiries and anything you requested continue, because no
one subscribed you to those."

There is an **Undo and keep receiving marketing** — and it does not silently
re-subscribe anybody: "Resubscribing is an explicit opt-in and is never switched
back on automatically. Save below to record your consent, or discard."

An unsubscribe link **never acts on arrival**. It shows a page and asks, because
a scanner following a link must not unsubscribe somebody.

Whatever they choose, the exact sentence they were shown is stored with the
decision: "The exact wording you saw and a timestamp are recorded as proof. The
choice then applies to every future email."

### How do I ask a contact to confirm their details or consent?
To ask a contact to confirm their details in Margince, open the contact and press **Ask them to confirm their details** in the **Communication permissions** panel.
1. Open the contact's page.
2. In **Communication permissions**, press **Ask them to confirm their details**.
Margince mails a private link to the contact's own recorded address; you cannot send it anywhere else. The link lasts 14 days and works once.
Also called: double opt-in, consent request, GDPR confirmation.

## Asking somebody to confirm their own details

The confirmation link, sent with **Ask them to confirm their details**, lets the
contact see what you hold about them, correct it, and say whether they want to
hear from you. It goes to their own recorded address; you cannot send it
anywhere else.

The link lasts 14 days and works once. What they see is the whole record you
hold, where each field came from, a plain question about staying in touch, and a
way to ask for removal.

This is also the only way to start a purpose that needs confirming: "Only this
contact can confirm this purpose through a link sent to their recorded address."
A confirmation an employee can complete on the contact's behalf is not evidence
that the contact agreed.

## Marketing sent from the composer

Choosing **Marketing** as the reason for contact names no single subscription,
so the unsubscribe link in that message is minted without a purpose. Pressed, it
stops all marketing to that recipient — the same sweep as **Stop all marketing**
in the preference centre — rather than one subscription.

To let a recipient leave one list and keep the rest, send through a path that
names its purpose.
