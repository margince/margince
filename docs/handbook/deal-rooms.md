# Deal Rooms

A **Deal Room** in Margince is a page you share with a buyer: the documents for
one deal, the conversation about them, and nothing else. "A page where the buyer
opens shared documents by link and discusses them." The buyer needs no account
and sees no part of your CRM.

You open a Deal Room from the deal's **Deal Room** tab. A buyer enters through a
personal link that you send them.

### How do I create a deal room?
To create a Deal Room in Margince, open the deal, choose its **Deal Room** tab and click **Open a Deal Room**.
1. Open **Deals** in the sidebar and open the deal.
2. Choose the **Deal Room** tab.
3. Click **Open a Deal Room**.
4. Enter the **Room title** — "The heading the buyer sees. You can change it later." It defaults to the deal's name.
5. Click **Open**. The room is live at once; there is no publish step.
A deal holds one room at a time. Without the Create permission on Deal Rooms the button is not shown.
Also called: buyer portal, data room, customer portal, microsite.

### How do I share a deal room with a buyer?
To share a Deal Room with a buyer in Margince, open the deal's **Deal Room** tab and click **Invite** in the **Access** panel.
1. In **Invite to Deal Room**, fill **Name** and **Email** (both required).
2. Under **Permissions**, choose **Read-only** ("Can read the documents and comments.") or **Read and comment** ("Can also ask questions and reply.").
3. Click **Invite**. The **Invitation link** is shown; click **Copy link** and send it to the buyer.
Each buyer needs an invitation of their own: the link is personal and works once, on one device.
Also called: invite a customer, give the client access, send the room link.

### Is the deal room link emailed to the buyer?
Whether Margince emails a Deal Room link depends on your installation: "The link is shown for you to copy. If a mail relay is configured, it is also emailed, but delivery is not guaranteed." When nothing was sent, the screen says "The link was not mailed. Copy it and send it yourself." Treat copying the link by hand as a normal path, not a failure.
Also called: the buyer did not get the invitation email.

### How do I add documents to a deal room?
To add a document to a Deal Room in Margince, open the deal's **Deal Room** tab and use the form under **Documents**.
1. Under **File from this deal**, choose **Pick a file**. Any file in the deal's Files area can be added, including uploads and email attachments.
2. Choose a **Group**: Commercial, Legal, Security and privacy, or Delivery and operations.
3. Click **Add to room**.
If the file is not on the deal yet, click **Upload a file** first; it is filed on the deal and appears in the picker. "Documents and comments appear to buyers immediately."
Also called: share a file with the buyer, upload a PDF to the room.

### How do I pause, close or set an end date on a deal room?
To pause or close a Deal Room in Margince, open the room's own page and use the **Room access** menu: **Pause**, **Resume**, **Close room** or **Set end date**. They are not on the deal's **Deal Room** tab: the room page is the deal's address with `/room` added (`#/deals/<deal id>/room`), and no button links to it today.
- **Pause** — "Buyers keep their links but see a paused page until you resume."
- **Close room** — "Buyers can still read. Nothing new can be added."
- **Set end date** — "Access stops on that day." Leave **Access ends on** empty for no end date.
Also called: stop sharing, lock the room, expire the room.

### How do I remove a buyer from a deal room?
To remove a buyer from a Deal Room in Margince, open the deal's **Deal Room** tab and, in the **Access** panel, open the buyer's row actions and choose **Revoke access**.
"Their session ends and their link stops working. Their comments stay visible and attributed. Requesting a new link does not restore access."
The same menu has **Issue new link** (their current link stops working) and **Change permissions**. Once the room is closed or expired the tab hides this menu; it stays on the room's own page (`#/deals/<deal id>/room`).
Also called: revoke access, kick out, remove a participant.

### How do I see what the buyer sees?
To see a Deal Room as the buyer does in Margince, open the deal's **Deal Room** tab and click **View as buyer**. It opens the real buyer page in a new tab as a read-only preview: "This is the buyer’s view. You can read everything but change nothing."
A preview is never counted as the buyer's engagement.
Also called: preview the room, buyer view.

## What a Deal Room holds

A Deal Room holds a **Room title** and a **Welcome message** — the buyer's first
paragraph on opening it — edited under **Title and welcome**. A **steward**,
which is the named human a buyer is pointed at for help, and which starts as the
deal's owner.

Then **documents**, drawn from the deal's own Files area and grouped as
Commercial, Legal, Security and privacy, and Delivery and operations; and
**threads** the two sides talk in.

## The five live states

A Deal Room is **Live**, **Paused**, **Closed**, **Expired** or **Archived**.

(You may see four more names — Draft, Building, Ready, Publishing — carried for
old records. Nothing produces them any more.)

There is no publish step. A room is live from the moment you open it, and an edit
to its title or welcome reaches the buyer on their next load.

**Pause** keeps every credential valid; only reading stops. The buyer sees
"Access is paused" and is told their link remains valid.

**Close** — "Buyers can still read the room. No document, comment or decision is
accepted afterward." You can still revoke buyers and issue links, but only from
the room's own page: the deal's tab hides those actions once the room is closed.

Read that carefully, because it is the unusual one. Closing a Deal Room is
**not** a freeze. A buyer keeps reading the room and downloading its documents;
neither side can add a document or say anything further. And because they read
the *live* room, a file you later archive on the deal stops being served here
too.

**Expired** is not a state anybody sets. It is worked out on every read from the
access end date, so a room stops serving the moment that date passes rather than
when a sweep gets round to it.

You can still **revoke somebody** from a closed room, on its page. Being unable to remove
someone from a room holding your signed contract, months after the deal closed,
is a real hazard.

### The access end date

A Deal Room's end date is set with **Set end date** — "Access stops on that
day." Setting one in the past takes effect immediately, which is how you cut
access short without ending the room.

One thing to know: the date is read in **UTC**, not your own timezone. If you are
west of UTC you lose part of that last day.

## Letting a buyer in

Buyers are invited to a Deal Room one address at a time. Each buyer gets a
**personal, one-time link that works once, on one device**, and every buyer
needs an invitation of their own.

If a buyer loses their link they can ask for a new one from the room's door with
**Request new link**. That request always answers the same way — "If that
address was invited, a new link is on its way." — and always reaches you, but a
new link is only mailed where the installation can send mail. Otherwise the
Access panel shows "Asked for a new link {when}. Issue one and send it
yourself."

### What a buyer sees, and what they never see
A buyer in a Deal Room sees the room's title and welcome, its documents, its
threads, the steward's name ("Your contact: {steward}."), and **their own**
participant record.

A buyer never sees the deal, the company, the amount, any other participant, or
anything else in your CRM. The room's own row is the whole of what is read.

Their session belongs to one buyer in one room and is re-resolved on every
request, so revoking somebody binds on their next click.

Every refusal at that door — an unknown link, a used one, an expired one, a
revoked one — answers **identically** ("This link no longer works"), so the
page cannot be used to work out which guesses were closer.

## View as buyer

**View as buyer** opens the real buyer page, through the real buyer door, as a
read-only preview. The preview also opens for a manager, an admin and anyone
holding write access on the deal; otherwise it says "Your access to this deal
does not include the buyer preview."

**A Deal Room preview is never counted as engagement.** Recording it would make
the Access panel report the buyer opening documents that you opened.

## Who has been in

The deal's Deal Room tab shows "{invited} invited · {active} signed in" and
"Last seen by a buyer: {when}".

Per seat, the Access panel shows invited / signed in / revoked, when they were
last seen, and "Documents downloaded: {count}".

## What happens to a room later

**Closing, winning or losing the deal does nothing to its Deal Room.** It keeps
serving until you pause, close or expire it.

**Archiving the deal** freezes the one move that grants access — you cannot
resume a paused room on an archived deal — while pause and close, which take
access away, still work.

**Erasing a contact** wipes their seat and revokes it, deletes their sessions and
what they downloaded, but **leaves their comments standing**, attributed to
"Erased Subject". The conversation is a record of what was said; the identity is
what erasure reaches.

## What an agent may do in a Deal Room

An agent may **open** a Deal Room, list rooms, read one, read its participants
and documents, and post a thread or comment on your side.

**An agent may not** edit a room, pause, resume, close or archive one, set its
expiry, preview it, add or remove a document, resolve a thread, or touch a
participant in any way — invite, correct, resend or revoke. The rule is stated
once in the product: deciding which outsider reads a deal's material is not a
judgement an agent makes.

Nor may an agent touch the buyer-facing door at all.

## Honest limits of Deal Rooms today

**There is no way to archive a Deal Room from the app.** The action exists in
the product, but nothing on any screen calls it. That matters because a deal
holds **one** room at a time, and opening a second needs the first archived — so
in practice a deal gets one Deal Room, and the refusal that tells you to archive
the old one names something you cannot do.

**The room's own page, which holds Pause, Resume, Close room and Set end date,
has no link in the app.** You reach it by adding `/room` to the deal's address.
A panel listing the rooms a contact can still enter exists in the code but is
not shown on the contact page.

**A buyer's first click on a dead link always shows the dead-link page.** There
is no check before the link is spent, so a link that was already used reads the
same as one that never worked.
