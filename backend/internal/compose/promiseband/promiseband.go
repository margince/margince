// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package promiseband sorts a promise read out of a transcript by how firmly
// the reading says it was made.
package promiseband

const (
	// FirmFloor is the lowest confidence of a firm reading. From here the
	// commitment rule chooses between a task and a proposal.
	FirmFloor = 0.7
	// TentativeFloor is the lowest confidence kept at all. "I will try" is
	// read in the band up to FirmFloor, and below it a reading is a guess.
	TentativeFloor = 0.3
)

// Band is what a reading's confidence makes of it.
type Band string

const (
	// Dropped is a reading too unsure to put to anyone.
	Dropped Band = "dropped"
	// Tentative is a promise kept only as a proposal a member accepts.
	Tentative Band = "tentative"
	// Firm is a promise the commitment rule decides on as stated.
	Firm Band = "firm"
)

// Of places one confidence in its band.
func Of(confidence float64) Band {
	switch {
	case confidence >= FirmFloor:
		return Firm
	case confidence >= TentativeFloor:
		return Tentative
	default:
		return Dropped
	}
}
