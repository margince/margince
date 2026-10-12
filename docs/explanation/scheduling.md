<!-- prose:plain -->
# Scheduling

What a meeting host sees is in the handbook page
[Meetings and booking](../handbook/meetings-and-booking.md). That covers hours, the booking link,
invite states and reminders. This page holds the reasons behind the engine. It says why hours
are personal, how delivery stays safe to run again, and why a failed write never reads as a booked
meeting.

## Hours belong to the host

The hours a host can be booked are a fact about one host, so each host sets their own. No setting for the whole
installation replaces them. Hours a host never set fall back to 09:00 to 17:00, Monday to Friday,
in the installation timezone. An agent acting for someone else may not set them, because when a
host can be booked is that host's call.

## Free times are read live, and a failed read shows no times

The invite engine reads the provider's busy time at the moment of the request. Google gets a
free/busy query plus only the event data it needs. Microsoft gets `calendarView`, asked for in UTC.
Margince trusts the times that answer returns, all-day bounds included. It refuses an answer that
ignores the timezone request, rather than reading it in the host's zone. Busy time never becomes a
CRM activity, and the public `/availability` API never returns titles, guests or event text.

A missing calendar, a wrong answer, a failed page, or a page count past its limit makes Margince
refuse to answer with times. Times that may be taken would mislead a guest, so none are shown.

Local holds, and moves still waiting on the provider, count as booked. The database rule that stops
two holds sharing time counts each one as open at its end. So one meeting can start where another
ends. A hold stored without a length blocks one hour until someone changes it.

Each request or move checks hours, notice, horizon, length and the live calendar again. A lock per
host puts local holds in a line. The delivery worker checks the provider once more right before it
writes. The provider calendar is not part of the database transaction. So another client can still
take the time between that check and the write: the gap is narrowed, not closed.

## Delivery is a command, not a call

Creating an invite commits one activity, its delivery command, and the audit and outbox rows, all
together. The answer is `pending`. The worker creates the provider event and asks the provider to
tell the guests. Only the provider's accept moves the state to `confirmed`. Confirmed proves the
provider accepted the invite, not that the guest read it or agreed.

Every write has to be safe to run again, because the worker cannot always tell whether the last
attempt landed:

- Google gets an event ID Margince works out the same way every time. Microsoft gets a transaction
  ID that never changes, and an added field on the event that Margince can search for.
- When an answer does not show whether a write worked, Margince looks the event up before it
  writes again.
- Delivery takes a lease with a version number and makes a limited number of attempts. After
  that, the meeting shows `needs attention`, and a retry works on the same invite.
- Each saved calendar ID points to the real calendar behind it. If the host connects a different
  account, an event Margince cannot find does not turn into a wrong cancel.

A move holds the new time while it keeps the old hold, and the activity moves when the provider
accepts. A cancel frees the time once the provider accepts it, or confirms the event does not
exist. Version checks refuse changes made from an old copy. A direct edit to the activity cannot
change a managed invite behind the host's back.

Provider capture matches the host's copy of the event to the existing activity, even before the
first copy is committed. A provider check every 15 minutes copies changed times and cancels back to
the same activity.

## Video links come with the create

A new meeting asks the host's calendar for a video link, unless the host turned the setting off. A
profile saved before the setting existed reads it as on. Google gets a call request keyed to the
invite's request ID, which never changes. So a create that runs again cannot ask for a second Meet.
Graph adds no link by itself. So for Outlook, Margince reads the calendar's default meeting
provider and sends it as `onlineMeetingProvider` with `isOnlineMeeting` set.

Only a create asks for a link. A move keeps the event's call and never writes an Outlook event's
body again, because Outlook keeps the join text there. A link Google is still making, or a read
that fails, leaves the delivered meeting without one; the meeting does not fail.

## One engine, and an invite only when asked

Browser and agent invite writes use the same activities store and the same provider code.
`invite_meeting` asks for approval first and needs send scope. The approval names the guest, the
time, subject, place and the full event text that goes outside. The delivery worker checks the
host's live rights again, and the send right of any passport the request came from.

So older callers keep working, `book_meeting` and `/bookings` stay record-only: they send no
invite. `/availability` without `reliable=true` stays a read of CRM data only, and the agent answer
says so. No email or ICS file stands in when a provider write fails. So a failed write never turns
into a claim that the meeting is confirmed.

The activity carries `invitation_status` apart from `meeting_status`. So delivery state and the
held or no-show outcome never write over each other.

Provider contracts: [Google: create an event](https://developers.google.com/workspace/calendar/api/v3/reference/events/insert),
[Google free/busy](https://developers.google.com/workspace/calendar/api/v3/reference/freebusy/query),
[Microsoft: the event type](https://learn.microsoft.com/en-us/graph/api/resources/event?view=graph-rest-1.0),
and [Microsoft: `calendarView`](https://learn.microsoft.com/en-us/graph/api/calendar-list-calendarview?view=graph-rest-1.0).
Provider account tests should include all-day events in a calendar that is not on UTC, and both
clock changes in the year.

## Links hold no secret in the clear

The host name comes from the host's current Account on every read, public pages and proposals
included. No stored name set to replace it, and no `host_name` in a request, is used. Company name
and logo come from the current anchor company. So replacing its mark changes every public page
without each host saving again. The public logo route serves only the PNG made from that company's
logo.

Manage and proposal tokens are random keys stored only as hashes. A manage or proposal URL is also
kept in the vault, because the delivery worker must put it in the invite. Public answers leave out
calendar IDs, provider event URLs, guest addresses and internal record links. Access logs leave out
the key part of the path, and public answers turn off caching and the `Referer` header.

The server links each personal proposal to the recipient's address and contact. A used proposal
stays a key the recipient holds until it would have run out. Opening it again returns the existing
meeting's manage link, for the case where the first answer never reached the guest. It never books
a second meeting.

Public clients may send a random `UUIDv4` `Idempotency-Key` for each booking, and send it again on
each retry. For 24 hours a retry returns the same invite and the same manage key. Margince stores
only the key hash and a hash of the request, so a changed form cannot use the key again. Host HTTP
replay records leave out proposal URLs and manage tokens.

## Erasure cancels what it cannot see

Erasure deletes proposals and their vault data. A small cancel record keeps the provider request and
event IDs. So a delivery that runs at the same time as the erasure can still be cancelled, and later
provider copies are not captured again. That record holds no guest, subject, event text or manage
key. An export for a data subject's access request includes the meeting data, without tokens or
delivery IDs.

## The reminder commits with its mark

The worker reads the provider event again before it builds a reminder, so provider moves and
cancels reach the meeting first. The reminder goes through the existing email engine. That engine
checks the rights of the sender, whether the sender can see the recipient, and whether the recipient may
get email. Delivery and its lasting mark commit together, so two workers cannot queue it twice. A
later meeting change cannot pull back mail already queued.
