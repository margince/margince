<!-- prose:plain -->
# Customer requests last longer than pipeline stages

A captured request and a sales deal have different lives. When a deal closes, Margince stops telling
the user how to move it down the pipeline. That does not answer the customer or complete a task.

## Finding a request, who owns it, and when it is complete

The owed verdict (`asks_us` or `informs_us`) decides what the sender asks for. The capture label
(`meeting`, `commitment`, or `noise`) says what the message is about. A request to set up a meeting
still needs action, even when its label is `meeting`. When a `noise` label disagrees, a confirmed
request stays open for review and is not handed to anyone on its own.

The activities module owns the request state. Its shared checks serve the email brief, the waiting
queue and the queue of mail still to classify. They also serve the reminders it files on its own, and
the deal's read of its own requests. That last read runs apart from the newest history, so a request can
still be reached after newer messages fill the timeline. The deal's open task read still supplies the
same tasks as the Worklist, closed deals too.

A later email proves a reply was sent; it does not prove the request was handled. An outbound message by
itself settles nothing. `"Thanks, I will check"` is a reply and closes nothing. So short notes back,
replies about something else and newer inbound messages all leave a confirmed request standing.

A reply does start a model reading of the thread. Once the workspace has written back, the settle pass
hands the conversation to a model. It sends the request and everything after it, each message marked as
from them or from us. It asks what our own reply did: `settled`, `still_owed`, or `unsure` below the
confidence floor. A request nobody has answered is never judged. With no model set up for the task, a
request stays owed until someone completes its reminder.

A `settled` verdict completes the reminder linked to the evidence, through the ordinary activity writer.
So it carries the audit row and the event that a human who completes it by hand would carry. `still_owed` may
change the title of a reminder with no date that the machine filed. The new title names what is still
open ("Send the offer" in place of the mail's subject line). It never touches a reminder a human accepted, dated or opened again.

An open reminder always wins over the machine's verdict. Someone who takes the work back up is the
answer, and the request counts as open again while their task stands. `unsure` records that the pass
asked and would not commit. That leaves the request owed and stops the pass from reading the same
conversation again until someone writes on it again.

Any user who completes the reminder linked to the evidence settles its source request. Marking a conversation as not sales removes it from request review. When a reader chooses
snooze or `not_mine`, that changes their own queue, not what is owed for every user.

## Whether a reply is still owed

One check answers this for every surface. The surfaces are the needs-reply badge on the timeline, the
contact page and the agent tools, the waiting lane, request review and the response time. It lives in
`activities/answered.go`, and `backend/gates/answerwalk_test.go` fails a second walk of a thread for our
reply.

A message that is not a request is answered by any of:

- our reply on the same thread;
- our mail to the sender with the same subject, once reply tags (`Re:`, `AW:`, `Antw:`) are removed.
  The sender is their address or any live address of their contact. That address must be named as the
  recipient of the outbound mail or on a To/Cc row. A Bcc copy does not count, and a mail sent on
  (`Fwd:`, `WG:`) is not an answer. The provider must have filed the mail as sent by us, so a message
  whose From line only names our mailbox proves nothing;
- a logged call, or a meeting that did happen, with the contact of the sender. The same with a colleague
  of theirs does not count.

An answer must be later than the mail, not the same moment: a reply in the same second stays owed.
Mail carries time only to the second, and nothing else says which was first. Evidence off the thread
counts only when the whole workspace may read it. A colleague's private reply or meeting must not clear
another seat's row, since a row going silent would show that the reply or meeting exists.

The subject match is used only here. Capture never links threads by subject, because two `Re: Invoice`
mails from two senders are two conversations; the address keeps them apart.

Such a message is owed while it is the newest inbound message on its thread and has no answer, judged
or not. A confirmed request stays owed through any of these answers until it is settled, as the
sections above say. A message that is not yet classified but is labelled as scheduling or as
`commitment` is kept the same way until the classifier reads it.

Two decisions end what is owed: a human marking the conversation not sales, and the `informs_us`
verdict of the classifier. `/worklist/hidden` counts and lists what each one takes out of the queue. The
verdict never takes out a request a human accepted. Mail from an obvious machine address is owed only
as a confirmed request.

The waiting lane is the owed set, made smaller by queue rules. The rules are the horizon, the sales
link, mail from colleagues' own company, and the snooze and `not_mine` choices of that reader. Each of
those has its own number in `/worklist/hidden` too. So when a badge says a reply is owed and a lane does
not show the row, there is always a counted reason.

## Capture on its own, and review of old mail

Each owed-verdict pass files reminders for requests that are already confirmed, before and after it
classifies. It creates at most 64 reminders per transaction, through the normal activity writer, and
keeps record links and source evidence. Locks on the source rows and a key on the activity make retries
and reviews that run at the same time idempotent.

The pass gives a request to a seat on its own only when the request is from the last 14 days. It also
needs one importing seat that may take it and that the captured mail headers addressed directly. A mail
that reached a mailbox does not by itself prove who owns the task. The task is personal, has no date, and
does not make up one.

A request whose labels disagree, or where it is unknown which seat is the recipient, stays open for
review inside the horizon. Past the waiting horizon a request leaves the Worklist, whatever the
classifier said and whether or not an open deal is linked. The queue holds current work, and old mail
stays on the record's timeline.

The one exception is a request a human holds. That is an open reminder a human wrote, either by taking
the request or by editing the reminder the system filed. Each pass also archives the reminders it filed
itself once their request passes the horizon with no human edit. It archives rather than completes,
because an old request is not an answered one. A request that passed the horizon within the last year
is counted under the past the horizon number in `/worklist/hidden`.

That number counts conversations rather than single requests, and older ones are not counted. The record
of such a request still offers **Create task** to take it back.

The deal offers **Create task** for a request that waits to be accepted. Both task and activity create
calls accept `request_activity_id`. The server checks that the caller may read the source evidence, and
copies its links. It gives the reminder to the caller and records that it was accepted. When a human
accepts a request that is not yet classified, the audited task records that choice. It does not change
the classifier verdict on the source message.

Accepting is for the caller; giving the task to someone else stays the existing task operation.
Completing that reminder states that the request is settled.

Passes that run on their own never open archived or completed reminders again. Accepting can open the original
archived reminder again, if it is not complete and the caller may update it. It keeps the reminder's
identity and owner. A completed reminder stays completed. Opening one again records an audit entry and
an `activity.updated` event.

## Who may see it, and when it updates

Source content is gated on every review and every accept, replay included. A private reminder cannot be
reached through a source that more users may read. The source may say a reminder already covers it,
without showing that reminder's identity, owner or content. Another reader is offered the source, not a
create task button that cannot work. The content of the task made from it stays subject to who may see
its source.

The deal's status read updates on the record's schedule, once every 60 seconds. The web read asks for
`facts_only`, so it cannot call a model even when facts change. It serves stored text that is still
good, or current text built from fixed rules. It updates the shared cache so Worklist actions agree with
the record. “Write it again” is the named way to update the text. Empty-state text reports what this
view shows; it does not claim the reader has no work at all.

A request's identity is the source message, not its subject or thread. Two different asks in one
conversation stay two things owed, and a later ask cannot settle an earlier one in silence. The queue
keeps those source messages rather than deciding from matching subjects that the work is the same.
