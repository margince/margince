// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// StandingSQL narrows a read of historical deals to the ones that still stand
// for an opportunity. Reports that count what closed read archived deals too.
// A deal merged into another is not one of them. The survivor carries its
// opportunity, and counting both would count one win twice.
// alias is a compile-time literal naming the deal table in the statement.
func StandingSQL(alias string) string {
	return alias + ".merged_into_id IS NULL"
}
