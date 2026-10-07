<!-- prose:plain -->
# Home: morning and weekly priorities

The personal morning brief has four parts:

- **Focus**: up to 6 cards the server picks, with proposals a user can act on and promises due today. They come first on the page, the reading cards that back them up come after, and the headline names the same cards.
- **Updates**: notices that only tell. A pinned notice keeps the place its user gave it.
- **Context**: task evidence opens in the shared task dialog, and contact details open in a separate drawer, so the page stays in place.
- **Full queue**: opens when asked for in Home, and does its own filters and pages. Future tasks and normal privacy work stay there with their deadlines and actions.

The brief time comes from the stored run; the agenda time comes from the live queue.

The team morning is a board for a named team, with routes to the work and current plan of each
member. Morning and Weekly keep the same team pick in the URL. Who is on the team now, and the rights
of the reader, decide who is on the board. Work across the workspace that nobody holds does not count
for a named team.

## What leads the day

New customers who wait, and response deadlines that come near, keep the first place. A thread that
has waited more than 14 days is recovery work. If it links to an open deal, it keeps the priority of
a deal at real risk; other old threads become normal work.

A privacy duty enters the time to prepare 7 days before the real deadline; the legal deadline
itself does not change. Some duties are recorded more than a day after their own deadline, which is
what an imported history makes. Such a duty is old work to review, not a missed deadline today. It
ranks as normal work with the reason "recorded after its deadline", and never takes a Focus card. A
duty that turned due while it was on record stays urgent, no matter how old it is.

A sent notice
leaves the Worklist. It stays owed until it is delivered, and a failed delivery puts it back.

Tasks due today or past due are urgent duties, unless they are general lead search work. Deal
recovery enters the urgent band in two cases. One is when the deal value is high next to the priced
risk of all deals. The other is when its expected close is past due or within 14 days.

Work for current customers comes before normal lead search. Real response deadlines and urgent
duties still lead both. Within each band, deadlines, value and then the other server rules decide
the order. Clients keep that order.

A close date Margince set for now is a reason to confirm or change the forecast. It is not
evidence of a customer promise. Cards keep the for-now flag and the forecast group, even when there
is no forecast. The deal work reading reports the deal value it knows in the queue a user picked.
It says plainly that the number may not be complete. It includes new deals and also recovery, and
it is not a forecast that counts risk, or expected revenue.

Only a task written by the lead SLA escalation copies a dated first response row. Other tasks
linked to a lead stay separate tasks, with their own due date and actions. A lead with no response
target set still shows as lead search work. It shows company, status, the admin source label and
last activity where there is one, and no made-up deadline.

Margince decides which cards may go in Focus before the 6-card limit. The risk engine applies its
past-close rule before its limited scan, and says so when a source it scans is not complete.

## What the daily brief can claim

Dates, reply direction, deal standing and its reason stay on the row. Details hold the evidence
that backs it up, not rank times put side by side. A task never takes on advice made for its deal.
A pin changes the personal order, and does not change how the server ranks the work as urgent.

Meetings to come count calendar entries, and leave out past meetings that wait for an outcome.
Margince does not know whether a user is prepared unless the source says so. A failed source is
not a zero; Margince names each failed source, with a retry. Permission limits and limited scans do
not show a general error: counts keep their limits, and paging stays visible. Deal values leave
out deals with no price, and state how many those are.

An open weekly promise with a due date that a user picked enters Today on that date, in the
timezone of the installation. Done uses the plan writer that is already there, and marks the agenda
as old. Promises with no date, future promises and done promises stay out of the due lane. To create
a promise, a user can link a CRM record that is already there, through the shared record picker. A
team lead can read the current plan of a member. The lead can also answer requests for help through
the writer that is already there, which checks permissions.

## What a weekly review can claim

A team snapshot that covers zero reps reports that no measure is available. It hides the
performance cards and the agenda. A snapshot that covers only some reps says so before the numbers,
and gives no performance result for the whole team. Old snapshots never change. The time a user
picks sets both the headline and the detail.

Losses and recorded SDR activity count for the headline. A missing snapshot does not pass as a
quiet week. Deal outcomes and the agenda for the manager conversation come before the numbers,
forecasts and notes that back them up, which a user can open. Forecasts keep the time they cover and
the date they were made. Notes do not claim that one thing caused another.

Margince does not show recorded lead responses as a share of SLA passes. A missing breach mark does
not prove someone set a response target. Both weekly counts and the lead scorecard use the same
group of new leads and the same closing time. A response recorded after that time cannot change the
closed week.

New team snapshots include deal recovery in their agenda. It comes after requests for help, missed
responses and missed promises, and before work on meeting notes or good news. The evidence comes
from the deal scorecard of each member, as it was stored. That means forecasts moved down, stages
moved back, close dates that are wrong, or missing next steps. A missing scorecard gives no finding.

## Reviewing automatic changes

Changes made for you shows recorded work, its subject, its reason and the time it happened. A
close date fix names the old and new date when the date changed. A change in confidence alone is
not called a date change. Automatic stage moves use the guarded undo of the stage history. Date
fixes use the restore of the record history.

Accept records a lasting, audited review of that change, and keeps the values it applied. It does
not replay the change, or turn a forecast nobody confirmed into a customer promise. Nobody can
accept a change after an undo, or after a newer change replaced it. A general stage notice tells; it
is not evidence that an agent changed the deal. New notices keep the real stage names at the time.

## Focus and the work queue

Morning's Focus is an added `/worklist` view. The server picks it before the filter and page limit
of the queue, in the rank order that is already there. A quiet day has zero cards.

Pins are rows a
user picked by name, and they do not change how urgent a row is. Preparing for a meeting may go in
Focus within 24 hours when it is not done yet; prepared meetings stay in Schedule. Privacy
preparing keeps its 7-day window. Sales work with no price may still go in Focus.

`focus.total` counts the cards that may go in Focus, and a group counts once.
`focus.urgent_remaining` counts urgent work under the cards that the cards do not name one by one.
It counts the same way as `summary.urgent`, and it stays visible even with no other queue page.
Source limits, failed reads and reads Margince kept back keep their own meaning for how much was
read. Not the last page, and not a complete Focus view, proves that Margince read every source in
full.

Home is where daily work starts. Its work queue opens in a drawer on the right, with the scope,
owner, filters, paging, actions, coaching and reviews it already has. The queue has its own
`queue_scope` dial, so it cannot change the Morning/Weekly view behind it. Old `#/worklist` links
and owner/`unassigned` links redirect to the same Home queue state. The API contract and the domain
writers are still available.

Context opens on its own, over the page. When it closes, keyboard focus goes back to the
control that opened it. The queue keeps its own filter and scroll state.

## Who did what, and promises

Stage change notices:

- **Actor**: a notice carries the original event actor and the time it happened, through live and retry delivery. Margince still writes the delivery as the automation, and a key per recipient and event stops a second copy of a notice.
- **Own changes**: Margince skips stage changes the recipient made. It drops them before delivery, and before the page limit in the lists of notices not yet read. Deal history still records them. Margince never reads a change by an author it does not know as one the recipient made.
- **Other changes**: other human changes name the member. Machine changes stay visible even when they ran for the recipient.
- **Words**: a stage update uses the deal name once, then the recorded from and to stages and who changed them.
- **Old notices**: they get these facts back through a link from the event that created the notice to its cause, the stage change event. Recorded stage names come first. Without a snapshot, only a stage setup nobody has edited gives a name. Edited setups stay unknown, and Margince does not compare clocks. Missing history is named plainly, and Margince never puts the owner in the place of the author.

The user ID a task is assigned to decides who holds the task. Task text that starts with the name
of the reader, in a form Margince knows, reads as "You need to …". The stored promise does not
change. Details keep the original words and evidence. Tasks that look the same, from a separate
transcript or deadline, stay separate duties. Text that looks the same cannot prove that one task
replaces another.

One rule turns a promise read out of a meeting transcript or an email into work
(`compose/commitmentdispatch.go`):

- A promise the customer made goes on their contact as something to watch. It never becomes a task. The deal page lists the open ones for its account (`GET /deals/{id}/commitments`) with the quoted words. A reader who may update contacts gets a way to dismiss a wrong reading.
- A promise a named colleague made becomes their task. When the confidence of the reader in the promise is at or above `CommitmentTaskConfidence`, Margince writes the task at once, captured by the reader (`agent:…`). Below that, Margince makes it a proposal to them as a `commitment_task` card, and writes it when they accept.
- A promise nobody can be named for goes as a proposal to the member the reading belongs to, and to accept it makes it theirs.
- A promise in mail only one member may read stays with that member. If a colleague made it in that mail, it goes as a proposal to the owner, not as a task for the colleague. The task it turns into is visible to its holder alone.
- In mail, only what the sender wrote counts: words they quote from earlier in the thread do not count as theirs.
- A task has a due date only when the conversation named a day. A promise with no date sits in the queue for today, with request reminders that have no date.
- A promise and the task it turned into close together (`compose/commitmentsettle.go`, consumer group `cg:commitment-settle`). To mark the task done closes the promise as kept, and to close the promise as kept completes the task. To open the task again leaves the promise kept, and to dismiss the promise leaves the task as it is.
- The task is keyed on the evidence of the promise (source, side, party and the words it was said in), archived tasks included. Some things never come back when Margince reads the conversation again. They are a task a rep archived, a proposal a rep refused, and a customer promise a rep dismissed.

## Close dates that hold

The repair that runs each night replaces missing or past dates. It keeps a good future date, even one Margince
set for now in an earlier run. A quiet deal may lower forecast confidence without a change to the
date. A new date is today plus the middle number of days per stage on record, times the stages still
open. It is rounded up to whole weeks, with at least 7 days.

Without enough history it is 14 days
per stage. All generated dates stay for-now dates, and the opt-out settings, the record of past
undos and the review controls still apply.

## Date formats, language and links

Installation settings put the date and time format next to the base currency. Date options are the
default of the app language, `DD.MM.YYYY`, `MM/DD/YYYY` and `YYYY-MM-DD`. Time options are the
default of the app language, 24-hour and 12-hour. Shared format helpers apply the setting across the
whole signed-in app, close dates a user types included. They do not change stored values or the
timezone for reports and records. The browser still controls how its own date input works.

The one true address is `#/home` in every language. The visible name changes with the language
(Home, Startseite, Trang chủ). Older `#/brief` links keep their query string as they redirect to
Home. Worklist links still open the queue with their owner and filters.

## Personal work and email requests

Mine means the work the reader holds, for managers and other members both. Access to a record does
not assign its work. A user picks the team view by name. Team exceptions follow who is on the team
now, and backlog checks across the workspace show only under All. Manager rights do not grant access
to the private mail of colleagues. Reviews of captured contacts belong to the member who imported
them, including proposals that already wait.

Disclosure cards need a contact the user can open. They follow the assigned officer, and if there is
no officer, the contact owner. The compliance queue keeps its own, separate officer rights.

| Part | Personal scope | Team or all scope |
| --- | --- | --- |
| Focus and deal drill-down | Owned deals, assigned tasks and confirmed requests sent to the user | Pick the team scope or the all scope of the queue by name |
| Team board and exceptions | Not there | Team members and visible work nobody holds, under the grants already there |
| Overnight | Own imported mail and owned projects, checked again when read | Does not become a team view just because the reader is a manager of colleagues |
| Contact conversations | The same stored thread, in one group | Each original keeps its content gate |

Not answered does not mean a user must act. Focus needs an `asks_us` result and a `commitment`
capture label. Missing, conflicting, only-to-tell or only-a-meeting labels stay open to review, and
do not claim to be urgent, even on an open deal. A confirmed first request needs no earlier outbound
message to get a look. The label step reads the new words of the sender, not earlier requests they
quote.

The request run each hour creates one personal task with no date for a confirmed request nobody
answered, when the mail names one importing seat. It keeps the source message and record links,
follows the `set-aside` state of the recipient, and never makes up a deadline. Results already
stored can run with no model set up. Private and limited mail stays outside this automatic label
step. When it is not clear who should get the task, Margince leaves it for review and does not
guess.

The reminder replaces its source email in the queue of the assignee. To complete it closes the
request for every reader. To archive a reminder that is not done makes the original request open to
review again, without another task. Only a user can open it again. A reply closes
only its own conversation, and does not prove that a promised item was delivered. The task stays
until someone handles it.

Its source action opens the original email reader. When a user can no longer reach the source, they
also can no longer read the text of the task made from it.

When no label model is available, mail it cannot judge stays in the conversation review queue. It
does not claim Focus priority or a confirmed team duty. This run does not label outbound mail: a
sent reply that only confirms does not prove that the recipient owes an answer. So the email move
stays unknown (`none`) unless there is clear evidence of a request. Margince never guesses the move
from the direction alone.

## Weekly measure and recovery

Personal reports cover the recorded work a member holds, even when that member is a manager. Team
reports are a manager view, picked by name, of member reports, gated by live team rights; they do
not show private message text. Margince decides who owns what when it generates the report. Once
measured, the report keeps that owner list and roster, and does not follow later changes.

The report window is the whole local week, Monday to Sunday. A task completed at 00:00 at the start
of the next Monday belongs to the next week. Completed tasks include work with no date and older
work that was past due, counted separately from the share of tasks due that week. Older snapshots
have no measure of completed tasks, not a made-up zero. Carryover includes tasks due in or before
the reviewed week that were not done when it closed.

A recorded lead response includes a response after the deadline; breach counts stay separate. So a
breach does not mean a lead is still not answered. Meeting follow-up counts need a named source link
to a task the user can read, created before the week ends. A task that shares a contact or account
is not enough evidence. A missing link is a gap in the record, not proof that the member failed to
follow up. The same content permission checks apply to task and contact evidence in the deal
scorecard.

Personal and team measures wait until the review hour set for Monday; a catch-up also runs on
Sunday. Both run before the model calls and mail, which a setup may leave out. The written report
text, its notes and a delivery that has not run yet, when they are set up, can still run on retries. A
service that is not set up does not keep measured members due. A team waits when one of its members
failed to be measured on the current run. The job picks a member who has read rights to the team,
and does not expect that any member can read a team report.

A team report with zero measured members can be replaced by its first measure. It keeps its ID, and
the audit log records the fix. Its roster is captured at that first measure. A snapshot that is not
empty, even one that covers only some members by name, never changes.

A team total is not available if a member snapshot is missing, cannot be converted, or uses another
currency. A week-on-week view names the week of the real earlier report, which may not be the week
just before. When coaching finds nothing, it says no priority was found; it claims nothing
about how hard someone worked. Links to the current plan and the work queue say that they point to
the work of today.

Older reports keep the measure rules used when they were created. Missing meeting notes rank below
recorded wins or kept promises.

The scheduled writer and the web reader share the same weekly engine, with the plan close step and
the forecast snapshots. A refused forecast does not drop the report of the work the member recorded.
