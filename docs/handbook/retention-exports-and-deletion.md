<!-- prose:plain -->
# What is kept, what is destroyed

What Margince keeps, for how long, what a delete destroys, and how you get data in and out. Read it before you promise anything to a customer or to someone who checks your records.

## The main rule: deleting in Margince archives

**In Margince, removing a record never destroys it.** That holds for a contact, company, deal, lead or project. To remove is to archive: the record leaves the live lists, becomes read-only, and is still stored, with its history and audit trail. Margince has no delete button for records. Data is destroyed in only two ways: a retention rule whose action is anonymize or erase, and an erasure request that has been carried out. Files and knowledge documents are the exception: you can delete those for good.

## Getting data out, getting data in, and removing records

### How do I export my data from Margince?
To export or download records from Margince (for example all your contacts), open **Filters and views** in the sidebar. Build a filter, then choose **Export CSV** or **Export JSON**.
1. Open **Filters and views**.
2. Pick the **Record type**: **Contacts**, **Companies** or **Deals**.
3. Choose **Add clause** and fill at least one clause. The export buttons show once the filter is complete.
4. Choose **Export CSV** or **Export JSON**. The file downloads.
The export holds only records you can see, and every export is written to the audit log. Agents cannot export.
Also called: download, get my data out, backup, extract to Excel or spreadsheet.

### How do I get all of my company's data out of Margince?
Margince has an export of the whole company: a ZIP with one CSV for each record type, plus files that describe how records link. The app has no button for it yet. Ask whoever runs your Margince to download it for you. Only an admin or ops user can have it made, agents cannot, and it holds only what that user can see. Every download goes into the audit log. For daily exports use **Filters and views → Export CSV**.

Also called: full export, data handover, migrate away, leave Margince.

### How do I import contacts from a spreadsheet?
To import contacts, companies or leads from a spreadsheet, save it as CSV and open **Settings → Data import**. Then choose **Start import**.
1. Choose **Start import** under **Import CSV file**.
2. Pick the **Row type**: **Prospects** (leads), **Companies** or **Contacts**.
3. Choose **Choose file** and pick the CSV.
4. In **Column mapping**, pick a **Field** for each column, or **Do not import**.
5. Choose **Preview import**, check the **Import preview**, then choose **Import {rows} rows**.
Only an admin or ops user can import. Nothing is written before you confirm.
Also called: upload a CSV, bulk import, Excel import.

### How do I undo an import?
To undo a CSV import in Margince, open **Settings → Data import** and choose **Undo import ({rows} rows)** on the import result.
Undo archives the records that import created (a lead is disqualified), unless someone edited them since. Those are listed under "Kept because they were edited after the import:". Records the import only updated are not changed back. If the undo stops part of the way, choose **Continue undo**. When done it says **Import undone**.

Also called: reverse an import, roll back an upload.

### How do I delete a contact permanently?
Margince has no delete button for a contact: **More actions → Archive** hides it but keeps it. To destroy a contact's data for good, an admin fulfills an **erasure** request in **Settings → Privacy and retention → Privacy requests** (see below). Companies, deals, leads and projects have no delete button either; an erasure request always names a contact. A lead's data is destroyed only by the **Leads that never converted** rule (when set to anonymize), or with the contact it belongs to. A deal is only archived, never destroyed. Companies and projects have no retention rule at all.

Also called: remove a contact, hard delete, purge, wipe.

### How do I handle a GDPR erasure request?
To erase a contact on request, an admin opens **Settings → Privacy and retention**, goes to **Privacy requests** and chooses **New request**.
1. Set **Kind** to **erasure** and pick the **Contact** (required).
2. Set **Due**, then choose **Open request**.
3. On the request, write the **Resolution** and choose **Fulfill**.
4. Type ERASE in **Type ERASE to confirm** and choose **Erase and suppress**.
This cannot be undone. A contact that the law says you must still keep is refused with **Blocked by legal hold**.
Also called: right to be forgotten, Art. 17, delete my data, data subject request, DSR.

### How do I answer a data access request?
To log a GDPR access request, an admin opens **Settings → Privacy and retention → Privacy requests** and chooses **New request**. Set **Kind** to **access**, fill **Subject reference** and **Due**, and choose **Open request**. The app says: "An access request is fulfilled manually: record what you sent in the resolution. This system does not assemble or export the data for you." Bring the data together (or have the package made, see below) and send it. Then write the **Resolution** and choose **Fulfill**.

Also called: subject access request, SAR, Art. 15, what do you hold about me.

### How do I archive a record?
To archive a contact or company, open it and choose **More actions → Archive**, then confirm "Archive this record? You can bring it back from its history." For a deal, choose **Archive deal**; for a project, **Archive project**. Leads are not archived: choose **Disqualify**. To see archived records on a list, choose **Show archived**.
Also called: hide, remove, retire a record.

### How do I restore an archived record?
An archived contact, company or deal comes back with **Undo** on the entry that archived it, in the record's **History**. Open it with **Show archived** on its list. You cannot bring back a project from the app. You can also take back a single field change with **Undo** in the record's **History**. Tags and pipelines have their own **Restore**; an archived team comes back with **Undo** on the "Team archived" notice.
Also called: unarchive, undelete, recover.

### How do I change how long Margince keeps data?
To change a retention rule, an Admin or Ops user opens **Settings → Privacy and retention** and uses the **Retention** card.
1. Choose **Edit** on a rule, or **Add policy** for a new one.
2. Set **Applies to**, **Window in days** (a whole number, at least 1) and **Action**.
3. If you like, fill **Lawful basis**, then choose **Save policy** or **Create policy**.
To pause a rule, turn **Enabled** off; **Delete policy** removes it fully.
Also called: data retention, retention period, auto-delete.

### How do I record a contact's consent?
To record consent, or to mark a contact as do-not-contact for a purpose, open the contact. Find the **Communication permissions** panel and choose **Manage consent and proof history**. In the side panel choose **Record consent** for a purpose, or **Withdraw**. Your team cannot record a purpose that needs double opt-in. Instead, choose **Ask them to confirm their details** on the same panel to mail the contact a private link. You add purposes in **Settings → Privacy and retention → Consent purposes → Add purpose**.

Also called: opt-in, opt-out, do not contact, unsubscribe someone, marketing permission, GDPR consent.

## Archive is not delete

Across the whole product, removing a record keeps it. An archived record leaves the live lists, but you can still open it.

Some examples of the difference in the product's own words:

- Archiving a project "removes this project from the active list and frees its
  key." The project is then read-only: "This project is archived and takes no
  changes."
- Archiving a document set stops the set, and everything filed in it, from showing up in
  search. Nothing is destroyed.
- Merging two records destroys nothing either: "Merge {source} into {target}?
  This archives {source}." Both values are kept in a merge; choosing a side decides
  which record stays and which value is shown first.

## A new company starts with retention rules

A new company starts with these retention rules already in place. You can edit all of them.

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

**Won deals** and **Saved report editions** can also have a rule, but start with none. Margince does not say when your company should stop keeping a won deal; that is your choice.

**Stored message originals** are the copies that capture keeps of every message it filed, apart from the timeline entry made from each one. On a large mailbox they take the most storage. An original is never destroyed while the law still says its mail must be kept. The timeline entry stays, so the same message cannot be captured again.

If you delete every rule, the screen tells you what that means: "No retention policy yet. Nothing in this installation ages out." One fixed time limit still runs with no rule: traces of AI embedding calls end after 90 days (below).

### What each retention window counts from
**Leads** and **contacts** count from when the record was created. **Captured activity** and **call transcripts** count from the date on the message itself, not from when it was filed. **Deals** count from when the deal was closed.

### The three retention actions

The three retention actions are **Archive**, **Anonymize** and **Erase**. With **Archive**, the record is kept, leaves the live lists and still exists. With **Anonymize**, the data that names someone is destroyed and the record stays. With **Erase**, the data is destroyed.

The app makes this clear: "Archive keeps the record. Anonymize and erase destroy data and are held back in retain-only mode."

Anonymize and erase clear different things. An erasure also reaches the first copies of captured messages and the files sent with those messages. It reaches the contact's leads and scores, their unsubscribe links and their Deal Room seats too. Anonymizing leaves all of those.

### What anonymize and erase do
**Anonymizing a lead** puts "Anonymized Lead" in place of the name, clears the email, title and company, and deletes the lead's score history.

**Anonymizing a contact** clears the names, title and address, sets the name to "Erased Subject", and clears every custom field. It deletes their email addresses, phone numbers, and their names on other sites and services. No block entry is written: the law allows the contact to come back.

**Erasing a call transcript** clears the text and puts "Erased" in place of the subject, and deletes the files too. It keeps who the meeting was with and when: the record of the meeting stays and what was said goes.

### Writing your own retention rule

A retention rule has three parts: **Applies to**, **Window in days**, and **Action**. A time limit is a whole number of days, **at least 1**. A limit of zero days would act on a record as soon as it was created.

Each scope holds **at most one** rule: "A policy for this scope already exists; each scope has at most 1 rule. Edit the existing policy instead." You cannot put two rules on one scope, and you cannot move a rule to a different scope.

Not every mix is allowed: there is no way to archive an AI call payload, or to anonymize an activity. What is allowed:

- erase or anonymize a contact;
- archive or erase an activity;
- archive a deal;
- archive or anonymize a lead;
- erase an AI call payload, a stored message original, a daily record of
  material deals at risk, or a saved report edition.

A rule can hold a **Lawful basis** if you like (the Article 6 reason for the time limit). It is recorded for whoever checks the rule later. The rules a new company starts with all say "storage limitation".

Rules act **each night**, and a live one shows as "Acting nightly". Each night works on up to 200 records for each rule, so years of old records take many nights to clear.

### One window nobody can change

Traces of AI embedding calls are kept for **90 days**, fixed. This runs with or without a rule, and no admin can edit it.

### Turning a retention policy off, versus deleting it

Turning a retention rule off and deleting it are not the same.

- **Enabled off**: the rule pauses and keeps its time limit. Nothing in that scope
  ends while it is off, and the time limit is still there when you turn it back
  on.
- **Delete policy**: "This removes the rule for {scope} entirely, so nothing in
  that scope ages out anymore. To pause the rule and keep its window, turn off
  Enabled instead."

### Retain-only mode

**Retain-only mode** is one switch above all the retention rules. While it is on:

> While on, this installation destroys nothing: no anonymizing and no erasing,
> whatever a policy below says. Archiving still runs; an archived record is
> kept, not destroyed.

A rule that would destroy data shows as "Paused by retain-only mode", so it does not look active. It will not act until the mode is turned off.

### Who can change retention
Only an **Admin** or **Ops** user can change retention. Everyone else cannot even read the rules, and the app says why: "Only an administrator or operations user can see retention policies. They set what this installation keeps for everyone."

## Privacy requests

The list of privacy requests, **Settings → Privacy and retention → Privacy requests**, holds "Data subject requests with their statutory deadlines". Only admins can open it by default, because the list names whoever asked. An admin can give that access to someone else by itself, without letting them manage members.

A request has a kind (access, rectify or erasure), a subject, an assignee, a due date, and a resolution. It moves through **In progress** and is closed by **Fulfill** or **Reject**. To close one you must write the answer: "Closing a request needs its answer." Once set, an assignee cannot be cleared.

**A closed request never opens again.** "Closed. A closed request cannot be reopened; a new concern needs a new request."

If two colleagues open the same request, the second sees "Someone else decided this request first. Review the current state below."

### Access requests

The screen says:

> An access request is fulfilled manually: record what you sent in the
> resolution. This system does not assemble or export the data for you.

There is no "download everything about this contact" button in the app, but Margince can put together the Art. 15 package. It holds the contact's record, their mail, their consent history, and the facts that show why the contact is in Margince at all. Ask whoever runs your Margince to download it for you.

- **Only a human user can have it made**, never an agent. By default that
  is an admin. A custom role can also be given the permission for privacy requests. The user who holds it also needs to delete contacts and to see every record in the company.
- **It answers access requests only**, never erasure or rectify requests.
- **It does not close the request.** Mark it fulfilled yourself once you have
  really sent it, and record what you sent in the resolution.

### Erasure requests

An erasure request **must name a contact in this company**. The app says:

> An erasure request must name a contact in this company, because fulfilling it erases that record. A free-text subject cannot be erased.

Carrying one out is hard to do by mistake. You type **ERASE** to confirm, and the warning reads:

> This permanently erases the contact across the whole system: record, captured
> activity and derived values. It cannot be undone. The erasure itself is
> audited.

The confirm button reads **Erase and suppress**.

## When erasure does not win: the retention floor

The law can say you must keep something that the contact has asked you to delete. Margince then shows the record as held, with the reason.

When the law says you must keep a record that an erasure would remove, you see **Blocked by legal hold**:

> This contact is inside a statutory retention window, so erasure does not take
> precedence here (Art. 17(3)(b)). The block applies to every role, including
> administrators, with no override. The attempt was audited.

### Restricted records

**Restricted records** are what the law makes you keep after an erasure: "which record, why and until when." Held records are hidden from every ordinary view and cannot be changed. The details that name someone are removed at once, and the screen reports how many fields were removed. They are erased when the time limit ends. The mail itself is hidden "so that it is not read."

The record kinds that can be held this way are Email, Call, Meeting and Message, in a group the app calls **Commercial correspondence**.

An admin or ops user can **Pin a record** under the floor by hand, for mail the automatic rule cannot see. The app gives mail with suppliers and about buying as the example. That mail must be kept under §257 HGB and has no deal in Margince to link it to. The record ID is on its audit entry.

An admin or ops user can also **Release** a held record. Read what that means before you do it:

> Releasing ERASES the record; it does not return it to use. The erasure request
> this obligation suspended is still open, so releasing completes it. This
> cannot be undone.

Releasing completes the erasure; the record does not come back into use. Every choice here is recorded in the audit trail with your name and your reason.

If nothing is held, the screen says so, and it is good news: "No records held. Every erasure so far was completed in full."

## Consent

Consent in Margince is recorded for each **purpose**. **Settings → Privacy and retention → Consent purposes** holds the list of purposes: the reasons this company uses the data of the humans in it. Each purpose has a key, a label, and a mark for whether it needs double opt-in.

You can only add to the list: "A purpose cannot be renamed or removed once created; the catalog is append-only. Choose the key carefully." On a contact you can **Record consent** or **Withdraw** consent for a purpose.

Your team cannot record a purpose that needs **double opt-in** at all. Only the contact can confirm one, by opening a link that works once, mailed to their own recorded address. Use **Ask them to confirm their details**. If a colleague could confirm it for the contact, it would not prove the contact agreed.

The default is **deny**. A purpose with no record for a contact is not consent.

Every consent change is written to a **Proof log**. It records who made the change and how: a Human, an Agent, the System, or a Connector. Where Margince has no record of it, it says "actor not recorded" and "source not recorded". You may one day have to prove consent, so Margince keeps the proof, not only the current state.

Agents cannot write consent at all; see [What the AI does](what-the-ai-does.md).

## The audit trail

The audit trail, **Settings → Audit log**, records "Every action, attributed to a user, agent or connector".

You can filter it by **Actor**, **Entity type**, **Entity ID**, **Action**, and a date range (**From**, **To**). Choose **Show change detail** on any entry to see the change and the **Authorization rule** that allowed it. Where something was done for someone else, the entry says so.

Only an admin can read the full trail: "Your role cannot read the full audit log. It records every actor and every record they accessed."

It is also where a record's full ID is shown, which you need to pin a record under the retention floor by hand.

## What the record export covers

There are two exports, and they answer different questions.

#### The list export

On **Filters and views** you press **Export CSV** or **Export JSON**; those are the only two file types. The screen offers **contacts, companies and deals**. You export either a filtered set or a saved view.

**It exports only what you can see**, by the same rules as the list on your screen. Only a human can export; an agent cannot. **Every export goes into the audit log**, so someone can always find out who made a copy of what, and when.

For everything about one human, use the access request package above.

#### The export of the whole company

The full export of your Margince: every record as CSV, how they link as JSON, and files that describe both, in the open `margince-export/1` format. It is the export for "give us our data". That may be for a move to another system, a check of your records or a request to take data with you. Only admin and ops users can have it made, it holds only what they can see, and it goes into the audit log. No screen offers it yet; ask whoever runs your Margince to download it for you.
