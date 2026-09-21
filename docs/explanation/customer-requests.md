# Customer requests outlive pipeline stages

A captured request and a sales opportunity have different lifecycles. Winning
or losing a deal ends advice about advancing its pipeline; it does not answer a
customer or complete a task.

## Recognition, responsibility, and completion

The owed verdict (`asks_us` or `informs_us`) judges intent. The capture label
(`meeting`, `commitment`, or `noise`) describes the message's topic. A scheduling
request is actionable even when its topic is `meeting`. A conflicting `noise`
label leaves a confirmed request reviewable instead of automatically assigning it.

The activities module owns the request state. Its shared predicates feed the
email summary, waiting queue, classification backlog, automatic reminders, and
the deal's record-scoped request read. The latter runs separately from recent
history, so a request remains reachable after newer messages fill the timeline.
The deal's existing open-task read continues to supply the same tasks as the
worklist, including on closed deals.

A later email proves a reply was sent, not that a request was fulfilled. The
existence of an outbound message settles nothing: "thanks, I will check" is a
reply and discharges nothing, so acknowledgements, unrelated replies and newer
inbound messages all leave a confirmed request standing.

What a reply does earn is a READING. Once the workspace has written back on the
thread, the request-settlement pass hands the conversation — the request and
everything after it, each message marked as from them or from us — to a model
and asks what our own words did: `settled`, `still_owed`, or `unsure` below the
confidence floor. A request nobody has answered is never judged, and an
installation with no model configured for the task behaves exactly as it did
before: a request stays owed until somebody ticks its reminder.

A `settled` verdict completes the evidence-linked reminder through the ordinary
activity writer, so it carries the audit row and the event a human ticking the
box carries. `still_owed` may sharpen a machine-filed, undated reminder to name
what is actually outstanding — "Send the quote" in place of the mail's subject
line — and never touches a reminder a human accepted, dated or reopened. An open
reminder always outranks the machine's verdict: somebody picking the work back
up is the answer, and the request counts as outstanding again while their task
stands. `unsure` records that the pass asked and would not commit, which leaves
the request owed and stops the same conversation being re-read until somebody
writes on it again.

Completing the evidence-linked reminder settles its source request, whoever
completes it. Dismissing a conversation as not sales removes it from request
review; a reader's snooze or not-mine decision changes that reader's queue, not
the obligation for everybody.

Unclassified mail with a captured thread uses reply evidence as a fallback.
Unthreaded mail needs a request verdict or scheduling/commitment evidence. An
unclassified message already labelled as scheduling or commitment remains a
candidate even after a reply, so historical reconciliation can still judge it.
An unrelated conversation cannot settle it, and unthreaded messages are never
matched to every other unthreaded message.

## Automatic capture and historical review

Each owed-verdict pass reconciles already-confirmed requests before and after
classification. It creates at most 64 reminders per transaction, through the
normal activity writer, preserving record links and source evidence. Source row
locks and the activity's natural key make retries and concurrent reviews idempotent.

Automatic assignment is limited to requests from the last fourteen days with
exactly one eligible importing seat directly addressed in the captured envelope.
Mailbox delivery alone does not prove responsibility. The task is personal,
undated, and does not invent a deadline.

Older requests, conflicting labels, and ambiguous recipients remain reviewable.
Age affects ranking, not whether a recognized obligation exists. Old scheduling
and commitment candidates remain eligible for the classifier, which drains its
bounded backlog across passes. This reconciles missed work without creating a
task for every historical message.

The deal offers **Create task** for a request awaiting acceptance. Both task and
activity creation accept `request_activity_id`: the server checks readable source
evidence, copies its links, assigns the reminder, and records acceptance. A human
accepting an unclassified request records that intent in the audited task, without
rewriting the source message's classifier verdict. Acceptance is for the caller;
reassignment remains the existing task operation. Completing that reminder
is an explicit statement that the request is settled.

Automatic passes never resurrect archived or completed reminders. Explicit
acceptance can restore the original archived, unfinished reminder when the caller
may update it; it preserves its identity and owner. A completed reminder stays
completed. Restoration records an audit entry and an `activity.updated` event.

## Visibility and freshness

Source content is gated on every review and acceptance, including replay. A
private reminder cannot be retrieved through a more widely readable source. The
source may say a reminder already covers it, without disclosing that reminder's
identity, owner or content; another reader is offered the source, not a task
creation button that cannot succeed.
The derived task's content remains subject to its source's visibility.

The deal's status read refreshes on the record's one-minute cadence. The web read explicitly requests `facts_only`, so it cannot call a model even
when facts change. It serves valid cached prose or a current deterministic card,
updating the shared cache so worklist actions agree with the record. “Write it again” remains the explicit prose
refresh. Empty-state wording reports what this
view found rather than claiming that the reader has no work anywhere.

Request identity is the source message, not its subject or thread. Two distinct
asks in one conversation remain two obligations; a later ask cannot silently
settle an earlier one. The queue preserves those source messages rather than
guessing from matching subjects that the work is identical.
