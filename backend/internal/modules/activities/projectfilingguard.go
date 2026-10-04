// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/retentionscope"
)

// FilingsStayUndoable answers, before a filing is made, whether UndoProjectFiling
// could still take it back: none of the named activities is restricted, archived,
// held through a link, or covered by an open erasure request, and the project
// itself is not under a legal hold. It is how a credential's own release of a
// project relink is judged by state and not by destination alone, because a filing
// the undo would then refuse is permanent.
//
// The hold and restriction tests are the ones the undo's own refusals apply
// (readProjectFilingFacts); an open erasure request is the addition, since it
// turns into a restriction without anyone touching the activity. An id that names
// no activity is not undoable either, the stricter answer.
func (s *Store) FilingsStayUndoable(ctx context.Context, activityIDs []ids.UUID, project ids.UUID) (bool, error) {
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
	var undoable bool
	err := s.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT (SELECT count(*) FROM activity a WHERE a.id = ANY($1)) = $2
			   AND NOT EXISTS (SELECT 1 FROM activity a WHERE a.id = ANY($1)
			                    AND (a.restricted_at IS NOT NULL OR a.archived_at IS NOT NULL
			                         OR `+retentionscope.HeldThroughAnyLink("a.id")+`
			                         OR `+retentionscope.UnderOpenErasure("a.id")+`))
			   AND NOT EXISTS (SELECT 1 FROM project p WHERE p.id = $3 AND p.legal_hold)`,
			distinct, len(distinct), project).Scan(&undoable)
	})
	return undoable, err
}
