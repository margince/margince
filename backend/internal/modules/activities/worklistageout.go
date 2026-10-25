// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// worklistAgeOutDays is how long past its due date an open task stays on the
// Worklist. Past it an unadopted task is history rather than today's work, and a queue
// that keeps every missed date forever is one nobody can finish. The task itself
// is untouched: still open and still on its record.
const worklistAgeOutDays = 30

// worklistAgeOutClause keeps a row unless it is a task due more than
// worklistAgeOutDays before asOf. A human reopening keeps the task current.
// A row still pinned in effect by the user that
// pinner names stays regardless, because a pin is someone keeping it on purpose.
// So does a task whose reminder is due for that user: they asked to be reminded.
//
// pinner is a SQL expression, not a value: the reader's own placeholder for the
// day's page, the assignee column for the team board that counts their day.
func worklistAgeOutClause(asOf time.Time, pinner, identities string, arg func(any) int) string {
	floor := arg(asOf.AddDate(0, 0, -worklistAgeOutDays))
	return storekit.CurrentTaskClause("a", asOf, identities, arg) + " AND " + sprintf(`(a.kind <> 'task' OR a.due_at IS NULL OR a.due_at >= $%d
		OR `+humanReopenedTaskSQL("a")+`
		OR (%s, a.id::text) IN (SELECT reader_id, row_id FROM %s wp WHERE wp.source = $%d)
		OR %s)`,
		floor, pinner, auth.EffectiveWorklistPinsSQL(arg), arg(string(crmcontracts.WorklistItemSourceTask)),
		dueReminderClause("a", pinner, asOf, arg))
}

func humanReopenedTaskSQL(alias string) string {
	return `EXISTS (SELECT 1 FROM audit_log adoption WHERE adoption.entity_type = 'activity'
 AND adoption.entity_id = ` + alias + `.id AND adoption.actor_type = 'human'
 AND adoption.before->>'is_done' = 'true' AND adoption.after->>'is_done' = 'false'
 AND ` + storekit.AuditNotFromAnImport("adoption") + `)`
}

// worklistClauses are the narrowings that make a read the Worklist's.
func worklistClauses(in ListActivitiesInput, reader ids.UUID, arg func(any) int) []string {
	if in.WorklistAsOf == nil {
		return nil
	}
	if in.OwnQueueOf != nil {
		reader = in.OwnQueueOf.UUID
	} else if in.AssigneeID != nil {
		reader = in.AssigneeID.UUID
	}
	pinner := sprintf("$%d", arg(reader))
	return []string{
		auth.LiveIntakeTaskClause("a"),
		worklistAgeOutClause(*in.WorklistAsOf, pinner, sprintf("$%d::jsonb", arg(in.mailIdentities)), arg),
	}
}
