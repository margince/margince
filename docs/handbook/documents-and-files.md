<!-- prose:plain -->
<!-- describes: frontend/src/features/voice-knowledge frontend/src/features/deals/dealfiles.tsx frontend/src/components/filerowverbs.tsx frontend/src/features/contacts/contactfiles.tsx frontend/src/features/companies/companydocuments.tsx frontend/src/features/records/activityfiles.tsx frontend/src/features/projects/projectdocuments.tsx frontend/src/components/adddocument.tsx backend/internal/compose/uploadsettings.go backend/internal/modules/activities/attachmenttypes.go backend/internal/modules/activities/documents.go backend/internal/modules/activities/attachmentlink.go backend/internal/modules/knowledge frontend/src/components/chatcapture.tsx frontend/src/features/leads/leadhistory.tsx backend/internal/compose/chatcapturefile.go backend/internal/compose/chattasks -->
# Documents and files

Margince holds files in three different places, for three different reasons:

1. **Documents** on a company, a contact, a deal or a project: the papers
   of the relationship.
2. **Files** on a deal: everything the deal has picked up, including
   attachments that arrived with email.
3. **Document sets** in Settings → Knowledge: bodies of text the whole
   company can ask questions of.

### How do I upload a document?
To upload a document in Margince, such as a customer's signed contract, open the company, contact or deal. Choose its **Documents** tab and click **Add document**. On a project, press **Documents** on the **Contracts** card and click **Add document** in the panel that opens.
1. On a company, set **About** to **This company** or **A deal** and pick the deal.
2. Choose a **Category** (a signed contract takes **SOW/Contract**); **Title** is optional.
3. Drop the file on **File**, or click to choose one, and click **Upload**.
Without write access the button is not shown. The contract record itself is added on its own, see [Contracts and invoices](contracts-and-invoices.md).
Also called: attach a file, add an attachment, add a PDF, upload a contract document.

### How do I add a link, such as a Google Doc?
To add a link to a Google Doc, a Google Sheet or a web page, open the company, contact or deal. Choose its **Documents** tab and click **Add document**.
1. Choose **Link** at the top of the form and paste the address into **Address**.
2. Margince reads the page and fills **Title** with the name the page gives.
3. Choose a **Category** and, on a company, **About**, as for a file. Click **Add link**.
Margince does not read Google files, so type a title for one. Without a title, the link shows the site's address.

The link opens in a new tab. Its tile says **Docs**, **Sheets**, **Slides**, **Forms**, **Drive** or **Web**. Margince keeps only the address, so whoever opens it needs access to the file itself. One address can be added to a record only once.
Also called: link a Google Drive file, add a URL, add a web link to a company.

### How do I attach a file to a contact?
To attach or upload a file to a contact in Margince, open the contact, choose its **Documents** tab and click **Add document**.
1. Open **Contacts** in the sidebar and open the contact.
2. Choose the **Documents** tab, then **Add document**.
3. Pick a **Category** (**SOW/Contract**, **Proposal/Offer**, **NDA/Legal** or **Other**); **Title** is optional.
4. Drop the file on **File**, or click to choose one, and click **Upload**.
The file then shows under **Documents**; click its title to download it. On an archived contact, or one you may not edit, **Add document** is turned off.
Also called: upload a file to a contact, add an attachment to a contact, store a CV.

### Which email did a file come from?
To see which email a file came from in Margince, open the contact's or the company's **Documents** tab. A file that arrived with a captured email shows that email under its name: "From {who} · {subject}". Click the line to open the email.
- The line shows only when you may read the email. A file from an email kept to a smaller audience is not listed for anyone outside that audience.
- A calendar invitation's file, such as `invite.ics`, is not listed on any **Documents** tab or in a deal room. Small images in a signature are not listed either. Both stay on the email in **History**.
- An uploaded file has no such line.
Also called: email attachment sender, where a document came from.

### How do I upload a PDF to a deal?
To upload a PDF to a deal in Margince, open the deal, choose its **Documents** tab and click **Add document** in the **Files** panel.
1. Open **Deals** in the sidebar and open the deal.
2. Choose the **Documents** tab, then **Add document**.
3. Choose a **Category** such as **Proposal/Offer** or **SOW/Contract**, and a **Title** if you like.
   Drop the PDF on **File**, and click **Upload**.
A file on a deal can be read for deal fields, and can be added to the deal's [Deal Room](deal-rooms.md).
Also called: attach a proposal to a deal, add a file to an opportunity.

### How do I attach a file to a meeting or call?
To attach a file to a meeting, call or note in Margince, add it while you log the activity.
1. In the **Log activity** form, drop the files on **Attach files**, or click it to choose them.
2. Choose **Log**. The files are uploaded once the activity is logged.
An empty file, a file over the limit, or a kind Margince does not keep is not added.
A line under **Attach files** names it and says why, before you log.
If the server still refuses a file, a message names it and says why. The others are still attached.

If an upload does not finish, Margince checks the activity's files first, so no message asks you to add a file that is already there. To add a file later, press **Files** on the activity in the record's History, then **Add document**. The activity panel that opens from search or the worklist lists the same files under **Files**. The size limit and the file kinds are the same as for any document. Whoever may read the activity may open its files.
Also called: attach a document to a meeting, add a file to a call, meeting attachments.

### What is the maximum file size for an upload?
The largest document you can upload in Margince is **25 MB by default**. Whoever runs your installation can change it to anything between 1 MB and 100 MB.
The upload form always shows the limit that applies ("Up to {size}."). A larger file is refused: "The file exceeds {size}, the limit on this installation. Choose a smaller file."

A document in a knowledge document set has a lower limit, **5 MB by default**, which can also be set between 1 MB and 100 MB.
Also called: file size limit, upload limit, how big can a file be.

### Which kinds of file can I upload?
Margince takes these kinds of file as a document on a record, or as a file on an email you write:
- PDF, Word, Excel, slide, OpenDocument and RTF files: `.pdf`, `.doc`, `.docx`, `.xls`, `.xlsx`, `.ppt`, `.pptx`, `.odt`, `.ods`, `.odp`, `.rtf`
- Text files: `.txt`, `.csv`, `.md`, `.html`, `.htm`
- Images: `.png`, `.jpg`, `.jpeg`, `.gif`, `.webp`, `.heic`, `.heif`, `.tif`, `.tiff`
- Archives and saved emails: `.zip`, `.eml`, `.msg`
Any other kind is refused, and the message names the kinds Margince takes. An `.svg` image and a program such as an `.exe` are refused. The file's content must also match its kind, so a program renamed to `.pdf` is refused too.
Files that arrive with a captured email are kept even when they are of another kind.
A document set takes fewer kinds; see [Which files can a document set take?](#which-files-can-a-document-set-take).
Also called: supported file types, file formats, can I upload an archive.

## Documents

A document in Margince is filed against **a company**, **a contact**, or
**one deal**. When you upload from a company, you choose which under a
field the app calls **About**.

That choice matters, and the app says why right on the form:

> Documents on a deal can be read for deal fields; documents on the company
> cannot.

So for the AI to read an offer and take the amount and close date out of it,
file it against the deal.

### Categories

Every document carries a category:

- **SOW/Contract**
- **Proposal/Offer**
- **NDA/Legal**
- **Email attachment**
- **Message attachment**
- **Other**

### States

- **Draft**
- **Current**
- **Final**
- **Superseded**

Superseded documents are hidden by default, so the list shows what is live. You
can show them with **Show superseded** to read the history; they are not
deleted. If everything left in a list is superseded, the app tells you: "Only superseded documents remain.
Show them to see the history."

### When an upload fails, in part or in full

A document may upload while its category and title fail to save. Then Margince
tells you: "Uploaded, but not filed. The file is stored and listed below, but its category and title were not
saved, so it is filed under Other."

If the whole upload fails: "Upload failed. Nothing was stored."

### Picking a deal to file against

The deal search on the upload form covers only some of the company's newest
deals. It says so: "Search covers this company’s {deals} newest deals and
shows the first {matches} matches. Older deals cannot be selected here."

## Having a file read for deal fields

### How do I have a document read for deal fields?
To have a document read for deal fields in Margince, open the company's **Documents** tab. Click **Show extracted fields** on a document filed on a deal, then click **Read this file**.
1. The reading comes back staged, not written: "AI read this file: {count} fields with evidence, staged for review. Accept to save."
2. Correct a value with **Edit** if needed.
3. Click **Accept {count} fields** to write them to the deal, or **Dismiss** to write nothing.
The button appears only on documents filed on a deal.
Also called: extract fields from a PDF, read the offer into the deal.

Four fields can come back from a reading: **Deal name**, **Amount**,
**Currency** and **Expected close date**.

- **Accept** writes those fields to the deal. It keeps the original parts of
  the text, so you can see later what each value was read from.
- **Edit** lets you correct a value before accepting it.
- **Dismiss** writes nothing. "Nothing was saved. The file stays attached."

The panel tells these outcomes apart:

- **Nobody has read this file yet.** "This file has not been read for deal
  fields yet."
- **It was read and states no deal fields.** "AI read this file and
  found none of the deal fields."
- **It could not be read at all.** "This file could not be read."

A field the file names but not clearly enough is left out and labelled
"omitted (stated, but not clearly enough to accept)". A field the file does not
name at all is labelled "omitted (not stated in this file)". Margince does
not guess a missing value.

## A chat on a contact, lead or deal

### How do I save a chat from WhatsApp on a contact, lead or deal?
To save a chat from a messaging app in Margince, click **Capture a chat**. It is on a contact's **Documents** tab, on a lead's **History** tab, and in a deal's **Files** panel.
1. Take a screenshot of the chat and paste it, drop it on **Screenshot**, or click to choose it.
2. AI reads the messages into **What was said**, marked "Copied from the screenshot by AI". Check the text and change it if needed.
3. Pick the **Chat app** and click **File on this contact**, **File on this lead** or **File on this deal**.
Nothing is saved on the record's history until you click the file button. **Discard** removes the screenshot again.
Also called: save a WhatsApp chat, log a Telegram message, add a chat screenshot.

The chat is saved as one message on the record, with the screenshot
attached to it. When the chat's header shows a name or number that is
not this contact's or lead's, the panel says so. Tick the box to save it here all the same. A deal has many contacts, so a chat on a deal gets no such warning. When AI cannot read the screenshot, or no AI model is set up,
the panel says why and **What was said** stays empty. You can type the text
yourself or save the screenshot alone.

A message whose text you left as AI read it is recorded as written by AI for
you. A text you typed or changed is recorded as yours.

### Does a filed chat change my open task?
Yes, when an AI model is set up. After you file a chat, AI reads it against your open tasks on the same record. It changes a task only when the chat clearly settles it:
- The chat shows the task's work happened, for example "I booked a slot for Thursday". The task is marked done.
- The chat shows the task waits, for example "ping me again next week". The task moves to that day.

Every other task stays as it was. AI reads only tasks that are yours and that you may change. It skips tasks Margince made, and a task you opened again after Margince closed it.
The change shows in **Done for you** with the chat line it rests on, and **Undo** puts the task back as it was. AI reads a chat once, when you first file it, so it never changes that task again.

The receipt quotes the chat only while you may read it. A deleted, archived or private chat leaves the receipt without its words.

The screenshot and the text are part of the record's data. Deleting a
contact's data on request removes both.

## Files on a deal

The **Files** panel on a deal's **Documents** tab holds more than a company's
documents. It lists what you uploaded on the deal, and what arrived with its
emails and messages: "No files on this deal yet. Upload a file, or link an email
with an attachment."

So an attachment on a captured email shows up here on its own. Its label says
where it came from: "Attachment of a message from {who}, {when}". When the
sender is unknown, it says "an unknown sender".

### How do I remove a file from a deal?
To remove a file from a deal in Margince, open the deal's **Documents** tab. Open the file's **⋯** menu on its tile or row in the **Files** panel, and choose **Hide from this deal** or **Delete**. The panel's **Grid** and **List** switch shows files as tiles or as rows, and your device keeps the choice.
- **Hide from this deal**: the deal stops listing the file at once. The message and its attachment stay on the activity and in the company library. The toast "Hidden from this deal" offers **Undo**. Later, bring it back with **Show hidden files** and **Show on this deal again**.
- **Delete**: asks first, and names the file as its tile does. "The file is removed from this deal, and buyers no longer see it in any Deal Room. The room entry stays until you remove it from the room."
Use hide when the file does not matter here; use delete when it should not be on the deal at all.
Also called: delete an attachment, remove a document.

### How do I edit or delete a document I uploaded?
To change or remove an uploaded document in Margince, open the **⋯** menu on its tile or row in **Documents**. This works on a contact, a company and a deal. Choose **Edit** or **Delete**. The menu is missing when you may not edit the record. It is also missing on a file that came with an email or chat message, because that file belongs to its message.
- **Edit**: opens a small dialog for **Title**, **Category** and **State** (**Draft**, **Current**, **Final** or **Superseded**). Only the fields you change are saved. On a company's library, a **Superseded** document leaves the list until you choose **Show superseded**. On a contact or a deal it stays in the list and is marked as superseded.
- **Delete**: asks first and names the file. The file leaves every Documents list that showed it. A saved link to it then opens a page that says "This file was deleted or you no longer have access."
Also called: rename a document, change a document's category, mark a document superseded, remove a wrong upload.

## Document sets — asking your documents questions

A **document set** in Margince is a body of text the company files, so that
anyone here can ask it questions in plain words. It is apart from record
documents, with its own purpose: "Document collections this company can query.
Answers use only filed documents, and questions they do not cover are refused."

You create document sets at **Settings → Knowledge**, and you ask them from
**Ask your documents**.

### How do I create a knowledge base I can ask questions?
To create a knowledge base in Margince, open **Settings → Knowledge** and click **New document set** in the **Document sets** head.
1. Open the account menu at the top right, choose **Settings**, then **Knowledge**.
2. Click **New document set**, and fill **Name** and **What this set covers** (both required).
   Write one real sentence: it is shown to anyone whose question the set does not cover.
3. Click **Create**.
4. Click the new set's row to open its documents, and drop one or more files on **Add document**.
   Then click **Add document** (or **Add {count} documents**).
Also called: knowledge base, FAQ, handbook, document library, corpus.

### Which files can a document set take?
A document set in Margince takes **plain text, Markdown, CSV or JSON** only: "PDF and Word files are not supported and are refused."
Each refused file is named: `{filename}: {message}`, for example "contract.docx: Word files are not supported. Save it as plain text, Markdown, CSV or JSON first."
The size limit for one document in a set is **5 MB by default**; see [What is the maximum file size for an upload?](#what-is-the-maximum-file-size-for-an-upload).

An empty file, or one over the limit, is named as soon as you drop it and is not added.
Once you click **Add document**, every file it sent leaves the drop zone; the ones refused are named below it.
Also called: can I upload a PDF to the knowledge base, supported formats.

### How do I ask my documents a question?
To ask your documents a question in Margince, open the command palette and choose **Ask your documents**.
1. Press ⌘K (Mac) or Ctrl+K, or click **Search or ask Margince** in the top bar.
2. Choose **Ask your documents**, the last row.
3. Pick the **Document set**, type **Your question** (at most 1,000 characters), and click **Ask**.

"Answers come only from one document set, questions the set does not cover are refused, and every sentence cites its passage."
Also called: search the knowledge base, ask the handbook, FAQ.

### Why the refusal helps

A document set answers only from what is filed in it. A question it does not
cover is refused rather than guessed at.

Ask something the set does not cover, and you get **"Not covered by this
set"**. It reads "{name} was searched in full and has nothing close enough to answer
this. It covers:" and then what the set says it covers.

Every sentence in an answer carries the passage it rests on. An answer written
from the passages says "Written by Margince from your documents". When only the
passages are shown, it says "Source passages only. No answer was written." So
you always know whether an AI model was used.

### When asking cannot answer

- **This set is still being read**: "{embedded} of {total} passages are
  searchable. Retry shortly. The question is not the problem."
- **Documents cannot be searched yet**: "No embedding model is chosen for search, so nothing was
  searched." With the right to change AI settings you get **Choose an embedding model**, which opens
  **AI models**, **Model tiers**. Without that right, it says an administrator can choose one there.
- **No document sets**: "This company has no documents yet, so there is
  nothing to search."
- **You cannot open this company’s documents**: "You do not have access to this
  document set. An administrator can grant access."

### Documents in a set

Each document in a set moves through a visible state as it is taken in:

- **Queued**
- **Importing…**
- **Could not be read**, with the reason under it

A document that was read shows no badge.

The set's row shows how many documents it holds and one badge for whether it
can be searched: **Searchable**, **Indexing**, **Not searchable** or
**Reindexing**. Open the set to see the figures: "{embedded} of {total}
passages searchable". When some passages cannot be searched, the open set says
why:

- **Not searchable**: "No embedding model is set up, so no passage can be
  searched."
- **Indexing**: "Passages are still being indexed."

It may be read again after a change
to how text is indexed. Then it says "Reindexing this set", and that no data was
lost. A question asked while that runs gets the answer that the set is not ready,
and is not answered from part of the set.

### Changing or removing a document set

**Edit set**, in the set's **…** (**Actions**) menu, changes its **Name**, **Description** and
**What this set covers**. Click **Save**. A set that ships with Margince can be edited too.

**Archive set**, in the same menu: "Archive {name}? The set and its documents are no longer
searchable. Nothing is deleted, and Show archived sets brings it back."

To bring an archived set back, turn on **Show archived sets** in the **Document sets** head. The
archived set shows the **Archived** badge. Choose **Restore set** in its **…** menu. Its documents
are searchable again. A set that was the one **Ask your documents** opens on stays an ordinary set.

**Delete**, in a document's **…** (**Actions**) menu: "Delete {name}? The file, its extracted
text and its search index are permanently deleted."

### Who can see which sets exist
Not everyone can see which document sets exist. If they are not yours to see,
the Knowledge page does not show an empty page. It says "You do not have access
to the list of document sets."

## Other files the product takes

Margince takes files in two more places, with their own limits:

- **A CSV file to import** (leads, companies or contacts), and **vCard
  files**: 10 MB by default.
- **Your own LinkedIn `Connections.csv`**: 8 MB by default.

All four limits (attachments, documents in a document set, CSV import, LinkedIn
import) are set per installation, each between 1 MB and 100 MB.
