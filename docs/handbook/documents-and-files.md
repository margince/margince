# Documents and files

Margince holds files in three different places, for three different reasons.
Knowing which is which saves a lot of confusion.

1. **Documents** on a company or a deal — the papers of the relationship.
2. **Files** on a deal — everything the deal has picked up, including
   attachments that arrived with email.
3. **Document sets** in Settings → Knowledge — bodies of text the whole
   organization can ask questions of.

## Documents

A document is filed against **a company** or against **a specific deal**. You
choose which when you upload it, under a field the app calls **About**.

That choice matters, and the app says why right on the form:

> A document on a deal can be read for deal fields; one on the company cannot.

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
can show them to read the history — they are not deleted. If everything left in
a list is superseded, the app tells you rather than showing what looks like an
empty list: "Only superseded documents are left here. Show them to read the
history."

### Uploading

Give it a file and a category. The **Title** is optional — left blank, the row
shows the filename.

**The size limit is 25 MB by default.** An operator running the installation
can change it, anywhere between 1 MB and 100 MB, and the form always shows the
number that installation actually accepts rather than a number from a manual.

If a document is uploaded but its category and title fail to save, the app tells
you exactly that instead of pretending it worked: "Uploaded, but not filed. The
file is on the record and listed below. Only its category and title were not
saved, so it is filed under Other."

If the whole upload fails: "Nothing was stored." Nothing half-happened.

### Picking a deal to file against

The deal search on the upload form covers a bounded number of the account's
newest deals. It says so, with the actual numbers, rather than silently
omitting old deals: "The search covers this account's {deals} newest deals and
offers the first {matches} matches. A deal older than those cannot be picked
here."

## Having a file read for deal fields

Open a document filed on a deal and you can ask for it to be read. Four fields
can come back:

- Deal name
- Amount
- Currency
- Expected close date

What you get is **staged, not written**. The panel says so: "AI read this file —
{count} fields it can ground, staged for your record (accept to persist)."

Then:

- **Accept** writes those fields to the deal, and keeps the original snippets so
  you can see later what each value was read from.
- **Edit** lets you correct a value before accepting it.
- **Dismiss** writes nothing. "Nothing was written. The file stays attached."

Three outcomes are kept carefully apart, because they are three different
answers:

- **Nobody has read this file yet.** No claim either way.
- **It was read and it states none of the deal fields.** A real answer.
- **It could not be read at all.** A failure, said as one.

A field the file mentions but not clearly enough is left out and labelled
"omitted (this file says something, but not clearly enough to accept)". A field
the file does not mention at all is labelled "omitted (not stated in this
file)". The system does not fill a gap with a guess.

## Files on a deal

The **Files** tab on a deal is broader than Documents. It is "what you uploaded
on this deal, and what arrived with its emails and messages."

So an attachment on a captured email shows up here automatically, labelled with
where it came from: "Attachment of a message from {who}, {when}". When the
sender is not known, it says "an unknown sender" rather than leaving it blank.

Two different removals, and the difference is the whole point:

**Hide from this deal.** The file stops being listed on this deal. That is all.
"The message and its attachment stay on the activity and in the company
library. Only this deal stops listing it." You can un-hide it later.

**Delete.** "The file is removed from this deal, and from any Deal Room sharing
it."

Use hide when the file is simply not relevant here. Use delete when it should
not be on the deal at all.

## Document sets — asking your documents questions

This is a separate feature with a separate purpose. A **document set** is a
body of text the organization files so that anyone here can ask it questions in
plain language.

You find them at **Settings → Knowledge**, and you ask them at **Ask your
documents**.

### What a set holds

Plain text, Markdown, CSV or JSON.

**There is no reader for PDFs or Word files here.** The app is explicit that one
"would be refused rather than filed empty" — it will not accept a file it cannot
read and then quietly hold nothing.

**The size limit for one document in a set is 5 MB by default.** That is lower
than the attachment limit on purpose: a corpus document is plain text by
construction, and 5 MB of plain text is roughly a million words, which is well
past any handbook. An operator can change it within the same 1–100 MB range.

### Creating a set

A set has a name and a sentence describing **what this set covers**. Write a
real sentence, not a label. The app explains why: that sentence is quoted back
to whoever asks a question the set does not cover, "so it is read at their least
patient moment."

### The refusal that makes it useful

> An answer comes only from what is filed here, and a question they do not cover
> is refused rather than guessed at.

When you ask something the set does not cover, you get **"Not covered by this
set"** — not a plausible-sounding paragraph assembled from nothing. This is the
single most important property of the feature.

Every sentence in an answer carries the passage it rests on. The app also
distinguishes an answer that was written from the passages ("Written from the
passages below") from one that is just the passages themselves ("The passages
themselves — nobody wrote a summary"), so you always know whether a model was
involved.

### Documents in a set

Each document moves through a visible state as it is taken in:

- **Waiting to be read**
- **Being read**
- **Searchable**
- **Could not be read**

The set shows its own coverage: "{documents} documents · {embedded} of {total}
passages searchable". If it is being re-read after a change to how text is
indexed, it says so and says nothing has been lost — asking it will report that
it is not ready rather than answering from half an index.

### Removing things

**Archive a set.** "The set and everything filed in it stop being searchable.
Nothing is destroyed."

**Delete a document.** "The file, the text taken from it and the search index
built on it are destroyed. This cannot be undone."

Two clearly different acts, described as two clearly different acts.

### Who can see which sets exist

Not everyone. If document sets are not yours to see, the app says exactly that —
"Which document sets exist is not yours to see" — rather than showing you an
empty page that reads like "there are none".

## Other files the product takes

Two more upload routes exist, with their own limits:

- **A CSV of companies to import** — 10 MB by default.
- **Your own LinkedIn `Connections.csv`** — 8 MB by default.

All four limits (attachments, corpus documents, CSV import, LinkedIn import)
are set per installation and must each sit between 1 MB and 100 MB. Above that
range the answer is a different design, not a bigger number.
