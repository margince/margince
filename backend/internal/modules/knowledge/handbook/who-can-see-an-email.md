<!-- prose:plain -->
# Who can see an email

Two things decide who can read an email in Margince, and **a "no" from either is a
no**:

1. **Record visibility**: who may find the contact, company or deal the message
   is filed under.
2. **The message's own audience**: who may read *this* message, whatever else
   they can see.

A high rank does not change the message's audience. A user who can read every
record in the company still cannot read a message they were not an audience
for. An admin is no different. The audit log and an export of data do not name the
subject or the files of a held message either.

Personal mail keeps no attached files. That covers a message on a thread held as
personal, from a sender that thread's verdict judged. It also covers a message
from a sender your verdict judged personal in the last 14 days. Margince keeps
the files of such a message out of the file store, and the first copy it stores
carries none of their data. Margince keeps the message itself. It still lists
each file by name and size, with its kind where the file name shows one, marked
as not kept.

A sender verdict does not apply to a sender you marked as business on the
Senders page. It does not apply to one you have replied to, or a contact you
write with.

Mail that came in before the verdict loses its files later, once you have
time to change your view. On a thread held as personal, Margince stops keeping its
files a week after you held it yourself, or a month after Margince did. It
deletes the stored copies in the day after that. Sharing the thread back before
then keeps them.

This skips archived mail, and mail under a legal hold or inside the time the law
keeps it. It also skips mail a privacy request is about, and mail a colleague
also imported.

The sweep of personal mail deletes all of the mail from a sender judged
personal, on the same timing. Marking the sender as business first stops it. The
sweep also keeps mail under a legal hold or inside the time the law keeps it.
It keeps mail a privacy request is about, and a colleague's copy of mail they
imported too.

### Who can see an email I captured or sent?
To see who can read an email in Margince, open the message from a timeline. The line under its subject shows **Team**, **Shared**, **Participants**, **Selected** or **Withheld**, with a sentence saying what that means.
- **Shared**: "Everyone in the company can read this."
- **Team**: "Everyone who can open the records this is filed against can read it."
- **Participants**: only those on the message can read it.
- **Selected**: only the colleagues and teams named under it.
- **Withheld**: you are not one of them, so the text is hidden from you.

Also called: email privacy, who can read my mail, email permissions.

## The audience marks

Every Margince message row carries one audience mark:

| Mark | What it means |
|---|---|
| **Shared** | Everyone in the company can read this |
| **Team** | Everyone who can open the records it is filed under |
| **Participants** | Only those on the message can read it |
| **Selected** | Only those named can read it |
| **Withheld** | You are not one of them |

"Team" here names the audience. It does not mean a named team. Who may find the
linked record still decides whether the row appears at all.

A withheld message still shows up. You see its date, which way it went and the
record it is filed under. The text is hidden: "This message is not shared with
you". You learn that a conversation happened, and nothing about what was said in
it.

### Why is an email hidden from me?
When an email in Margince shows "This message is not shared with you" or **Withheld**, you were not in its audience. The mailbox that captured it keeps it to the colleagues who were on it.

The most likely reasons: the thread is **Held until classified** and nothing has judged it yet, or its owner set their mailbox to **Always held**. Or the sender or domain is under **Private correspondence**, or an administrator turned **Email sharing** off.
Only the mailbox owner can open it up, by choosing **Share with the company** on the message. Ask them; an administrator cannot open it for you.
Also called: email not visible, cannot read email, content for participants only.

## Why this one is held

A held Margince message carries a reason, so you can see why it is held:

| Reason | What happened |
|---|---|
| **Held until classified** | Nothing has judged it yet, and mail nothing has judged is held |
| **Held by your setting** | Your mailbox asked for it |
| **Held by the company** | An administrator turned mail sharing off |
| **Held by classification** | A model judged the thread and held it |
| **Marked confidential** | The sender said so in the subject line |
| **Held: counterparty mail** | You hold mail with one of the sides |
| **Kept private** | A human decided, and that decision stays |
| **Held: no record** | Something judged the sender, so it is filed under nothing |
| **Held: no counterparty** | It named nobody Margince could create a record for |

A later verdict clears some of these and not others:

- **Held until classified** clears when a verdict comes in.
- **Kept private** stays. Only a human's own decision sets it, and nothing
  automatic changes it.
- **Held: counterparty mail** and **Marked confidential** both win over a later
  verdict. A model that finds a thread to be plain business says nothing about whether you
  want your lawyer's mail in a shared CRM.
- **Held by the company** comes from the company setting, and no verdict
  clears it. Only an administrator turning sharing back on opens that mail, and
  only mail captured after that.
- **Held: no counterparty** is the one hold that filing the message lifts. It
  means only "nothing has filed it yet", and it ends the moment something does.

The reason itself is hidden with the text, because a reason like "held because
it is about a colleague" says what the message is about.

### How do I change who can read mail from my mailbox?
To change who can read mail captured from your mailbox in Margince, open **Settings → Connections** and change **Mail visibility** under that mailbox.
1. Open the account menu, choose **Settings**, then **Connections**.
2. Under the mailbox, set **Mail visibility** to **Held until classified**, **Always held** or **Shared with the team**.
3. When you narrow it, the **Apply to captured mail?** dialog offers **Also narrow captured mail**.
4. Confirm with **Change mail visibility**.

**Shared with the team** is refused until an administrator allows it for the company.
Also called: mailbox privacy, mail posture, share my inbox.

## The three postures a mailbox can ask for

You set the mailbox posture at **Settings → Connections**, under each mailbox,
as **Mail visibility**.

**Held until classified** is the default for every new mailbox. A message stays
with whoever was on it until a model finds the thread to be plain business.
Nothing is shared before a decision. So a model that is down or out of money
leaves mail held, not open.

**Always held** works the same way, without the model. You share a thread
yourself, one at a time.

**Shared with the team** makes mail open to read the moment it comes in. It is
off unless an administrator allows it for the company. In Germany and Austria,
reading a colleague's mailbox into a shared CRM needs a works council
agreement. Margince does not check that you have one.

Changing the posture applies to mail captured after the change. The same dialog
offers to narrow what is already captured. It never opens up captured mail; to
open older mail, share its threads one by one.

### How do I turn email sharing off for the whole company?
To turn email sharing off for everyone in Margince, an administrator opens **Settings → Capture rules**. In the **Email sharing** card, they switch off **Share captured mail with the team**.
1. Open **Settings**, then **Capture rules**.
2. In **Email sharing**, switch **Share captured mail with the team** off and save.

From then on, only those on each message can see new mail. The card warns: "With email sharing off, the CRM is hard to use".
The same card holds **Allow mailboxes to share on arrival**, which lets mailboxes pick **Shared with the team**.
Only an administrator or an Ops user can change this.

## The company-wide floor

**Settings → Capture rules → Email sharing** decides whether captured mail is
shared with colleagues at all. It is on by default.

When it is off, every message captured from then on is held to those on it,
whatever any mailbox asks for. The app warns you: "With email sharing off, the
CRM is hard to use."

It touches everybody, so it lives with the company rules instead of on your own
connections page.

### How do I keep all mail with a contact or company private?
To keep your mail with one contact or one company private in Margince, open their page and use **Private correspondence**. Use **Keep private** for the address, or **Keep all of {domain} private** for the whole domain.
1. Open the contact or company page.
2. In the **Private correspondence** panel, press **Keep private** or **Keep all of {domain} private**.
3. Confirm in **Keep this correspondence private?** with **Keep private**.

Mail is still captured and you can still read it; colleagues cannot. It covers new mail only. Undo it with **Lift**, which also applies to new mail only.
Also called: confidential client, lawyer mail, hide emails from colleagues.

## Holding your mail with one contact or company

**Private correspondence**, on a contact's or a company's page, keeps your mail
with one side to those on it, without deciding message by message. A domain hold
covers the whole domain, which is most often what you want for a lawyer.

It holds mail from then on, and lifting it opens nothing again: "Lifting applies
to new mail. Mail already held stays held."

### How do I share an email thread with my team?
To share an email thread in Margince, open the captured message from a timeline and choose **Share with the company** beside its visibility mark. Or use **Share with the team** in **Settings → Connections → Held threads**.
1. On a contact, company or deal timeline, open the email.
2. Beside the mark under the subject, press **Share with the company**. It "Applies to the whole thread."
3. To go back, press **Make private**.

Or press **Share with the team** on the thread under **Settings → Connections → Held threads**.
Only the mailbox owner can share. If a colleague also holds it, you see how many other seats still do.
Also called: release an email, unhide a thread, make an email public.

## Sharing a thread

You change who can read a captured message by **sharing its thread**.

Sharing lets go of **your own hold only**. If a colleague still holds the same
message, Margince tells you how many other seats do, but never who. The **Held
threads** card says the same: "A thread opens only when every recipient agrees."

That is the rule when a message reached two mailboxes. Each owner adds what
their own mailbox asks for, and the message ends at the **strictest** of those.

### How do I change who can read one logged message?
To change who can read one logged message in Margince, press **Change visibility** on its row and answer "Who may read this message?".
1. On the timeline, find the message and press **Change visibility**.
2. Choose **Everyone in the company**, **Participants only**, or name the colleagues and teams who may read it.
3. Press **Save visibility**.

It "Applies to this message only, not to the thread or the contact." Captured email does not offer this button; share or hold its thread instead.
Also called: restrict a message, message permissions.

## Changing one message's audience

**Change visibility** asks "Who may read this message?": everyone in the company,
only those on the message, or a named set of colleagues and teams. It is offered
on messages you logged yourself. Mail a mailbox captured does not offer it.

**It changes one message.** The control says so: "Applies to this message
only, not to the thread or the contact."

That is how it is different from sharing, above. **Share with the company** on a
thread "Applies to the whole thread." and lets go of your own hold on all of it.
Changing a message's visibility changes that one message.

## What you can see of other seats' capture

Your own connections need no permission from anyone, because it is your own mail.

Messages that came in through a connection for the whole company, like the
Telegram connection, are owned by no one user. Margince shows them to seats given that
access.

**No permission reaches a colleague's mailbox.** The view for the whole company
never shows mail from another seat's own mailbox.

## Where each control lives

| What you want | Where |
|---|---|
| This mailbox's posture | Settings → Connections → Mail visibility |
| The company-wide floor | Settings → Capture rules → Email sharing |
| Hold mail with one contact or company | Private correspondence, on the contact's or company's page |
| Share one thread | The message itself, or Settings → Connections → Held threads |
| One logged message's audience | Change visibility, on its row |
| Who can see the *record* | The contact or company header |
