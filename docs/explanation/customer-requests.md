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

## Whether a reply is still owed

One check answers this for every surface: the needs-reply badge on the
timeline, the contact page and the agent tools, the waiting lane, request
review and the response time. It lives in `activities/answered.go`, and
`backend/gates/answerwalk_test.go` fails a second walk of a thread for our
reply.

A message that is not a request is answered by any of:

- our reply on the same thread;
- our mail to the sender with the same subject once reply prefixes (`Re:`,
  `AW:`, `Antw:`) are stripped. The sender is their address or any live
  address of their contact, named as the outbound's recipient or on a To/Cc
  row; a blind copy does not count, and a forward (`Fwd:`, `WG:`) is not an
  answer. The provider must have filed the mail as sent by us, so a message
  whose From merely names our mailbox proves nothing;
- a logged call or a held meeting with the sender's contact. The same with a
  colleague of theirs does not count.

An answer must be strictly later than the mail: a reply in the same second
stays owed, because mail carries second precision and nothing else says which
came first. Evidence off the thread counts only when the whole workspace may
read it. A colleague's private reply or meeting must not clear another seat's
row, since the row going quiet would disclose that it exists.

The subject match is used only here. Capture never joins threads on a subject,
because two "Re: Invoice" mails from two senders are two conversations; the
address keeps them apart.

Such a message is owed while it is the newest inbound on its thread and
unanswered, judged or not. A confirmed request stays owed through any of these
answers until it is settled as described above; an unclassified message
labelled as scheduling or commitment is kept the same way until the classifier
reads it.

Two judgements end the obligation, and `/worklist/hidden` counts and lists what
each one hides: a human marking the conversation not sales, and the classifier's
`informs_us` verdict. A request a human accepted is never hidden by the verdict.
Mail from an obvious machine address is owed only as a confirmed request.

The waiting lane is the owed set narrowed by queue rules: the horizon, the
sales link, colleagues' domains and the reader's own snoozes and not-mine
choices. Each of those has its own figure in `/worklist/hidden` too, so a
badge that says a reply is owed and a lane without the row always differ for
a counted reason.

## Automatic capture and historical review

Each owed-verdict pass reconciles already-confirmed requests before and after
classification. It creates at most 64 reminders per transaction, through the
normal activity writer, preserving record links and source evidence. Source row
locks and the activity's natural key make retries and concurrent reviews idempotent.

Automatic assignment is limited to requests from the last fourteen days with
exactly one eligible importing seat directly addressed in the captured envelope.
Mailbox delivery alone does not prove responsibility. The task is personal,
undated, and does not invent a deadline.

Conflicting labels and ambiguous recipients remain reviewable inside the horizon.
**Past the waiting horizon a request leaves the Worklist**, whatever the
classifier said and whether or not an open deal is linked: the queue holds
current work, and old mail stays on the record's timeline. The one exception is
a request a human holds, meaning an open reminder a human wrote, by taking the
request or by editing the reminder the system filed. Each pass also archives the
reminders it filed itself once their request passes the horizon untouched;
archiving rather than completing, because an aged request is not an answered
one. A request that aged out within the last year is counted under the
past-horizon figure in `/worklist/hidden`, which counts conversations rather
than single requests; older ones are not counted. Its record still offers
**Create task** to take it back.

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
