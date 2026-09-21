// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingmap

// What capture should DO with one event, and why — as distinct from what an
// event MAPS to, which is meetingmap.go beside this.
//
// The two questions separated when a cancellation stopped being a drop. Before
// that, "do not write this" was one answer with one reason attached, and it
// lived naturally beside the mapping. Now the same not-writing splits in two:
// an event never captured is dropped, and a meeting already on the timeline is
// CLOSED. Which of those a reader gets is the whole of what this file decides.

// Settlement is what capture should DO with one event: write it, cancel a
// meeting already written, or drop it.
type Settlement int

const (
	// SettleCapture writes the meeting.
	SettleCapture Settlement = iota
	// SettleCancel means the meeting is off — the organizer called it off, or
	// the connected account declined it. A meeting captured while it was live
	// must be marked cancelled rather than left standing; one never captured
	// has nothing to mark, and the writer's own upsert answers that.
	SettleCancel
	// SettleDrop means this event never belongs in the timeline at all.
	SettleDrop
)

// SkipReason names why a meeting is intentionally dropped, or reports that it
// should be captured. It is the two-way form of Settle below, kept for the
// callers that only ask whether to write — a cancelled meeting reads as a skip
// here, which is what it is for a caller that cannot cancel one.
func (m Meeting) SkipReason() (string, bool) {
	reason, settle := m.Settle()
	return reason, settle != SettleCapture
}

// Settle decides what happens to this event, and why.
//
// A CANCELLED or DECLINED event is not simply dropped. Dropping is right for one
// that was never captured, and wrong for one that was: the meeting is already on
// the timeline and on the reader's schedule, and no later pull ever mentions it
// again — the row stands as booked forever. So the two answers are told apart,
// and the writer marks what it already has.
//
// The order matters. Cancellation is asked FIRST, before the party rules below,
// because those rules answer "is this worth capturing" and a meeting already
// captured has had that question answered in the affirmative once. Asking them
// first would drop the cancellation of a meeting whose attendee list changed
// after it was booked, and leave exactly the stale row this exists to clear.
//
// A RECURRING meeting cancels one occurrence at a time, and that falls out of
// how both connectors pull rather than from anything here: Google lists with
// singleEvents=true and Microsoft walks calendarView, so each occurrence arrives
// as its own event carrying its own id. Declining next Tuesday therefore closes
// next Tuesday's meeting and leaves the rest of the series booked, which is what
// the calendar means. A connector that pulled series masters instead would hand
// this one id for every occurrence, and one decline would close them all.
//
// The owner's domain is a FLOOR here, not the authority. The workspace's
// registered domains are what decide internal-vs-external for mail and calendar
// alike, and only the capture writer can read them — a connector holds no
// database handle by design. So this drops what the owner's own domain alone
// proves internal, and the writer widens that set, never narrows it.
func (m Meeting) Settle() (string, Settlement) {
	if m.id == "" {
		// Nothing to key on, so nothing to write and nothing to find again.
		return "no event id", SettleDrop
	}
	if m.cancelled {
		return "cancelled", SettleCancel
	}
	if m.ownerDeclined {
		return "declined by the calendar owner", SettleCancel
	}
	// An event naming nobody but the owner is a block in their own calendar —
	// focus time, a reminder, a flight. Nobody was met, so there is no
	// interaction to log. This asks whether there was a second party at all,
	// not whose side they were on, so it needs no knowledge of any domain.
	if len(m.addresses) <= 1 {
		return "no party besides the owner", SettleDrop
	}
	// The owner-domain floor. The workspace's registered domains are the
	// authority (formulas §20) and the writer applies them over the full party
	// set, which is wider than this — but that set can be empty or incomplete,
	// and an internal meeting stored while it is would be readable by the whole
	// workspace. This drops what the owner's own domain alone can prove
	// internal; the writer widens it, never narrows it.
	if !m.hasExternal {
		return "no party outside the owner's domain", SettleDrop
	}
	return "", SettleCapture
}
