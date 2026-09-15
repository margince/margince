# Deal Rooms

A **Deal Room** is a page you share with a buyer: the documents for one deal, the
conversation about them, and nothing else. The buyer needs no account and sees no
part of your CRM.

You open one from the deal. A buyer enters through a personal emailed link.

## What a room holds

A **title** and a **welcome message** — the buyer's first paragraph on opening
it. A **steward**, which is the named human a buyer is pointed at for help, and
which starts as the deal's owner.

Then **documents**, drawn from the deal's own Files area and grouped as
Commercial, Legal, Security & Privacy, and Delivery & Operations; and **threads**
the two sides talk in.

## The five live states

**Live**, **Paused**, **Closed**, **Expired**, **Archived**.

(You may see four more names — draft, building, ready, publishing — carried for
old records. Nothing produces them any more.)

There is no publish step. A room is live from the moment you open it, and an edit
to its title or welcome reaches the buyer on their next load.

**Pause** — "Buyers keep their links but see a paused page until you resume."
Every credential stays valid; only reading stops.

**Close** — "Buyers keep reading; nothing can be added or said."

Read that carefully, because it is the unusual one. Close is **not** a freeze. A
buyer keeps reading the room and downloading its documents; neither side can add
a document or say anything further. And because they read the *live* room, a file
you later archive on the deal stops being served here too.

**Expired** is not a state anybody sets. It is worked out on every read from the
access end date, so a room stops serving the moment that date passes rather than
when a sweep gets round to it.

You can still **revoke somebody** from a closed room. Being unable to remove
someone from a room holding your signed contract, months after the deal closed,
is a real hazard.

### The access end date

**Set an end date** — "Access stops on that day." Leave it empty for no end date.
Setting one in the past takes effect immediately, which is how you cut access
short without ending the room.

One thing to know: the date is read in **UTC**, not your own timezone. If you are
west of UTC you lose part of that last day.

## Letting a buyer in

You invite one address at a time. Each buyer gets a **personal, one-time link.
It works once, on one device**, and every buyer needs an invitation of their own.

Two levels: **Read only** — "Can read the documents and the conversation." — and
**Read and comment** — "Can also ask questions and reply."

**Whether the link is emailed depends on your installation.** Where a mail relay
is configured, Margince tries to send it; where one is not, you copy the link and
send it yourself, and the screen says so: "The link was not mailed. Copy it and
send it yourself." Treat copying by hand as a normal path, not a failure.

If a buyer loses their link they can ask for a new one from the room's door. That
request always answers the same way, and always reaches you — but a new link is
only mailed where the installation can send mail. Otherwise you will see "Asked
for a new link {when}. Issue one and send it yourself."

### What a buyer sees, and what they never see

They see the room's title and welcome, its documents, its threads, the steward's
name, and **their own** participant record.

They never see the deal, the company, the amount, any other participant, or
anything else in your CRM. The room's own row is the whole of what is read.

Their session belongs to one buyer in one room and is re-resolved on every
request, so revoking somebody binds on their next click.

Every refusal at that door — an unknown link, a used one, an expired one, a
revoked one — answers **identically**, so the page cannot be used to work out
which guesses were closer.

### Revoking

"Their session ends now and their link stops working. Their comments stay visible
and attributed. Access cannot be restored by them asking for a link."

## View as buyer

**View as buyer** opens the real buyer page, through the real buyer door, as a
read-only preview: "This is what a buyer would see. You can read everything and
change nothing."

**A preview is never counted as engagement.** Recording it would make the Access
panel report the buyer opening documents that you opened.

## Who has been in

The deal's Deal Room card shows "{invited} invited · {active} signed in" and
**Last seen by a buyer**.

Per seat, the Access panel shows invited / signed in / revoked, when they were
last seen, and how many documents they downloaded.

## What happens to a room later

**Closing, winning or losing the deal does nothing to the room.** It keeps
serving until you pause, close or expire it.

**Archiving the deal** freezes the one move that grants access — you cannot
resume a paused room on an archived deal — while pause and close, which take
access away, still work.

**Erasing a contact** wipes their seat and revokes it, deletes their sessions and
what they downloaded, but **leaves their comments standing**, attributed to
"Erased Subject". The conversation is a record of what was said; the identity is
what erasure reaches.

A contact's own page lists "Rooms this contact can still enter", which is how you
find where a departed contact still has a door.

## What an agent may do

An agent may **open** a room, list rooms, read one, read its participants and
documents, and post a thread or comment on your side.

**An agent may not** edit a room, pause, resume, close or archive one, set its
expiry, preview it, add or remove a document, resolve a thread, or touch a
participant in any way — invite, correct, resend or revoke. The rule is stated once in the product: deciding which outsider reads
a deal's material is not a judgement an agent makes.

Nor may an agent touch the buyer-facing door at all.

## Two honest limits today

**There is no way to archive a room from the app.** The action exists in the
product, but nothing on any screen calls it. That matters because a deal holds
**one** room at a time, and opening a second needs the first archived — so in
practice a deal gets one Deal Room, and the refusal that tells you to archive the
old one names something you cannot do.

**A buyer's first click on a dead link always shows the dead-link page.** There is
no check before the link is spent, so a link that was already used reads the same
as one that never worked.
