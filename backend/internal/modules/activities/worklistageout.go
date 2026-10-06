// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// worklistAgeOutDays is how long past its due date an open task stays on the
// Worklist. Past it the task is history rather than today's work, and a queue
// that keeps every missed date forever is one nobody can finish. The task itself
// is untouched: still open and still on its record.
const worklistAgeOutDays = 30

// worklistAgeOutClause keeps a row unless it is a task due more than
// worklistAgeOutDays before asOf. A row still pinned in effect by the user that
// pinner names stays regardless, because a pin is someone keeping it on purpose.
//
// pinner is a SQL expression, not a value: the reader's own placeholder for the
// day's page, the assignee column for the team board that counts their day.
func worklistAgeOutClause(asOf time.Time, pinner string, arg func(any) int) string {
	floor := arg(asOf.AddDate(0, 0, -worklistAgeOutDays))
	return sprintf(`(a.kind <> 'task' OR a.due_at IS NULL OR a.due_at >= $%d
		OR EXISTS (SELECT 1 FROM %s wp WHERE wp.source = $%d AND wp.row_id = a.id::text))`,
		floor, effectivePinsSQL(pinner, arg), arg(string(crmcontracts.WorklistItemSourceTask)))
}

// worklistClauses are the narrowings that make a read the Worklist's.
func worklistClauses(in ListActivitiesInput, reader ids.UUID, arg func(any) int) []string {
	if in.WorklistAsOf == nil {
		return nil
	}
	pinner := sprintf("$%d", arg(reader))
	return []string{
		auth.LiveIntakeTaskClause("a"),
		worklistAgeOutClause(*in.WorklistAsOf, pinner, arg),
	}
}
