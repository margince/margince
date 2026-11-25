// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// StandingSQL narrows a read of historical deals to the ones that still stand
// for an opportunity. Archived deals are read on purpose by reports that count
// what closed. A deal merged into another is not one of them: its opportunity
// is carried by the survivor, and counting both would count one win twice.
// alias is a compile-time literal naming the deal table in the statement.
func StandingSQL(alias string) string {
	return alias + ".merged_into_id IS NULL"
}
