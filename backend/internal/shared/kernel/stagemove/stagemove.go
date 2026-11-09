// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package stagemove names where a deal's stage move came from. In the kernel
// because the deals writer and the stage readers in compose may not import
// each other.
package stagemove

// The values of deal_stage_history.provenance. A row saved before the column
// existed holds NULL.
const (
	// Recorded is a move made in Margince.
	Recorded = "recorded"
	// Imported is a move an import run brought in; the row names the run.
	Imported = "imported"
	// Synced is a move another CRM made that an extension mirrored in.
	Synced = "synced"
)

// madeElsewhere lists the values for a move another system made. Such a move
// shows how that system's users sold, not how this team sells.
const madeElsewhere = "('" + Imported + "','" + Synced + "')"

// MadeElsewhereSQL is true for a move another system made. column is a
// compile-time literal.
func MadeElsewhereSQL(column string) string {
	return column + " IN " + madeElsewhere
}

// NotMadeElsewhereSQL is true for a move made here and for a move saved before
// provenance was kept. column is a compile-time literal.
func NotMadeElsewhereSQL(column string) string {
	return "(" + column + " IS NULL OR " + column + " NOT IN " + madeElsewhere + ")"
}
