<!-- prose:plain -->
# Importing data

**Data import** brings companies, contacts, prospects, deals and activities into Margince from CSV files. You find it in Settings, in the **Data** group. Only an admin or ops user can import. Nothing is written until you read the test run and choose to import.

### What can I import?
You can import companies, contacts, prospects (leads), deals and activities, from one CSV file or from one ZIP of CSV files.

In a ZIP the file name gives the record type: `companies.csv`, `contacts.csv`, `leads.csv`, `deals.csv` or `activities.csv`. Each type takes one file. A ZIP holds at most 20 files and cannot have a password.

Margince imports the files in that order, companies first and activities last. So a contact can name a company from the same ZIP, and a deal can name a company or a contact. An activity can name any of them.

A file whose name gives no record type is listed under **Files left out**. Nothing in it is read.

Also called: bulk import, upload a spreadsheet, move from another CRM, Excel import.

### What is in the template?
**Download template** on **Data import** gives a ZIP with one CSV for each record type. Each file has a column for each field the import takes. A custom field has its label as the column name.

The ZIP holds `companies.csv`, `contacts.csv`, `leads.csv`, `deals.csv` and `activities.csv`.

`README.txt` in the ZIP lists every pipeline with its stages, so a deal row can name them as written there. **Preview template** shows the files first. Point at a column to see what goes in it, an example, and whether a new row needs it.

Also called: import template, column help, import format.

### What are the steps of an import?
To import a file, open **Settings → Data import** and drop the file on **Upload your data**.
1. On **Check your files**, set **Import as** for each file, or choose **Skip this file**. Then choose **Next: map {count} files**.
2. In **Map columns**, pick a **Field** for each column, or **Do not import**. Choose **Run test**.
3. Read the **Test run**. Fix the file and choose **Fix and re-upload**, or choose **Import {count} rows**.
4. **Importing** shows each file as it goes. **Result** shows what was written.
5. To take the import back, choose **Undo this import** on the result, or **Undo** in **Recent imports**.
Also called: upload a CSV, import steps, import a ZIP.

### What does the test run tell me?
**Test run** reads the whole file and writes nothing. For each record type it counts the rows that are new, the rows that update a record, and the rows that stay the same.

It lists each problem with its file, row and column. Each problem has a kind, such as **Broken link**, **Invalid value**, **Unknown owner** or **New tag**. **problems.csv** downloads the list. Its columns are file, row, column, problem and value.

The test run also counts the deals that land as closed history. **Stop test** ends a test that is still running.

Also called: import preview, check an import, import report, test run.

### Which problems skip a row, and which stop nothing?
A problem never stops the whole import. It either skips that one row, or the row still lands and the problem is only reported. Every skipped row is listed in **problems.csv**.

These skip the row:
- a value its field does not take, such as an amount that is not a number or a date not in `YYYY-MM-DD`;
- a company link that finds no company, or finds more than one (**Broken link**);
- a row with no value in its `ref` column, or a `ref` that an earlier row of the same file already used;
- an email address, domain or LinkedIn address that another live record already holds, when the row would create a second record under it;
- a row whose record was archived since the earlier import;
- a contact row whose email belongs to a sending system, such as `noreply@`, `notifications@` or `receipts@`;
- a record type you may not create, or a change to a record you may not edit.

A contact row with **First name** and **Last name** filled takes its full name from those two. This holds even when the file also has a **Full name** column. A contact you add or edit in Margince keeps the full name you type.

These stop nothing. The row lands, and the problem is listed:
- an owner email that no user has (**Unknown owner**): you become the owner, except for a lead, which stays unassigned;
- a tag that does not exist yet (**New tag**): the import creates it;
- a company name that is already in Margince: the import creates a second company and files the pair for review (**Duplicate**).
Also called: rows skipped, import errors, why was my row not imported.

### What does **Import anyway** do?
**Import anyway** lands the rows that have a broken link, without the link. It appears when the test run finds one. Tick the box, and the test runs again with the button changed.
The result lists every link that was left out as **Link left out**. A row with another problem is still skipped.
Also called: keep rows with broken links, import without the company link.

### How long does an import take, and can I leave the page?
You can leave the page. An import runs on its own once it starts. **Recent imports** shows when it is done, and its file name opens it again.

While it runs, **Importing** shows the rows done and the rows in all. It also shows the rows per second, the time passed and the minutes left. The time left shows only after the import has run for about 10 seconds, and it is a guess, not a promise.

If an import stops before the end, it says **Import stopped partway** and why. Choose **Resume import** to go on from that row, not from the start.

Also called: progress bar, time left, import stuck, continue an import.

### What is the source name?
**Source name** keeps the records of one source together. Use the same name on a later upload, and Margince updates the records it made the first time. Use another name, and the same `ref` in the new file means a different record.

A ZIP starts with the source name of your last ZIP import, or else with the ZIP's own name. One CSV starts with none. A name has at most 80 characters: letters, numbers, dots, `_` or `-`.

While an import of one source name is still writing, Margince does not test another file under that name. Your file waits as a draft in Recent imports. Wait for the other import to finish, then select **Continue** on the draft. A draft that nobody touches for 7 days is canceled.

Also called: re-import a ZIP, upload the export again.

### What is the `ref` column?
`ref` is your own key for a row: the ID from your old CRM, or any text you choose. Files link to each other through it, and a later upload finds the row again by it.

A row is found by its `ref` within its source name and its record type. Company `a1` and contact `a1` are two keys. A deal always needs a `ref`, because two deals can share a name.

A row without a `ref` is matched by a company's display name, or by a contact's or prospect's email address.

Also called: ID from another system, key column, record ID from my old system.

### How do owners, tags and custom fields import?
- **Owner:** put the email of a Margince user in `owner_email`. If no user has it, you become the owner; a lead stays unassigned. The test run lists each such address once, with the number of rows that name it.
- **Tags:** separate tags with `;` in `tags`. A new tag is created, and the test run lists it. A tag column only adds tags, and never removes one. **Tag for this import** files every record the import creates under one more tag. Records it only updates keep their own tags.
- **Custom fields:** a custom field has its own column. A list field takes one of its allowed values. A field that takes several values has them split by `;`.
Also called: record owner, tag a group of records, import custom fields.

### How does an address in one column import?
A company or contact file may hold the whole address in one column. Map that column to `address.line1` and map no other address column. Margince then splits the address into its parts.

If Margince cannot split it, the address stays in line 1, and the test run warns about that row. A file that also maps another address part keeps line 1 as it is written.
Also called: import a full address, split an address column.

### How do I set a prospect's status?
A `leads.csv` row can carry a `status`: `new`, `contacted` or `engaged`. Case does not matter. An empty box leaves a new prospect as `new`, and an existing one as it is.

A file cannot set a prospect to qualified or disqualified. Use **Qualify** or **Disqualify** on the prospect. A row with another word is skipped, and the problem lists the allowed values.
Also called: import lead status, set the stage of a lead.

### How do I map a lead source from my old system to Origin?
A `leads.csv` file can map a column to **Origin** and another to **Origin note**. Origin says where the lead came from. It is not the source name above, which says which import wrote the row.

Margince matches the value to an origin in Settings by its name, ignoring case. "Cold Outreach" and "cold_outreach" both file the lead under **Cold outreach**. A value that matches no origin is kept as the lead's origin note, and the origin stays empty. Margince never guesses one. An empty box leaves the origin empty.

A later file never replaces an origin a lead already has, because you may have set it by hand. It only fills a lead that has none. A lead with no origin gets no points for it, so an import does not lower a score.
Also called: import lead source, map lead origin.

### How do I import deals?
A `deals.csv` row is one deal. Each row needs a `ref`. A new deal also needs a `name` and a `stage`.

| Column | What goes in it |
|---|---|
| `pipeline` | The pipeline's name. Empty means the default pipeline. |
| `stage` | The stage's name in that pipeline. Case does not matter. |
| `amount` and `currency` | The value as a plain number, and its three-letter currency code. An amount needs a currency. |
| `close_date` | `YYYY-MM-DD`. An open deal's date cannot be in the past. |
| `company_ref` | The deal's company, found as a contact's is. |
| `contact_ref` | A contact on the deal, by `ref`, email or Margince ID. The contact becomes a stakeholder. |
| `lost_reason` | Needed when the stage is a lost one. Written only when the row creates the deal. |

A deal on a won or lost stage is imported as closed history, with the close date in the file. Imported deal changes start no automation and earn no commission. Webhooks still receive them, marked as imported.

A deal row is skipped, with its reason, in four cases. Its stage is not in the pipeline. It moves a deal to another pipeline. It moves a deal that is already closed. Or an open deal's close date is in the past.

A currency with no exchange rate for the close day also skips the row.
Also called: import opportunities, import closed deals, import pipeline history.

### How do I import activities?
An `activities.csv` row is one note, call, meeting or email, put on the timeline at the day it happened. Every row needs a `ref`, a `type` and an `occurred_at`. An import from the import page also needs at least one entry in `links`.

| Column | What goes in it |
|---|---|
| `type` | `note`, `call`, `meeting` or `email`. A message and a task are not imported. |
| `subject` and `body` | A short title, and the text of the note or of what was said. |
| `occurred_at` | `YYYY-MM-DD`, or a date and time with its zone. A date alone means the start of that day, in UTC. |
| `links` | Records as `type:key`, split by `;`, such as `deal:D-1001; contact:jo@acme.com`. The type is company, contact, lead or deal. At most 25. |
| `duration_minutes` | Whole minutes, for a call or a meeting. |
| `meeting_status` | `booked`, `held`, `no_show` or `canceled`, for a meeting only. |
| `direction` | `inbound` or `outbound`. Written only when the row creates the activity. |
| `owner_email` | The Margince user the activity is attributed to. If none has this email, you are. |

A contact's last activity becomes the newest imported date when it is newer. A task does not count, and neither does a `no_show` or `canceled` meeting.
Imported activities start no automation and sit on no follow-up reminder.

A row is skipped, with its reason, in these cases:
- its type is missing or unknown, or it is a message or a task;
- its time has no zone, or is after now;
- it has minutes or a meeting status that its type does not take;
- it links a call or a meeting to a company;
- it has a link in the wrong form, or none of its links finds a record;
- you may not add activities to one of the records it links to.
Also called: import notes, import calls, import meetings, import emails, import timeline history.

### How do I import activities for one contact?
On the contact, choose **More actions**, then **Import activities**, and upload one CSV. It opens as **Importing into** the contact's name.
Every row goes on that contact's timeline, so the file needs no `links` column. A ZIP is not taken here. Only an admin or ops user sees the item.
Also called: import a contact's history, add past calls to a contact.

### What if a link is broken?
A contact's or deal's `company_ref` names its company. It is the company's `ref` in this upload or an earlier one, its domain, its exact name, or its Margince ID. A row whose link finds no company is skipped and listed as **Broken link**. See **Import anyway** above.

An activity's `links` name a record the same way, by `ref` or Margince ID, or for a contact by email. A link that finds no record is a **Broken link**. With **Import anyway** the activity keeps its other links, and one with none left is still skipped. A link to a row that this import skipped, such as a deal with no exchange rate, is a broken link too. The problem says the row was skipped.

Also called: missing company, link not found.

### What happens to a company I already have?
A company that Margince already holds, and that no import made, is not matched by its name. The import creates a second company and files the two for review as a possible duplicate. A contact whose email address is already held is skipped.
To change companies you already have, correct them by their ID. A company you cannot see is not counted or named in the test run.
Also called: duplicates on import, import made copies.

### Does an import file contacts with the same first name as duplicates?
No. Two contacts that share only a first name are not filed, even at one company. Their last names must match and their first names must be close.
Also called: duplicate queue too long after import, same name flagged.

### Why did a pair leave the duplicate list without anyone deciding it?
Margince checks the open duplicate list every night, and each time it starts. It closes a pair that today's rules would no longer suggest, for example two contacts who share only a first name. It also closes two companies with close names that share no address, phone number or contact.
A pair someone has dismissed or reopened is never closed this way. A closed pair keeps its history, and it comes back by itself if the two records start to match again.
Also called: duplicate gone, shorter duplicate list, fewer duplicates.

### Does Margince find duplicates that were already there before?
Yes. Every night Margince compares each company and contact that changed that day with all the others. It lists each new pair it finds on the duplicate list. An upgrade compares every record once.

So two companies that one import wrote at the same time are listed, and so are records written before a matching rule existed. A pair someone has dismissed or merged is never listed again. Margince never merges a pair by itself.
Also called: old duplicates, duplicates from before, duplicates not found, existing duplicates.

### How do I correct companies with a spreadsheet?
To correct companies in Margince from a CSV, give each row the company's ID and map that column to **id**.
1. Use **Export CSV** from **Filters and views**; its **id** column holds each ID.
2. Edit the file, then drop it on **Upload your data** in **Settings → Data import**.
3. On **Check your files**, set **Import as** to **Companies**, then choose **Next: map 1 file**.
4. In **Map columns**, map **id** to **id**, and the name column to **display_name**.
5. Choose **Run test**.
An empty ID creates a company. An ID that matches no company is skipped and named.
Also called: bulk edit companies, update companies from Excel, fix company data.

## Importing the same file again

### What happens if I import the same file again?
Importing a file again with the same source name updates the records the file made. It does not copy them. These rules hold for every record type:
- **Rows are matched on the `ref`** within the source name and the record type.
- **A row that did not change writes nothing.** The test run counts it as the same. When every row is the same or skipped, the test run says **Nothing to import**. **Done** closes the run, and there is nothing to undo.
- **A changed row updates only its changed fields.** Every other field of the record stays as it is.
- **A new `ref` creates a new record.**
- **A missing row is left alone.** A re-import never archives or deletes a record because its row is gone.
- **A changed `ref` is a new row.** Margince cannot tell it is the old record under a new key. It follows the duplicate rules above: a company name already held creates a copy for review, and an email address already held skips the row.
- **The file wins.** A field the file carries is set to the file's value, even if someone edited it in Margince since. A column the file does not carry is never touched.
- **An empty box changes nothing.** You cannot clear a value by emptying its box.
- **Emails, phone numbers, domains and addresses are merged.** The file's value is added or set as the main one, and the ones already on the record are kept. Only the address parts that have a column are replaced.
- **A clash skips one row.** If another record already holds a changed value, such as an email address or a domain, Margince skips that row with the reason. It goes on with the rest.
- **A row whose record was archived is skipped** with the reason "This row's record was archived". Restore the record to update it.
- **Owners, tags and custom fields:** the same rules hold. An owner email that no user has leaves the owner as it is.
- **Re-importing after an undo works for every object.** That includes prospects, because undo frees the `ref` and the same file creates the records again.

A deal also has its own rules. A re-import moves an open deal to a later stage, but never to another pipeline, and never reopens a closed deal. It writes `lost_reason` only when the row creates the deal. An activity's `direction` and `owner_email` are written only when the row creates it.

An example. ZIP 1 has the contacts `a1`, `a2` and `a3`. ZIP 2, with the same source name, has `a1`, `a2` with a new last name, `a3` and `a4`.
- `a1` and `a3` are the same, and nothing is written.
- `a2` is updated, and only its last name changes.
- `a4` is created.
- Nobody is copied.

Also called: re-import, upload again, update from a spreadsheet, import twice, duplicates after a second upload.

### What does undo keep?
Undo reverses one import, in reverse order: activities, then deals, then prospects, then contacts, then companies. The steps are in [What is kept, what is destroyed](retention-exports-and-deletion.md). These are the rules:
- Undo archives only the records that import created. A prospect is disqualified instead.
- Records the import only updated keep their new values. Undo never puts back the old values.
- A record someone edited after the import stays. It is listed under "Left in place by the undo:", with the reason beside it.
- A company that a live deal or contact still names stays too. It is listed under the same heading, with the number of deals and contacts that name it. A company the import only matched by its ID is never archived.
- New tags stay, because they may be in use.
- A finished undo frees the `ref` values of that import. The same file can be imported again, and it creates the records again.
- A record that undo kept is still found by its `ref`, so a later import updates it.
Also called: reverse an import, roll back an upload, import again after undo.

### What are the limits?
| Limit | Value |
|---|---|
| File size | 10 MB, unless whoever runs your installation set another. **Upload your data** shows the number. |
| Rows | 15,000 in one upload, all files together. A larger upload is refused: split it into smaller files. |
| Files in a ZIP | 20, one for each record type. |
| File format | CSV in UTF-8. |
| Source name | 80 characters: letters, numbers, dots, `_` or `-`. |
| Who | Admin and ops users. |

A spreadsheet app can change a value on its own. A value that starts with `=`, `+`, `@` or a minus sign may be read as a sum. Long numbers or dates may change their form. Save as CSV in UTF-8 and check those values before you upload.
Also called: import limits, file too big, too many rows, who can import.
