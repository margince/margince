# What is kept, what is destroyed

This page answers four questions: what Margince keeps, for how long, what a
delete actually destroys, and how you get data in and out of the system. Read it
before you promise anything to a customer or an auditor.

## The general rule: deleting in Margince archives

**In Margince, removing a contact, company, deal, lead or project never destroys
it.** Removal is an archive: the record leaves the live lists, becomes read-only,
and is still stored, with its history and audit trail. Margince has no delete
button for records. Data is destroyed in exactly two ways: a retention policy
whose action is anonymise or erase, and a fulfilled erasure request. Files and
knowledge documents are the exception: those can be deleted outright.

## Getting data out, getting data in, and removing records

### How do I export my data from Margince?
To export or download records from Margince — for example all your contacts — open **Filters and views** in the sidebar, build a filter, then choose **Export CSV** or **Export JSON**.
1. Open **Filters and views**.
2. Pick the **Record type**: **Contacts**, **Companies** or **Deals**.
3. Choose **Add clause** and complete at least one clause. The export buttons appear once the filter is complete.
4. Choose **Export CSV** or **Export JSON**. The file downloads.
The export holds only rows you can see, and every export is written to the audit log. Agents cannot export.
Also called: download, get my data out, backup, extract to Excel or spreadsheet.

### How do I get all of my company's data out of Margince?
Margince has a whole-company export bundle (a ZIP of one CSV per record type, relational JSON and manifests), but the app has no button for it. An Admin or Ops user downloads it from the API (`GET /exports/bundle`) while signed in; it is row-scoped to them and audited. For everyday exports use **Filters and views → Export CSV**.
Also called: full export, data handover, migrate away, leave Margince.

### How do I import contacts from a spreadsheet?
To import contacts, companies or leads from a spreadsheet, save it as CSV and open **Settings → Data import**, then choose **Start import**.
1. Choose **Start import** under **Import CSV file**.
2. Pick the **Row type**: **Prospects** (leads), **Companies** or **Contacts**.
3. Choose **Choose file** and pick the CSV.
4. In **Column mapping**, pick a **Field** per column or **Do not import**.
5. Choose **Preview import**, check the **Import preview**, then choose **Import {rows} rows**.
Only an administrator or operations user can import. Nothing is written before you confirm.
Also called: upload a CSV, bulk import, Excel import.

### How do I undo an import?
To undo a CSV import in Margince, open **Settings → Data import** and choose **Undo import ({rows} rows)** on the import result.
Undo archives the rows that import created (a lead is disqualified), unless someone edited them since. Those are listed under "Kept because they were edited after the import:". Rows the import only updated are not changed back. If the undo stops partway, choose **Continue undo**. When done it says **Import undone**.
Also called: reverse an import, roll back an upload.

### How do I delete a contact permanently?
Margince has no delete button for a contact: **More actions → Archive** hides it but keeps it. To destroy a contact's data for good, an administrator fulfils an **erasure** request in **Settings → Privacy and retention → Privacy requests** (see below). Companies, deals, leads and projects have no delete button either; an erasure request always names a contact. A lead's data is destroyed only by the **Leads that never converted** rule or with the contact it belongs to; a deal is only ever archived, never destroyed; companies and projects have no retention scope at all.
Also called: remove a contact, hard delete, purge, wipe.

### How do I handle a GDPR erasure request?
To erase a contact on request, an administrator opens **Settings → Privacy and retention**, goes to **Privacy requests** and chooses **New request**.
1. Set **Kind** to **erasure** and pick the **Contact** (required).
2. Set **Due**, then choose **Open request**.
3. On the request, write the **Resolution** and choose **Fulfill**.
4. Type ERASE in **Type ERASE to confirm** and choose **Erase and suppress**.
This cannot be undone. A contact inside a statutory retention window is refused with **Blocked by legal hold**.
Also called: right to be forgotten, Art. 17, delete my data, data subject request, DSR.

### How do I answer a data access request?
To log a GDPR access request, an administrator opens **Settings → Privacy and retention → Privacy requests**, chooses **New request**, sets **Kind** to **access**, fills **Subject reference** and **Due**, and chooses **Open request**. The app says: "An access request is fulfilled manually: record what you sent in the resolution. This system does not assemble or export the data for you." Gather the data (or have the package assembled, see below), send it, then write the **Resolution** and choose **Fulfill**.
Also called: subject access request, SAR, Art. 15, what do you hold about me.

### How do I archive a record?
To archive a contact or company, open it and choose **More actions → Archive**, then confirm "Archive this record? There is no undo." For a deal, choose **Archive deal**; for a project, **Archive project**. Leads are not archived: choose **Disqualify**. To see archived rows on a list, choose **Show archived**.
Also called: hide, remove, retire a record.

### How do I restore an archived record?
Margince cannot restore an archived contact, company, deal or project: there is no unarchive. The archived record can still be opened read-only, and **Show archived** on a list shows it. Individual field changes can be reversed with **Undo** in the record's **History**. Tags and pipelines have their own **Restore**; an archived team comes back with **Undo** on the "Team archived" notice.
Also called: unarchive, undelete, recover.

### How do I change how long Margince keeps data?
To change a retention rule, an Admin or Ops user opens **Settings → Privacy and retention** and uses the **Retention** card.
1. Choose **Edit** on a rule, or **Add policy** for a new one.
2. Set **Applies to**, **Window in days** (a whole number, at least 1) and **Action**.
3. Optionally fill **Lawful basis**, then choose **Save policy** or **Create policy**.
To pause a rule, turn **Enabled** off; **Delete policy** removes it entirely.
Also called: data retention, retention period, auto-delete.

### How do I record a contact's consent?
To record consent, or to mark a contact as do-not-contact for a purpose, open the contact, find the **Communication permissions** panel and choose **Manage consent and proof history**; in the drawer choose **Record consent** for a purpose, or **Withdraw**. A purpose that needs double opt-in cannot be recorded by staff; choose **Ask them to confirm their details** on the same panel to mail the contact a private link instead. Purposes are added in **Settings → Privacy and retention → Consent purposes → Add purpose**.
Also called: opt-in, opt-out, do not contact, unsubscribe someone, marketing permission, GDPR consent.

## Archive is not delete

Across the whole product, ordinary removal is **soft**. Archiving sets a date on
the record; the record stays fetchable. It leaves the live lists. It does not
cease to exist.

Some examples of the difference in the product's own words:

- Archiving a project "removes this project from the active list and frees its
  key." The project is then read-only: "This project is archived and takes no
  changes."
- Archiving a document set stops the set and everything filed in it being
  searchable. Nothing is destroyed.
- Merging two records destroys nothing either: "Merge {source} into {target}?
  This archives {source}." Both values survive a merge; choosing a side decides
  which record stands and which value is shown first.

## A new company starts with six retention rules

Margince does not ship empty here. A new company is created with six retention
rules already in place, so it is compliant with storage limitation from the
first day rather than after someone remembers to configure it. All six are
editable.

| What it covers | Kept for | Then |
|---|---|---|
| Leads that never converted | 365 days | Anonymise |
| All captured activity | 1095 days (3 years) | Archive |
| Call transcripts | 365 days | Erase |
| Contacts with no consent and no deal | 730 days (2 years) | Anonymise |
| Lost deals | 1825 days (5 years) | Archive |
| AI call payloads | 365 days | Erase |

Two more scopes exist that you can write a rule for, and both are deliberately
left empty:

- **Won deals.** The product takes no view on when your company should stop
  keeping a won deal. That is your decision, not its default.
- **Stored originals.** Capture keeps the original of every message it filed,
  separately from the timeline entry it became. Until you write a rule here,
  nothing ages those originals out on their own; they are only reached when
  the activity they belong to is reached, or by an erasure. On a busy mailbox
  they are usually the largest thing in the database, so this is the rule worth
  writing first.

  What it may destroy is bounded by the activity it belongs to: an original is
  never destroyed while the correspondence it is the original *of* is held under
  the statutory floor, and the timeline entry stays standing so the same message
  cannot be captured again.

If you delete every rule, the screen tells you what that means: "No retention
policy yet. Nothing in this installation ages out."

### What each retention window counts from
The start of a retention window matters more than readers expect. **Leads** and
**contacts** count from when the record was created. **Captured activity** and
**call transcripts** count from the message's own date: when it was sent or
received, not when it was filed. **Deals** count from when the deal was closed.

### The three retention actions

The three retention actions are **Archive**, where the record is kept, leaves
the live lists and still exists; **Anonymise**, where identifying data is
destroyed and the record survives; and **Erase**, where the data is destroyed.

The app draws the line for you: "Archive keeps the record. Anonymize and erase
destroy data and are held back in retain-only mode."

Anonymise is **not** erase-minus-a-detail. The two clear different things. An
erasure also reaches the raw captured messages, the attachments those messages
carried, the contact's lead rows and scores, their unsubscribe tokens and their
Deal Room seats. Anonymising leaves all of those.

### What anonymise and erase actually do
**Anonymising a lead** replaces the name with "Anonymized Lead", clears the
email, title and company, and deletes the lead's score history.

**Anonymising a contact** clears the names, title and postal address, sets the
name to "Erased Subject", clears every custom field, and deletes their email
addresses, phone numbers, social handles and channel identities. No suppression
entry is written: the contact may lawfully come back.

**Erasing a call transcript** clears the body and replaces the subject with
"Erased", and purges the attachments (the bytes, not just the rows). It
deliberately keeps who the meeting was with and when. The record of the meeting
survives and its content goes, because who it was with *is* the record.

### Writing your own retention rule

A retention rule has three parts: **Applies to**, **Window in days**, and
**Action**. A window is a whole number of days, **at least 1**; a zero-day
window would act on a record the moment it was created.

Each scope carries **at most one** rule: "A policy for this scope already
exists; each scope has at most 1 rule. Edit the existing policy instead." There
is no stacking, and you cannot re-point an existing rule at a different scope.

Not every combination is allowed, because only some have anything to execute
them: there is no way to archive an AI call payload, or to anonymise an
activity. The legal pairs are: erase or anonymise a contact, archive or erase
an activity, archive a deal, erase an AI call payload, erase a stored original,
and anonymise a lead.

A rule can carry an optional **Lawful basis** (the Article 6 basis the window is
argued from), recorded for whoever audits the row later. The six seeded rules
all carry "storage limitation".

Rules act **nightly**, and a live one shows as "Acting nightly". Each pass
handles up to 200 records per rule, so the first run against years of backlog
drains over several nights rather than in one.

### One window nobody can change

AI embedding call traces are kept for **90 days**, fixed. It is an operational
cap, not a per-company setting, and no administrator can edit it.

### Turning a retention policy off, versus deleting it

Turning a retention policy off and deleting it are different, and the app is
careful about it.

- **Enabled off**: the rule pauses and keeps its window. Nothing in that scope
  ages out while it is off, and the window is still there when you turn it back
  on.
- **Delete policy**: "This removes the rule for {scope} entirely, so nothing in
  that scope ages out anymore. To pause the rule and keep its window, turn off
  Enabled instead."

### Retain-only mode

**Retain-only mode** is one switch above all the retention policies. While it is
on:

> While on, this installation destroys nothing: no anonymizing and no erasing,
> whatever a policy below says. Archiving still runs; an archived record is
> kept, not destroyed.

A policy that would destroy data shows as "Paused by retain-only mode" rather
than looking active. It will not act until the mode is turned off.

### Who can change retention
Only an **Admin** or **Ops** user can change retention. Everyone else cannot
even read the rules, and the app says why rather than hiding the section: "Only
an administrator or operations user can see retention policies. They set what
this installation keeps for everyone."

## Privacy requests

The privacy request queue, **Settings → Privacy and retention → Privacy
requests**, holds "Data subject requests with their statutory deadlines". Reaching it takes
the **privacy request** grant, because the queue names whoever asked; the grant
is seeded to admins and can be delegated on its own, without member
administration.

A request has a kind (access, rectify or erasure), a subject, an assignee, a due
date, and a resolution. It moves through **In progress** and is closed by
**Fulfill** or **Reject**. Closing one requires you to write the answer:
"Closing a request needs its answer." Once set, an assignee cannot be cleared.

**A closed request never reopens.** "Closed. A closed request cannot be
reopened; a new concern needs a new request."

If two colleagues open the same request, the second is told "Someone else
decided this request first. Review the current state below." rather than being
allowed to decide it twice.

### Access requests: by hand in the app, a package through the API

The screen states it without softening:

> An access request is fulfilled manually: record what you sent in the
> resolution. **This system does not assemble or export the data for you.**

There is no "download everything about this contact" button in the app, but the
product does assemble the Art. 15 package: the contact's record, their
correspondence, their consent history, and the evidence of why the contact exists
at all. It is served by `GET /data-subject-requests/{id}/package`, which nothing
in the interface calls yet, so reaching it means calling that endpoint directly.

- **It is privileged twice.** Reaching the request takes the **privacy request**
  grant and a human; assembling the package also takes the **contact delete**
  grant over an unbounded row scope, and a human again, so no agent passport can
  assemble one.
- **It answers access requests only**, never erasure or rectification.
- **It does not close the request.** Mark it fulfilled yourself once you have
  actually sent it, and record what you sent in the resolution.

### Erasure requests

An erasure request **must name a contact in this company**: "An erasure request
must name a contact in this company, because fulfilling it erases that record. A
free-text subject cannot be erased."

Fulfilling one is deliberately hard to do by accident. You type **ERASE** to
confirm, and the warning is exact:

> This permanently erases the contact across the whole system: record, captured
> activity and derived values. It cannot be undone. The erasure itself is
> audited.

The confirm button reads **Erase and suppress**.

## When erasure does not win: the retention floor

Sometimes the law requires keeping something that a data subject has asked you
to delete. Margince handles this as a visible, named state rather than a silent
partial success.

When an erasure hits a statutory retention obligation, you see **Blocked by
legal hold**:

> This contact is inside a statutory retention window, so erasure does not take
> precedence here (Art. 17(3)(b)). **The block applies to every role, including
> administrators, with no override.** The attempt was audited.

There is no administrator who can force it. That is the point.

### Restricted records

**Restricted records** are what a statutory retention obligation holds after an
erasure: "which record, why and until when." Held records are hidden from every
ordinary view, unchangeable, redacted now — identifiers are removed immediately,
and the screen reports how many fields were removed — and erased when the window
closes. The correspondence itself is hidden "so that it is not read."

The record kinds that can be held this way are Email, Call, Meeting and Message,
under a class the app calls **Commercial correspondence**.

An admin or ops user can **Pin a record** under the floor by hand, for
correspondence the automatic rule cannot recognise. The app gives supplier and
purchasing mail as the example, which qualifies under §257 HGB and has no deal in
this product to hang off. The record ID is on its audit entry.

An admin or ops user can also **Release** a held record. Read what that means
before doing it:

> Releasing ERASES the record; it does not return it to use. The erasure request
> this obligation suspended is still open, so releasing completes it. This
> cannot be undone.

Release is not "make it usable again". Release is "finish the deletion". Every
decision here is recorded in the audit trail with your name and your stated
reason.

If nothing is held, the screen says so, and it is good news: "No records held.
Every erasure so far was completed in full."

## Consent

Consent in Margince is recorded per **purpose**. **Settings → Privacy and
retention → Consent purposes** carries the registry of purposes: the reasons
this company processes personal data. Each purpose has a key, a label, and a
flag for whether it requires double opt-in.

The catalogue is **append-only**: "A purpose cannot be renamed or removed once
created; the catalog is append-only. Choose the key carefully." Against a
contact you can **Record consent** or **Withdraw** consent for a purpose.

A purpose that requires **double opt-in** cannot be recorded by staff at all.
Only the contact can confirm one, by opening a single-use link mailed to their
own recorded address: use **Ask them to confirm their details**. This is the
point of double opt-in: a confirmation an employee can complete on the
contact's behalf is not evidence that the contact agreed.

The default is **deny**. A purpose with no record for a contact is not consent.

Every consent change is written to a **Proof log** that records who did it and
how: a Human, an Agent, the System, or a Connector, or honestly "actor not
recorded" and "source not recorded" where nothing is known. Consent is a claim
you may one day have to defend, so the product keeps the evidence rather than
just the current state.

Agents cannot write consent at all; see [What the AI does](what-the-ai-does.md).

## The audit trail

The audit trail, **Settings → Audit log**, records "Every action, attributed to
a user, agent or connector".

You can filter it by **Actor**, **Entity type**, **Entity ID**, **Action**, and a
date range (**From**, **To**), and choose **Show change detail** on any entry to
see the change and the **Authorization rule** that allowed it. Where something
was done on someone's behalf, the entry says so.

Only an admin can read the full trail: "Your role cannot read the full audit
log. It records every actor and every record they accessed."

It is also where a record's full ID is shown, which pinning a record under the
retention floor by hand needs.

## What the record export covers

There are two exports, and they answer different questions.

#### The list export

On **Filters and views** you press **Export CSV** or **Export JSON**; those are
the only two formats. The screen offers three record types: **contacts,
companies and deals**. The export API also accepts leads and projects. You
export either a filtered slice or a saved view.

**It exports only what you can see**: the rows go through exactly the same
visibility rules as the list on your screen. **It is human-only**: an agent
cannot export. **Every export writes an audit entry**, so someone can always
find out who took a copy of what, and when.

It is a record export, not a contact dossier: everything about one individual
is the subject-access package above.

#### The whole-workspace bundle

The installation's data handover: every object as CSV, the relationships as
JSON, and manifests describing both, in the open `margince-export/1` format —
the export for "give us our data", whether for a migration, an audit or a
portability request. It is admin and ops only, audited, and row-scoped to whoever
asks. No screen offers it yet: it is served by `GET /exports/bundle`.
