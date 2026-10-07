<!-- prose:plain -->
# Writing and sending mail

[Capture](capture.md) is how mail comes **in**. This page is how it goes **out**:
the composer, what Margince checks before a send goes out, and setting a send
for later.

Who may then read what you sent is a separate question, answered in
[Who can see an email](who-can-see-an-email.md).

### How do I send an email to a contact?
To send an email to a contact in Margince, open the contact's page and choose **Email** in its header. Fill in the composer and press **Send**.
1. Open the contact. The button may read **Write**, or **Message on {transport}** when chat is the only channel.
2. Check **To** (at least one recipient), and add **Cc** or **Bcc** if you need them.
3. Fill in **Subject** and the message **Body**.
4. Pick a **Reason for contact**.
5. Check the draft in the **Send email** dialog, then press **Send** once.

"No address and no thread to reply to." means the contact has no address to write to.
Also called: write an email, email a customer, message a client.

### Where can I start writing an email?
To start an email in Margince, use any of these:
- the contact header's **Email** button
- **Write email** or **Draft reply** in a record's **Email** panel
- **Send email** on a deal
- **New email** or **Write email** on Home
- **Reply** on any message in a timeline.

Every one of these opens the same composer, so the fields, checks and send times are the same from each.
A company page opens the composer with the company's contacts to choose from. With none on file it says "No contacts at this company yet. Write the message manually, or add a contact first."
Also called: compose, new message, reply to an email.

### How do I send a follow-up email?
To send a follow-up email in Margince, open the contact's **History** tab and press **Reply** on the message. Or press the button on its Worklist row.
1. On the record, press **Reply** on the email. After your own email, the composer reads "Following up on your email “{subject}”"; after theirs, "Replying to “{subject}”".
2. From **Home** → **Show Worklist**, press **Read and reply**, **Draft reply** or **Write email** on the row.
3. Pick a **Reason for contact** and press **Send**, or set a time. A reply to their own message needs no reason.
Also called: chase a customer, nudge, follow up, reply to a thread.

### How do I reply to one particular message in a thread?
To reply to one message in Margince, open the composer from the record and pick the conversation under **Continue thread?**. Then click that message in the **This thread** column.
The message you picked is marked. The line above the recipients reads "Replying to “{subject}” · {when}", or "Following up on your email" for one you sent.

Hold the mouse on **Preview** to see a message's text, and press **Read full email** to open all of it. Press **New email** to leave the thread for a new message. Each message you pick keeps its own draft while the composer is open. Also called: answer an older email, reply to a specific message.

### How do I file an email I send under a project?
To file an email you send under a project in Margince, choose the project under **Project** in the composer. Margince puts its key, such as `[NER-1]`, at the start of the **Subject**.
Choosing **No project** takes the key out; deleting it from the text does not.

The list starts on the thread's own project. If there is none, it starts on the deal's, and then on the company's only live project. Writing from a project page files under that project. With no live project to offer, the list is not shown. Also called: tag an email with a project, project key in the subject.

### What do the composer's error messages mean?
The composer in Margince does not send until four things are filled in, and names each one under its field.
- "Add at least one recipient.": **To** is empty.
- "Enter a subject.": **Subject** is empty.
- "Enter a message before sending.": the **Body** is empty.
- "Select the reason for contact.": you picked no **Reason for contact**. A reply to their own message needs none.

A channel reply, such as Telegram, has no subject, so Margince checks only the body and the reason.
Also called: validation error, why is Send not working.

### Can I use an email template?
Margince has no email templates, snippets or canned replies in the composer. To start from something ready, press **Draft with AI**, which drafts in your own writing voice. Or keep a message with **Save as draft** and come back to it. Margince adds your **Email signature** for you.

Offer templates are a different thing: they shape an offer, not an email. See [Offers and the rate card](offers-and-products.md).
Also called: canned response, saved reply, mail template, snippet.

## The composer

You reach the Margince composer from a contact, a company, a deal, a lead, or
the reply action on any message.

To and Cc are on the form. **Bcc** is a button until you press it. Then it says
what it means: other recipients cannot see these addresses, or that anyone else
has a copy.

A channel reply, such as Telegram, drops the subject and Cc, because the channel
has neither.

### How do I attach a file to an email?
To attach a file to an email in Margince, press **Attach** in the composer. Pick a file from **On this record**, or choose **Upload file**.
1. In the composer, press **Attach**.
2. Pick a file under **On this record**, or use **Upload file** ("Drop a file here, or choose one").
3. The file shows in the message; take it out with its **Remove** button.

**On this record** shows the 25 newest files filed on the record. **Upload
file** first files the upload on the record, then attaches it, so the history
keeps every file you sent.

One message carries at most 10 files. Past that, the composer asks you to send
the rest in a second message. A reply does not offer back the files that came in
with the message you answer.
Also called: add an attachment, send a document, attach a PDF.

### How do I use AI to draft an email?
To have AI draft an email in Margince, open the composer. Say what the email is for in **Purpose of the email**, and press **Draft with AI**.
1. Open the composer from a contact, company or deal.
2. On a company, choose the contact under **Draft to**, and if you like, the deal under **Related to**.
3. Type the purpose, such as "follow up on last week's quote".
4. Press **Draft with AI** (**Draft reply with AI** for a reply). It shows **Drafting…** while it writes.
5. Read and edit the draft, then send it as you always do.

With no AI model set up, AI drafts are not there, and you write the email yourself.
Also called: AI writer, generate an email, write it for me.

### Drafting with AI

**Draft with AI** writes an email from what the record holds, in your voice. It
tells you what it read ("Based on: {inputs}"), and **Why this draft?** shows
why it wrote what it did.

Every AI draft is marked **AI-assisted draft**, with a note that AI wrote it and
that you should check and edit it before sending.

Your voice profile has a number that goes up with each build, shown after
"Built from your writing samples". A profile Margince is still building is
marked **Provisional voice**. It still shapes the draft the same way a finished
one does.

If Margince could not load the profile, the draft says so:

> **This draft is not in your voice.**
> Your voice profile could not be loaded,
> so this draft does not use your voice. Draft again, or edit before sending.

**Discard draft** marks the draft as a miss for your Voice DNA. Margince never
keeps the text it wrote.

**Save as draft** keeps a message you have not finished, so you can come back
to it. It does not send anything.

### The four rewrites

The AI draft offers four rewrites under **Rewrite**, on text the model wrote that
you have not edited yet. They are **Shorter**, **Warmer**, **More formal** and
**Add a deadline**.

They are gone the moment you type, because they rewrite the model's text, not
yours.

### Why are you writing?
The **Reason for contact** field says why you are writing. The composer explains
it:

> The record determines what is allowed; the reason lets the send be checked
> against it.

The answers are:

- Follow-up they requested
- Active deal
- Quote or proposal they requested
- Support for a purchase
- Invoice or payment
- Their contract
- Their customer relationship
- Marketing

Margince does not ask for a reason when you reply to their own message: the
recipient wrote first, so no reason is needed.

The consent check reads the reason, and it runs *before* you send. It shows one
of three answers: **Ready to send**, **No recorded reason to contact this
recipient**, or **Message cannot be sent**. A check with no answer never counts
as permission. The screen says the check did not finish, and that sending runs
it again.

Some purposes that Margince knows, such as a security notice, are not offered
here. A sender who could pick one could send marketing that looks like a
security notice.

### Why can't I send an email to this contact?
When Margince will not send an email to a contact, the composer names the reason above the **Send** button. Where there is a fix, it names that too.
- **Send blocked: no consent**: there is no consent for this purpose. Use **Review consent**, or **Request review** where it is offered.
- **No recorded reason to contact this recipient**: press **Record reason for contact** and say why, under your name.
- **Message cannot be sent**: a reason nobody can lift, such as "The recipient objected to marketing."
- A mailbox connected only to read mail: press **Reconnect your mailbox** and allow sending.
Also called: email blocked, cannot email customer, send refused.

## What a send refuses, and what it only warns about

A send without consent is refused:

> **Send blocked: no consent**
> A recipient has not given consent for this purpose, so the send was blocked.
> With no consent, the answer is always no.

The reason is always clear. Some say that nobody can lift them: "The recipient
objected to marketing", and no one here can change that, an administrator
too. Others name the fix. An address that does not take mail must be
corrected, and no one can let it through. When several records share one
address, Margince cannot tell who the recipient is, and you merge the records to
fix it.

**A bouncing address is a warning.** You can still send. The composer says the
last mail to that address came back, and none has gone through after it. Send it
all the same, or use another address.

**A colleague's mailbox** gets a note, and you can still send: your reply goes
out from your own mailbox, under your name.

**A mailbox that cannot send** is refused, with the fix. The mailbox is
connected only to read mail, without permission to send. Reconnect it and allow
sending. A mailbox you connected before Margince could send cannot be changed in
place. The **Reconnect your mailbox** link opens Settings → Connections.

**One recipient per unsubscribe link.** A message with an unsubscribe link goes
to one recipient at a time, because that link is the recipient's own consent
record. Send it once per recipient, with no Cc.

**Chat channel limits.** A channel such as Telegram may carry no files at all.
Or it may limit how many files, how large each one is, and how large they
are in total. It may also limit how long the text under them is. When a message goes past one of these, the
composer names the limit and the number before you send.

### How do I set my email signature?
To set your email signature in Margince, open the account menu, choose **Settings**, then **Account**, and use **Edit signature** under **Email signature**.
1. Open the account menu at the top right and choose **Settings**.
2. Open **Account**.
3. Under **Email signature**, press **Edit signature**, type plain text and save.

Leave it empty and a send ends with a short sign-off and your name, such as
"Best regards". The sign-off follows the message's language. If a message is too
short to tell, Margince uses the installation's language, then English. The
composer shows the sign-off under the body before you send. The AI never writes
a sign-off; this signature is the one that goes out. A message an agent sends
carries no sign-off.

A sent message reads, in this order: your message, your signature, any notice
the law needs, then the unsubscribe footer. The copy on your timeline leaves out
the recipient's own unsubscribe link, because that link acts for the recipient.
Also called: sign-off, email footer, sender signature.

### How do I schedule an email to send later?
To schedule an email in Margince, open the arrow beside **Send** (**Other ways to send**), choose **Schedule send**, pick a time, and confirm.
1. Write the email as you always do.
2. Press the arrow beside **Send** and choose **Schedule send**.
3. Pick **Tomorrow morning** (08:00), **Tomorrow afternoon** (13:00) or **Monday morning** (08:00). Or pick **Select date and time**, then a **Date** and one of 08:00, 09:00, 13:00 or 17:00.
4. Press **Schedule send** in the **Schedule email** dialog.

**Send now** goes back to sending at once.

You see "Email scheduled. It has not been sent yet." with a link to **Scheduled messages**. The dialog says what happens next:

> The email is sent at the selected time, and the consent and mailbox checks run
> again then. Until then it can be moved or canceled from Scheduled messages.

Also called: send later, delayed send, timed email.

### How do I cancel or reschedule a scheduled email?
To cancel or move a scheduled email in Margince, open **Scheduled messages** from the command palette (⌘K or Ctrl+K). Use **Reschedule** or **Withdraw** on the message.
1. Press ⌘K (Mac) or Ctrl+K, type "Scheduled messages" and open it. The note shown after you set a send time links there too.
2. To move it, press **Reschedule**, pick the new time and press **Reschedule**. Only the time can change.
3. To cancel it, press **Withdraw**, then **Withdraw message** in "Withdraw this message?".
Also called: unschedule, delete a scheduled email, change send time.

The **Scheduled messages** page lists your messages that are set for later and
not sent yet. Only you can see them. It has no sidebar row. It puts messages in
three groups: **Held for your action**, **Scheduled** and **Sent or withdrawn**.

**Reschedule** changes the time only. Margince checked the text itself, so to
change the text, withdraw the message and write it again. **Withdraw** warns
that the message will not be sent, nothing reaches the timeline, and sending it
later means writing it again.

### Held

A scheduled message is **Held** when its time came and a check refused it. The
reason says which check, and what to do:

- **A recipient withdrew their consent** after you set the send.
- **Your seat or mailbox changed**, so it cannot be sent as you.
- **Its send time passed** while the system was not running, and it is now too
  late to send. Reschedule or withdraw it.
- **The send job ran out of attempts.** Reschedule it to try again.
- **A check blocked it when it was due.** Nothing was sent.

A held message also adds an approval card that never runs out. See
[Approvals](approvals.md).

## What the recipient controls

Every marketing message carries a footer with two links: unsubscribe, and
manage preferences. Both are the recipient's own private links.

**The preference page** says that each purpose is separate. The recipient
cannot switch off messages they need for something they did, such as an order note. All other purposes are theirs to control.

For each purpose it shows one of three states. It is on because they asked, on
because they did not say no, or off because they asked you to stop. Only a clear
"no" from them counts as off.

**Security and service messages are always on**, and the page says why. They
are needed for something the recipient asked for, such as a password reset or a
confirmation.

**Stop all marketing** switches off every marketing purpose at once and leaves
the rest alone. Replies to their own questions, and anything they asked for, go
on, because no one signed them up for those.

**Undo and keep receiving marketing** does not sign anyone up again by itself.
Signing up again needs a clear "yes", and Margince never switches it back on
without one. The recipient saves to record their consent, or drops the change.

An unsubscribe link **never acts the moment it opens**. It shows a page and
asks, because a program that opens links by itself must not unsubscribe someone.

Whatever they choose, Margince stores the words they were shown with the
decision, and the time. That is the record of it, and the choice then holds for
every later email.

### How do I ask a contact to confirm their details or consent?
To ask a contact to confirm their details in Margince, open the contact. In the **Communication permissions** panel, press **Ask them to confirm their details**.
1. Open the contact's page.
2. In **Communication permissions**, press **Ask them to confirm their details**.

Margince mails a private link to the contact's own recorded address; you cannot send it to any other address. The link lasts 14 days and works once.
Also called: double opt-in, consent request, GDPR confirmation.

### How do I send a privacy notice to a contact?
To send a privacy notice in Margince, open the **Privacy notice owed** item on your Worklist (**Home**, then **Show Worklist**) and press **Send privacy notice**.
1. Open the **Privacy notice owed** item.
2. Press **Send privacy notice**, or **Ask them to confirm their details**, which tells them too.
3. If they already know, or the law lets you skip it, press **End the duty…** and record why.

The notice goes to the contact's own recorded address. If it comes back, the duty is owed again.
Also called: GDPR Art. 14 notice, disclosure duty, information obligation.

### Why does Margince say "this installation cannot send mail"?
"Not sent: this installation cannot send mail" means your installation has no server set up to send its own mail. So the privacy notice or confirmation link was not sent, and the duty stays open.

This mail never goes through your connected Gmail or Outlook. It carries a private link to the contact's own record, which must not sit in your sent mail. Ask your administrator to set up the installation's own mail. Until then, tell the contact another way and press **End the duty…**.
Also called: privacy notice not sent, cannot send notice.

### Which mailbox does Margince send from?
Mail you write in Margince goes out from your own connected mailbox, so the contact sees your address.
Mail Margince writes by itself goes out from the installation's own address, never from yours. That is the privacy notice, the confirmation link, password reset, invitations, and the weekly review and morning brief sent by email. Your administrator sets that address up.
Also called: sender address, from address, send as me.

## Asking someone to confirm their own details

You send the confirmation link with **Ask them to confirm their details**. It
lets the contact see what you hold about them and correct it. They can also say
whether they want you to write to them. It goes to their own recorded address;
you cannot send it to any other address.

The link lasts 14 days and works once. They see the whole record you hold and
where each field came from. They also see a plain question about staying in
touch, and a way to ask you to remove them.

This is also the only way to start a purpose that needs a confirmation. The
screen says that only this contact can confirm this purpose, through a link sent
to their recorded address.

If a colleague could finish a confirmation for the contact, it could not show
that the contact agreed.

## Marketing sent from the composer

Choosing **Marketing** as the reason for contact names no single list. So the
unsubscribe link in that message stops all marketing to that recipient, as
**Stop all marketing** on the preference page does.

To let a recipient leave one list and keep the rest, send it a way that names
its purpose.
