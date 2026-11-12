<!-- prose:plain -->
<!-- describes: frontend/src/features/capture/connection-settings.tsx frontend/src/components/mail-sharing.tsx frontend/src/features/capture/emailaccesseditor.tsx frontend/src/features/settings/sharedmailboxes.tsx frontend/src/features/capture/held-threads.tsx backend/internal/modules/capture/mailposture.go backend/internal/modules/capture/sinkaudience.go backend/internal/modules/activities/audience.go backend/internal/modules/activities/audienceholds.go backend/internal/compose/readerrendering frontend/src/features/settings/internalmailsettings.tsx frontend/src/features/settings/internalmailhistoryscope.tsx frontend/src/features/capture/internalmailhistorychoice.tsx backend/internal/modules/capture/internalmailpolicy.go backend/internal/modules/capture/internalmailcapture.go backend/internal/modules/capture/internalmailhistory.go backend/internal/modules/activities/internalmail.go backend/internal/modules/capture/sharedmailbox.go backend/internal/platform/auth/sharedmailboxreader.go -->
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
- **Selected**: the named colleagues and teams. Internal mail about work names colleagues only, and the participants on that message keep access.
- **Withheld**: you are not one of them, so the text is hidden from you.

Also called: email privacy, who can read my mail, email permissions.

In Settings, **Mail not shared with the team** is a place to find and correct a sharing decision.
Leaving mail unshared needs no action. Search by subject or participant, narrow the dates or reason,
then open a message to see who can read it and change sharing. Another owner's hold still applies.

**Sender decisions** works the same way: find an address, name or domain, then correct the decision.
**My overrides** shows your own choices. Removing an override does not restore mail already destroyed.

### How do I keep email between colleagues in Margince?
<!-- opens: settings/capture -->
To keep email between colleagues in Margince, an admin turns on **Internal operational mail** in **Settings → Mail capture**, chooses the mail connections, and presses **Save**. Mail between colleagues is not kept until then.
Also called: internal mail, colleague email, capture internal threads.

## Mail between colleagues

Mail between colleagues is not kept by default. An admin can turn on
**Internal operational mail** in **Settings → Mail capture**. Choose the
mail connections, then **Choose folders** where a source needs a smaller set.
Use **Save** to apply the choices. The mailbox still needs its own grant from
its mail service. The admin must arrange the right approval and staff notice;
the switch does not check those for them.

New mail from those sources starts with **Participants** access. A participant
can open one message, choose **Selected**, and name the colleagues who may read
it. The participants on the message keep access. The choice replaces the named
readers of that message.

A shared mailbox can be a source too. The list names it by its address. Its
mail is also read by the mailbox's reader team, including members who are not
on the message. Admins outside that team do not read it. A shared mailbox has
no owner to start a history import, so it captures new mail only.

Sharing one message does not share its thread or give a team access.
Other members see no message or count. A link to a company, deal or project
does not change that. This mail adds no contact and is not proof of a client
reply or a reason to write to a client.

Capture starts with mail received after the source was chosen. To approve an
older range, select the option to allow a separate internal history import.
Then set the first date in the field below it.
That choice starts no import. The mailbox owner opens **Import older email**,
reads the scope, and chooses **Include authorized internal mail in a new history
import** before **Start import**. An ordinary history import still leaves
internal mail out.

The usual import windows and mail rules still apply.

Turning capture off or removing a source stops new internal mail. Stored mail
keeps its reader limits and the usual privacy rules for files, removal and
retention. A changed mailbox grant needs a new **Save** before capture starts
again. **Junk**, **Deleted** and **Drafts** stay out. Meetings keep their usual
rules.

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

The most likely reasons: the thread is **Held until checked** and nothing has judged it yet, or its owner set their mailbox to **Always held**. Or the sender or domain is under **Private correspondence**, or an administrator turned **Email sharing** off.
Only the mailbox owner can open it up, by choosing **Share with the company** on the message. Ask them; an administrator cannot open it for you.
Also called: email not visible, cannot read email, content for participants only.

### Does showing an email in my language share it with anyone?
No. **Show in my language** works only on an email you can read, and nobody else sees the version in your language. Margince does not save it. A message that is withheld from you offers no button, because its words are not yours to read.
Also called: a held email in my language, who sees a translation.

### Can I read a mail I sent when only a colleague's mailbox captured it?
Yes, that one mail. You wrote to a customer and copied a colleague, and Margince
has only your colleague's copy. You can open that copy because you sent it. You
cannot open the other mail on that thread, or the customer's record, unless your
colleague shares them. A promise you made in that mail comes to you as your own
task or request, and your colleague can read that task too.

This works only when your sign-in address is at one of your company's own
domains. The mail server that received the mail must also have confirmed that it
came from that domain.
Also called: my own sent mail, mail I sent to a customer, copied colleague.

### Why does a mail that several colleagues received show only once?
Margince keeps one copy of a mail for each mailbox that received it. A timeline
shows the copies you can read as one row, marked **Also in 1 other mailbox**. The
row names every record those copies are filed under that you can see. A copy you
cannot read stays a row of its own, marked withheld. How often you were in touch
with a contact counts the mail once.
Also called: duplicate email, the same email twice, one mail in two mailboxes.

## Why this one is held

A held Margince message carries a reason, so you can see why it is held:

| Reason | What happened |
|---|---|
| **Held until checked** | Nothing has judged it yet, and mail nothing has judged is held |
| **Held by your setting** | Your mailbox asked for it |
| **Held by the company** | An administrator turned mail sharing off |
| **Held by classification** | A model judged the thread and held it |
| **Marked confidential** | The sender said so in the subject line |
| **Held: counterparty mail** | You hold mail with one of the sides |
| **Kept private** | A human decided, and that decision stays |
| **Held: no record** | Something judged the sender, so it is filed under nothing |
| **Held: no counterparty** | It named nobody Margince could create a record for |
| **In a shared mailbox** | A shared mailbox brought it in, and it keeps its mail to its reader team |

A later verdict clears some of these and not others:

- **Held until checked** clears once Margince has sorted the thread. It stays
  on a thread with an attached file Margince could not read, such as a scan,
  because nobody knows what that file holds. The mailbox owner can still choose
  **Share with the company**.
- **Kept private** stays. Only your decision or your agent's sets it, and nothing
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
To change who can read mail captured from your mailbox in Margince, open **Settings → Connections**, open the mailbox, and change **Mail visibility**.
1. Open the account menu, choose **Settings**, then **Connections**.
2. Press the arrow at the end of the mailbox's row.
3. Set **Mail visibility** to **Held until checked**, **Always held** or **Shared with the team**.
4. When you narrow it, the row **Apply to captured mail?** shows. Tick **Also narrow captured mail** to narrow mail already captured too.
5. Press **Save** on the **Unsaved changes** bar.

**Shared with the team** is refused until an administrator allows it for the company.
Also called: mailbox privacy, mail posture, share my inbox.

## The three postures a mailbox can ask for

You set the mailbox posture at **Settings → Connections**, in each mailbox's
settings, as **Mail visibility**.

**Held until checked** is the default for every new mailbox. A message stays
with whoever was on it until a model finds the thread to be plain business.
Nothing is shared before a decision. So a model that is down or out of money
leaves mail held, not open.

**Always held** works the same way, without the model. You share a thread
yourself, one at a time.

**Shared with the team** makes mail open to read the moment it comes in. It is
off unless an administrator allows it for the company. In Germany and Austria,
reading a colleague's mailbox into a shared CRM needs a works council
agreement. Where there is no works council, each colleague must agree. Margince does not check that you have one.

Changing the posture applies to mail captured after the change. The same dialog
offers to narrow what is already captured. It never opens up captured mail; to
open older mail, share its threads one by one.

### How do I turn email sharing off for the whole company?
To turn email sharing off for everyone in Margince, an administrator opens **Settings → Mail capture**. In the **Mail sharing and sync** card, they switch off **Share captured mail with the team**.
1. Open **Settings**, then **Mail capture**.
2. In **Mail sharing and sync**, switch **Share captured mail with the team** off and save.

From then on, only those on each message can see new mail. The card warns: "With email sharing off, the CRM is hard to use".
The same card holds **Allow mailboxes to share on arrival**, which lets mailboxes pick **Shared with the team**.
Only an administrator or an Ops user can change this.

## The company-wide floor

**Settings → Mail capture → Sharing and sync** decides whether captured mail is
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
covers the whole domain, which is most often what you want for a lawyer or an
accountant.

It holds mail from then on, and lifting it opens nothing again: "Lifting applies
to new mail. Mail already held stays held."

### How do I share an email thread with my team?
To share an email thread in Margince, open the captured message from a timeline and choose **Share with the company** beside its visibility mark. Or open Settings → **Connections** → **Activity**, find it under **Mail not shared with the team**, and choose **Preview and change sharing**.
1. On a contact, company or deal timeline, open the email.
2. Beside the mark under the subject, press **Share with the company**. It "Applies to the whole thread."
3. To go back, press **Make private**.

Or choose **Preview and change sharing** on the thread under **Mail not shared with the team**, in Settings → **Connections** → **Activity**. It opens the email.
Only the mailbox owner can share. If a colleague also holds it, you see how many other seats still do.
Also called: release an email, unhide a thread, make an email public.

### How do I find a contact's mail that is not shared with the team?
To find mail with one contact that your mailbox holds back, open the contact's timeline. On a held email, choose **Show held mail with this contact**. It opens **Mail not shared with the team** with only that contact's threads; **Show all threads** drops the filter.
Also called: hidden mail with a contact, private threads, held email.

## Sharing a thread

You change who can read a captured message by **sharing its thread**.

Sharing lets go of **your own hold only**. If a colleague still holds the same
message, Margince tells you how many other seats do, but never who. A thread opens
to the team only when every mailbox that holds it lets go.

That is the rule when a message reached two mailboxes. Each owner adds what
their own mailbox asks for, and the message ends at the **strictest** of those.

### How do I change who can read one logged message?
To change who can read one logged message in Margince, use **Change visibility** under **More actions** (⋯) on its row.
1. On the timeline, find the message, open **More actions** (⋯) on its row and press **Change visibility**.
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

### How do I connect a shared mailbox?
<!-- opens: settings/integrations -->
To connect a shared mailbox in Margince, an administrator opens **Settings → Integrations → Shared mailboxes** and presses **Connect a shared mailbox**.
1. Choose the mail service: **IMAP**, **Microsoft 365** or **Google Workspace**.
2. Enter the **Mailbox address**, choose the **Team that reads it**, and choose **Held** or **Shared**.
3. Enter the login of the mail service, then press **Connect a shared mailbox**.

**Change** on its row changes the team or the visibility, for all mail it already captured. **Import older mail** and **Disconnect** are in the row's menu.
Also called: sales@ inbox, team mailbox, group mailbox.

## Shared mailboxes

A shared mailbox, such as sales@ or support@, belongs to no one user. An
administrator connects it and names one team as its readers.

**Who reads it.** The members of its reader team read its mail, and so do the
those on each message. Nobody else does, whatever their rank or the records
they can open. When the same email also reached a colleague's own mailbox, that
mailbox's rules count too, and the strictest of them wins.

**Held or shared.** A shared mailbox is **Held** or **Shared**. **Held** keeps
each message to the reader team and those on it. **Shared** opens its mail
to everyone who can open the records it is filed under. An administrator must
first allow mailboxes to share, in **Settings → Mail capture**.

A change of readers or of held and shared applies to all mail the mailbox has
already captured, not only to new mail.

**Removing a team member.** A member taken off the reader team loses its mail
at their next read. Their Home stops showing it at once, too.

**A delete in the shared mailbox.** Deleting a message in the mail service
removes nothing in Margince.

Mail that came in only through a shared mailbox adds no contact, company, task
or Worklist row. It is still filed under records that already exist.

When you open such an email, the reason under its subject says **In a shared
mailbox**. The line below it names the mailbox and the team that reads it.

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
| The company-wide floor | Settings → Mail capture → Sharing and sync |
| Hold mail with one contact or company | Private correspondence, on the contact's or company's page |
| Share one thread | The message itself, or Settings → Connections → Activity → Mail not shared with the team |
| One logged message's audience | Change visibility, under More actions on its row |
| A shared mailbox and who reads it | Settings → Integrations → Shared mailboxes |
| Who can see the *record* | The contact or company header |
