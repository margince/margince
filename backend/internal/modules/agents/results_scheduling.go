// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// What the scheduling reads answer, split from results.go when that file
// reached its length ceiling. The boundary is a real one: a free/busy window
// is the only answer on this surface whose honesty depends on what fed it, so
// the type saying so lives beside the slots rather than among the record
// shapes.

import "time"

// FreeSlot is one interval a host is free, as the scheduling store reports it.
type FreeSlot struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// CalendarBacking is what a free/busy answer rests on.
type CalendarBacking string

const (
	// CalendarBacked means a live calendar connection feeds the meetings this
	// answer is computed from, so an empty window is a reading of the host's
	// own diary.
	CalendarBacked CalendarBacking = "calendar"
	// CalendarUnbacked means the acting seat has no calendar connected, so the
	// window is only what this CRM happens to hold.
	CalendarUnbacked CalendarBacking = "none"
	// CalendarBackingUnknown means the host is somebody else, and whether their
	// calendar is connected is not this seat's to read. The window is still
	// only what this CRM holds — that much is true of every host — but nothing
	// here says anything about that contact's account.
	CalendarBackingUnknown CalendarBacking = "unknown"
)

// AvailabilityResult is what check_availability answers.
//
// Truncated is not decoration: the walk stops at a cap, and a model handed a
// capped list with nothing marking it will tell a rep there is no later
// opening — the same failure AtRiskReport.Truncated exists to prevent.
type AvailabilityResult struct {
	Slots     []FreeSlot `json:"slots"`
	Truncated bool       `json:"truncated"`
	// CalendarBacking says what the free list was computed FROM: the host's own
	// diary, or only the meetings this CRM happens to hold. "none" makes a full
	// day of free slots mean "nothing is recorded here", which is not the same
	// claim as "the host is free" and must never be reported as one — the
	// warning beside it is the instruction, this is the fact a caller branches
	// on.
	//
	// THREE STATES, NOT TWO, and the third is a privacy boundary rather than a
	// nicety. Whether a colleague has a live calendar connection is their
	// account's business: capture is per-user, and answering it for an
	// arbitrary host_user_id would turn this tool into a roster-wide readout of
	// who has connected Google or Microsoft and whose grant has since failed.
	// So a host who is not the acting seat answers "unknown", which is the same
	// bytes whatever that host has actually connected.
	CalendarBacking CalendarBacking `json:"calendar_backing"`
}
