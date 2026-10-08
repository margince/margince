<!-- prose:plain -->
# Scheduling

Margince gives three ways to set up a meeting with a contact. You can offer two or three times,
or send a calendar invite for a time you both agree on. You can also share a personal booking
link that works once. The account menu's `My booking link` is a separate public page that works
many times, so it can go in an email signature. You do not need to choose a contact first to use
it.

## Free times and calendars

Each host sets their own work days, daily start and end, and IANA timezone under
`Settings → Meetings → Bookable hours`. Hours a host never set use 09:00–17:00, Monday to
Friday, in the installation timezone. The scheduling profile on the same settings page adds four more settings:

- the meeting length;
- the shortest notice a guest must give;
- free time kept on both sides of a meeting;
- the last day a guest may book.

When only one provider is connected, Margince uses it; when more are connected, the host chooses.

The host sets the calendar new meetings go to, and any other calendars that block time, on this
page. `My booking link` holds the settings to share, pause and replace the public URL. Open times
start every 15 minutes in the host's timezone, and the meeting length does not change with that
step. Guests can choose the timezone they see times in.

The invite engine checks what is booked right now. It looks in the calendar set for new
meetings, and in the other blocking calendars on the same provider. Google uses a free/busy query plus only the
event data it needs; Microsoft uses `calendarView`. Internal meetings, private events, time a user
blocks out to work alone, and all-day busy events all count as booked. None of them becomes a CRM
activity. The public `/availability` API never returns titles, guests or event text.

A missing calendar, a wrong answer, a failed page, or a page count that runs past its limit makes
Margince refuse to answer with times.

Local holds, and moves still waiting on the provider, count as booked too. A hold stores its own
length. The database rule that stops two holds from sharing time counts each one as open at its
end. So one meeting can start right where another ends. A hold stored without a length blocks one
hour until someone changes it. Calendar capture keeps the length the provider gives when it gives
one.

A search whose times are all past the last day to book, or all inside the notice time, returns an
error that says why. It does not return an empty calendar. The invite screen shows the last date a guest can
book. It can search the rest of that time one part at a time. All-day events marked busy always
block booking.

Each time someone asks for an invite or moves one, the server checks the rules again. It checks
hours, notice, the last day to book, meeting length and the live calendar. A lock per host puts local holds in a line, one at
a time. The delivery worker checks the provider calendar once more right before it writes. An
outside calendar is not part of the database transaction. So a different calendar client can
still book the same time between that last check and the write.

## Provider delivery, and what happens when it fails

A calendar invite needs a connected calendar that Margince may write to. A calendar connected for
reads only must be connected again: Google adds `calendar.events.owned` and Microsoft uses
`Calendars.ReadWrite`. The host chooses a Google calendar they own or a Microsoft calendar they
can edit. Consent to connect a calendar names event writes; consent to connect a mailbox is
separate.

Each saved calendar ID points to the real calendar behind it. So if the host connects a different
account, an event Margince cannot find does not turn into a wrong cancel. When a calendar refuses
writes from Margince, the meeting shows `needs attention`.

## Video call links

A new meeting asks the host's calendar for a video call link, unless the host turned `Add a video
call link to new meetings` off. The setting lives on the scheduling profile, and a profile saved
before the setting existed reads it as on. Each invite and each personal proposal can set it
another way for that one meeting. A proposal keeps its setting when the guest accepts it, and a
public booking follows the host's setting.

Google Calendar receives a conference request keyed to the invitation's stable
request ID, so a retried create cannot ask for a second Google Meet. For Outlook,
Margince reads the calendar's own default online meeting provider and sends it
as `onlineMeetingProvider` with `isOnlineMeeting` set, because Graph applies
none by itself. That is Microsoft Teams for a work or school account, and no
link for a calendar that offers none. Only creation asks for a link; a reschedule keeps the conference the event already
has, and never rewrites an Outlook event's body, where Outlook keeps the join
details.

Only a new meeting asks for a link. A move keeps the call the event already has. It never writes
the body of an Outlook event again, because Outlook keeps the join text there.

Margince reads the link from the provider's answer. Google may still be making it, so Margince
reads the event again for some seconds and then stops. A link still on its way, or a read that
fails, leaves the delivered meeting without a link; the meeting does not fail. A calendar that
makes no link has still delivered the invite. The meeting is `confirmed` without a link, and the
host can add one in the calendar.

The public booking page names the calendar's video app (Google Meet or Microsoft Teams) each time
new meetings ask for a link. It does so even for a calendar that then offers none. What a guest
reads carries the join link, but never which provider the host uses. The host can still type a
phone number, the address of a place, or an existing call link as the place to meet.

Creating an invite commits one activity, its delivery command, and the audit and outbox rows, all
together. The answer is `pending`. The worker creates the provider event and asks the provider to
tell the guests; only the provider's accept changes the state to `confirmed`. Confirmed proves the
provider accepted the invite. It does not prove the guest read it or agreed to the meeting.

Google gets an event ID that Margince works out the same way every time. Microsoft gets a
transaction ID that never changes, and an added field on the event that Margince can search for.
When an answer does not show whether a write worked, Margince looks the event up before it writes
again. Delivery takes a lease with a version number and makes a limited number of attempts. After
that, the meeting shows `needs attention`. Retry works on the same invite.

Provider capture matches the host's copy of the event to the existing activity. It does so even
before the first copy is committed.

A move holds the new time while it keeps the old hold. The activity moves when the provider
accepts the update. A cancel frees the time once the provider accepts it, or confirms the event
does not exist. Version checks refuse changes made from an old copy. A direct edit to the activity
cannot change an invite Margince manages behind the host's back. Meeting status values such as held and
no-show stay available.

Every 15 minutes, a provider check looks at confirmed invites for recent meetings and meetings
still to come. It copies changed times and cancels to the same activity.

## Public and personal links

Meetings settings starts with the host's booking link, which works many times, and a Copy action.
The host can share the same link directly or add it to an email signature. Every host can preview,
copy, pause, start again or replace it there or from `My booking link`. Preview needs the host's
session, and it shows the saved page even while booking is paused.

Preview reads the host's live free times through the `/availability` endpoint that needs sign-in,
with the saved length, hours and calendar rules. A host can choose and look through times, but
the confirm button is off and preview sends no booking request. Guests still cannot book a paused
page.

Margince reads the host name from the name on the host's current Account, on every profile read.
That covers pages that need no sign-in, and personal proposals too. Margince does not use a stored
name set to replace it, or a `host_name` value in a request. A name over the published limit of
200 is made shorter.

Company name and logo come from the current anchor company, on personal proposals too. Margince
does not use a company name set per host. The logo stays public even when every booking page is
paused. The public logo route serves only the PNG that Margince made from that company's logo. It
cannot reach other companies or read their profiles. When someone removes or replaces the anchor
company's mark, the public image changes without each host saving their settings again.

The host types the notice time in hours, and Margince stores it in minutes. A notice of 24 hours
means a full day before the meeting. It does not mean a limit at 00:00. The place field takes the address of a place or an
existing Google Meet, Zoom or Teams link. A made link comes from the video call setting above,
never from the place field.

Replacing the link turns off the old public URLs. It does not turn off the private links that
guests already hold to manage their meetings. A paused page stops new public booking. Personal
proposals already sent keep working until they run out; pause does not pull back invites.

Profile answers hold only what the guest needs: the public host name, company, logo, meeting
subject, time and place, and booking rules. The logo keeps its shape, and the page footer uses the
same Margince name mark as the outside Deal Room.

The server links each personal proposal to the recipient's address and contact. A proposal runs
out after 7 days, or at the last time it offers if that comes sooner, and a guest can use it once.
Times in a proposal are offers and do not hold the time; accepting one checks the calendar again.
The guest may choose another free time. The host reads the proposal email in the existing email
writer before sending it.

A contact's open proposals list only the host's own links that are not used yet, not run out and
not pulled back, newest first. Pulling one back archives the proposal's activity, and the guest's
link stops working at once.

Browser and agent invitation writes use the same activities store and provider
adapter. `invite_meeting` is a confirm-first, send-scoped operation. The approval
names the recipient, interval, subject, location and the full description sent externally. The delivery worker rechecks
the acting host's live permissions and any originating passport's send authority.

Margince captures consent to the meeting separate from any marketing consent, which a guest need
not give. Calendar invite text holds a manage link. Someone with the full invite, such as a user
who manages or reads the host's calendar, can use it to move or cancel the meeting. So hosts
should think about who can read their calendar.

Erasure deletes proposals and deletes the matching vault data. A small cancel record keeps the
provider request and event IDs. So a delivery that runs at the same time as erasure can still be
canceled. That record holds no guest, subject, event text or manage key. Margince attempts the
cancel again while the provider cannot be reached.

The record also stops capture of provider copies that come later. Exports for a data subject's access request include the meeting data,
without tokens or internal delivery IDs.

## One engine, and an invite only when asked

Browser and agent invite writes use the same activities store and the same provider code.
`invite_meeting` is an action that asks for approval first and needs send scope. The approval
names the guest, the time, subject, place and the full event text that goes outside. The delivery
worker checks the host's live rights again, and the send right of any passport the request comes
from.

So older callers keep working, `book_meeting` and `/bookings` stay record-only actions: they send
no invite. `/availability` without `reliable=true` stays a read of CRM data only, and the agent answer
carries a note that says so. New booking screens always ask for `reliable` free times and calendar
delivery. No email or ICS file stands in when a provider write fails. So a failed write never
turns into a claim that the meeting is confirmed.

The activity record carries `invitation_status` separate from `meeting_status`. On the timeline,
buttons open delivery status, retry, move and cancel. A confirmed meeting can open the existing
meeting brief.

Provider contracts: [Google: create an event](https://developers.google.com/workspace/calendar/api/v3/reference/events/insert),
[Google free/busy](https://developers.google.com/workspace/calendar/api/v3/reference/freebusy/query),
[Microsoft: the event type](https://learn.microsoft.com/en-us/graph/api/resources/event?view=graph-rest-1.0),
and [Microsoft: `calendarView`](https://learn.microsoft.com/en-us/graph/api/calendar-list-calendarview?view=graph-rest-1.0).

## A reminder email

The host can turn on one email reminder an hour before meetings that have not started. The email
writes the time in the host's timezone and names that timezone. The worker reads the provider
event again before it builds the email, so provider moves and cancels update the same meeting
first. Margince sends only within 15 minutes of the reminder time; a missed window shows as
`unavailable`.

The existing email engine checks the live rights of the sender, whether the sender can see the
recipient, and whether the recipient may get email. Reminder delivery and its lasting mark commit
together, so two workers at once cannot queue it twice. A canceled meeting cannot queue a
reminder. A later meeting change cannot pull back mail already queued or delivered. The host sees
`pending`, `queued` or `unavailable` on the meeting page; queued means Margince asked for
delivery, not that the mail reached the recipient.

A used personal proposal stays a key the recipient holds until it would have run out. Opening it
again gives back the existing meeting's manage link after an answer never reached the guest. It
never books a second meeting.

Public clients may send a new random `UUIDv4` `Idempotency-Key` for each new booking, and send the
same key again on each retry of it. For 24 hours, such a retry gives back the same invite and the
same guest manage key.
Margince stores only the key hash and a hash of the request; key URLs stay in the vault. A changed
form cannot use the key again. Host HTTP replay records also leave out proposal URLs and manage
tokens.

## Running the migration

Migration `1790401426_bookings_reserve_their_exact_interval` builds the booking rule that stops
two holds sharing time again. It takes a lock on the whole activity table that blocks every other
user of it. Schedule it for a time when the app is not busy. Its lock waits at most three seconds,
so on a busy installation the migration fails rather than waits without end.

Margince asks for Microsoft `calendarView` answers in UTC, and it trusts the times they return,
including where all-day events start and end. An answer that does not follow the timezone request
is refused, not read in the host's timezone. Provider account tests should include all-day events
in a calendar that is not on UTC, and both clock changes in the year.
