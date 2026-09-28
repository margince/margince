// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package calendarbacking is what a free/busy answer rests on, in the words
// both doors publish it with.
//
// A window of free slots means two different things depending on where it came
// from. Read off a connected calendar, a busy period is one the host's diary
// actually holds. Derived from this CRM's own records, it says nothing about
// the rest of their day — and the two are indistinguishable in the answer
// itself, so a reader handed a full grid concludes an empty diary and tells the
// host a meeting they are having is not happening.
//
// The REST door and the `check_availability` tool both say which it is, and
// ADR-0055 is why they say it the same way: the two surfaces do not get to
// disagree about what an answer means. The values are the contract's enum, so
// this is where they are spelled and neither transport writes its own.
//
// Tier 0 rather than a module, because `activities` serves one door and
// `agents` the other, and a module never imports a sibling. stdlib only, which
// the tier requires — these are three strings and a decision over two booleans.
package calendarbacking

// The three answers, and the whole of what a caller may be told.
const (
	// Backed: the host's own connected calendar was read.
	Backed = "calendar"
	// Unbacked: no calendar is connected for them, so the window is this CRM's
	// own records and proves nothing about their diary.
	Unbacked = "none"
	// Unknown: this answer cannot say. Either the host is somebody else, whose
	// connector state is not this answer's to report, or the window was
	// computed without consulting a calendar at all — see
	// WithoutReadingTheCalendar. A reader must not take it as proof that
	// another host was even identified.
	Unknown = "unknown"
)

// WithoutReadingTheCalendar is what an answer may claim when its window came
// from this CRM's records alone.
//
// A host having connected a calendar is not the same fact as this read having
// consulted it, and only the second one licenses `calendar`. Both doors take a
// path that computes CRM-only slots, so both pass their answer through here —
// a door that reported connector state instead would say the diary was read
// when it was not, which is the one claim this vocabulary exists to stop.
//
// Unbacked survives: no calendar connected is true of the account however the
// window was computed.
func WithoutReadingTheCalendar(backing string) string {
	if backing == Backed {
		return Unknown
	}
	return backing
}
