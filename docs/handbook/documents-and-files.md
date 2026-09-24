# Documents and files

Margince holds files in three different places, for three different reasons.
Knowing which is which saves a lot of confusion.

1. **Documents** on a company, a contact or a deal — the papers of the
   relationship.
2. **Files** on a deal — everything the deal has picked up, including
   attachments that arrived with email.
3. **Document sets** in Settings → Knowledge — bodies of text the whole
   company can ask questions of.

### How do I upload a document?
To upload a document in Margince, open the company, contact or deal, choose its **Documents** tab and click **Add document**.
1. Open the record and choose the **Documents** tab.
2. Click **Add document**.
3. On a company, choose **About**: **This company** or **A deal** (then pick the deal under **Search company deals**).
4. Choose a **Category**; it starts as Other. **Title** is optional and defaults to the filename.
5. Drop the file on **File** ("Drop a file here, or click to choose one") and click **Upload**.
Without write access to the record the button is not shown. A signed contract document is uploaded the same way, with **Category** set to **Contract**; the contract record itself is added separately, see [Contracts and invoices](contracts-and-invoices.md).
Also called: attach a file, add an attachment, add a PDF, upload a contract document.

### How do I attach a file to a contact?
To attach or upload a file to a contact in Margince, open the contact, choose its **Documents** tab and click **Add document**.
1. Open **Contacts** in the sidebar and open the contact.
2. Choose the **Documents** tab, then **Add document**.
3. Pick a **Category** (**Contract**, **Offer**, **Legal** or **Other**); **Title** is optional.
4. Drop the file on **File**, or click to choose one, and click **Upload**.
The file then lists under **Documents**; click its title to download it. On an archived contact, or one you may not edit, **Add document** is disabled.
Also called: upload a file to a contact, add an attachment to a contact, store a CV.

### How do I upload a PDF to a deal?
To upload a PDF to a deal in Margince, open the deal, choose its **Documents** tab and click **Add document** in the **Files** panel.
1. Open **Deals** in the sidebar and open the deal.
2. Choose the **Documents** tab, then **Add document**.
3. Choose a **Category** such as **Offer** or **Contract**, optionally a **Title**, drop the PDF on **File**, and click **Upload**.
A file filed on a deal can be read for deal fields and can be added to the deal's [Deal Room](deal-rooms.md).
Also called: attach a proposal to a deal, add a file to an opportunity.

### What is the maximum file size for an upload?
The maximum size of a document uploaded in Margince is **25 MB by default**. An operator running the installation can change it to anything between 1 MB and 100 MB, and the upload form always shows the number that installation accepts ("Up to {size}."). A larger file is refused: "The file exceeds {size}, the limit on this installation. Choose a smaller file."
A document in a knowledge document set has a lower limit, **5 MB by default**.
Also called: file size limit, upload limit, how big can a file be.

## Documents

A document in Margince is filed against **a company**, **a contact**, or
**a specific deal**. When you upload from a company you choose which under a
field the app calls **About**.

That choice matters, and the app says why right on the form:

> Documents on a deal can be read for deal fields; documents on the company
> cannot.

So if you want the AI to be able to read an offer and pull the amount and close
date out of it, file it against the deal.

### Categories

Every document carries a category:

- **Contract**
- **Offer**
- **Legal**
- **Email attachment**
- **Message attachment**
- **Other**

### States

- **Draft**
- **Current**
- **Final**
- **Superseded**

Superseded documents are hidden by default so the list shows what is live. You
can show them with **Show superseded** to read the history — they are not
deleted. If everything left in a list is superseded, the app tells you rather
than showing what looks like an empty list: "Only superseded documents remain.
Show them to see the history."

### When an upload half-works or fails

If a document is uploaded but its category and title fail to save, Margince
tells you exactly that instead of pretending it worked: "Uploaded, but not
filed. The file is stored and listed below, but its category and title were not
saved, so it is filed under Other."

If the whole upload fails: "Upload failed. Nothing was stored." Nothing
half-happened.

### Picking a deal to file against

The deal search on the upload form covers a bounded number of the company's
newest deals. It says so, with the actual numbers, rather than silently
omitting old deals: "Search covers this company’s {deals} newest deals and
shows the first {matches} matches. Older deals cannot be selected here."

## Having a file read for deal fields

### How do I have a document read for deal fields?
To have a document read for deal fields in Margince, open the company's **Documents** tab, click **Show extracted fields** on a document filed on a deal, then click **Read this file**.
1. The reading comes back staged, not written: "AI read this file: {count} fields with evidence, staged for review. Accept to save."
2. Correct a value with **Edit** if needed.
3. Click **Accept {count} fields** to write them to the deal, or **Dismiss** to write nothing.
The button appears only on documents filed on a deal.
Also called: extract fields from a PDF, read the offer into the deal.

Four fields can come back from a reading: **Deal name**, **Amount**,
**Currency** and **Expected close date**.

- **Accept** writes those fields to the deal, and keeps the original snippets so
  you can see later what each value was read from.
- **Edit** lets you correct a value before accepting it.
- **Dismiss** writes nothing. "Nothing was saved. The file stays attached."

Three outcomes are kept carefully apart, because they are three different
answers:

- **Nobody has read this file yet.** "This file has not been read for deal
  fields yet."
- **It was read and it states none of the deal fields.** "AI read this file and
  found none of the deal fields."
- **It could not be read at all.** "This file could not be read."

A field the file mentions but not clearly enough is left out and labelled
"omitted (stated, but not clearly enough to accept)". A field the file does not
mention at all is labelled "omitted (not stated in this file)". The system does
not fill a gap with a guess.

## Files on a deal

The **Files** panel on a deal's **Documents** tab is broader than a company's
documents. It lists what you uploaded on the deal, and what arrived with its
emails and messages: "No files on this deal yet. Upload a file, or link an email
with an attachment."

So an attachment on a captured email shows up here automatically, labelled with
where it came from: "Attachment of a message from {who}, {when}". When the
sender is not known, it says "an unknown sender" rather than leaving it blank.

### How do I remove a file from a deal?
To remove a file from a deal in Margince, open the deal's **Documents** tab, open the file's row actions in the **Files** panel, and choose **Hide from this deal** or **Delete**.
- **Hide from this deal** — "The message and its attachment stay on the activity and in the company library. Only this deal stops listing it." Bring it back with **Show hidden files** and **Show on this deal again**.
- **Delete** — "The file is removed from this deal, and from any Deal Room sharing it."
Use hide when the file is simply not relevant here; use delete when it should not be on the deal at all.
Also called: delete an attachment, remove a document.

## Document sets — asking your documents questions

A **document set** in Margince is a body of text the company files so that
anyone here can ask it questions in plain language. It is a separate feature
from record documents, with a separate purpose: "Document collections this
company can query. Answers use only filed documents, and questions they do not
cover are refused."

You create document sets at **Settings → Knowledge**, and you ask them from
**Ask your documents**.

### How do I create a knowledge base I can ask questions?
To create a knowledge base in Margince, open **Settings → Knowledge** and fill the **New document set** form under **Document sets**.
1. Open the account menu at the top right, choose **Settings**, then **Knowledge**.
2. Under **New document set**, fill **Name** and **What this set covers** (both required). Write one real sentence: it is shown to anyone whose question the set does not cover.
3. Click **Create set**.
4. On the new set, click **Show documents**, drop one or more files on **Add document**, and click **Add document** (or **Add {count} documents**).
Also called: knowledge base, FAQ, handbook, document library, corpus.

### Which files can a document set take?
A document set in Margince accepts **plain text, Markdown, CSV or JSON** only: "PDF and Word files are not supported and are refused." Margince will not accept a file it cannot read and then quietly hold nothing; each refused file is named, "{filename}: {message}". To use a PDF or Word document, save it as plain text or Markdown first.
The size limit for one document in a set is **5 MB by default**.
Also called: can I upload a PDF to the knowledge base, supported formats.

### How do I ask my documents a question?
To ask your documents a question in Margince, open the command palette and choose **Ask your documents**.
1. Press ⌘K (Mac) or Ctrl+K, or click **Search or ask Margince** in the top bar.
2. Choose **Ask your documents**, the first row.
3. Pick the **Document set**, type **Your question**, and click **Ask**.
"Answers come only from one document set, questions the set does not cover are refused, and every sentence cites its passage."
Also called: search the knowledge base, ask the handbook, FAQ.

### What a document set holds
**The size limit for one document in a set is 5 MB by default.** That is lower
than the attachment limit on purpose: a corpus document is plain text by
construction, and 5 MB of plain text is roughly a million words, which is well
past any handbook. An operator can change it within the same 1–100 MB range.

There is no reader for PDFs or Word files in a document set; the app refuses
one rather than filing it empty.

### The refusal that makes it useful

A document set answers only from what is filed in it, and a question it does
not cover is refused rather than guessed at.

When you ask something the set does not cover, you get **"Not covered by this
set"** — "{name} was searched in full and has nothing close enough to answer
this. It covers:" followed by the set's own description. Not a
plausible-sounding paragraph assembled from nothing. This is the single most
important property of the feature.

Every sentence in an answer carries the passage it rests on. The app also
distinguishes an answer that was written from the passages ("Written by
Margince from your documents") from one that is just the passages themselves
("Source passages only. No answer was written."), so you always know whether a
model was involved.

### When asking cannot answer

- **This set is still being read** — "{embedded} of {total} passages are
  searchable. Retry shortly. The question is not the problem."
- **No search index is configured** — "Nothing was searched. This installation
  has no search index configured."
- **No document sets** — "This company has no documents yet, so there is
  nothing to search."
- **You cannot open this company’s documents** — "You do not have access to this
  document set. An administrator can grant access."

### Documents in a set

Each document in a set moves through a visible state as it is taken in:

- **Queued**
- **Indexing…**
- **Searchable**
- **Could not be read** — with "Why this file could not be read"

The set shows its own coverage: "{documents} documents · {embedded} of {total}
passages searchable". If it is being re-read after a change to how text is
indexed, it says so — "Reindexing this set" — and says no data was lost; asking
it will report that it is not ready rather than answering from half an index.

### Removing things from a document set

**Archive set** — "Archive this document set? The set and its documents are no
longer searchable. Nothing is deleted."

**Delete** a document — "Delete this document? The file, its extracted text and
its search index are permanently deleted."

Two clearly different acts, described as two clearly different acts.

### Who can see which sets exist
Not everyone can see which document sets exist. If they are not yours to see,
the Knowledge page says exactly that — "You do not have access to the list of
document sets." — rather than showing you an empty page that reads like "there
are none".

## Other files the product takes

Margince takes files on two more upload routes, with their own limits:

- **A CSV file to import** (prospects, companies or contacts), and **vCard
  files** — 10 MB by default.
- **Your own LinkedIn `Connections.csv`** — 8 MB by default.

All four limits (attachments, document-set documents, CSV import, LinkedIn
import) are set per installation and must each sit between 1 MB and 100 MB.
Above that range the answer is a different design, not a bigger number.
