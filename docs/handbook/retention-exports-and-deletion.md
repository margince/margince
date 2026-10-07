# What is kept, what is destroyed

What Margince keeps, for how long, what a delete destroys, and how you get data
in and out. Read it before you promise anything to a customer or an auditor.

## The general rule: deleting in Margince archives

**In Margince, removing a contact, company, deal, lead or project never destroys
it.** Removal is an archive: the record leaves the live lists, becomes read-only,
and is still stored, with its history and audit trail. Margince has no delete
button for records. Data is destroyed in only two ways: a retention policy
whose action is anonymise or erase, and a fulfilled erasure request. Files and
knowledge documents are the exception: those can be deleted outright.

## Getting data out, getting data in, and removing records

### How do I export my data from Margince?
To export or download records from Margince (for example all your contacts), open **Filters and views** in the sidebar, build a filter, then choose **Export CSV** or **Export JSON** under **⋯** (**More for this filter**).
1. Open **Filters and views**.
2. Press **New filter** and pick the record type.
3. Press **Add condition** and complete it.
4. Press **⋯** (**More for this filter**) at the foot, then **Export CSV** or **Export JSON**. The file downloads.
The export holds only records you can see, and every export is written to the audit log. Agents cannot export.
Also called: download, get my data out, backup, extract to Excel or spreadsheet.

### How do I get all of my company's data out of Margince?
Margince has a whole-company export bundle (a ZIP of one CSV per record type, plus files describing how records link), but the app has no button for it yet. Ask whoever runs your installation to download it for you. Only an administrator or operations user can have it made, agents cannot, and it holds only what that user can see. Every download is audited. For everyday exports use **Filters and views** → **New filter** → … → **⋯** (**More for this filter**) → **Export CSV**.
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
Undo archives the records that import created (a lead is disqualified), unless someone edited them since. Those are listed under "Kept because they were edited after the import:". Records the import only updated are not changed back. If the undo stops partway, choose **Continue undo**. When done it says **Import undone**.
Also called: reverse an import, roll back an upload.

### How do I delete a contact permanently?
Margince has no delete button for a contact: **More actions → Archive** hides it but keeps it. To destroy a contact's data for good, an administrator fulfils an **erasure** request in **Settings → Privacy and retention → Privacy requests** (see below). Companies, deals, leads and projects have no delete button either; an erasure request always names a contact. A lead's data is destroyed only by the **Leads that never converted** rule (when set to anonymise) or with the contact it belongs to. A deal is only ever archived, never destroyed. Companies and projects have no retention rule at all.
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
To log a GDPR access request, an administrator opens **Settings → Privacy and retention → Privacy requests** and chooses **New request**. Set **Kind** to **access**, fill **Subject reference** and **Due**, and choose **Open request**. The app says: "An access request is fulfilled manually: record what you sent in the resolution. This system does not assemble or export the data for you." Gather the data (or have the package assembled, see below), send it, then write the **Resolution** and choose **Fulfill**.
Also called: subject access request, SAR, Art. 15, what do you hold about me.

### How do I archive a record?
To archive a contact or company, open it and choose **More actions → Archive**, then confirm "Archive this record? You can bring it back from its history." For a deal, choose **Archive deal**; for a project, **Archive project**. Leads are not archived: choose **Disqualify**. To see archived records on a list, choose **Show archived**.
Also called: hide, remove, retire a record.

### How do I restore an archived record?
An archived contact, company or deal comes back with **Undo** on the entry that archived it in the record's **History**; open it with **Show archived** on its list. A project cannot be brought back from the app. Individual field changes can be reversed with **Undo** in the record's **History** too. Tags and pipelines have their own **Restore**; an archived team comes back with **Undo** on the "Team archived" notice.
Also called: unarchive, undelete, recover.

### How do I change how long Margince keeps data?
To change a retention rule, an Admin or Ops user opens **Settings → Privacy and retention** and uses the **Retention** card.
1. Choose **Edit** on a rule, or **Add policy** for a new one.
2. Set **Applies to**, **Window in days** (a whole number, at least 1) and **Action**.
3. Optionally fill **Lawful basis**, then choose **Save policy** or **Create policy**.
To pause a rule, turn **Enabled** off; **Delete policy** removes it entirely.
Also called: data retention, retention period, auto-delete.

### How do I record a contact's consent?
To record consent, or to mark a contact as do-not-contact for a purpose, open the contact, find the **Communication permissions** panel and choose **Manage consent and proof history**. In the drawer choose **Record consent** for a purpose, or **Withdraw**. A purpose that needs double opt-in cannot be recorded by staff; choose **Ask them to confirm their details** on the same panel to mail the contact a private link instead. Purposes are added in **Settings → Privacy and retention → Consent purposes → Add purpose**.
Also called: opt-in, opt-out, do not contact, unsubscribe someone, marketing permission, GDPR consent.

## Archive is not delete

Across the whole product, ordinary removal keeps the record. An archived record
leaves the live lists, but you can still open it.

Some examples of the difference in the product's own words:

- Archiving a project "removes this project from the active list and frees its
  key." The project is then read-only: "This project is archived and takes no
  changes."
- Archiving a document set stops the set and everything filed in it being
  searchable. Nothing is destroyed.
- Merging two records destroys nothing either: "Merge {source} into {target}?
  This archives {source}." Both values survive a merge; choosing a side decides
  which record stands and which value is shown first.

## A new company starts with retention rules

A new company starts with these retention rules already in place. You can edit
all of them.

| What it covers | Kept for | Then |
|---|---|---|
| Leads that never converted | 365 days | Archive |
| All captured activity | 1095 days (3 years) | Archive |
| Call transcripts | 365 days | Erase |
| Contacts with no consent and no deal | 730 days (2 years) | Anonymize |
| Lost deals | 1825 days (5 years) | Archive |
| AI call payloads | 365 days | Erase |
| Stored message originals | 730 days (2 years) | Erase |
| Daily record of material deals at risk | 90 days | Erase |

**Won deals** and **Saved report editions** can also have a rule, but start
with none. Margince takes no view on when your company should stop keeping a
won deal; that is your decision.

**Stored message originals** are the copies capture keeps of every message it
filed, separately from the timeline entry each became. On a busy mailbox they
take the most storage. An original is never destroyed while its correspondence
is held under the statutory retention floor. The timeline entry stays, so the
same message cannot be captured again.

If you delete every rule, the screen tells you what that means: "No retention
policy yet. Nothing in this installation ages out." One fixed window still
runs with no policy: AI embedding call traces age out after 90 days (below).

### What each retention window counts from
**Leads** and **contacts** count from when the record was created. **Captured
activity** and **call transcripts** count from the message's own date: when it
was sent or received, not when it was filed. **Deals** count from when the deal
was closed.

### The three retention actions

The three retention actions are **Archive**, where the record is kept, leaves
the live lists and still exists; **Anonymize**, where identifying data is
destroyed and the record survives; and **Erase**, where the data is destroyed.

The app draws the line for you: "Archive keeps the record. Anonymize and erase
destroy data and are held back in retain-only mode."

Anonymise and erase clear different things. An erasure also reaches the
original captured messages, the attachments those messages
carried, the contact's leads and scores, their unsubscribe links and their Deal Room
seats. Anonymising leaves all of those.

### What anonymise and erase actually do
**Anonymising a lead** replaces the name with "Anonymized Lead", clears the
email, title and company, and deletes the lead's score history.

**Anonymising a contact** clears the names, title and postal address, sets the
name to "Erased Subject", clears every custom field, and deletes their email
addresses, phone numbers, social handles and channel identities. No suppression
entry is written: the contact may lawfully come back.

**Erasing a call transcript** clears the body and replaces the subject with
"Erased", and deletes the attachment files themselves. It keeps who the meeting
was with and when: the record of the meeting stays and its content goes.

### Writing your own retention rule

A retention rule has three parts: **Applies to**, **Window in days**, and
**Action**. A window is a whole number of days, **at least 1**; a zero-day
window would act on a record the moment it was created.

Each scope carries **at most one** rule: "A policy for this scope already
exists; each scope has at most 1 rule. Edit the existing policy instead." There
is no stacking, and you cannot re-point an existing rule at a different scope.

Not every combination is allowed: there is no way to archive an AI call
payload, or to anonymise an activity. The allowed pairs are:

- erase or anonymise a contact;
- archive or erase an activity;
- archive a deal;
- archive or anonymise a lead;
- erase an AI call payload, a stored message original, a daily record of
  material deals at risk, or a saved report edition.

A rule can carry an optional **Lawful basis** (the Article 6 basis the window is
argued from), recorded for whoever audits the rule later. The rules a new
company starts with all carry "storage limitation".

Rules act **nightly**, and a live one shows as "Acting nightly". Each night
handles up to 200 records per rule, so a backlog of years takes several nights
to clear.

### One window nobody can change

AI embedding call traces are kept for **90 days**, fixed. This runs with or
without a policy, and no administrator can edit it.

### Turning a retention policy off, versus deleting it

Turning a retention policy off and deleting it are different.

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
even read the rules, and the app says why: "Only
an administrator or operations user can see retention policies. They set what
this installation keeps for everyone."

## Privacy requests

The privacy request queue, **Settings → Privacy and retention → Privacy
requests**, holds "Data subject requests with their statutory deadlines". Only
administrators can open it by default, because the queue names whoever asked.
An administrator can give that access to someone else on its own, without
letting them manage members.

A request has a kind (access, rectify or erasure), a subject, an assignee, a due
date, and a resolution. It moves through **In progress** and is closed by
**Fulfill** or **Reject**. Closing one requires you to write the answer:
"Closing a request needs its answer." Once set, an assignee cannot be cleared.

**A closed request never reopens.** "Closed. A closed request cannot be
reopened; a new concern needs a new request."

If two colleagues open the same request, the second is told "Someone else
decided this request first. Review the current state below."

### Access requests

The screen says:

> An access request is fulfilled manually: record what you sent in the
> resolution. This system does not assemble or export the data for you.

There is no "download everything about this contact" button in the app, but
Margince can assemble the Art. 15 package: the contact's record, their
correspondence, their consent history, and the evidence of why the contact
exists at all. Ask whoever runs your installation to download it for you.

- **Only a human user can have it assembled**, never an agent. By default that
  is an administrator. A custom role can be given the privacy requests
  permission too; its holder also needs to delete contacts and to see every
  record in the company.
- **It answers access requests only**, never erasure or rectification.
- **It does not close the request.** Mark it fulfilled yourself once you have
  actually sent it, and record what you sent in the resolution.

### Erasure requests

An erasure request **must name a contact in this company**: "An erasure request
must name a contact in this company, because fulfilling it erases that record. A
free-text subject cannot be erased."

Fulfilling one is hard to do by accident. You type **ERASE** to confirm, and
the warning reads:

> This permanently erases the contact across the whole system: record, captured
> activity and derived values. It cannot be undone. The erasure itself is
> audited.

The confirm button reads **Erase and suppress**.

## When erasure does not win: the retention floor

Sometimes the law requires keeping something that a data subject has asked you
to delete. Margince then shows the record as held, with the reason.

When an erasure hits a statutory retention obligation, you see **Blocked by
legal hold**:

> This contact is inside a statutory retention window, so erasure does not take
> precedence here (Art. 17(3)(b)). The block applies to every role, including
> administrators, with no override. The attempt was audited.

### Restricted records

**Restricted records** are what a statutory retention obligation holds after an
erasure: "which record, why and until when." Held records are hidden from every
ordinary view and cannot be changed. Their identifiers are removed at once, and
the screen reports how many fields were removed. They are erased when the
window closes. The correspondence itself is hidden "so that it is not read."

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

Releasing finishes the deletion; the record does not come back into use. Every
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
own recorded address: use **Ask them to confirm their details**. A confirmation
an employee could complete for the contact would not prove the contact agreed.

The default is **deny**. A purpose with no record for a contact is not consent.

Every consent change is written to a **Proof log** that records who did it and
how: a Human, an Agent, the System, or a Connector. Where nothing is known, it
says "actor not recorded" and "source not recorded". You may one day have to
prove consent, so Margince keeps the evidence along with the current state.

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

On **Filters and views** you build a filter and choose **Export CSV** or
**Export JSON** under **⋯** (**More for this filter**); those are the only two formats.
The screen offers **contacts, companies, deals and leads**. You export either a
new filter or a saved view.

**It exports only what you can see**, by the same rules as the list on your
screen. Only a human can export; an agent cannot. **Every export is audited**,
so someone can always find out who took a copy of what, and when.

For everything about one individual, use the access-request package above.

#### The whole-workspace bundle

The installation's data handover: every record as CSV, how they link as JSON,
and files describing both, in the open `margince-export/1` format. It is the
export for "give us our data", whether for a migration, an audit or a
portability request. Only admin and ops users can have it made, it holds only
what they can see, and it is audited. No screen offers it yet; ask whoever runs
your installation to download it for you.
