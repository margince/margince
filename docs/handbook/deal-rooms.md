<!-- prose:plain -->
<!-- describes: frontend/src/features/deals/dealroompage.tsx frontend/src/features/deals/dealroomaccess.tsx frontend/src/features/deals/dealroomdocuments.tsx frontend/src/features/deals/buyerroom.tsx frontend/src/components/dealroom.tsx backend/internal/modules/dealrooms -->
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
4. Enter the **Room title**: "The heading the buyer sees. You can change it later." It starts as the deal's name.
5. Click **Open**. The room is live at once; there is no step to publish it.
A deal holds one room at a time. Without the Create permission on Deal Rooms the button is not shown.
Also called: buyer portal, data room, customer portal, microsite.

### How do I share a deal room with a buyer?
To share a Deal Room with a buyer in Margince, open the deal's **Deal Room** tab and click **Invite** in the **Who may enter** panel.
1. In **Invite to Deal Room**, fill **Name** and **Email** (both required).
2. Under **Permissions**, choose **Read-only** ("Can read the documents and comments.") or **Read and comment** ("Can also ask questions and reply.").
3. Click **Invite**. The **Invitation link** is shown; click **Copy link** and send it to the buyer.
Each buyer needs an invitation of their own: the link is personal and works once, on one device.
Also called: invite a customer, give the client access, send the room link.

### Is the deal room link emailed to the buyer?
Whether Margince emails a Deal Room link depends on your installation: "The link is shown for you to copy. If a mail relay is configured, it is also emailed, but delivery is not guaranteed."
When nothing was sent, the screen says "The link was not mailed. Copy it and send it yourself." Copying the link and sending it yourself is the normal way.
Also called: the buyer did not get the invitation email.

### How do I add documents to a deal room?
To add a document to a Deal Room in Margince, open the deal's **Deal Room** tab and use the form at the foot of **Documents**.
1. Under **File from this deal**, choose **Pick a file**. Any file in the deal's Files area can be added, including uploads and email attachments.
2. Choose a **Group**: Commercial, Legal, Security and privacy, or Delivery and operations.
3. Click **Add to room**.
If the file is not on the deal yet, click **Upload a file** first. Margince files it on the deal, and it appears in the picker. "Documents and comments appear to buyers immediately."
Also called: share a file with the buyer, upload a PDF to the room.

### Why does a document in my deal room say "Buyers do not see this"?
A document in a deal room says **Buyers do not see this** when its file is no longer in the deal's Files area. The file was deleted, hidden, or its email was unlinked. The tile loses its **Read** button, and threads about the file lose their reply button. To clear the badge, remove the file from the room.
Also called: missing room file, deleted document in a room, buyer cannot open a file, stale document.

### How do I pause, close or set an end date on a deal room?
To pause or close a Deal Room in Margince, open the deal's **Deal Room** tab and click **Manage room**.
1. Under **Room access**, choose **Live**, **Paused** or **Closed**. Choosing **Closed** shows "Close this Deal Room?" in the dialog; nothing changes until you save.
   - **Live**: "Buyers can read, download and comment."
   - **Paused**: "Buyers keep their links but see a paused page until you resume."
   - **Closed**: "Buyers can still read. Nothing new can be added."
2. Set **Access ends on**: "Leave empty for no end date. The day is read in UTC."
3. Click **Save changes**.
A paused room also shows **Resume** in the notice on the tab. A closed, expired or archived room cannot be changed here: "This room has ended. Its shared content is kept as a record."
Without permission to change Deal Rooms, **Manage room** is not shown.
Also called: stop sharing, lock the room, expire the room.

### How do I change the room's title, welcome or contact?
To change what a buyer reads first in Margince, open the deal's **Deal Room** tab and click **Manage room**. **Your contact** is beside **Access ends on**; **Room title** and **Welcome message** are under **Title and welcome**.
- **Room title** is the heading the buyer sees.
- **Welcome message** is "The first paragraph a buyer reads."
- **Your contact** is "The colleague a buyer turns to."
Click **Save changes**. "Changes reach buyers the next time they open the room."
Also called: rename the room, edit the welcome text, change the buyer's contact.

### What does the deal room tab show?
The **Deal Room** tab leads with the room's title, its state, its welcome and who is in it. Under that:
- **Get the room ready** lists four steps: write a welcome, invite a buyer, add a document and start a thread. Margince ticks each one off from the room itself, and the list goes away once all four are done.
- **Room brief** says in a few sentences what the buyers did and what is waiting on you. Each sentence names what it was written from. The line under it says **Written by Margince** when the AI wrote it. It says **Compiled from CRM records** when no AI was set up or it did not answer.
- **Documents** stand beside the **Conversation**, which holds every thread and names the document each one is about.
- **Who may enter** lists each buyer as **Invited**, **Signed in**, **Link requested** or **Revoked**, with the actions for their link.

**Get the room ready** is not shown on a closed, expired or archived room, or without permission to change it. **Room brief** is not shown when there is nothing to report, or when it could not be read.
Also called: room overview, deal room summary.

### How do I remove a buyer from a deal room?
To remove a buyer from a Deal Room in Margince, open the deal's **Deal Room** tab. In the **Who may enter** panel, open the buyer's row actions and choose **Revoke access**.
"Their session ends and their link stops working. Their comments stay visible and attributed. Requesting a new link does not restore access."

The same menu has **Issue new link** (their current link stops working) and **Change permissions**.
Once the room is closed, expired or archived, the tab hides this menu. Click **Manage room**, then **Open the room’s page**, to reach it there.
Also called: revoke access, kick out, remove a participant.

### How do I see which deal rooms a contact is in?
To see the Deal Rooms a contact can still enter in Margince, open the contact and choose the **Deals** tab. The **Deal Rooms** panel lists each room with its state.
- **Open** goes to the room's own page.
- **Revoke access** ends that contact's seat in the room. It is the same revoke that the room's **Who may enter** panel does.
The panel is not shown when the contact holds no seat, or when your role cannot read Deal Rooms.
Also called: which rooms has this buyer been invited to, remove someone who left the customer from every room.

### How do I see what the buyer sees?
To see a Deal Room as the buyer does in Margince, open the deal's **Deal Room** tab and click **View as buyer**.
It opens the real buyer page in a new tab as a read-only preview: "This is the buyer’s view. You can read everything but change nothing."
A preview is never counted as the buyer's engagement.
Also called: preview the room, buyer view.

## What a Deal Room holds

A Deal Room holds a **Room title** and a **Welcome message**, edited under
**Title and welcome**. The welcome is the buyer's first paragraph on
opening the room. It names a **steward**: the colleague a buyer turns to for
help, who starts as the deal's owner.

Then it holds **documents**, taken from the deal's own Files area and grouped as
Commercial, Legal, Security and privacy, and Delivery and operations. It also
holds **threads** the two sides talk in.

## Room states

A Deal Room is **Live**, **Paused**, **Closed**, **Expired** or **Archived**.

There is no step to publish a room. A room is live from the moment you open it.
An edit to its title or welcome reaches the buyer the next time they open
the page.

**Pause** keeps every buyer's link valid; only reading stops. The buyer sees
"Access is paused" and reads that their link is still valid.

**Close**: "Buyers can still read the room. No document, comment or decision is
accepted afterward." You can still revoke buyers and issue links, but only from
the room's own page, reached with **Manage room** and **Open the room’s page**.
The deal's tab hides those actions once the room is closed.

Note that closing does not freeze the room. A buyer keeps
reading the room and downloading its documents; neither side can add a document
or say anything more. They read the *live* room, so a file you later archive
on the deal stops being there too.

Nobody sets **Expired**. A room expires the moment its access end date passes.

You can still **revoke somebody** from a closed room, on its page. So you can
remove someone from a room that holds your signed contract, months after the
deal closed.

### The access end date

A Deal Room's end date is set in **Access ends on**, under **Manage room**:
access stops on that day. Setting one in the past takes effect at once. That is how you cut access
short without ending the room.

One thing to know: the date is read in **UTC**, not in your own time zone. If
you are west of UTC, you lose part of that last day.

## Letting a buyer in

Buyers are invited to a Deal Room one address at a time. Each buyer gets a
**personal link** that works once, on one device, and every buyer
needs an invitation of their own.

A buyer who loses their link can ask for a new one from the room's sign-in
page, with **Request new link**. The answer is always the same: "If that
address was invited, a new link is on its way." The request always reaches you.
But a new link is only mailed where the installation can send mail. If not,
the Access panel shows "Asked for a new link {when}. Issue one and send it
yourself."

### What a buyer sees, and what they never see
A buyer in a Deal Room sees the room's title and welcome, its documents and its
threads. They see the steward's name ("Your contact: {steward}.") and **their
own** participant record.

A buyer never sees the deal, the company, the amount, any other participant, or
anything else in your CRM.

Revoking somebody takes effect on their next click.

Every link that no longer works shows the same page, whether it was unknown,
used, expired or revoked: "This link no longer works".

## View as buyer

**View as buyer** opens the real buyer page as a read-only preview. The preview
also opens for a manager, an admin and anyone with write access on the deal.
For anyone else it says "Your access to this deal does not include the buyer
preview."

**A preview never counts as engagement**, so the Access panel never reports the
buyer opening documents that you opened.

## Who has been in

The deal's Deal Room tab shows "{invited} invited · {active} signed in" and
"Last seen by a buyer: {when}".

Per seat, the Access panel shows invited / signed in / revoked, when they were
last seen, and "Documents downloaded: {count}".

## What happens to a room later

Closing, winning or losing the deal **does not change its Deal Room**. The room
keeps working until you pause, close or expire it.

**Archiving the deal** blocks the one action that gives access back: you cannot
resume a paused room on an archived deal. Pause and close, which take access
away, still work.

**Erasing a contact** removes and revokes their seat. It also deletes their
sign-ins and the record of what they downloaded. Their comments **stay**, under
the name "Erased Subject": erasure removes who said it, not what was said.

## What an agent may do in a Deal Room

An agent may **open** a Deal Room, list rooms, read one, read its participants
and documents, and add a thread or comment on your side.

**An agent may not** edit a room, or pause, resume, close or archive one. It
may not set the end date, preview a room, add or remove a document, or resolve
a thread. It may not touch a participant in any way (invite, correct, send again
or revoke). Only a human decides which outsiders read the papers of a deal.

An agent also has no access to the buyer's side of the room.

## Limits of Deal Rooms

**No archive button.** You cannot archive a Deal Room from the app. A deal holds
one active room at a time, and opening a second needs the first archived. So in
the app each deal gets only one Deal Room. The message that tells you to
archive the old room names something the app does not offer.

**Every dead link shows the same page.** A link that was already used
looks the same as one that never worked.
