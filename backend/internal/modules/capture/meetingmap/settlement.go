// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingmap

import "strings"

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
	// SettleCancel means the organizer called the meeting off. A meeting captured while it was live
	// must be marked cancelled rather than left standing; one never captured
	// has nothing to mark, and the writer's own upsert answers that.
	SettleCancel
	// SettleDrop means this event never belongs in the timeline at all.
	SettleDrop
)

// Reason names why an event is not captured as a booked meeting. The set is
// closed because the calendar counters use each one as a label.
type Reason string

// The reasons Settle gives. The text is what a skip error and a test read.
const (
	ReasonNoEventID    Reason = "no event id"
	ReasonCancelled    Reason = "cancelled"
	ReasonNoOtherParty Reason = "no party besides the owner"
)

// Label is the reason as a metric label value: the text in snake case.
func (r Reason) Label() string {
	return strings.ReplaceAll(strings.ReplaceAll(string(r), "'", ""), " ", "_")
}

// SkipReason names why a meeting is intentionally dropped, or reports that it
// should be captured. It is the two-way form of Settle below, kept for the
// callers that only ask whether to write — a cancelled meeting reads as a skip
// here, which is what it is for a caller that cannot cancel one.
func (m Meeting) SkipReason() (string, bool) {
	reason, settle := m.Settle()
	return string(reason), settle != SettleCapture
}

// Settle decides what happens to this event, and why.
//
// An organizer cancellation precedes the party checks because it may retire
// a meeting whose guests changed. Attendee responses belong to the roster.
//
// Whose side the parties are on is not asked here. Only the capture writer can
// read the workspace's own domains and consumer list. So the record states
// whether every party is on the owner's domain (ToRecord), and the writer
// decides.
func (m Meeting) Settle() (Reason, Settlement) {
	if m.id == "" {
		// Nothing to key on, so nothing to write and nothing to find again.
		return ReasonNoEventID, SettleDrop
	}
	if m.cancelled {
		return ReasonCancelled, SettleCancel
	}
	// An event naming nobody but the owner is a block in their own calendar —
	// focus time, a reminder, a flight. Nobody was met, so there is no
	// interaction to log. This asks whether there was a second party at all,
	// not whose side they were on, so it needs no knowledge of any domain.
	if len(m.addresses) <= 1 {
		return ReasonNoOtherParty, SettleDrop
	}
	return "", SettleCapture
}
