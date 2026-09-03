# What is kept, what is destroyed

This page answers four questions: what Margince keeps, for how long, what a
delete actually destroys, and what you can get out of the system.

Read it before you promise anything to a customer or an auditor.

## A new organization starts with six rules

Margince does not ship empty here. A new organization is created with six
retention rules already in place, so it is compliant with storage-limitation
from the first day rather than after someone remembers to configure it.

These are the defaults. All six are editable.

| What it covers | Kept for | Then |
|---|---|---|
| Leads that never converted | 365 days | Anonymise |
| All captured activity | 1095 days (3 years) | Archive |
| Call transcripts | 365 days | Erase |
| People with no consent and no deal | 730 days (2 years) | Anonymise |
| Lost deals | 1825 days (5 years) | Archive |
| AI call payloads | 365 days | Erase |

There is a seventh scope you can write a rule for — **Won deals** — and it is
deliberately left empty. The product takes no view on when your organization
should stop keeping a won deal. That is your decision, not its default.

If you delete every rule, the screen tells you what that means: "No retention
policy yet — nothing in this installation ages out."

### What each window counts from

This matters more than people expect.

- **Leads** and **people** count from when the record was created.
- **Captured activity** and **call transcripts** count from the message's own
  date — when it was sent or received, not when it was filed.
- **Deals** count from when the deal was closed.

### The three actions

- **Archive** — the record is kept. It leaves the live lists, and it still
  exists.
- **Anonymise** — identifying data is destroyed, the record survives.
- **Erase** — the data is destroyed.

The app draws the line for you: "Archive keeps the record; anonymise and erase
destroy data."

Anonymise is **not** erase-minus-a-detail. The two clear different things. An
erasure also reaches the raw captured messages, the attachments those messages
carried, the person's lead rows and scores, their unsubscribe tokens and their
Deal Room seats. Anonymising leaves all of those.

### What anonymise and erase actually do

**Anonymising a lead** replaces the name with "Anonymized Lead", clears the
email, title and company, and deletes the lead's score history.

**Anonymising a person** clears the names, title and postal address, sets the
name to "Erased Subject", clears every custom field, and deletes their email
addresses, phone numbers, social handles and channel identities. No suppression
entry is written — the person may lawfully come back.

**Erasing a call transcript** clears the body and replaces the subject with
"Erased", and purges the attachments — the bytes, not just the rows. It
deliberately keeps who the meeting was with and when. The record of the meeting
survives and its content goes, because who it was with *is* the record.

### Writing your own

A rule has three parts: **Applies to**, **Window in days**, and **Action**. A
window is a whole number of days, **at least 1** — a zero-day window would act
on a record the moment it was created.

Each scope carries **at most one** rule. There is no stacking, and you cannot
re-point an existing rule at a different scope; a different scope is a different
rule.

Not every combination is allowed. The legal pairs are: erase or anonymise a
person, archive or erase an activity, archive a deal, erase an AI call payload,
and anonymise a lead.

A rule can carry an optional **lawful basis** — the Article 6 basis the window is
argued from — recorded for whoever audits the row later. The six seeded rules all
carry "storage limitation".

Rules act **nightly**, and a live one shows as "Acting nightly". Each pass
handles up to 200 records per rule, so the first run against years of backlog
drains over several nights rather than in one.

### One window nobody can change

AI embedding call traces are kept for **90 days**, fixed. It is an operational
cap, not a per-organization setting, and no administrator can edit it.

### Turning a policy off, versus deleting it

These are different and the app is careful about it.

- **Enabled off** — the rule pauses and keeps its window. Nothing in that scope
  ages out while it is off, and the window is still there when you turn it back
  on.
- **Delete policy** — the rule is gone entirely, and nothing in that scope ages
  out any more.

If you just want to pause, turn Enabled off. Do not delete.

### Retain-only posture

There is one switch above all the policies. While **retain-only** is on:

> this installation destroys nothing: no anonymising and no erasing, whatever a
> policy below says. Archiving still runs — an archived record is kept, not
> destroyed.

A policy that would destroy data shows as "Suppressed by retain-only" rather
than looking active. It will not act until the posture is turned off.

### Who can change this

Only an **Admin** or **Ops**. Everyone else cannot even read the ladder, and the
app says why rather than hiding the section: "It sets what this installation
keeps for everybody, so it is not shown more widely."

## Archive is not delete

Across the whole product, ordinary removal is **soft**. Archiving sets a date on
the record; the record stays fetchable. It leaves the live lists. It does not
cease to exist.

Some examples of the difference in the product's own words:

- Archiving a project "removes this project from the live list and frees its
  key." The project is then read-only: "This project is archived and takes no
  changes."
- Archiving a document set: "The set and everything filed in it stop being
  searchable. Nothing is destroyed."
- Merging two records destroys nothing either. Both values survive a merge;
  choosing a side decides which record stands and which value is shown first.

Genuine destruction happens in exactly two ways: a retention policy whose action
is anonymise or erase, and a fulfilled erasure request.

## The privacy inbox

**Settings → Privacy inbox** holds data-subject requests with their statutory
deadlines. Only an admin can see it, because the queue names the people who
asked.

A request has a kind, a subject, an assignee, a due date, and a resolution. It
moves through **In progress** and is closed by **Fulfil** or **Reject**.
Closing one requires you to write the answer — "Closing a request needs its
answer."

**A closed request never reopens.** A new concern is a new request.

If two people open the same request, the second is told "This request moved on —
someone else decided it first" rather than being allowed to decide it twice.

### Access requests are done by hand

This is the most important thing on this page for anyone planning their process,
and the product states it without softening:

> An access request is fulfilled by hand: record what you sent in the
> resolution. **This system does not assemble or export the data for you.**

There is no "download everything about this person" button. If you need to
answer a subject access request, you gather the data yourself and record what
you sent. Plan for that.

### Erasure requests

An erasure request **must name a person in this organization**. A free-text
subject cannot be erased, because there is no record to erase.

Fulfilling one is deliberately hard to do by accident. You type **ERASE** to
confirm, and the warning is exact:

> This permanently erases the person across the whole system — record, captured
> activity, and derived values. It cannot be undone. The erasure is itself
> audited.

So an erasure reaches the contact record, everything captured about them, and
the values derived from that. And the act of erasing is itself written into the
audit trail.

## When erasure does not win: the retention floor

Sometimes the law requires keeping something that a person has asked you to
delete. Margince handles this as a visible, named state rather than a silent
partial success.

When an erasure hits a statutory retention obligation, you are told:

> Blocked — legal hold. This person is inside a statutory retention window, so
> erasure does not win here (Art. 17(3)(b)). **The block applies to every role,
> including admin — there is no override.** The attempt was audited.

There is no administrator who can force it. That is the point.

### Restricted records

What is held that way appears under **Restricted records**: which record, why,
and until when. Held records are:

- hidden from every ordinary view
- unchangeable
- redacted now — identifiers are removed immediately, and the screen reports how
  many fields were removed
- erased when the window closes

The correspondence itself is not shown, and the app says why: "it is restricted
precisely so it is not read."

The record kinds that can be held this way are Email, Call, Meeting and Message,
under a class the app calls **Commercial correspondence**.

An admin or ops can **pin** a record under the floor by hand, for correspondence
the automatic rule cannot recognise — the app gives supplier and purchasing mail
as the example, which qualifies under §257 HGB and has no deal in this product
to hang off.

An admin or ops can also **Release** a held record. Read what that means before
doing it:

> Releasing ERASES the record. It does not put it back in use: the erasure
> request this obligation suspended is still outstanding, so lifting the
> obligation completes it. This cannot be undone.

Release is not "make it usable again". Release is "finish the deletion". Every
decision here is recorded in the audit trail with your name and your stated
reason.

If nothing is held, the screen says so, and it is good news: "No record is being
held — every erasure so far could be completed in full."

## Consent

**Settings → Privacy & audit** carries a registry of **purposes** — the reasons
this organization processes personal data. Each purpose has a key, a label, and
a flag for whether it requires double opt-in.

The catalogue is **append-only**. "A purpose cannot be renamed or removed once
created. Choose the key carefully."

Against a contact you can **Grant** or **Withdraw** consent for a purpose.

A purpose that requires **double opt-in** cannot be granted from this screen at
all. Only the contact can confirm one, by opening a single-use link mailed to
their own recorded address — use **Ask them to confirm their details**. This is
the point of double opt-in: a confirmation an employee can complete on the
contact's behalf is not evidence that the contact agreed.

The default is **deny**. A purpose with no record for a person is not consent.

Every consent change is written to a **proof log** that records who did it and
how: a Human, an Agent, the System, or a Connector — or honestly "actor not
recorded" and "source not recorded" where nothing is known. Consent is a claim
you may one day have to defend, so the product keeps the evidence rather than
just the current state.

Agents cannot write consent at all. See
[What the AI does](what-the-ai-does.md).

## The audit trail

**Settings → Privacy & audit → Audit log** records "every action, attributed —
human, agent, or connector."

You can filter it by actor, entity type, entity id, action, and a date range,
and expand any entry to see the change in detail and the authorization rule that
allowed it. Where something was done on someone's behalf, the entry says so.

Only an admin can read the full trail: "It records every actor and every record
they touched, so it is not shown more widely."

The audit trail is how you find out what actually happened. It is also where a
record's id is shown in full, which you need if you ever pin a record under the
retention floor by hand.

## Exporting records

There is one export in the product, and it is worth knowing exactly what it is
so you do not promise more than it does.

From a list you can press **Export CSV** or **Export JSON**. Those are the only
two formats.

You can export five kinds of record: **contacts, companies, deals, leads and
projects**. You export either a filtered list, a saved view, or a dynamic list.

Three things about it:

- **It exports only what you can see.** The rows go through exactly the same
  visibility rules as the list on your screen.
- **It is human-only.** An agent cannot export.
- **Every export writes an audit entry.** Someone can always find out who took a
  copy of what, and when.

This is a record export, not a person export. It does not assemble everything
held about one individual — see the access-request section above for why that is
a manual job.
