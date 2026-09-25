# Contacts, companies, leads, deals and projects

Margince holds five kinds of record. This page explains what each one is, how
they connect, which connections are required, and how you create, link, merge,
archive and correct them.

## The five, in one line each

**Contact** — an individual you do business with. Navigation calls this
**Contacts**, and so does everything behind it.

**Company** — a company you deal with. Navigation calls this **Companies**.

**Lead** — a prospect you have not qualified yet. Deliberately kept apart from
your contacts.

**Deal** — a specific piece of business, moving through stages toward won or
lost.

**Project** — the body of work a client relationship is actually made of. It
starts while a deal is still open and outlives the close.

Around these sit **activities** — the timeline of emails, calls, meetings, notes
and tasks — and **documents**, covered on their own pages.

### What is the difference between a lead and a deal?
A lead in Margince is someone you have not qualified yet; a deal is a specific
piece of business with a value, moving through pipeline stages toward won or
lost. A lead lives on **Leads** and has no company record behind it. A deal
lives on **Deals** and may name a company, contacts and a project. Qualifying a
lead turns it into a contact, and can open a deal in the same step.
Also called: prospect versus opportunity.

### What is the difference between a contact and a company?
A contact in Margince is an individual you do business with; a company is the
business they work for or that you deal with. A contact is linked to a
company through an employment relationship, and that link is optional: a
contact with no employer is still a contact. Also called: account, client or
customer for a company.

## What connects to what

The links between Margince records are looser than in most CRMs, on purpose.

| Link | Required? |
|---|---|
| Contact → Company | **Optional.** It lives on an employment relationship, not a field. |
| Lead → Company | **There is none.** A lead carries a company *name* as free text. |
| Deal → Pipeline and stage | **Required.** |
| Deal → Company | **Optional.** |
| Deal → Contact | **Optional.** Contacts sit on a deal as stakeholders. |
| Deal → Project | Optional, at most one, and both must name the same company. |
| Project → Company | **Required — at least one, always.** |
| Activity → any record | Optional. One you log with no links is shared with everyone; a captured one with no links stays held. |

Two of these catch readers out.

**A deal does not need a company.** Name, pipeline, stage and source are
required. Value, company and expected close date are optional. Currency is not
required on its own — it is atomic with the amount: a figure with no currency is
refused, and a currency with no figure is refused. A deal carrying no value
carries no currency either. Qualifying a lead creates a deal with no company
attached at all, because a lead has no company to attach.

**A lead has no company record behind it.** That free-text name is not a
pointer. This is the whole reason leads are segregated: bulk-sourced prospects do
not get to create half-formed companies in your CRM.

## Contacts

A contact needs a name and a source. Everything else is optional.

### How do I add a new contact?
To add a new contact in Margince, open **Contacts** in the sidebar and choose **New contact**.
1. Enter the **Full name** (required).
2. Optionally fill **First name**, **Last name**, **Title** and **LinkedIn**.
3. Choose **Add email** for each address and set its **Type** (Work, Personal or Other); choose **Add phone** for each number.
4. Fill any custom fields, then choose **Create**. It stays disabled until the full name is filled.
If the contact already exists, **View existing record** opens it instead.
Also called: create a contact, add someone to the CRM, new customer contact.

### How do I add a contact quickly, or import contacts from vCards?
To add a contact quickly in Margince, open **Contacts** and choose **Quick capture**; to add many from a card file, choose **Import vCards**.
- **Quick capture** asks for **Full name** (required), **Title**, **Company**, **LinkedIn**, **Email** and **Phone**, and confirms with "{name} saved"; the form stays open.
- The **Company** box in Quick capture lists existing companies matching what you type: pick one to attach it. A name you do not pick creates a new company.
- **Import vCards** asks for a **vCard file**: choose **Select .vcf file**. Each card is reported as Added, Missing fields added, Possible duplicate or Skipped.
Also called: import contacts, upload a .vcf.

### How do I edit a contact?
To edit a contact in Margince, open it and click the value to change in the **Details** panel beside the record. There is no Edit button: each field changes in place.
1. If the panel is hidden, choose **Show details and permissions**.
2. Click the value (an empty one says **Not set**), type and press Enter, or pick from the list. Esc cancels.
3. **Email**, **Phone** and **Address** open a small form with **Type**, **Primary** and **Remove**; press **Save**.
**Full name**, **Title**, **LinkedIn**, **Owner** and custom fields change the same way. First and last name are set only at creation.
Also called: update a contact, change a phone number, rename.

### Why can't I edit a contact or company?
A contact or company in Margince takes no edits when it is archived ("This contact is archived and takes no changes.") or when it is not yours to change: "You cannot edit this contact. Ask the owner to share it, or an administrator for edit rights." Fields you cannot edit show as plain text, with the reason on hover.
If someone else changed the same field while you were editing, it is refused: "This record changed since it was opened. Reload and retry."
Also called: read-only record, edit greyed out, cannot change.

### How do I link a contact to a company?
To link a contact to a company in Margince, open the contact's page and choose **Add company** in its **Companies** panel.
1. Under **Employer**, search for and pick the company. It must already exist; create it first under **Companies** if it does not.
2. Optionally enter the **Role** and a **Start date** (YYYY-MM or YYYY-MM-DD).
3. Tick **This is their current employer** if it is.
4. Choose **Create**.
To change or end the link later, use the row's **More actions** menu: **Edit employment**, **Mark as ended** or **Remove**.
Also called: set a contact's company, assign to an account, employer.

### How do I match a purchased job history entry to a company?
When purchased employment history in Margince names an employer it cannot match, the contact's **Companies** section shows **Company match needed.**; choose **Match company**.
Pick an existing company, or enter its **Confirmed company website** to create it, then choose **Save company link**. The choice applies to every unmatched role at that employer. **Dismiss evidence** drops an entry that should not become a link. A company name alone never creates a company. Also called: unresolved employer, imported job history.

Emails are typed **work, personal or other**; phones **work, mobile, home or
other**. One number may appear twice under two types — a switchboard that is
both work and mobile — and editing the list keeps the two rows apart.

The contact header says **who can see this contact**: private to its owner, or
everyone in the company; a company header says the same. See
[Seats, roles and who can see what](seats-roles-and-access.md).

A contact's job title appears in two places and they are not the same thing. The
title on the contact record is a convenience copy; the authoritative one lives on
the employment relationship with the company. A contact can have exactly one
current primary employer, or none.

Other things a contact carries: an owner, a consent record per purpose, a
relationship strength, when they were last active, and — if they came from a
lead — a pointer back to that lead.

### How do I merge duplicate contacts?
To merge two duplicate contacts in Margince, open the contact you want to remove, choose **More actions**, then **Merge contact**.
1. Search for the other contact and pick it under **Select record to keep**.
2. Read the confirmation: "Merge {source} into {target}? This archives {source}."
3. Choose **Merge**. Margince opens the contact you kept.
The contact you merged away is archived, not deleted. Leads have no merge; deals have no merge either.
Also called: combine, deduplicate, remove a duplicate.

### How do I call a contact?
To call a contact from Margince, open the contact and click the phone number in its header or **Details** panel; it opens your phone or calling app. Margince does not dial or record calls itself.
The **Call** button in the contact header opens the **History** tab, so you can log the call afterwards with **Log activity** → **Call**.
Also called: phone a contact, click to call, dial.

### Can I add a photo to a contact, or change a company's logo?
No. Margince has no photo upload for a contact: its avatar is drawn from the name. A customer company's logo is read from its website and cannot be uploaded. Only your own company's logo is set, in **Settings → Company profile** under **Company logo**.
Also called: profile picture, avatar, company image.

## Companies

A company needs a display name and a source.

### How do I add a company?
To add a company — a new customer, client or account — in Margince, open **Companies** in the sidebar and choose **New company**.
1. Enter the **Company name** (required).
2. Optionally fill **Legal name**, **Industry** and **Company size**.
3. Choose **Add domain** for each web domain; a domain row you add must be filled.
4. Fill any custom fields, then choose **Create**.
If the company already exists, **View existing record** opens it instead. The website address is derived from the primary domain, so there is no website box.
Also called: new account, new business, add a client or customer.

### How do I edit a company?
To edit a company in Margince, open it and click the value to change in its **Details** panel; each field changes in place, with no Edit button.
1. If the panel is hidden, choose **Show details**.
2. Click a value, change it, and press Enter or pick from the list. Esc cancels.
Fields: **Company name**, **Legal name**, **Industry**, **Company size**, **Owner**, **Lifecycle**, **Relationship type**, **LinkedIn URL**, **Address**, **Domains**, **Parent company**, **Description**, custom fields, and **Register / VAT ID** under **Registration**. The website follows the primary domain.
Also called: rename a company, update an account.

### How do I change the owner of a contact or company?
To change who owns a contact or company in Margince, open the record and pick a colleague in the **Owner** row of its **Details** panel. The change saves as soon as you pick.
You need edit rights on the record: your own, your team's, one shared with you for writing, or a role that edits every record. A private contact must keep an owner ("This field is required.").
There is no bulk reassign for contacts or companies. For many deals or leads, see [Lists, filters and views](lists-filters-and-views.md).
Also called: reassign a contact, hand over an account, transfer ownership, account owner.

Two separate fields describe a company, and mixing them up is a common mistake.

**Lifecycle** — where the account stands with you. One value only:
**unknown, target, prospect, opportunity, customer, former customer,
disqualified.** New companies start at unknown.

**Relationship types** — what the company *is* to you. Several at once:
**customer, partner, supplier, investor, portfolio company, competitor, other.**

So a company can be a customer *and* a supplier. It cannot be both a prospect and
a customer, because that is one question with one answer.

Size bands are: 1-10, 11-50, 51-200, 201-500, 501-1000, 1001-5000, 5000+.

A company may have a parent company, one level only — no chains, no cycles. The
website address is derived from the primary domain rather than typed.

One company in your installation is marked as the **anchor** — that is your own
company, the one Margince knows things about on your behalf.

### How do I merge duplicate companies?
To merge two duplicate companies in Margince, open the company you want to remove, choose **More actions**, then **Merge company**.
1. Search for the other company and pick it under **Select record to keep**.
2. Read the confirmation: "Merge {source} into {target}? This archives {source}."
3. Choose **Merge**. Margince opens the company you kept.
The merged-away company is archived, not deleted. An archived company cannot be merged.
Also called: combine companies, deduplicate accounts.

### How do I stop a company coming back from email?
To archive a company that is not a real company and stop mail creating it again, open it, choose **More actions**, then **Not a company**.
1. Enter the **Reason this is not a company**.
2. Confirm. Margince archives the company and blocks its domain, so later messages from that domain do not create it again.
An administrator can unblock the domain in Settings under Capture. The action is offered only where the company has a domain and your seat may do both halves.
Also called: junk company, spam domain, block a domain.

## Leads, deals and projects

Creating, qualifying and reopening leads, creating deals and projects, and
how each connects are on their own page:
[Leads, deals and projects](leads-deals-and-projects.md).

## Archiving, restoring and deleting records

### How do I archive a contact, company, deal or project?
To archive a record in Margince, open it, choose **More actions**, then the archive entry: **Archive** on a contact or company, **Archive deal** on a deal, **Archive project** on a project.
Confirm the dialog. For contacts and companies it reads "Archive this record? There is no undo."
Several deals can be archived at once with **Archive** in the Deals list's bulk bar. A lead is not archived; disqualify it instead.
An archived record leaves the live list; turn on **Show archived** on the list to see it again. It becomes read-only.
Also called: remove, hide, deactivate a record.

### Can I restore or unarchive an archived record?
No. Margince has no way to restore an archived contact, company, deal or project: archiving is final from the app, and an archived record takes no edits, merges or undo. You can still open it with **Show archived** and read its history.
If you archived by mistake, create the record again, or ask an administrator.
Tags and pipelines do have a **Restore**; an archived team comes back with **Undo** on its archived notice. A disqualified lead can be reopened with **Reopen**.
Also called: unarchive, undelete, bring back a record.

### How do I delete a contact or company?
You cannot delete a company or a contact in Margince: there is no delete button for a contact, company, lead, deal or project. To remove one, archive it — open the record, choose **More actions** → **Archive** — which keeps the record but takes it off the live lists.
A contact's data is destroyed only by an erasure request or a retention rule; no erasure request or retention rule reaches a company. See [What is kept, what is destroyed](retention-exports-and-deletion.md).
Files and knowledge documents do have their own **Delete**.
Also called: delete a company, remove a company, delete a contact, remove permanently, erase.

## Notes and activities

There is no separate "notes" feature in Margince. A note is an activity, and
activities come in six kinds: **email, call, meeting, note, task, message.**

### Where can I see everything that happened with a customer?
To see everything that happened with a contact or company in Margince, open the record's **History** tab: one timeline of emails, meetings, calls, notes, tasks, messages and field changes.
1. Under **Timeline filter**, pick **All**, **Threads** (email conversations), **Activities** or **Changes**.
2. Narrow it with **All kinds** (Email, Calls, Meetings, Notes…), **Search this timeline**, and **From** and **To** dates.
A company's History also holds what reached it through its deals and the contacts who work there.
Also called: timeline, activity log, customer history, interaction history.

### How do I log a note, call or meeting on a record?
To log an activity in Margince, open the contact, company, lead or deal and choose **Log activity** in its header.
1. Pick the **Type**: Note, Task, Call or Meeting.
2. Set the **Date** (or **Due date** for a task), and **Assignee** or **Attendees** where offered.
3. Enter a **Subject** (required) and the **Details**. For a meeting transcript, tick **This text is a transcript**; you can then paste it or **Or upload a file** (.txt only).
4. Choose **Log**.
Without permission you see "You do not have permission to log activities on this record."
Also called: add a note, record a call, write a comment.

### How do I add a task on a record?
To add a task in Margince, open the contact, company, lead or deal and choose **Add task** in its header. It opens the **Log activity** form started on the Task type.
Enter a **Subject** (required), a **Due date** and an **Assignee** (or leave it **Unassigned**), then choose **Log**.
Open tasks have **Done** and **Snooze 1 day**.
Also called: create a to-do, follow-up, reminder.

One activity can link to several records at once — a contact and a deal, say.
One you log with no links is visible to everyone; a *captured* message with
nothing to link to stays held, because nothing has judged who it belongs to.

A meeting carries a status: **booked, held, no-show, canceled.**

**Activities are never hard-deleted.** If something is filed against the wrong
record, the fix is **Relink**, not delete.

### How do I edit or delete a note, call or meeting I logged?
You cannot edit or delete a logged activity in the Margince app: a timeline row has **Relink** and, except for email, **Change visibility**, but no Edit or Delete. To correct one, log a new activity with the right details. An open task can still be moved with **Move to** (a new date), **Snooze 1 day** or **Done**.
To move an activity to the right record, choose **Relink**, search under "Search contacts, companies, deals, leads or projects", tick **Replace existing link** to swap rather than add, and choose **Relink**.
Also called: fix a note, remove an activity, change a task's due date, move an email to another deal.

### Can I mention a colleague or comment on a record?
No. Margince has no @mentions and no comment thread on a record. To tell a colleague something about a record, log a **Note** with **Log activity**, or give them a task with **Add task** and pick them as **Assignee**. To ask a colleague to introduce you, use **Request introduction** on the contact.
Also called: tag a teammate, @mention, leave a comment, notify a colleague.

### Who can see a note, call or meeting I log?
A note, call or meeting you log in Margince can be read by anyone who can open at least one record it is linked to; one with no links is readable by everyone. To narrow it, press **Change visibility** on its timeline row, choose **Participants only** or name the colleagues and teams who may read it, and press **Save visibility**.
Colleagues outside that audience see that it exists, not what it says, and an administrator's wider access does not override it. Captured mail works differently: see [Who can see an email](who-can-see-an-email.md).
Also called: private note, who sees my notes, hide a call, make a note private.

A captured message's audience comes from the importing mailbox; a new mailbox
holds its mail until a classifier judges the thread ordinary. Change it by
sharing the thread, not by editing the row.

## Putting a change back

### How do I undo a change on a record?
To undo a change in Margince, open the record's history and choose **Undo** on the entry. On a contact, lead or deal, open the **History** tab and pick **Changes**; on a company, choose **More actions** → **Full history**.
1. Find the entry and choose **Undo**.
2. If it changed more than one field, or a link between records, confirm in **Undo this change?**.
An undone change shows **Redo**. Where **Undo** is refused, the button says why before you press it.
Only a human can undo; an agent cannot.
Also called: revert, roll back, put a change back.

Every Margince record keeps a history of what changed, who changed it and when,
and you can put a single **entry** back — whole, not the record to a point in
time. There is no per-field undo: if one change touched four fields, all four
return together, and the confirmation lists them — "Fields reverting to their
values before this change: {count}".

Putting a change back is an ordinary edit, not a special power: it goes through
the same rules as typing the old value yourself, and appears in the history as
its own entry, which can itself be undone.

Not every entry can be put back, and the history says which and why before you
press anything. The three reasons you will actually meet:

- **The field moved again since.** Putting the entry back would silently discard
  whatever was written after it, so it is refused. The reason names the field,
  so you can look at what happened in between and decide.
- **The record was archived.** A change cannot be put back onto an archived
  record, and an archived record cannot be restored.
- **The change did not come from an editable path.** Some entries record things
  the record's own edit path cannot write — an entry that would have to clear a
  field nothing can clear. These are shown as history, not as something to undo.

If somebody else edits the record between your reading the screen and pressing
the button, the change is refused and nothing is written. Read the reason, look
at the record again, and decide from what is there now.

**An agent cannot put a change back.** It is a human's authority on purpose:
otherwise an agent could reach a change it was never allowed to make directly by
making it, and then undoing the undo.

### Can two users edit the same record at the same time?
Yes. Two users can edit one contact, company, deal or lead in Margince at once: each save sends only the fields that user changed, so edits to different fields both land. If a colleague saved the same field first, your save is refused with "This record changed since it was opened. Reload and retry." and nothing from your form is written; close it, reload the record, check the new value and edit again. A project or offer refuses the save after any change since you opened it. Nothing locks a record or shows who else has it open.
Also called: edit conflict, simultaneous editing, someone overwrote my change.

## Custom fields

Custom fields in Margince are fields your company adds beyond the columns every
installation has. Seven types — **text, number, date, currency, picklist,
multiple choice, yes/no** — on six record types: contacts, companies, deals,
leads, projects and contracts. A multiple-choice field holds several answers at
once, and a filter finds the record by any one of them.

### How do I add a custom field?
To add a custom field in Margince, open Settings → **Fields**, pick the **Object**, and choose **New field**.
1. Choose the **Object**: Deal, Company, Contact, Lead, Project or Contract.
2. Choose **New field**, enter a **Label** and pick a **Type** (a picklist needs its options, a currency field its currency code).
3. Confirm with **Add field**.
The new field then appears in that record's create and edit forms. Without the right to change fields you see "You have read-only access to custom fields."
More on managing fields is in [Settings](settings.md).
Also called: custom property, extra column, new attribute.

**Activities cannot carry a custom field**, and that is deliberate rather than
an oversight: a custom field on an activity could be created and never read
back, so the product refuses to offer what it could not serve.

An administrator names the field; what sits behind it is derived from that name
once and never changes, so renaming moves the label and leaves your reporting
intact.

## Tags

A tag is a shared word this company files records under. **Anyone can apply one;
only admin and ops seats add, rename or retire them.**

Every tag has its own page listing the records carrying it, grouped by type.

### How do I tag a record?
To tag a contact, company or deal in Margince, open the record, find its **Tags** panel and choose **Add tag**.
1. Type in **Search tags** and pick an existing tag. A tag the record already carries shows **Already added**.
2. To take one off, choose **Remove {name}** on the tag, then **Remove from this record**.
Anyone can apply a tag, but only Admin and Ops users can create, rename or retire one, in **Settings → Tags**. If no tag matches, the picker says "No tag with that name. An administrator or operations user can add one."
Also called: label, add a label, categorise a record.

Renaming a tag renames it everywhere — there is one word, not a copy per record.

Two administrator actions are worth knowing:

- **Retire** takes a tag out of use without touching the records that carry it,
  and **Restore** brings it back.
- **Merge** folds one tag into another and **cannot be undone**: "Records
  carrying {name} will carry the other tag instead, and the name is released for
  anyone to use again." Afterwards it reports what actually moved — "{moved}
  records moved to the surviving tag. {collapsed} already carried both, so their
  duplicate was dropped." An agent may not merge tags on its own; a merge is
  staged for a human.

## Money

Margince stores amounts as whole minor units plus a currency code. There is no
floating-point money anywhere in the product.

Most currencies have two decimal places; some, such as yen, won and dong, have
none. The product carries its own table, because two standards disagree on
about ten currencies.

**Two currencies are never added together.** A column holding more than one
currency shows no total at all — it says "several currencies — no single total".
Adding euros to dollars produces a number that is not money, so Margince refuses
rather than guessing a rate.
