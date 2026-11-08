// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commsauthz

// RecordedThrough is how a stop was first recorded
// (communication_suppression.recorded_through). Every copy keeps the value of
// the row it was copied from, so a merge never makes a press look typed.
type RecordedThrough string

const (
	// RecordedByColleague is a colleague signed in to a seat, typing the stop.
	RecordedByColleague RecordedThrough = "colleague"
	// RecordedByAgent is an agent typing the stop on its user's passport.
	RecordedByAgent RecordedThrough = "agent"
	// RecordedBySubject is the contact's or lead's own press on a link Margince mailed.
	RecordedBySubject RecordedThrough = "subject"
	// RecordedByMachine is the product: a bounce, or the objection a kept
	// withdrawal leaves.
	RecordedByMachine RecordedThrough = "machine"
	// RecordedUnknown is a row whose first recorder could not be read. No rule
	// treats it as typed by a colleague.
	RecordedUnknown RecordedThrough = "unknown"
)
