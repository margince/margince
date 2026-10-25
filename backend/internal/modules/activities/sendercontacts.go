// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SenderContacts names the contact each message came from, keyed by activity
// id. A message whose sender is no contact this caller may read has no entry.
//
// A draft uses it to tell the addressee's mail from other participants'.
// It applies the activity content gate itself, then the contact grant and row scope,
// so an activity this caller cannot read yields nothing.
func (s *Store) SenderContacts(ctx context.Context, activityIDs []ids.UUID) (map[ids.UUID]ids.UUID, error) {
	if len(activityIDs) == 0 {
		return nil, nil
	}
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return nil, err
	}
	senders := make(map[ids.UUID]ids.UUID, len(activityIDs))
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		args := []any{activityIDs}
		content, err := auth.ActivityContentClause(ctx, "a", func(v any) int {
			args = append(args, v)
			return len(args)
		})
		if err != nil {
			return err
		}
		scope, err := auth.ScopeClauseFor(ctx, "contact", "p", func(v any) int {
			args = append(args, v)
			return len(args)
		})
		if err != nil {
			return err
		}
		q := `
			SELECT DISTINCT ON (ap.activity_id) ap.activity_id, ap.contact_id
			  FROM activity_participant ap
			  JOIN activity a ON a.id = ap.activity_id
			  JOIN contact p ON p.id = ap.contact_id
			 WHERE ap.activity_id = ANY($1) AND ap.role = 'from'
			   AND a.archived_at IS NULL AND (` + content + `)
			   AND ap.contact_id IS NOT NULL AND p.archived_at IS NULL`
		if scope != "" {
			q += ` AND (` + scope + `)`
		}
		q += ` ORDER BY ap.activity_id, ap.created_at, ap.id`
		rows, err := tx.Query(ctx, q, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var activity, contact ids.UUID
			if err := rows.Scan(&activity, &contact); err != nil {
				return err
			}
			senders[activity] = contact
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return senders, nil
}
