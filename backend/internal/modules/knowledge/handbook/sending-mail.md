<!-- prose:plain -->
<!-- describes: frontend/src/features/mail frontend/src/design-system/emailthread.tsx frontend/src/components/sendpermission.tsx backend/internal/modules/activities/sendcore.go backend/internal/modules/activities/scheduledsend.go backend/internal/modules/activities/scheduledstop.go backend/internal/modules/activities/signature.go backend/internal/modules/comms/gates.go backend/internal/modules/consent/gate.go backend/internal/modules/consent/mailpreview.go backend/internal/modules/consent/instructioncorrection.go backend/internal/modules/consent/instructionconsume.go backend/internal/modules/activities/wordingcorrection.go backend/internal/compose/draftcore backend/internal/compose/replydraft.go backend/internal/compose/replyconversation.go frontend/src/lib/activityfiling.ts frontend/src/lib/messagevoice.ts backend/internal/modules/activities/signaturetemplate.go -->
# Writing and sending mail

### How do I send an email to a contact?
To send an email to a contact in Margince, open the contact's page and choose **Email** in its header. Fill in the composer and press **Send**.
1. Open the contact. The button may read **Write**, or **Message on {transport}** when chat is the only channel.
2. Check **To** (at least one recipient). Press **Cc and Bcc** if you need copies.
3. Fill in **Subject** and the message **Body**.
4. Pick a **Reason for contact**.
5. Press **Send** once. Margince checks the email when you press it, not while you write.

"No address and no thread to reply to." means the contact has no address to write to. Also called: write an email, email a customer, message a client.

### Where can I start writing an email?
- the contact header's **Email** button, or an email address on a record, a card or the contacts list. The address is the recipient, and the email is filed under the record you are on. Without a connected mailbox, the address opens your own mail app.
- **Write email** or **Draft reply** in a record's **Email** panel
- **Send email** on a deal
- **New email** or **Write email** on Home
- **Reply** on any message in a timeline
- **Write email** on a call, note, meeting or task in a timeline, when it names one contact with an email address. A task Margince filed itself offers none.

From a call, note, meeting or task, it opens a new email that names the entry as "About: {subject}", not a reply. A company page opens the composer with the company's contacts to choose from. With none on file it says "No contacts at this company yet. Write the message manually, or add a contact first." Also called: compose, new message, reply to an email.

### How do I send a follow-up email?
To send a follow-up email in Margince, open the contact's **History** tab and press **Reply** on the message. Or press the button on its Worklist row.
1. On the record, press **Reply** on the email. After your own email, the composer reads "Following up on “{subject}”"; after theirs, "Replying to “{subject}”".
2. From **Home** → **Show Worklist**, press **Read and reply**, **Draft reply** or **Write email** on the row.
3. Pick a **Reason for contact** and press **Send**, or set a time. A reply to their own message needs no reason.
Also called: chase a customer, nudge, follow up, reply to a thread.

### How do I reply to one particular message in a thread?
To reply to one message in Margince, open the email reader and press **Reply** below that message. Select an older message first to show its text and **Reply**. The newest message appears first and opens with the one you came from. Close the composer to return to the thread.

The **…** button below your reply shows only the message you are answering. **Draft reply** answers that selected message, even when newer mail is in the thread. Mail on the thread helps choose the language and form of address.

Pick a message under **Threads** in the composer too. The line above the recipients names it.
Rest the mouse on **Preview** to see a message's text, and press **Read full email** to read the one you answer. Press **New email** or **Start new instead** to leave the thread for a new message. Each message you pick keeps its own draft while the composer is open. Also called: answer an older email, reply to a specific message.

### How do I file an email I send under a project?
To file an email you send under a project in Margince, choose the project in the composer's head, beside its title. Margince puts its key, such as `[NER]`, at the start of the **Subject**, so the client's answer is filed under the project too.
The switch **Add project key to subject (helps filing)** under the subject is on. Turn it off to send this email without the key; the next email starts with it on. Choosing **No project** also takes the key out; deleting it from the text does not.

The list starts on the thread's own project, then on the deal's, then on the company's only live project. Writing from a project page files under that project; with no live project to offer, the project list is hidden. A new email from a company has one **Related to** list there instead: the company's live projects, its open deals ("Deal: {name}"), and **Nothing specific**. When the company has no live project and no open deal, the list is hidden. A deal has no key, so choosing one takes the key out of the subject and files the email under the deal. Also called: tag an email with a project, project key in the subject, file an email under a deal.

### What do the composer's error messages mean?
The composer in Margince does not send until four things are filled in, and names each one under its field.
- "Add at least one recipient.": **To** is empty.
- "Enter a subject.": **Subject** is empty.
- "Enter a message before sending.": the **Body** is empty.
- "Select the reason for contact.": you picked no **Reason for contact**. A reply to their own message needs none.

A channel reply, such as Telegram, has no subject, so Margince checks only the body and the reason.
Also called: validation error, why is Send not working.

### Can I use an email template?
Margince has no email templates, snippets or canned replies in the composer. To start from something ready, press **Draft email**, which drafts in your own writing voice. Margince also keeps what you type as a draft, so you can come back to it. Only you can open your draft. If you leave and an admin hands over your work, your drafts go to the colleague who takes over. Margince adds your **Email signature** for you.

Offer templates are a different thing: they shape an offer, not an email. See [Offers and the rate card](offers-and-products.md).
Also called: canned response, saved reply, mail template, snippet.

## The composer

You reach the Margince composer from a contact, a company, a deal, a lead, or
the reply action on any message.

The composer opens as a window at the bottom of the page, so the page stays in view
behind it. It stays open when you go to another page. Fold it to its title bar
with **Minimize**, make it larger with **Expand**, or drag its top or left
side. Drag its title bar to slide it left or right. On a phone it fills the
screen.

The bar at the top says which thread you answer. Press **Change** or **Continue
a thread** to pick another thread, or **New email** to start a new one. Type in
the search box to find an older thread, and press **Show more** to see more of
them. On a
reply, **Show earlier messages** under the text shows what came before. That
text is only for you to read; it is not sent.

Anything that stops the send shows in a warning bar above **Send**, with what
you can do about it there.

To is on the form. **Cc and Bcc** is a link until you press it. Under **Bcc** the composer says: "Other recipients do not see these addresses." A recipient Margince knows shows as the contact's name. Point at the name, or reach it with Tab, to see the address and the company.

Click in **To**, **Cc** or **Bcc** to see up to eight contacts of the record. When you write to a contact, the list offers that contact first. The contacts around them come next: colleagues at the company they work at now, and the contacts on the same deals. From a company, deal or project, the company's contacts are offered. Type a name or an address to find more. Once you type two letters, your own colleagues in Margince show under **Colleagues**.

Your own address is never offered, and you only see contacts you can open in Margince. The list closes once a recipient is added. An address is on a message only once. Type or paste one already on it, even in capital letters, and the composer keeps your text and names its field: "Already in To." When **To** is empty as the composer opens, you start in **To**; else you start in the message.

A channel reply, such as Telegram, drops the subject and Cc, because the channel
has neither.

### How do I attach a file to an email?
To attach a file to an email in Margince, press **Attach** beside the text formatting, next to **Send**. Pick a file under **Attach from this record**, or choose **Upload file**.
1. In the composer, press **Attach** beside the text formatting, next to **Send**.
2. Pick a file under **Attach from this record**, or use **Upload file** ("Drop a file here, or choose one"). Press a file again to take it off.
3. The file shows under the text; take it out with its **Remove** button.

**Attach from this record** shows the 25 newest files filed on the record. **Upload
file** first files the upload on the record, then attaches it, so the history
keeps every file you sent.

One message carries at most 10 files. Past that, the composer asks you to send
the rest in a second message. A reply does not offer back the files that came in
with the message you answer.
Also called: add an attachment, send a document, attach a PDF.

### How do I use AI to draft an email?
To have AI draft an email in Margince, open the composer. Say what the email is for in **Purpose of the email** above the message, and press **Draft email**.
1. Open the composer from a contact, company or deal. **Draft reply**, **Draft follow-up** or **Create draft** on a suggestion opens it already drafting, unless it holds your words.
2. On a company, put the contact you write to in **To**. The draft is written for the first contact of that company in **To**. Until there is one, the composer says "To draft with AI, add a contact from this company to To."
3. If it is not drafting yet, type the purpose, such as "follow up on last week's quote".
4. Then press **Draft email** (**Draft reply** for a reply). It shows **Drafting…** while it writes, and an empty message field shows moving dots where the text will appear.
5. Read and edit the draft, then send it as you always do.

**Email** starts a new message and waits for you to press **Draft email**. Its subject does not come from an earlier thread. A suggested **Draft a follow-up** continues the message named by the suggestion. If that message can no longer be read, the action says so. With no AI model set up, AI drafting is not available, and you write the email yourself.
Also called: AI writer, generate an email, write it for me.

### Drafting with AI

**Draft email** writes an email from what the record holds, in your voice. **Why this draft**, closed until you open it, shows what it read ("Based on: {inputs}"), with a link to each record it read.

A restored draft keeps its purpose and AI note. The **Saved draft restored** notice makes clear that you are opening saved work. Every AI draft is marked **AI-assisted draft**, with a note that AI wrote it and that you should check and edit it before sending.

Your voice profile has a number that goes up with each build, shown after "Built from your writing samples". A profile Margince is still building is marked **Provisional voice**. It still shapes the draft the same way a finished one does.

If Margince could not load the profile, the draft says so:

> **This draft is not in your voice.**
> Your voice profile could not be loaded,
> so this draft does not use your voice. Draft again, or edit before sending.

Once a draft stands, the purpose Margince wrote it to stays above the draft. Change the purpose and press **Redraft** to get a draft for the new purpose.

**Discard draft** marks the draft as a miss for your Voice DNA. Margince never keeps the text it wrote.

Margince keeps a message you have not finished as a draft while you type. The foot says **Draft saved**, and closing the window keeps it too. It does not send anything. **Delete draft** deletes the message.

A saved draft also keeps the deal and project you chose. Margince checks that you can still use them when you open the draft again.

### The rewrites

The AI line above the message offers **Polish**, **Shorter**, **Warmer**, **More formal** and **Add a deadline**. **Polish** makes the words clear and keeps what the mail says. They work on any text in the message, whether AI drafted it or you wrote it. After a rewrite, the AI line says that AI wrote the words, and undo in the message brings your text back.

If you added a link or a format, or the message is too long, the rewrites are greyed out and the line says why.

### How do I change the language of an AI draft?
To change the language of an AI draft, pick another language in the list on the AI line above the message. The list shows the language the draft is in, and names each language in its own words, such as **Deutsch**. Margince writes the words in the message again in that language, for the same purpose.

If you edited the words, only an AI model writes your edited words again. With no model set up, or with a link or format you added, your words and the language stay as they are. The composer then says "Your edits were kept. Draft again when ready."

Once you pick a language, **Draft email**, **Redraft**, the rewrites and a saved draft keep it. Until then, the language of a subject you wrote yourself decides it, when Margince can tell that language. If it cannot, Margince takes it from the mail on the thread, then from your settings. If the new draft fails, your words and the language stay. When Margince cannot tell the language of a draft, it shows no list.
Also called: draft in another language, write the email in German, change the email language.

### Why are you writing?
The **Reason for contact** field says why you are writing. The line beside it says what your answer does to the email. The answers are:

- **Met in person**: you were with them in person, for example at an event or a visit, and have their card. **Send** saves the meeting on the contact, then sends. This covers personal mail for a year after the meeting, never marketing.
- **They asked for it**: they asked you to write, or asked for a quote. Their request must be on record.
- **Open deal**: the email is about a deal they are on. Pick it in **Deal this follow-up is about**.
- **Existing customer**: support, an invoice, a contract or their account.
- **Marketing**: an offer or a newsletter. Only this answer adds an unsubscribe link and the full company footer. It needs their consent or a past purchase.

A reply to their own message needs no reason. When you ask the assistant to move a meeting, such as "Termin verschieben", the draft picks **They asked for it**. **Met in person** shows only when you write from a contact or a lead, to their own address alone.

Margince checks the email once, when you press **Send**. Nothing is checked while you write. The check reads your reason, the record and two laws. Rules on marketing follow the law where each recipient lives. Your reason to contact someone must hold there and where your own company is based.

When the check allows every recipient, the email is queued and the composer closes. When it does not, the email is not sent, and the bar above **Send** says why (below). Your text, recipients and attachments stay.

Some purposes that Margince knows, such as a security notice, are not offered here. A sender who could pick one could send marketing that looks like a security notice.

### Why can't I send an email to this contact?
When Margince does not send an email, the bar above **Send** names the first reason. An example is "{address} objected to marketing.". The bar also names the law behind it, with a link to its text. When both laws apply, it names both, the recipient's first: "(GDPR Art. 6(1); Law 91/2025/QH15 Art. 9)".

Below the reason, the bar says where Margince takes the recipient to live, and where it read that, such as their employer's address. When that is wrong, open the place on the recipient in **To** and pick the right one for this message.

With more than one reason, "{count} more findings" lists the other recipients and reasons. The recipients it names are marked in **To**, **Cc** and **Bcc**.

- **Send anyway**: when your reason may lift every refusal, the bar asks **Why send this one message anyway?**. Write your reason and press **Send anyway**. This sends only this message and grants no permission for later ones. The recipient's choice stays as it is: the next message is refused again. Your name, your reason and the message are kept in a record that cannot be edited. Reps hold this right by default.
- When the refusal cannot be lifted with a reason, the bar says "Edit the message or remove this recipient to send it." Examples are an address that does not take mail, or one on several records or none. Correct the address, merge the records or remove that recipient.
- A mailbox connected only to read mail: press **Reconnect your mailbox** and allow sending.

Changing the message, a recipient or the subject puts the warning away. Press **Send** again: Margince checks the new version, and your reason stays in the box. Removing an offer from the text can make the refusal go away.

Only you can press **Send anyway** for your own message, in the composer. An agent cannot, and neither can a colleague or an admin: the message goes from your mailbox, with your name and signature.

If that approved send fails, open its review and choose **Correct spelling or punctuation**. You must own the message, have approved it yourself and still hold permission to edit and approve mail. Confirm that its meaning and facts stay the same, then choose **Save correction**. The original approval stays on record; the message stays held until you retry. A change in meaning, recipients, sender, purpose or attachments needs a new message and approval. A later stop needs a new decision.

Margince no longer makes a standing exception. One that already exists covers its named sending company and purpose until someone takes it back on the contact's page. It records a staff decision and grants no consent.

## What a send refuses, and what it only warns about

A marketing send without consent is refused:

> **Permission needed for this message**
> Marketing needs the recipient's consent, and none is on record.

Nobody can send marketing after the recipient objected, an administrator included.
Correct an address that does not take mail. If several records share one address,
merge them so Margince can tell who the recipient is.

**Mail that comes back** needs a check of the address. The composer says the last
mail came back, and none has gone through since. If the address does not take mail,
sending is blocked.
Correct that address before trying again.

**A colleague's mailbox** gets a note on a reply, and you can still send. Your
reply goes to the recipients in **To**, from your own mailbox and under your name.

**A thread you are not on** gets a note on a reply, unless you work on the deal it
is filed under. You can still send. The note reads "You are not on this thread",
and names the deal owner when you can see that deal and it has an owner.

**A mailbox that cannot send** is refused, with the fix. The mailbox is
connected only to read mail, without permission to send. Reconnect it and allow
sending. A mailbox you connected before Margince could send cannot be changed in
place. The **Reconnect your mailbox** link opens Settings → Connections.

**Company footer.** Business mail carries the company name, address and privacy contact. When one is missing, the bar says so. An admin fills them in once under **Settings → Company**, and every later email carries the footer.

Mail to several contacts keeps To, Cc and Bcc as written and leaves out private links. Read and confirm this with your reason. A contact's private link is never shared.

**Chat channel limits.** A channel such as Telegram may carry no files at all.
Or it may limit how many files, how large each one is, and how large they are in total. It may also limit how long the text under them is. When a message goes past one of these, the
composer names the limit and the number before you send.

### How do I set my email signature?
Open **Settings → Account → Edit signature** to choose plain text or HTML.
Your own HTML comes before the company template. With neither set, Margince uses
your plain text or a short greeting. See [Your email signature](email-signatures.md)
for the preview, clearing HTML and what a formatted signature keeps.

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
To cancel or move a scheduled email in Margince, press the clock beside the bell in the top bar. Use **Reschedule**, **Edit** or **Withdraw** on the message.
1. Press the clock, **Scheduled messages**. Its number counts waiting and held messages, and it changes colour when one is held.
2. To move it, press **Reschedule**, then **Tomorrow morning**, **Tomorrow afternoon** or **Monday morning**. Or pick a day, type a time and press **Move message**. A toast offers **Undo**. Reschedule changes only the time.
3. To change the words or the recipients, press **Edit**. The composer opens with your message. Press **Schedule send** to replace it. The old message is withdrawn once the new one is accepted. A reply stays in its thread. A message sent for a marketing purpose has no **Edit**; withdraw it and write it again.
4. To cancel it, press **Withdraw**, then **Withdraw message** in "Withdraw this message?".
Also called: unschedule, delete a scheduled email, change send time.

The **Scheduled messages** page lists the messages you set for later. Only you can see them.
It has no sidebar row. With none and a mailbox connected, **Write a message** opens Contacts.
The toast after you set a send time links there, and so does the command palette (⌘K or Ctrl+K).

Waiting messages sit on a timeline, one block for each day. A line above them
counts down to the next one. **All**, **Held** and **Today** narrow the list.
Held messages come first, under **Held for your action**. Messages already sent
or withdrawn wait behind **Sent or withdrawn**.

**Reschedule** changes the time only. Margince checked the text itself, so to
change the text, press **Edit**. A message without **Edit** has to be withdrawn
and written again. **Withdraw** warns
that the message will not be sent, nothing reaches the timeline, and sending it
later means writing it again.

### Held

A scheduled message is **Held** when a check refused it, or when a recipient answered first. The
reason says which, and what to do:

- **A recipient withdrew their consent** after you set the send.
- **A recipient stopped marketing** after you scheduled a marketing message. It is held at once.
- **New mail came in from a recipient** after you scheduled a deal follow-up or a quote. Read it first.
- **Your seat or mailbox changed**, so it cannot be sent as you.
- **Its send time passed** while the system was not running, and it is now too late to send. Reschedule or withdraw it.
- **The send job ran out of attempts.** Reschedule it to try again.
- **A check blocked it when it was due.** Nothing was sent.

A held message also adds an approval card that never runs out. See [Approvals](approvals.md). After reading a recipient's reply, accept the card or reschedule the message to send it again. Another reply received after that review pauses it again. Only mail you can read counts.

## What the recipient controls

Marketing email normally includes unsubscribe and manage-preferences links. Both belong to the recipient. An email sent with a recorded exception may leave out links the send warning named.

**The preference centre** says that each purpose is separate. The recipient
cannot switch off messages they need for something they did, such as an order note. All other purposes are theirs to control.

For each purpose it shows one of three states. It is on because they asked, on
because they did not say no, or off because they asked you to stop. Only a clear
"no" from them counts as off.

**Security and service messages are always on**, and the page says why. They
are needed for something the recipient asked for, such as a password reset or a confirmation.

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

Margince first shows you the mail as it will go out, and sends nothing until you confirm. You can pick another language than the installation's, and the page the link opens asks in it too. Margince mails a private link to the contact's own recorded address; you cannot send it to any other address. The link lasts 14 days and works once.
Also called: double opt-in, consent request, GDPR confirmation.

### How do I send a privacy notice to a contact?
Mail from the contact does not create an Article 14 task. If a duty is still owed, open **Privacy notice owed** on your Worklist (**Home**, then **Show Worklist**) and press **Send privacy notice**.
1. Open the **Privacy notice owed** item.
2. Press **Send privacy notice**, or **Ask them to confirm their details**, which tells them too.
3. If they already know, or the law lets you skip it, press **End the duty…** and record why.

Before it goes, Margince shows you the notice and lets you pick its language, as it does for the confirmation request. The notice goes to the contact's own recorded address. If it comes back, the duty is owed again.
Also called: GDPR Art. 14 notice, disclosure duty, information obligation.

### Why does a contact say “Review source”?
The record does not say where the contact's details come from, so no notice deadline is set.
A user with privacy access answers how the details came in, under **Source** on the contact or in **Settings → Privacy and retention**.
If the details come from the contact, the review ends. Data from someone else may still need a notice.
**Due no later than** is the last possible date. The duty may already be due.

**Met face to face** asks for the day of the meeting. It allows personal follow-up mail, never marketing or a newsletter. No privacy notice is owed, because they learned at the meeting who you are. A meeting you record when a send is refused also allows follow-up mail.

### Why does Margince say "this installation cannot send mail"?
"Not sent: this installation cannot send mail" means your installation has no server set up to send its own mail. So the privacy notice or confirmation link was not sent, and the duty stays open.

This mail never goes through your connected mailbox. It carries a private link to the contact's own record, which must not sit in your sent mail. Ask your administrator to set up the installation's own mail. Until then, tell the contact another way and press **End the duty…**.
Also called: privacy notice not sent, cannot send notice.

### Which mailbox does Margince send from?
Mail you write in Margince goes out from your own connected mailbox, so the contact sees your address.
Mail Margince writes by itself goes out from the installation's own address, never from yours. That is the privacy notice, the confirmation link, password reset, invitations, and the weekly review and morning brief sent by email. Your administrator sets that address up. A privacy notice or confirmation request you send shows in the contact's history as **Sent by Margince for you**, with that address.
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
**Stop all marketing** in the preference centre does.

To let a recipient leave one list and keep the rest, send it in a way that names
its purpose.
