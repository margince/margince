# Who can see an email

This is the question the product gets asked most, and the answer is spread over
several settings, so it is worth one page.

Two different things decide it, and **a "no" from either is a no**:

1. **Record visibility** — who may discover the contact, company or deal the
   message is filed against.
2. **The message's own audience** — who may read *this* message, whatever they
   can see elsewhere.

The second is not overridden by seniority. Somebody who can read every record in
the company still does not read a message they were not an audience for. An
admin is no exception, and neither the audit log nor an export names a held
message's subject or attachments.

## The four answers

Every message row carries one mark:

| Mark | What it means |
|---|---|
| **Team** | Everyone in the company can read this |
| **Participants** | Only those on the message can read it |
| **Selected** | Only those named can read it |
| **Withheld** | You are not one of them |

"Team" never means a team in the Teams sense — it is about the audience, not a
named group. Who may discover the linked record still decides whether the row
appears at all.

A withheld message is **not hidden**. You see a row with its date and its kind,
and nothing else. You learn that a conversation happened and nothing about what
was in it.

## Why this one is held

A held message carries a reason, and the product names nine of them rather than
leaving you to guess. A verdict nobody can see is a verdict nobody can correct.

| Reason | What happened |
|---|---|
| **Held until classified** | Nothing has judged it yet, and unjudged is held |
| **Held by your setting** | Your mailbox asked for it |
| **Held by the company** | An administrator turned mail sharing off |
| **Held by a classification** | A classifier judged the thread and held it |
| **Marked confidential** | The sender said so in the subject line |
| **Held, mail with this party** | You hold mail with one of the parties |
| **Kept private** | A human decided, and that decision is a lock |
| **Held, no record** | Something judged the sender, so it is filed under nothing |
| **Held, nobody to file it under** | It named nobody a record could be created for |

Which of those a later verdict can clear is the whole design:

- **Held until classified** is exactly what a verdict is for, and clears.
- **Kept private** is a lock. Nothing but a human's own decision writes it, and
  the derivation refuses to move a row carrying it.
- **Held, mail with this party** and **Marked confidential** both outrank a
  later verdict. A classifier concluding a thread is ordinary says nothing about
  whether you want your lawyer's mail in a shared CRM.
- **Held by the company** is not a mailbox posture and no verdict clears it.
  Only an administrator turning sharing back on opens those rows, and only for
  mail captured afterwards.
- **Held, nobody to file it under** is the one hold that a link lifts. No
  judgement was made about anyone, so it means only "nothing has filed it yet",
  and it stops being true the moment something does.

The reason itself is withheld along with the content, because a reason like
"held because personnel" describes what the message is about.

## The three postures a mailbox can ask for

At **Settings → Connections**, under each mailbox.

**Held until classified** — the default for every new mailbox. A message stays
with whoever was on it until a classifier judges the thread ordinary business.
Nothing is shared before a decision, so a classifier that is down or out of
budget leaves mail held rather than open.

**Always held** — the same, minus the classifier. You share a thread yourself,
one at a time, from its row.

**Shared with the team** — readable the moment it lands. Off unless an
administrator allows it for the company, because reading an employee's mailbox
into a shared CRM is what a works-council agreement covers in Germany and
Austria. Margince does not verify that one exists.

Changing the posture governs mail captured **afterwards**. The same dialog offers
to narrow what is already captured; it only ever narrows, because re-opening what
was captured under a stricter answer is a separate decision.

## The company-wide floor

**Settings → Capture rules → Email sharing** decides whether captured mail is
shared with colleagues at all. On by default.

Turned off, every message captured from then on is held to its participants
whatever any mailbox asks for. The app warns you honestly that doing so "will
make usage of the CRM difficult."

It is everybody's business rather than one seat's, which is why it lives with the
company rules and not on your own connections page.

## Holding your mail with one party

**Private correspondence**, on a contact's or a company's page, keeps your mail
with one party to the participants without deciding message by message. A domain
hold covers the whole firm, which is usually what you want for a lawyer or an
accountant.

It binds mail from then on, and lifting it re-opens nothing.

## Sharing a thread

You change a captured message's audience by **sharing its thread**, not by
editing the row. The row refuses a direct edit and says so.

Sharing releases **your own hold only**. If a colleague is still holding the same
message, the response tells you how many other seats are — and never who.

That is the rule when a message reached two mailboxes: each owner contributes
what their own mailbox asks for, and the message ends at the **strictest** of
those.

## Setting the audience on a message you send

When you write a message, you set its audience before it goes: **Who may read
this message?** — everyone in the company, only the participants, or only those
you name.

It applies to the whole thread rather than the one message, which is what stops a
reply quietly widening what the original narrowed.

## What you can see of other seats' capture

Your own connections need no permission from anyone — it is your own mail.

Messages that arrived through a company-wide connection, like the Telegram bot,
belong to nobody in particular and are shown to seats granted that access.

**No permission grant ever reaches a colleague's mailbox.** The company-wide view
never returns another seat's personal rows.

## Where each control lives

| What you want | Where |
|---|---|
| This mailbox's posture | Settings → Connections, under the mailbox |
| The company-wide floor | Settings → Capture rules → Email sharing |
| Hold mail with one party | The contact's or company's page |
| Share one thread | The thread's own row on a timeline |
| A sent message's audience | The composer, before you send |
| Who can see the *record* | The contact or company header |
