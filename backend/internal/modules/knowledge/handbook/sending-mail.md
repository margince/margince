# Writing and sending mail

[Capture](capture.md) is how mail comes **in**. This page is how it goes **out**:
the composer, what is checked before a send lands, and scheduling one for later.

Who may then read what you sent is a separate question, answered in
[Who can see an email](who-can-see-an-email.md).

### How do I send an email to a contact?
To send an email to a contact in Margince, open the contact's page and choose **Email** in its header, fill in the composer and press **Send**.
1. Open the contact. The button may read **Write**, or **Message on {transport}** when chat is the only channel.
2. Check **To** (at least one recipient), add **Cc** or **Bcc** if you need them.
3. Fill **Subject** and the message **Body**.
4. Pick a **Reason for contact**.
5. Review the draft in the **Send email** dialog, then press **Send** once.
"No address and no thread to reply to." means the contact has no address to write to.
Also called: write an email, email a customer, message a client.

### Where can I start writing an email?
To start an email in Margince, use any of these:
- the contact header's **Email** button
- **Write email** or **Draft reply** in a record's **Email** panel
- **Send email** on a deal
- **New email** or **Write email** on Home
- **Reply** on any message in a timeline.
Every entry point opens the same composer, so the fields, checks and scheduling are identical wherever you begin.
A company page opens the composer with the company's contacts to choose from; with none on file it says "No contacts at this company yet. Write the message manually, or add a contact first."
Also called: compose, new message, reply to an email.

### How do I send a follow-up email?
To send a follow-up email in Margince, open the contact's **History** tab and press **Reply** on the message to follow up, or press the suggested verb on its Worklist row.
1. On the record, press **Reply** on the email. Following your own email, the composer reads "Following up on your email “{subject}”"; answering theirs, "Replying to “{subject}”".
2. From **Home** → **Show Worklist**, press **Read and reply**, **Draft reply** or **Write email** on the row.
3. Pick a **Reason for contact** (a reply to their own message needs none) and press **Send**, or schedule it.
Also called: chase a customer, nudge, follow up, reply to a thread.

### How do I reply to one particular message in a thread?
To reply to one particular message in Margince, open the composer from the record, pick the conversation under **Continue thread?**, then click the exact message in the **This thread** column.
The chosen message is highlighted. The line above the recipients reads "Replying to “{subject}” · {when}", or "Following up on your email" for one you sent. Hover **Preview** to see a message's text, and press **Read full email** to open it whole. Press **New email** to leave the thread for a fresh message. Each message you pick keeps its own draft while the composer is open. Also called: answer an older email, reply to a specific message.

### How do I file an email I send under a project?
To file an email you send under a project in Margince, choose the project under **Project** in the composer; Margince puts its key, such as `[NER-1]`, at the front of the **Subject**.
Choosing **No project** takes the tag out; deleting it from the text does not. The picker starts on the thread's own project, else the deal's, else the company's only live project. Writing from a project page files under that project. With no live project to offer, the picker is not shown. Also called: tag an email with a project, project key in the subject.

### What do the composer's error messages mean?
The composer in Margince refuses to send until four things are filled in, and names each one under its field.
- "Add at least one recipient.": **To** is empty.
- "Enter a subject.": **Subject** is empty.
- "Enter a message before sending.": the **Body** is empty.
- "Select the reason for contact.": no **Reason for contact** is chosen. A reply to their own message needs none.
A channel reply, such as Telegram, has no subject, so only the body and the reason are checked.
Also called: validation error, why is Send not working.

### Can I use an email template?
Margince has no email templates, snippets or canned replies in the composer. To start from something ready-made, press **Draft with AI**, which drafts in your own writing voice, or keep a message with **Save as draft** and return to it. Your **Email signature** is added for you.
Offer templates are a different thing: they shape an offer, not an email. See [Offers and the rate card](offers-and-products.md).
Also called: canned response, saved reply, mail template, snippet.

## The composer

The Margince composer is reached from a contact, a company, a deal, a lead, or
the reply action on any message.

To and Cc stand on the form. **Bcc** is a button until you press it, and says
what it means: "Other recipients cannot see these addresses or that anyone else
was copied."

A channel reply, such as Telegram, drops the subject and Cc, because the channel has
neither.

### How do I attach a file to an email?
To attach a file to an email in Margince, press the paperclip (**Attach**) in the composer and pick a file from **On this record** or choose **Upload file**.
1. In the composer, press **Attach**.
2. Pick a file under **On this record**, or use **Upload file** ("Drop a file here, or choose one").
3. The file shows in the message; remove it with its **Remove** button.

**On this record** shows the 25 most recent files filed on the record. **Upload
file** files the upload on the record before it is attached: "The file is stored
on this record first, so the history keeps every attachment sent."

One message carries at most 10 files: "A message can include at most 10 files. Send the rest in a second message."
A reply does not offer back the files that came in with the message you are
answering.
Also called: add an attachment, send a document, attach a PDF.

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
It tells you what it read ("Based on: {inputs}"), and **Why this draft?** opens
the reasoning.

Every AI draft is marked **AI-assisted draft**: "This draft was written by AI.
Review and edit it before sending."

Your voice profile carries a version, shown as "Built from your writing samples ·
v{n}". A profile still being built is marked **Provisional voice**: "Your Voice
DNA is still being built. It shapes this draft the same way a finished one
would."

If the profile could not be loaded, the draft says so:

> **This draft is not in your voice.**
> Your voice profile could not be loaded,
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
The **Reason for contact** field says why you are writing. The composer explains
it:

> The record determines what is allowed; the reason lets the send be checked
> against it.

Eight answers: Follow-up they requested · Active deal · Quote or proposal they
requested · Support for a purchase · Invoice or payment · Their contract · Their
customer relationship · Marketing.

You are not asked for a reason when you reply to their own message: "This
replies to the recipient's own message, so no reason is needed."

The reason drives the consent check, which runs *before* you send and shows one
of three answers: **Ready to send**, **No recorded reason to contact this
recipient**, or **Message cannot be sent**. An unanswered check is never read as
permission: "The send check did not complete. Sending runs the check again."

Some purposes Margince recognises, such as a security notice, are not offered
here. A sender who could pick one could send marketing dressed up as a security
notice.

### Why can't I send an email to this contact?
When Margince will not send an email to a contact, the composer names the reason above the **Send** button and, where there is one, the fix.
- **Send blocked: no consent**: no consent for this purpose. Use **Review consent**, or **Request review** where offered.
- **No recorded reason to contact this recipient**: press **Record reason for contact** and say why, under your name.
- **Message cannot be sent**: a reason nobody can lift, such as "The recipient objected to marketing."
- A capture-only mailbox: press **Reconnect your mailbox** and approve sending.
Also called: email blocked, cannot email customer, send refused.

## What a send refuses, and what it merely warns about

A send without consent is a refusal:

> **Send blocked: no consent**
> "A recipient has not granted consent for this purpose, so the send was blocked
> (denied by default)."

The reason is specific. Some say that nobody can overrule them: "The recipient
objected to marketing. No one here can lift this, including an administrator."
Others name the actual fix: "This address does not
accept mail. Correct the address; an override does not apply.", and "Several
records share this address, so the recipient cannot be identified. Merge the
records to resolve this."

**A bouncing address is a warning.** You can still send. "Mail to {addresses} is
bouncing: the last delivery was refused and none has succeeded since. Send
anyway, or use another address."

**A colleague's mailbox** gets a note, and you can still send: "Your reply is sent from your
own mailbox, under your name."

**A mailbox that cannot send** is refused with the remedy: "Your mailbox is
connected for capture only, without permission to send. Reconnect it and approve
sending; a mailbox connected before sending existed cannot be upgraded in place."
The **Reconnect your mailbox** link opens Settings → Connections.

**One addressee per unsubscribe link.** A message carrying an unsubscribe link
reaches one addressee at a time, because that link is the recipient's own consent record. Send it once per
recipient, with no Cc.

**Chat channel limits.** A channel such as Telegram may carry no files at all,
or limit the number of files, each file's size, their total size and the length
of the caption. When a message breaks one, the composer names the limit and the
number before you send.

### How do I set my email signature?
To set your email signature in Margince, open the account menu, choose **Settings**, then **Account**, and use **Edit signature** under **Email signature**.
1. Open the account menu at the top right and choose **Settings**.
2. Open **Account**.
3. Under **Email signature**, press **Edit signature**, type plain text and save.

Leave it empty and a send closes with a short greeting and your name, such as
"Best regards". The greeting follows the message's language. A message too short
to tell uses the installation's language, then English. The composer shows the
sign-off under the body before you send. The AI never writes a sign-off; this
signature is the one that goes out. A message an agent sends carries no sign-off.

A sent message reads, in order: your message, your signature, any required
disclosure, then the unsubscribe footer. The copy on your timeline leaves out the
recipient's personal unsubscribe link, because that link acts for the recipient.
Also called: sign-off, email footer, sender signature.

### How do I schedule an email to send later?
To schedule an email in Margince, open the caret beside **Send** (**Other ways to send**), choose **Schedule send**, pick a time, and confirm.
1. Write the email as usual.
2. Press the caret beside **Send** and choose **Schedule send**.
3. Pick **Tomorrow morning** (08:00), **Tomorrow afternoon** (13:00) or **Monday morning** (08:00), or **Select date and time** and pick a **Date** and one of 08:00, 09:00, 13:00 or 17:00. **Send now** goes back to sending immediately.
4. Press **Schedule send** in the **Schedule email** dialog.

You see "Email scheduled. It has not been sent yet." with a link to **Scheduled messages**. The dialog says what happens next:

> The email is sent at the selected time, and the consent and mailbox checks run
> again then. Until then it can be moved or canceled from Scheduled messages.

Also called: send later, delayed send, timed email.

### How do I cancel or reschedule a scheduled email?
To cancel or move a scheduled email in Margince, open **Scheduled messages** from the command palette (⌘K or Ctrl+K) and use **Reschedule** or **Withdraw** on the message.
1. Press ⌘K (Mac) or Ctrl+K, type "Scheduled messages" and open it. The toast shown after scheduling links there too.
2. To move it, press **Reschedule**, pick the new time and press **Reschedule**. Only the time can change.
3. To cancel it, press **Withdraw**, then **Withdraw message** in "Withdraw this message?".
Also called: unschedule, delete a scheduled email, change send time.

The **Scheduled messages** page: "Your scheduled messages that have not been
sent yet. Only you can see them." It has no sidebar row, and it groups messages
as **Held for your action**, **Scheduled** and **Sent or withdrawn**.

**Reschedule** changes the time only. The checks were made against the content,
so to change the text, withdraw the message and write it again. **Withdraw**
says: "“{subject}” will not be sent, and nothing reaches the timeline. Sending
it later means writing it again."

### Held

A scheduled message is **Held** when the moment came and a check refused it. The
reason says which, and what to do:

- **A recipient withdrew their consent** after you scheduled it.
- **Your seat or mailbox changed**, so it cannot be sent as you.
- **Its send time passed** while the system was not running, and it is now too
  late to send. Reschedule or withdraw it.
- **The send job ran out of attempts.** Reschedule it to retry.
- **A check blocked it when it was due.** Nothing was sent.

A held message also raises an approval card that never expires. See
[Approvals](approvals.md).

## What the recipient controls

Every marketing message carries a footer with two destinations: unsubscribe, and
manage preferences. Both are the recipient's own private links.

**The preference centre**: "Each purpose is separate. Transactional messages
cannot be switched off here because you need them; all other purposes are yours
to control."

It shows three states per purpose: on because they asked, on because they have
not objected, or off because they asked you to stop. Only an explicit objection
reads as off.

**Security and service messages are locked on**, and the page says why: "They
are needed for something you requested, such as a password reset or a
confirmation."

**Stop all marketing** switches off every marketing purpose at once and leaves
the rest alone: "Switches off every marketing purpose above.
Replies to your own inquiries and anything you requested continue, because no
one subscribed you to those."

**Undo and keep receiving marketing** does not re-subscribe anybody by itself:
"Resubscribing is an explicit opt-in and is never switched
back on automatically. Save below to record your consent, or discard."

An unsubscribe link **never acts on arrival**. It shows a page and asks, because
a scanner following a link must not unsubscribe somebody.

Whatever they choose, the sentence they were shown is stored with the decision: "The exact wording you saw and a timestamp are recorded as proof. The
choice then applies to every future email."

### How do I ask a contact to confirm their details or consent?
To ask a contact to confirm their details in Margince, open the contact and press **Ask them to confirm their details** in the **Communication permissions** panel.
1. Open the contact's page.
2. In **Communication permissions**, press **Ask them to confirm their details**.
Margince mails a private link to the contact's own recorded address; you cannot send it anywhere else. The link lasts 14 days and works once.
Also called: double opt-in, consent request, GDPR confirmation.

### How do I send a privacy notice to a contact?
To send a privacy notice in Margince, open the **Privacy notice owed** item on your Worklist (**Home**, then **Show Worklist**) and press **Send privacy notice**.
1. Open the **Privacy notice owed** item.
2. Press **Send privacy notice**, or **Ask them to confirm their details**, which tells them too.
3. If they already know, or an exemption applies, press **End the duty…** and record why.
The notice goes to the contact's own recorded address. If it bounces, the duty is owed again.
Also called: GDPR Art. 14 notice, disclosure duty, information obligation.

### Why does Margince say "this installation cannot send mail"?
"Not sent: this installation cannot send mail" means your installation has no SMTP relay set up, so the privacy notice or confirmation link was not sent and the duty stays open.
This mail never goes through your connected Gmail or Outlook: it carries a private link to the contact's own record, which must not sit in your Sent folder. Ask your administrator to set up the installation's outgoing mail. Until then, tell the contact another way and press **End the duty…**.
Also called: privacy notice not sent, cannot send notice.

### Which mailbox does Margince send from?
Mail you write in Margince goes out from your own connected mailbox, so the contact sees your address.
Mail Margince writes by itself goes out from the installation's own address, never from yours: the privacy notice, the confirmation link, password reset, invitations, and the emailed weekly review and morning brief. Your administrator sets that address up.
Also called: sender address, from address, send as me.

## Asking somebody to confirm their own details

The confirmation link, sent with **Ask them to confirm their details**, lets the
contact see what you hold about them, correct it, and say whether they want to
hear from you. It goes to their own recorded address; you cannot send it
anywhere else.

The link lasts 14 days and works once. What they see is the whole record you
hold, where each field came from, a plain question about staying in touch, and a
way to ask for removal.

This is also the only way to start a purpose that needs confirming. The screen
says: "Only this contact can confirm this purpose through a link sent to their
recorded address."

A confirmation an employee can complete on the contact's behalf is not evidence
that the contact agreed.

## Marketing sent from the composer

Choosing **Marketing** as the reason for contact names no single subscription,
so the unsubscribe link in that message stops all marketing to that recipient,
as **Stop all marketing** in the preference centre does.

To let a recipient leave one list and keep the rest, send through a path that
names its purpose.
