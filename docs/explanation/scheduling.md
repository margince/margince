# Scheduling

Margince supports three ways to arrange a meeting with a contact: propose two or
three times, send a calendar invitation for an agreed time, or share a personal
one-use booking link. The account menu's **My booking link** is a separate,
reusable public page suitable for an email signature. It does not require a
contact to be selected first.

## Availability and calendars

Each host controls their working days, daily start and end, and IANA timezone
under Settings → Account → Bookable hours. Unset hours use 09:00–17:00 Monday to
Friday in the installation timezone. The scheduling profile adds duration,
minimum notice, a buffer on both sides, and a booking horizon. Slots advance in
15-minute increments in the host's timezone; their duration is independent of
that increment. Guests can choose their display timezone.

The invitation engine checks live occupancy in the selected destination
calendar and additional blocking calendars on the same provider. Google uses
free/busy admission plus minimal event occupancy; Microsoft uses calendarView.
Internal meetings, private events, focus blocks and all-day busy events count
without becoming CRM activities. Titles, attendees and descriptions are not
returned by the public availability API. A missing calendar, invalid response,
failed page or exhausted pagination bound refuses availability.

Local reservations and pending reschedules are included. New reservations store
their exact duration; the exclusion constraint uses half-open intervals so
adjacent meetings can coexist. Older reservations retain their historical
one-hour exclusion until explicitly changed. Calendar capture preserves the
provider's duration when it is supplied.

The server validates hours, notice, horizon, duration and live occupancy when an
invitation is requested or moved. A host lock serializes local reservations.
The delivery worker checks provider occupancy again immediately before writing.
An external calendar is not part of the database transaction, so an independent
calendar client can still race that final check.

## Provider delivery and recovery

A calendar invitation requires a connected calendar with write permission.
Existing read-only connections must be reconnected: Google adds
`calendar.events.owned` and Microsoft uses `Calendars.ReadWrite`. The host chooses
an owned Google calendar or an editable Microsoft calendar. Calendar connection consent explicitly includes event writes; mailbox connection consent is separate. Destination IDs are bound to the actual calendar, so reconnecting a different account cannot turn a missing event into a false cancellation. Lost calendar authority surfaces as **needs attention**. A video-call URL,
telephone number or physical address can be entered as the location; this flow
does not create a conferencing account or generate a new conferencing link.

Creating an invitation commits one activity, its delivery command, audit and
outbox evidence. The response is **pending**. The worker creates the provider
event and requests attendee notifications; only provider acceptance changes the
state to **confirmed**. Confirmation proves the provider accepted the invitation,
not that the guest read it or accepted the meeting.

Google receives a deterministic event ID. Microsoft receives a stable
transaction ID and a queryable extended property. An uncertain response is
looked up before another write is attempted. Delivery uses versioned leases and
bounded attempts, then surfaces **needs attention**. Retry operates on the same
invitation. Provider capture resolves the organizer's echo onto the existing
activity, including before the first receipt is committed.

A reschedule reserves the new interval while retaining the old reservation.
The activity moves when the provider acknowledges the update. Cancellation
releases the interval after the provider acknowledges it or confirms the event
is absent. Version checks reject stale changes. Direct activity edits cannot
silently change a managed invitation; meeting outcomes such as held and no-show
remain available. Provider reconciliation checks recent and future confirmed
invitations every fifteen minutes and imports changed intervals or cancellations
onto the same activity.

## Public and personal links

Every host can create, preview, copy, pause, resume or replace their public link.
Replacing it revokes old public URLs; it does not revoke existing guests' private
meeting-management links. A paused page stops new public bookings. Already issued personal proposals remain usable until their expiry; pause is not a recall of invitations. Profile
responses contain only the public host name, company, logo, meeting details and
availability policy needed by the guest. The logo keeps its aspect ratio and
the footer uses the same Margince wordmark component as the external Deal Room.

Personal proposals bind the recipient address and contact on the server. They
expire after seven days, or the last proposed time if sooner, and can be consumed
once. Offered times are proposals, not holds; accepting one rechecks availability.
The guest may choose another available time. The host reviews the proposal email
in the existing composer before sending it.

Management and proposal tokens are random capabilities stored as hashes. A
management and proposal URL also lives encrypted in the existing vault because the delivery
worker needs to include it in the invitation. Public responses exclude calendar
IDs, provider event URLs, recipient addresses and internal record links. Access
logs redact the capability segment and public responses disable caching and
referrers. Consent is captured separately from optional marketing consent.
Calendar invite descriptions include a management link. Anyone with the full
invite, including calendar delegates and readers, can use it to reschedule or
cancel; hosts should share their calendar accordingly.

Erasure deletes proposals and destroys corresponding vault material. A minimal
cancellation tombstone retains provider request/event identifiers so delivery
that raced erasure can still be canceled. It contains no attendee, subject,
description or management capability. Cleanup retries while provider access is
unavailable, and the tombstone suppresses capture of late provider echoes; subject-access exports include the meeting data without tokens
or internal delivery identifiers.

## One engine and explicit invitation intent

Browser and agent invitation writes use the same activities store and provider
adapter. `invite_meeting` is a confirm-first, send-scoped operation. The approval
names the recipient, interval, subject, location and the full description sent externally. The delivery worker rechecks
the acting host's live permissions and any originating passport's send authority.

For compatibility, `book_meeting` and `/bookings` remain record-only operations:
they send no invite. `/availability` without `reliable=true` remains a CRM-only
read and the agent response carries a caveat. New booking screens always request
reliable availability and calendar delivery. There is no email/ICS fallback that
silently turns a failed provider write into a claim of confirmation.

The activity projection carries `invitation_status` independently of
`meeting_status`. Timeline controls open delivery status, retry, reschedule and
cancel; confirmed meetings can open Margince's existing preparation brief.

Provider contracts: [Google event insertion](https://developers.google.com/workspace/calendar/api/v3/reference/events/insert),
[Google free/busy](https://developers.google.com/workspace/calendar/api/v3/reference/freebusy/query),
[Microsoft event resource](https://learn.microsoft.com/en-us/graph/api/resources/event?view=graph-rest-1.0),
and [Microsoft calendar view](https://learn.microsoft.com/en-us/graph/api/calendar-list-calendarview?view=graph-rest-1.0).

## Optional reminder

The host can enable one email reminder an hour before future meetings. Its time
is written in the host’s configured timezone and labels that timezone. The
worker refreshes the provider event before preparing it, so provider moves and
cancellations update the same meeting first. Margince sends only within fifteen
minutes of the reminder's due time; a missed window is shown as unavailable.
The existing email engine checks the sender's live authority, recipient
visibility and communication permission. Reminder delivery and its durable
marker commit together; concurrent workers cannot stage it twice. A canceled
meeting cannot stage a reminder. Mail already queued or delivered cannot be
recalled by a later meeting change. The host sees pending, queued or unavailable
on the meeting page; queued means delivery was requested, not that it arrived.

A used personal proposal remains a recipient capability until its original
expiry. Reopening it recovers the existing meeting's management link after a
lost response; it never reserves a second meeting.

Public clients may send a fresh random UUIDv4 `Idempotency-Key` for each request.
For 24 hours, retrying the same form with that key recovers the same invitation
and guest management capability. Only the key hash and request digest are stored;
capability URLs remain in the vault. A changed form cannot reuse the key. Host
HTTP replay storage likewise excludes proposal URLs and management tokens.

## Deployment

The exact-interval migration rebuilds the booking exclusion constraint and takes
an exclusive activity-table lock. Schedule a maintenance window; the three-second
lock timeout makes a busy deployment fail rather than wait indefinitely. This is
not an online, zero-lock migration. Shipped migrations remain unchanged.

Microsoft calendarView responses are requested in UTC and their returned
instants are authoritative, including all-day boundaries. A response that ignores
the timezone request is refused rather than guessed in the host's timezone.
Provider-account certification should include all-day events in a non-UTC
calendar and both daylight-saving transitions.
