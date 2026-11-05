// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package meetingoutcome holds the one rule for "this meeting was held": a
// recorded held outcome. A start time in the past is not one. Nor is a
// calendar sync with no status, or a transcript on the row. A meeting nobody
// reported stays "awaiting outcome" on every surface that counts or proves.
package meetingoutcome

// Held is the activity.meeting_status value of a meeting someone reported as
// having taken place.
const Held = "held"

// IsHeld is HeldSQL for a reader holding the row in Go; status is "" when the
// meeting has none.
func IsHeld(status string) bool {
	return status == Held
}

// HeldSQL admits a meeting row under alias whose recorded outcome is held.
// alias is a compile-time identifier.
func HeldSQL(alias string) string {
	return "(" + alias + ".meeting_status = '" + Held + "')"
}
