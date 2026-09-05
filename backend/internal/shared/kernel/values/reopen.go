// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

// What a rep is waiting for when they set something aside.
//
// A snooze used to mean one thing: come back on Thursday. That made every
// set-aside a guess about when the world would move, and the rep was usually
// wrong in one of two directions — too early and the item is still dead, too
// late and the customer waited on them. The two things they actually meant are
// held here.
//
// Spelled in this package rather than in briefs or activities because BOTH set
// aside work and both store the condition, and the two tables' CHECK
// constraints hold the same set. A second copy in either module is a set that
// can drift from the one the database will accept.

import "fmt"

// ReopenCondition names what lifts a snooze.
type ReopenCondition string

const (
	// ReopenOnTime lifts at a stored instant — the original snooze.
	ReopenOnTime ReopenCondition = "time"
	// ReopenOnReply lifts when the counterparty writes back. No instant: the
	// wait is on a person, and putting a deadline on it would re-surface work
	// on a day nothing happened.
	ReopenOnReply ReopenCondition = "reply"
	// ReopenOnMeeting lifts once a named meeting is over. It names the meeting,
	// because "after the meeting" says nothing without saying which one.
	ReopenOnMeeting ReopenCondition = "meeting"
)

// ReopenConditions is every condition, in the order a chooser should offer
// them: the moment first because it is what a rep already knows, then the two
// that wait on the world.
//
// Held by: TestEveryTableAdmitsExactlyTheConditionsGoCanWrite (backend/gates/reopenconditionparity_test.go),
// which derives the same set from both tables' CHECK constraints and requires
// them equal. Its sibling in that file holds the contract's enum against this
// set too, so a condition added here and nowhere else fails twice.
//
// Returned as a fresh slice because a package-level array would let one caller
// reorder every other caller's chooser.
func ReopenConditions() []ReopenCondition {
	return []ReopenCondition{ReopenOnTime, ReopenOnReply, ReopenOnMeeting}
}

// WantsInstant says whether this condition stores a snoozed_until, and
// NeedsReference whether it stores a reopen_ref. Together they are the shape
// both tables' CHECK constraints hold, asked in Go before the write rather than
// discovered as a constraint violation the client cannot read.
func (c ReopenCondition) WantsInstant() bool { return c == ReopenOnTime }

// NeedsReference says whether the condition names another row.
func (c ReopenCondition) NeedsReference() bool { return c == ReopenOnMeeting }

// ParseReopenCondition turns a wire value into a condition, refusing anything
// outside the set the database would refuse anyway.
//
// The default is deliberate rather than convenient: an ABSENT condition is a
// time snooze, because that is what every caller written before this existed
// meant, and every row stored before it was one.
func ParseReopenCondition(raw *string, field string) (ReopenCondition, error) {
	if raw == nil || *raw == "" {
		return ReopenOnTime, nil
	}
	for _, known := range ReopenConditions() {
		if ReopenCondition(*raw) == known {
			return known, nil
		}
	}
	return "", &ParseError{
		Field: field, Code: "unknown_reopen_condition",
		Message: fmt.Sprintf("a snooze waits for one of %v", ReopenConditions()),
	}
}
