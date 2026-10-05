// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// FilingsStayUndoable answers, before a filing is made, whether UndoProjectFiling
// could still take it back. It asks the verdict's own blockers (readFilingBlockers)
// of the named activities — restricted, archived, held, under an open erasure
// request, kept by another basis or by a qualifying deal — and of the project, so
// the judgement a credential's own release rests on is the one the undo and the
// read apply, not a copy of it. A filing the undo would then refuse is permanent.
// An id that names no activity is not undoable either, the stricter answer.
//
// It reads through the caller's handle, the approval's own transaction when it is
// judging a decision, so it opens no second connection beside a locked row.
func FilingsStayUndoable(ctx context.Context, q Querier, activityIDs []ids.UUID, project ids.UUID) (bool, error) {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return false, err
	}
	distinct := make([]ids.UUID, 0, len(activityIDs))
	seen := map[ids.UUID]bool{}
	for _, activity := range activityIDs {
		if !seen[activity] {
			seen[activity] = true
			distinct = append(distinct, activity)
		}
	}
	if len(distinct) == 0 {
		return false, nil
	}
	blockers, err := readFilingBlockers(ctx, q, distinct)
	if err != nil || blockers.found != len(distinct) || blockers != (filingBlockers{found: len(distinct)}) {
		return false, err
	}
	var held bool
	err = q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM project p WHERE p.id = $1 AND p.legal_hold)`, project).Scan(&held)
	return err == nil && !held, err
}
