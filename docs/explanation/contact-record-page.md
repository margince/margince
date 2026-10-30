<!-- prose:plain -->
<!-- describes: backend/internal/compose/contact360 frontend/src/features/contacts/contactpage.tsx frontend/src/features/contacts/contactoverview.tsx frontend/src/features/contacts/contacttoday.tsx frontend/src/features/contacts/contactconsentpanel.tsx frontend/src/features/contacts/contactstanding.ts frontend/src/components/latestinbound.tsx backend/internal/modules/activities/latestinbound.go frontend/src/features/contacts/contactcaseentry.tsx -->
# The contact record page

The Overview tab puts recorded context and real work first. A new contact must not look like an old relationship that is now silent.

## Reading order

The newest thing the contact sent in the last 7 days that nobody has answered is "Just in". The server reads it (`GET /activities/latest-inbound`, `backend/internal/modules/activities/latestinbound.go`). Mail and messages come from the `waiting_reply` read. A website form counts until a later sent mail or message, a call or a held meeting answers it. An item a task stands for is left out.

`RecordNextBar` (`frontend/src/components/latestinbound.tsx`) puts the item in the Next bar on every record type. It takes the bar when nothing urgent is next: a late item, a moment or a move the page marks urgent. Otherwise it is one line above the bar. A bar whose move already cites the item draws alone.

**Create task** accepts a mail's request when the server says it can (`accepts_request`), so finishing the task settles it. Anything else becomes a task made from the item. "Noticed" hides it in this browser, and no older item takes its place. On the other tabs it is one line under the badges.

After it, a moment that needs action, an open task or a promise still owed comes first. Otherwise the relationship brief comes first. When no brief exists, a plain line that states the recorded identity takes its place. Source and the date the record was created show beside the owner in the header. A failed brief has its own retry; a failure does not turn into an empty profile.

For a contact an agent created, the source names the tool and the colleague it worked for, each when the create audit row records it. `captured_by` names only the passport, so `created_by` reads them from that row.

The `thin_relationship` rule needs an activity timeline that loaded and is empty, and no recorded colleague connections. Its label is `No interactions recorded`. It supplies no made-up evidence that the relationship changed. The `thin_relationship` result shows as a short line that states what the record covers, not as a next step. They cover the records the reader can see, not everything anyone knows about the contact.

An empty block, whether for reading, for promises or for a conversation, does not show on the Overview tab. Real tasks still show when a moment is dismissed, and they open the shared task window. The link to the full work queue says that it opens the whole queue. What the system knows about past conversations shows once; the History tab holds the full story in time order. Apart from that one line, the sidebar does not show the newest activity a second time.

A relationship is marked inbound only, outbound only, or both ways. A pulse, colleague list or signal list that loaded but is empty does not show; a limit set by role is still stated. Missing times do not prove that a relationship runs only one way.

Editing identity, choosing the default address and resolving where the contact works stay with the parts that already do them.

While the contact is in an open duplicate case, the record header shows a **Possible duplicates** button with a count (`contactcaseentry.tsx`). The count is the number of other contacts of the case that the reader can see. The button opens the same review the Worklist case row opens, so both places merge through one decision. It does not show when the contact is in no case, or when the case is merged or gone.

## Permission to write

The sidebar answers permission per purpose and reach per channel, through the existing consent guard. A reader who decides whether to write needs both answers, and they come from different records. An answer that is still loading, one that failed, and one that loaded but is unknown each look different. Each message still passes its own send check, which decides.

The guard sends each reason twice. `reason` is an English sentence for an agent. `reason_code` is what the panel says in the reader's language (`consentreason.ts`), with `reason_at` for a dated objection. A seeded purpose is named from the catalog too; an operator's own purpose keeps its label. A code the client does not know yet shows the server's sentence.

`Manage consent & proof history` opens the existing consent ledger. A recorded grant and send permission are different things: other recorded legal reasons can change what the guard says. Recording or taking back consent loads the ledger, the guard, the contact read and the message review again. When only the subject confirmed, it stays recorded as subject-only.

`Ask them to confirm their details` lives in that sidebar panel. It uses the existing endpoint and the recorded recipient. It does not show for a contact the caller cannot write, and it is off when there is no email address. A request in the queue is neither a delivery nor a grant. The existing states for an error, for no mail and for a request past its time limit stay visible.

On a small screen, `Details & permissions` opens the same sidebar content over the page. On desktop it is the normal sidebar panel, which keeps whether it was open. Who can see the record, the privacy of mail, and permission to send stay separate controls.

## Focus and the work queue

Focus chooses and orders entries the normal way, with no pin controls and no stored pins that change it. The separate full work queue keeps its pins as they work today. The backend chooses Focus before it applies personal pins to the queue. So old stored pins cannot pull a note that asks for no action into Focus, or change its order.

The Next bar stays about this contact. With no move, no task and nothing just in, it says whose turn it is. It reads that from `last_exchange` in `GET /activities/latest-inbound`, the contact's newest call, meeting or message. `activities.WhoseMove` decides it, the same function the company page's strip uses, so the two cannot disagree.

A move whose action opens the composer drafts the message on the press and is drawn in the AI indigo. `momentDrafts` in `contactpage.drawers.tsx` decides both, so the bar, the **Meetings** tab and the press cannot disagree. A move that only opens a form or a record keeps the accent.
