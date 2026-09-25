# Who can see an email

Who can see an email in Margince is the question the product gets asked most,
and the answer is spread over several settings, so it is worth one page.

Two different things decide who can read an email, and **a "no" from either is a
no**:

1. **Record visibility** — who may discover the contact, company or deal the
   message is filed against.
2. **The message's own audience** — who may read *this* message, whatever they
   can see elsewhere.

The message's audience is not overridden by seniority. A user who can read every
record in the company still does not read a message they were not an audience
for. An admin is no exception, and neither the audit log nor an export names a
held message's subject or attachments.

### Who can see an email I captured or sent?
To see who can read an email in Margince, open the message from a timeline: the line under its subject shows **Team**, **Shared**, **Participants**, **Selected** or **Withheld**, with a sentence saying what that means.
- **Shared**: "Everyone in the company can read this."
- **Team**: "Everyone who can open the records this is filed against can read it."
- **Participants**: only those on the message can read it.
- **Selected**: only the colleagues and teams named below it.
- **Withheld**: you are not one of them, so the content is hidden from you.
Also called: email privacy, who can read my mail, email permissions.

## The audience marks

Every Margince message row carries one audience mark:

| Mark | What it means |
|---|---|
| **Shared** | Everyone in the company can read this |
| **Team** | Everyone who can open the records it is filed against |
| **Participants** | Only those on the message can read it |
| **Selected** | Only those named can read it |
| **Withheld** | You are not one of them |

"Team" never means a team in the Teams sense — it is about the audience, not a
named group. Who may discover the linked record still decides whether the row
appears at all.

A withheld message is **not hidden**. You see the row — its date, its direction,
and the record it is filed against — with the content withheld: "This message is
not shared with you." You learn that a conversation happened and nothing about
what was said in it.

### Why is an email hidden from me?
When an email in Margince shows "This message is not shared with you" or **Withheld**, you were not in its audience: the mailbox that captured it is holding it to the colleagues who were on it.
Common reasons: the thread is **Held until classified** and nothing has judged it yet; its owner set their mailbox to **Always held**; the sender or domain is under **Private correspondence**; or an administrator turned **Email sharing** off.
Only the mailbox owner can release it, by choosing **Share with the company** on the message. Ask them; an administrator cannot open it for you.
Also called: email not visible, cannot read email, content for participants only.

## Why this one is held

A held Margince message carries a reason, and the product names nine of them
rather than leaving you to guess. A verdict nobody can see is a verdict nobody
can correct.

| Reason | What happened |
|---|---|
| **Held until classified** | Nothing has judged it yet, and unjudged is held |
| **Held by your setting** | Your mailbox asked for it |
| **Held by the company** | An administrator turned mail sharing off |
| **Held by classification** | A classifier judged the thread and held it |
| **Marked confidential** | The sender said so in the subject line |
| **Held: counterparty mail** | You hold mail with one of the parties |
| **Kept private** | A human decided, and that decision is a lock |
| **Held: no record** | Something judged the sender, so it is filed under nothing |
| **Held: no counterparty** | It named nobody a record could be created for |

Which of those a later verdict can clear is the whole design:

- **Held until classified** is exactly what a verdict is for, and clears.
- **Kept private** is a lock. Nothing but a human's own decision writes it, and
  the derivation refuses to move a row carrying it.
- **Held: counterparty mail** and **Marked confidential** both outrank a later
  verdict. A classifier concluding a thread is ordinary says nothing about
  whether you want your lawyer's mail in a shared CRM.
- **Held by the company** is not a mailbox posture and no verdict clears it.
  Only an administrator turning sharing back on opens those rows, and only for
  mail captured afterwards.
- **Held: no counterparty** is the one hold that a link lifts. No judgement was
  made about anyone, so it means only "nothing has filed it yet", and it stops
  being true the moment something does.

The reason itself is withheld along with the content, because a reason like
"held because personnel" describes what the message is about.

### How do I change who can read mail from my mailbox?
To change who can read mail captured from your mailbox in Margince, open **Settings → Connections** and change **Mail visibility** under that mailbox.
1. Open the account menu, choose **Settings**, then **Connections**.
2. Under the mailbox, set **Mail visibility** to **Held until classified**, **Always held** or **Shared with the team**.
3. When you narrow it, the **Apply to captured mail?** dialog offers **Also narrow captured mail**; confirm with **Change mail visibility**.
**Shared with the team** is refused until an administrator allows it for the company.
Also called: mailbox privacy, mail posture, share my inbox.

## The three postures a mailbox can ask for

The mailbox posture is set at **Settings → Connections**, under each mailbox, as
**Mail visibility**.

**Held until classified** — the default for every new mailbox. A message stays
with whoever was on it until a classifier judges the thread ordinary business.
Nothing is shared before a decision, so a classifier that is down or out of
budget leaves mail held rather than open.

**Always held** — the same, minus the classifier. You share a thread yourself,
one at a time.

**Shared with the team** — readable the moment it lands. Off unless an
administrator allows it for the company, because reading an employee's mailbox
into a shared CRM is what a works-council agreement covers in Germany and
Austria. Margince does not verify that one exists.

Changing the posture governs mail captured **afterwards**. The same dialog offers
to narrow what is already captured; it only ever narrows, because re-opening what
was captured under a stricter answer is a separate decision.

### How do I turn email sharing off for the whole company?
To turn email sharing off for everyone in Margince, an administrator opens **Settings → Capture rules** and switches off **Share captured mail with the team** in the **Email sharing** card.
1. Open **Settings**, then **Capture rules**.
2. In **Email sharing**, switch **Share captured mail with the team** off and save.
From then on, new mail is visible only to those on each message: "With email sharing off, the CRM is hard to use."
The same card holds **Allow mailboxes to share on arrival**, which makes **Shared with the team** available to mailboxes.
Only an administrator or operations user can change this.

## The company-wide floor

**Settings → Capture rules → Email sharing** decides whether captured mail is
shared with colleagues at all. On by default.

Turned off, every message captured from then on is held to its participants
whatever any mailbox asks for. The app warns you honestly: "With email sharing
off, the CRM is hard to use."

It is everybody's business rather than one seat's, which is why it lives with the
company rules and not on your own connections page.

### How do I keep all mail with a contact or company private?
To keep your mail with one contact or one company private in Margince, open their page and use **Private correspondence**: **Keep private** for the address, or **Keep all of {domain} private** for the whole firm.
1. Open the contact or company page.
2. In the **Private correspondence** panel, press **Keep private** or **Keep all of {domain} private**.
3. Confirm in **Keep this correspondence private?** with **Keep private**.
Mail is still captured and you can still read it; colleagues cannot. It covers new mail only. Undo it with **Lift**, which also applies to new mail only.
Also called: confidential client, lawyer mail, hide emails from colleagues.

## Holding your mail with one party

**Private correspondence**, on a contact's or a company's page, keeps your mail
with one party to the participants without deciding message by message. A domain
hold covers the whole firm, which is usually what you want for a lawyer or an
accountant.

It binds mail from then on, and lifting it re-opens nothing: "Lifting applies to
new mail. Mail already held stays held."

### How do I share an email thread with my team?
To share an email thread in Margince, open the captured message from a timeline and choose **Share with the company** beside its visibility mark, or use **Share with the team** in **Settings → Connections → Held threads**.
1. On a contact, company or deal timeline, open the email.
2. Beside the mark under the subject, press **Share with the company**. It "Applies to the whole thread."
3. To go back, press **Make private**.
Or press **Share with the team** on the thread under **Settings → Connections → Held threads**.
Only the mailbox owner can share; if a colleague also holds it, you see how many other seats still do.
Also called: release an email, unhide a thread, make an email public.

## Sharing a thread

You change a captured message's audience by **sharing its thread**, not by
editing the row.

Sharing releases **your own hold only**. If a colleague is still holding the same
message, the response tells you how many other seats are — and never who. The
**Held threads** card says the same: "A thread opens only when every recipient
agrees."

That is the rule when a message reached two mailboxes: each owner contributes
what their own mailbox asks for, and the message ends at the **strictest** of
those.

### How do I change who can read one logged message?
To change who can read one logged message in Margince, press **Change visibility** on its row and answer "Who may read this message?".
1. On the timeline, find the message and press **Change visibility**.
2. Choose **Everyone in the company**, **Participants only**, or name the colleagues and teams who may read it.
3. Press **Save visibility**.
It "Applies to this message only, not to the thread or the contact." Captured email does not offer this button; share or hold its thread instead.
Also called: restrict a message, message permissions.

## Changing one message's audience

**Change visibility** asks "Who may read this message?": everyone in the company,
participants only, or a named set of colleagues and teams. It is offered on
messages logged by hand, not on mail a mailbox captured.

**It reaches exactly one message.** The control says so: "Applies to this message
only, not to the thread or the contact."

That is the difference from sharing, above. **Share with the company** on a
thread "Applies to the whole thread." and releases your own hold on all of it;
changing a message's visibility moves that one row.

## What you can see of other seats' capture

Your own connections need no permission from anyone — it is your own mail.

Messages that arrived through a company-wide connection, like the Telegram bot,
belong to nobody in particular and are shown to seats granted that access.

**No permission grant ever reaches a colleague's mailbox.** The company-wide view
never returns another seat's personal rows.

## Where each control lives

| What you want | Where |
|---|---|
| This mailbox's posture | Settings → Connections → Mail visibility |
| The company-wide floor | Settings → Capture rules → Email sharing |
| Hold mail with one party | Private correspondence, on the contact's or company's page |
| Share one thread | The message itself, or Settings → Connections → Held threads |
| One logged message's audience | Change visibility, on its row |
| Who can see the *record* | The contact or company header |
