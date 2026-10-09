<!-- prose:plain -->
# Contacts, companies, leads, deals and projects

Margince holds five kinds of record: contacts, companies, leads, deals and projects.

## Each record in one line

**Contact**: a single human you do business with. The sidebar calls this **Contacts**.

**Company**: a company you work with. The sidebar calls this **Companies**.

**Lead**: a possible customer you have not qualified yet, kept apart from your contacts.

**Deal**: one piece of business, moving through stages to won or lost.

**Project**: the work that a client relationship is made of. It starts while a deal is still open and goes on after the deal closes.

Next to these are **activities** (the timeline of emails, calls, meetings, notes and tasks) and **documents**. Each has its own page.

### What is the difference between a lead and a deal?
A lead in Margince is someone you have not qualified yet. A deal is one
piece of business with a value, moving through pipeline stages to won or lost. A lead lives on **Leads** and has no company record behind it. A deal lives on **Deals** and may name a company, contacts and a project. Qualifying a lead turns it into a contact, and can open a deal in the same step.
Also called: prospect versus opportunity.

### What is the difference between a contact and a company?
A contact in Margince is a single human you do business with. A company is the
business they work for, or that you work with. A contact is linked to a company through a job link. That link is not required: a contact with no employer is still a contact.
Also called: account, client or
customer for a company.

## What connects to what

Margince asks for fewer links between records than most CRM products.

| Link | Required? |
|---|---|
| Contact → Company | **Optional.** It is a job link, with a role and dates. |
| Lead → Company | **There is none.** A lead holds a company *name* as free text. |
| Deal → Pipeline and stage | **Required.** |
| Deal → Company | **Optional.** |
| Deal → Contact | **Optional.** Contacts are on a deal as stakeholders. |
| Deal → Project | Not required, at most one, and both must name the same company. |
| Project → Company | **Required: at least one, always.** |
| Activity → any record | Not required. One you log with no links is shared with everyone; a captured one with no links stays held. |

Many users get two of these wrong.

**A deal does not need a company.** Name, pipeline, stage and source are required. Value, company and expected close date are not. Value and currency go together: you cannot give one without the other. A deal with no value has no currency either. Qualifying a lead creates a deal with no company at all, because a lead has no company to link.

**A lead has no company record behind it.** A lead's company is plain text, not a link to a company record, so imported leads do not create half-filled companies in your CRM.

## Contacts

A contact needs a name and a source. Everything else is up to you.

### How do I add a new contact?
To add a new contact in Margince, open **Contacts** in the sidebar and choose **New contact**.
1. Enter the **Full name** (required).
2. If you like, fill **First name**, **Last name**, **Title** and **LinkedIn**.
3. Choose **Add email** for each address and set its **Type** (Work, Personal or Other); choose **Add phone** for each number.
4. Fill any custom fields, then choose **Create**. It stays turned off until the full name is filled.
If the contact is already in Margince, **View existing record** opens it instead.
Also called: create a contact, add someone to the CRM, new customer contact.

### How do I add a contact quickly, or import contacts from vCards?
To add a contact quickly in Margince, open **Contacts** and choose **Quick capture**. To add many from a card file, choose **Import vCards**.
- **Quick capture** asks for **Full name** (required), **Title**, **Company**, **LinkedIn**, **Email** and **Phone**, and confirms with "{name} saved"; the form stays open.
- The **Company** box in Quick capture lists the companies that match what you type: pick one to link it. A name you do not pick creates a new company.
- **Import vCards** asks for a **vCard file**: choose **Select .vcf file**. Each card is reported as Added, Missing fields added, Possible duplicate or Skipped.
Also called: import contacts, upload a .vcf.

### How do I edit a contact?
To edit a contact in Margince, open it and click the value to change in the **Details** panel next to the record. There is no Edit button: each field changes in place.
1. If the panel is hidden, choose **Show details and permissions**.
2. Click the value (an empty one says **Not set**), type and press Enter, or pick from the list. `Esc` stops.
3. **Email**, **Phone** and **Address** open a small form with **Type**, **Primary** and **Remove**; press **Save**.
**Full name**, **Title**, **LinkedIn**, **Owner** and custom fields change the same way. First and last name are set only when the contact is created.
Also called: update a contact, change a phone number, rename.

### Why can't I edit a contact or company?
A contact or company in Margince takes no edits when it is archived ("This contact is archived and takes no changes."). It also takes none when it is not yours to change: "You cannot edit this contact. Ask the owner to share it, or an administrator for edit rights." Fields you cannot edit show as plain text, with the reason when you point at them.
If someone else changed the same field while you were editing, your edit is refused: "This record changed since it was opened. Reload and retry."

Also called: read-only record, edit greyed out, cannot change.

### How do I link a contact to a company?
To link a contact to a company in Margince, open the contact's page and choose **Add company** in its **Companies** panel.
1. Under **Employer**, search for the company and pick it. It must already be in Margince; create it first under **Companies** if it is not.
2. If you like, enter the **Role** and a **Start date** (`YYYY-MM` or `YYYY-MM-DD`).
3. Tick **This is their current employer** if it is.
4. Choose **Create**.
To change or end the link later, use the row's **More actions** menu: **Edit employment**, **Mark as ended** or **Remove**.
Also called: set a contact's company, assign to an account, employer.

### How do I match a purchased job history entry to a company?
Purchased job history can name an employer Margince cannot match. Then the contact's **Companies** part shows **Company match needed.**; choose **Match company**.
Pick a company that is already there, or enter its **Confirmed company website** to create it, then choose **Save company link**. Your choice counts for every role at that employer that has no match yet. **Dismiss evidence** drops an entry that should not become a link. A company name alone never creates a company.

Also called: unresolved employer, imported job history.

Emails have the types **work, personal or other**; phones **work, mobile, home or other**. One number may show two times under two types, such as a main number that is both work and mobile. Editing the list keeps the two entries apart.

The contact header says **who can see this contact**: only its owner, or everyone in the company; a company header says the same. See [Seats, roles and who can see what](seats-roles-and-access.md).

A contact's job title shows in two places. The title on the contact record is a copy, there to save you time. The one that counts is on the job link with the company. A contact can have one current main employer, or none.

A contact also holds an owner, a consent record for each purpose, a relationship strength and when they were last active. If they started as a lead, it holds a link back to that lead.

### How do I merge duplicate contacts?
To merge two duplicate contacts in Margince, open the contact you want to remove, choose **More actions**, then **Merge contact**.
1. Search for the other contact and pick it under **Select record to keep**.
2. Read the question: "Merge {source} into {target}? This archives {source}."
3. Choose **Merge**. Margince opens the contact you kept.
The contact you merged away is archived, not deleted. Leads have no merge; deals have no merge either.
Also called: combine, deduplicate, remove a duplicate.

### How do I call a contact?
To call a contact from Margince, open the contact and click the phone number in its header or **Details** panel. It opens your phone or calling app. Margince does not make or record calls itself.
The **Call** button in the contact header opens the **History** tab, so you can log the call after with **Log activity** → **Call**.
Also called: phone a contact, click to call, dial.

### Can I add a photo to a contact, or change a company's logo?
No. Margince has no photo upload for a contact: its picture is made from the name. A customer company's logo is read from its website and cannot be uploaded. Only your own company's logo is set, in **Settings → Company profile** under **Company logo**.
Also called: profile picture, avatar, company image.

## Companies

A company needs a display name and a source.

### How do I add a company?
To add a company (a new customer, client or account) in Margince, open **Companies** in the sidebar and choose **New company**.
1. Enter the **Company name** (required).
2. If you like, fill **Legal name**, **Industry** and **Company size**.
3. Choose **Add domain** for each domain; a domain row you add must be filled.
4. Fill any custom fields, then choose **Create**.
If the company is already in Margince, **View existing record** opens it instead. The website address comes from the main domain, so there is no website box.
Also called: new account, new business, add a client or customer.

### How do I edit a company?
To edit a company in Margince, open it and click the value to change in its **Details** panel. Each field changes in place, with no Edit button.
1. If the panel is hidden, choose **Show details**.
2. Click a value, change it, and press Enter or pick from the list. `Esc` stops.


The fields are **Company name**, **Legal name**, **Industry**, **Company size**, **Owner**, **Lifecycle** and **Relationship type**. Then come **LinkedIn URL**, **Address**, **Domains**, **Parent company**, **Description**, custom fields, and **Register / VAT ID** under **Registration**. The website follows the main domain.
Also called: rename a company, update an account.

### How do I check a company's VAT number?
To check a VAT number with the EU VAT register, type it into **Register / VAT ID** in the company's **Details** panel. Margince checks a new number on its own.
1. Open the company, and find **Register / VAT ID** under **Registration** in **Details**.
2. Type the number as it is written, and press Enter.
3. Press the mark beside the number to read the answer.
The mark shows **Valid**, **Not valid**, or "not yet checked with the register". **Check with the register** or **Check again** asks once more.
The answer arrives a few seconds later, while the button shows "Checking with register…". A number checked in the last five minutes is not checked again.
Also called: VIES check, check a VAT ID, tax number check.

### What does the VAT check answer mean?
The mark opens the receipt: **Register result**, **Number consulted**, **Registered to**, **Registered address**, **Consulted on** and **Consultation number**.
**Valid** means the number is real, not that it belongs to this company. Read **Registered to**: a number copied from a website often belongs to another company.

**Consultation number** is the proof a tax office accepts. It reads "None issued." until your own VAT ID is filled in on **Settings → Company profile**.
When the number changed after a check, the receipt says "The number on this record changed after this check." Press **Check again**.

A grey question mark means the answer could not be loaded; press it to try again.
If a check says "This installation does not consult the VAT register.", an administrator has to turn it on.
Also called: VAT ID valid, VAT receipt, Registered to.

### How do I change the owner of a contact or company?
To change who owns a contact or company in Margince, open the record and pick a colleague in the **Owner** row of its **Details** panel. The change saves as soon as you pick.
You need edit rights on the record: your own, your team's, one shared with you for writing, or a role that edits every record. A private contact must keep an owner ("This field is required."). To give many records to a colleague at once, see [Lists, filters and views](lists-filters-and-views.md).
Also called: reassign a contact, hand over an account, transfer ownership, account owner.

### How do I claim a company nobody owns?
To claim a company nobody owns in Margince, open it and pick yourself in **Owner**, in the facts under its name. The company is yours as soon as you pick.
Everyone can read a company nobody owns, and nobody can edit it. So its **Details** panel takes no changes until someone claims it. Any role that may edit companies can claim one; nobody can claim an archived company.
Also called: take an unowned account, assign a company to me, claim an account.

Two fields describe a company, and many users mix them up.

**Lifecycle**: where the company stands with you. One value only: **unknown, target, prospect, opportunity, customer, former customer, disqualified.** New companies start at unknown.

**Relationship types**: what the company *is* to you. Many at once: **customer, partner, supplier, investor, portfolio company, competitor, other.**

So a company can be a customer *and* a supplier. It cannot be both a prospect and a customer, because that is one question with one answer.

Size bands are: 1-10, 11-50, 51-200, 201-500, 501-1000, 1001-5000, 5000+.

A company may have a parent company, one level only: a parent has no parent of its own, and no company is its own parent. The website address comes from the main domain; you do not type it.

One company in your Margince is marked as the **anchor**: your own company, the one Margince knows things about for you.

### How do I merge duplicate companies?
To merge two duplicate companies in Margince, open the company you want to remove, choose **More actions**, then **Merge company**.
1. Search for the other company and pick it under **Select record to keep**.
2. Read the question: "Merge {source} into {target}? This archives {source}."
3. Choose **Merge**. Margince opens the company you kept.
The company you merged away is archived, not deleted. You cannot merge an archived company.
Also called: combine companies, deduplicate accounts.

### How do I stop a company coming back from email?
To archive a company that is not a real company, and stop mail from creating it again, open it. Choose **More actions**, then **Not a company**.
1. Enter the **Reason this is not a company**.
2. Confirm. Margince archives the company and blocks its domain, so later mail from that domain does not create it again.
An admin can remove the block on the domain in Settings under Capture. You see the action only where the company has a domain and your seat may do both parts.
Also called: junk company, spam domain, block a domain.

## Leads, deals and projects

Leads, deals and projects have their own page. It says how to create, qualify and open again leads, how to create deals and projects, and how each connects: [Leads, deals and projects](leads-deals-and-projects.md).

## Archiving, restoring and deleting records

### How do I archive a contact, company, deal or project?
To archive a record in Margince, open it and choose **More actions**. Then choose the archive entry: **Archive** on a contact or company, **Archive deal** on a deal, **Archive project** on a project.

Confirm the box. For contacts and companies it reads "Archive this record? You can bring it back from its history." You can archive many contacts, companies or deals at once with **Archive** in the list's bar for many records. A lead is not archived; disqualify it instead.

An archived record leaves the live list; turn on **Show archived** on the list to see it again. It becomes read-only.
Also called: remove, hide, deactivate a record.

### Can I restore or unarchive an archived record?
Yes, for a contact, company or deal. Open the record with **Show archived** and go to its history. Find the entry that archived it and press **Undo**. The record comes back.

Its email addresses, phone numbers, links, list members and tags come back too, where they still can. Anything that could not come back is named in the history. Undo is refused when the record was archived again since, merged into another record, or erased. It is also refused when another record now holds its email address or domain.

Margince archives some records on its own, such as emails it judged to be of no use. Those are listed in the **Since your last brief** panel on Home. That panel says what Margince has done while you were away. A contact, company or deal there has its own **Undo**.

 You cannot bring back an archived email or meeting yet. You cannot bring back a project from the app; create it again, or ask an admin. An archived record takes no edits or merges.

Tags and pipelines do have a **Restore**; an archived team comes back with **Undo** on its archived notice. You can open a disqualified lead again with **Reopen**.
Also called: unarchive, undelete, bring back a record.

### Can I undo a record Margince created?
Yes. The **Since your last brief** panel on Home groups what Margince created on its own. For example, "Created contact" holds the contacts it found in your mail. Open the line to see every record, and press **Undo** on one to archive it. The same **Undo** is on the "Created" entry in the record's history.

Undo is refused once a colleague has changed the record, so their work is never archived with it. A lead cannot be undone this way; disqualify it instead.
Also called: remove an imported contact, take back an automatic record.

### How do I delete a contact or company?
You cannot delete a company or a contact in Margince: there is no delete button for a contact, company, lead, deal or project. To remove one, archive it (open the record, choose **More actions** → **Archive**). That keeps the record but takes it off the live lists.

A contact's data is destroyed only by an erasure request or a retention rule. No erasure request or retention rule reaches a company. See [What is kept, what is destroyed](retention-exports-and-deletion.md). Files and knowledge documents do have their own **Delete**.
Also called: delete a company, remove a company, delete a contact, remove permanently, erase.

## Notes and activities

Margince has no notes of their own: a note is an activity. The kinds of activity are **email, call, meeting, note, task, message.**

### Where can I see everything that happened with a customer?
To see everything that happened with a contact or company in Margince, open the record's **History** tab. **All** is one timeline of emails, meetings, calls, notes, tasks and messages. Field changes are listed under **Changes** alone.
1. Under **Timeline filter**, pick **All**, **Threads** (email conversations) or **Changes**.
2. Cut it down with **All kinds** (Email, Calls, Meetings, Notes…), **Search this timeline**, and **From** and **To** dates.
A company's History also holds what reached it through its deals and the contacts who work there.
Also called: timeline, activity log, customer history, interaction history.

### How do I log a note, call or meeting on a record?
To log an activity in Margince, open the contact, company, lead or deal and choose **Log activity** in its header.
1. Pick the **Type**: Note, Task, Call or Meeting.
2. Set the **Date** (or **Due date** for a task), and **Assignee** or **Attendees** where you see them.
3. Enter a **Subject** (required) and the **Details**. If you enter a meeting transcript, tick **This text is a transcript**. You can then paste the text, or use **Or upload a file** (`.txt` only).
4. Choose **Log**.
Without permission you see "You do not have permission to log activities on this record."
Also called: add a note, record a call, write a comment.

### How do I add a task on a record?
To add a task in Margince, open the contact, company, lead or deal and choose **Add task** in its header. It opens the **Log activity** form set to the Task type.
Enter a **Subject** (required), a **Due date** and an **Assignee** (or leave it **Unassigned**), then choose **Log**. Open tasks have **Done** and **Snooze 1 day**.
Also called: create a to-do, follow-up, reminder.

One activity can link to many records at once, such as a contact and a deal. Everyone can see one you log with no links. A *captured* message with nothing to link to stays held, because nothing has judged who it belongs to.

A meeting holds a status: **booked, held, no-show, canceled.**

**Activities are never fully deleted.** If something is filed on the wrong record, the fix is **Relink**, not delete.

### How do I edit or delete a note, call or meeting I logged?
You cannot edit or delete a logged activity in the Margince app. An activity on the timeline has **Relink** and, except for email, **Change visibility**, but no Edit or Delete. To fix one, log a new activity with the right details. You can still move an open task with **Move to** (a new date), **Snooze 1 day** or **Done**.
To move an activity to the right record, choose **Relink** and search under "Search contacts, companies, deals, leads or projects". Tick **Replace existing link** to change the link and not add one, then choose **Relink**.

Also called: fix a note, remove an activity, change a task's due date, move an email to another deal.

### Can I mention a colleague or comment on a record?
No. Margince has no @mentions and no comments on a record. To tell a colleague something about a record, log a **Note** with **Log activity**. Or give them a task with **Add task** and pick them as **Assignee**. To ask a colleague to introduce you, use **Request introduction** on the contact.
Also called: tag a teammate, @mention, leave a comment, notify a colleague.

### Who can see a note, call or meeting I log?
A note, call or meeting you log in Margince can be read by anyone who can open at least one record it is linked to. One with no links can be read by everyone. To see who may read it, press **Change visibility** on it in the timeline. Choose **Participants only** or name the colleagues and teams who may read it, and press **Save visibility**.
Colleagues not in that group see that it is there, not what it says, and an admin's wider access does not change that. Captured mail works in another way: see [Who can see an email](who-can-see-an-email.md).

Also called: private note, who sees my notes, hide a call, make a note private.

Who can see a captured message comes from the mailbox it was brought in from. A new mailbox holds its mail until Margince judges the thread to be ordinary. Change it by sharing the thread; the message itself has no visibility setting.

## Putting a change back

### How do I undo a change on a record?
To undo a change in Margince, open the record's history and choose **Undo** on the entry. On a contact, lead or deal, open the **History** tab and pick **Changes**; on a company, choose **More actions** → **Full history**.
1. Find the entry and choose **Undo**.
<!-- prose:allow residue quotes the Undo this change? dialog title as the screen shows it -->
2. If it changed more than one field, or a link between records, confirm in **Undo this change?**.
A change you have undone shows **Redo**. Where **Undo** is refused, the button says why before you press it. Only a human can undo; an agent cannot.
Also called: revert, roll back, put a change back.

Every Margince record keeps a history of what changed, who changed it and when. You can put back a single **entry** as a whole; you cannot take the record back to a point in time. There is no undo for a single field: if one change changed four fields, all four go back together, and the question lists them:
<!-- prose:allow residue quotes the undo confirmation as the screen shows it -->
"{count} fields revert to their values before this change:".

Putting a change back is an ordinary edit. It follows the same rules as typing the old value yourself, and shows in the history as its own entry, which you can undo too.

Not every entry can be put back, and the history says which and why before you press anything. The reasons you will see:

- **The field changed again since.** Putting the entry back would lose
  whatever was written after it, so it is refused. The reason names the field,
  so you can look at what happened in between and decide.
- **The record was archived.** A change cannot be put back on an archived
  record; bring a contact, company or deal back first with **Undo** on its archive entry.
- **You cannot make that change by hand.** Some entries record changes no edit
  can make, such as clearing a field that must not be empty. They show as
  history only.

If someone else edits the record between your reading the screen and pressing the button, the change is refused and nothing is written. Read the reason, look at the record again, and decide from what is there now.

**An agent cannot put a change back.** Only a human can undo a change.

### Can two users edit the same record at the same time?
Yes. Two users can edit one contact, company, deal or lead in Margince at once. Each save changes only the fields that user changed, so edits to different fields both go through. If a colleague saved the same field first, your save is refused with "This record changed since it was opened. Reload and retry."

Nothing from your form is saved; close it, load the record again, check the new value and edit again. A project or offer refuses the save after any change since you opened it. Nothing locks a record or shows who else has it open.
Also called: edit conflict, simultaneous editing, someone overwrote my change.

## Custom fields

Custom fields in Margince are fields your company adds to the ones every Margince has. The types are **text**, **number**, **date**, **currency**, **picklist**, **multiple choice** and **yes/no**, on contacts, companies, deals, leads, projects and contracts. A field of many choices holds many answers at once, and a filter finds the record by any one of them.

### How do I add a custom field?
To add a custom field in Margince, open Settings → **Fields**, pick the **Object**, and choose **New field**.
1. Choose the **Object**: Deal, Company, Contact, Lead, Project or Contract.
2. Choose **New field**, enter a **Label** and pick a **Type**. A picklist needs its choices, and a currency field needs its currency.
3. Confirm with **Add field**.
The new field then shows in that record's create and edit forms. Without the right to change fields you see "You have read-only access to custom fields." More on fields is in [Settings](settings.md).
Also called: custom property, extra column, new attribute.

**Activities cannot hold a custom field.**

Renaming a field changes its label and keeps your reports working.

## Tags

A tag is a shared word this company files records under. Anyone can put one on a record; **only admin and ops seats** add, rename or retire them.

Every tag has its own page that lists the records with it, grouped by type.

### How do I tag a record?
To tag a contact, company or deal in Margince, open the record, find its **Tags** panel and choose **Add tag**.
1. Type in **Search tags** and pick a tag. A tag the record already has shows **Already added**.
2. After you tag a contact, Margince may ask "Also tag {company} with {tag}?". It asks when you may change their current company and the company has no such tag yet. **Tag {company}** adds it there too. **Not now** changes nothing.
3. After you tag a company, Margince asks "Also tag contacts at this company with {tag}?". **Choose contacts** lists who works there now, each with a box to tick. A contact who already has the tag shows "(already tagged)". **Continue** previews the change like any bulk change. The preview names each contact you may not change, and the result offers **Undo**.
4. To take one off, choose **Remove {name}** on the tag. It comes off at once. The toast "{name} removed from this record" offers **Undo**. **Undo** puts the tag back with the same name and date under **Added by**. A retired tag comes off with no **Undo**.

To see who put a tag on a record and when, point at the tag or move to it with the keyboard.

Anyone can put a tag on a record, but only Admin and Ops users can create, rename or retire one, in **Settings → Tags**. If no tag matches, the picker says "No tag with that name. An administrator or operations user can add one."
Also called: label, add a label, categorise a record.

Renaming a tag renames it everywhere, because every record shares the one tag.

Two admin actions are worth knowing:

- **Retire** takes a tag out of use without changing the records that have it,
  and **Restore** brings it back.
- **Merge** puts one tag into another and **cannot be undone**. The warning
  reads: "Records carrying {name} will carry the other tag instead, and the name
  is released for anyone to use again". After that it reports what moved:
  "{moved} records moved to the surviving tag. {collapsed} already carried both,
  so their duplicate was dropped". An agent may not merge tags on its own; it
  can only suggest a merge for a human to approve.

### Can Margince suggest a tag from mail and meeting notes?
Yes, for tags an admin marks as suggested. Margince never adds such a tag by itself: you accept or dismiss each suggestion.

1. In **Settings → Tags**, choose **Edit** on the tag.
2. Fill in **Words that show interest**, separated by commas, for example "pricing for Product X, Product X demo".
3. Tick "Suggest this tag when mail or meeting notes use these words" and save.

Once an hour Margince reads the last 30 days of captured mail, meetings, notes and calls. When one of them uses the listed words, it suggests the tag on the contact or company that item is filed under. Each entry in the list needs at least three characters, and case does not matter.

A suggestion waits in your Worklist with the other decisions. It names the tag and lists each mail or note it came from. **Add tag** puts the tag on the record as you, and then offers it to the company or its contacts as above. **Not this tag** dismisses it for everyone. The same tag comes back on that record only when newer mail or notes use the words.

You see a suggestion only when you may read the record and every item it lists. A suggestion that came from mail only its owner can read is shown to that owner alone.

## Money

Margince stores every amount as a whole number of the currency's smallest unit, such as cents for euros, together with its currency. Most currencies have two places after the point; some, such as yen, won and dong, have none.

**Two currencies are never added together.** A column that holds more than one currency shows no total at all; it says "several currencies, no single total". Margince does not guess an exchange rate to add euros to dollars.
