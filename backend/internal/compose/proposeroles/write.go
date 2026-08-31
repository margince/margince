// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package proposeroles

// What a surviving proposal becomes on the record.
//
// The two constants are the whole of the marking. `Source` says the channel a
// row came through, `CapturedBy` says who acted — and together they are what
// lets a reader find every seat this ever wrote, and undo them, without the
// product needing a second "is this AI" column.

const (
	// Source is the DM-CONV-11 channel for a seat read out of messages.
	Source = "ai_proposal"
	// CapturedBy is the acting identity on a proposed seat.
	//
	// The `agent:` prefix is what the ai_written filter matches, so a seat
	// written here is discoverable as agent-authored by the query the rest of
	// the product already uses. Nothing else has to be told about it.
	CapturedBy = "agent:propose_roles"
)
