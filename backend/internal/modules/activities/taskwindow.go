// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The window a task read asks for: what is due by an instant, what is due
// after one, or the span between them.
//
// Its own file because the two bounds are one idea — what makes them safe is
// that they MEET rather than overlap — and because the filter they came from
// had outgrown its length ceiling.

package activities

import "github.com/margince/margince/backend/internal/platform/database/storekit"

// openTaskWindowClauses narrows a task read to the window the caller asked for:
// an upper bound, a lower one, or both.
//
// Extracted from listActivitiesFilter, which the length ceiling had outgrown —
// and the two arms belong together anyway, because what makes them safe is that
// their bounds MEET rather than overlap.
// reminders widens the upper bound: a due reminder is today's work.
func openTaskWindowClauses(in ListActivitiesInput, reminders string, arg func(any) int) []string {
	openTask := "a.kind = 'task' AND NOT a.is_done AND " + storekit.TaskDueSQL("a") + " IS NOT NULL"
	var clauses []string
	if in.OpenAndDueBy != nil {
		// Strictly before the instant, which is what deadline.Passed means and
		// what this clause replaced. The bound the caller passes is the END of
		// the day, so `<=` would put a task due at exactly tomorrow 00:00 on
		// today's list — a promise reported late a day early.
		deadline := sprintf(storekit.TaskDueSQL("a")+" < $%d", arg(*in.OpenAndDueBy))
		if in.IncludeEmailRequests && in.OpenAndDueAfter == nil {
			// An undated reminder of a request and an undated promise are both
			// owed now: nobody named a later day for either.
			deadline = "(" + deadline + sprintf(" OR ("+storekit.TaskDueSQL("a")+" IS NULL AND a.source_system = ANY($%d)))",
				arg(storekit.UndatedTaskSources()))
		}
		if in.OrRemindersDue && reminders != "" {
			deadline = "(" + deadline + " OR " + reminders + ")"
		}
		clauses = append(clauses, "a.kind = 'task' AND NOT a.is_done AND "+deadline)
	}
	if in.OpenAndDueAfter != nil {
		// The open end of the same window, INCLUSIVE of the instant, so it
		// meets the exclusive upper bound above exactly: a task due at tomorrow
		// 00:00 is outside today's read and inside the upcoming one, and none
		// can fall between the two or be counted by both.
		// The due date is either the stored one or NULL, so the plain column
		// bound holds whenever the window's does. Spelled for the task index.
		from := arg(*in.OpenAndDueAfter)
		clauses = append(clauses,
			sprintf(openTask+" AND a.due_at >= $%[1]d AND "+storekit.TaskDueSQL("a")+" >= $%[1]d", from))
	}
	if in.ExceptRemindersDue && reminders != "" {
		clauses = append(clauses, "NOT COALESCE("+reminders+", FALSE)")
	}
	return clauses
}
