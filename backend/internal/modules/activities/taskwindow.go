// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The window a task read asks for: what is due by an instant, what is due
// after one, or the span between them.
//
// Its own file because the two bounds are one idea — what makes them safe is
// that they MEET rather than overlap — and because the filter they came from
// had outgrown its length ceiling.

package activities

// openTaskWindowClauses narrows a task read to the window the caller asked for:
// an upper bound, a lower one, or both.
//
// Extracted from listActivitiesFilter, which the length ceiling had outgrown —
// and the two arms belong together anyway, because what makes them safe is that
// their bounds MEET rather than overlap.
func openTaskWindowClauses(in ListActivitiesInput, arg func(any) int) []string {
	const openTask = "a.kind = 'task' AND NOT a.is_done AND a.due_at IS NOT NULL"
	var clauses []string
	if in.OpenAndDueBy != nil {
		// Strictly before the instant, which is what deadline.Passed means and
		// what this clause replaced. The bound the caller passes is the END of
		// the day, so `<=` would put a task due at exactly tomorrow 00:00 on
		// today's list — a promise reported late a day early.
		clauses = append(clauses,
			sprintf(openTask+" AND a.due_at < $%d", arg(*in.OpenAndDueBy)))
	}
	if in.OpenAndDueAfter != nil {
		// The open end of the same window, INCLUSIVE of the instant, so it
		// meets the exclusive upper bound above exactly: a task due at tomorrow
		// 00:00 is outside today's read and inside the upcoming one, and none
		// can fall between the two or be counted by both.
		clauses = append(clauses,
			sprintf(openTask+" AND a.due_at >= $%d", arg(*in.OpenAndDueAfter)))
	}
	return clauses
}
