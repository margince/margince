// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func lockNoteMentions(ctx context.Context, tx pgx.Tx, pattern string) ([]ids.UUID, error) {
	rows, err := tx.Query(ctx, `SELECT id FROM activity WHERE kind='note'
 AND (subject ~* @pattern OR body ~* @pattern) ORDER BY id FOR UPDATE`, pgx.StrictNamedArgs{"pattern": pattern})
	if err != nil {
		return nil, err
	}
	notes, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil || len(notes) == 0 {
		return notes, err
	}
	for _, query := range []string{
		`SELECT id FROM company WHERE id IN(SELECT company_id FROM activity_link WHERE activity_id=ANY(@notes::uuid[])) ORDER BY id FOR SHARE`,
		`SELECT id FROM contact WHERE id IN(SELECT contact_id FROM activity_link WHERE activity_id=ANY(@notes::uuid[])) ORDER BY id FOR SHARE`,
		`SELECT id FROM deal WHERE id IN(SELECT deal_id FROM activity_link WHERE activity_id=ANY(@notes::uuid[])) ORDER BY id FOR SHARE`,
		`SELECT id FROM lead WHERE id IN(SELECT lead_id FROM activity_link WHERE activity_id=ANY(@notes::uuid[])) ORDER BY id FOR SHARE`,
		`SELECT id FROM project WHERE id IN(SELECT project_id FROM activity_link WHERE activity_id=ANY(@notes::uuid[])) ORDER BY id FOR SHARE`,
	} {
		if _, err := tx.Exec(ctx, query, pgx.StrictNamedArgs{"notes": notes}); err != nil {
			return nil, err
		}
	}
	return notes, nil
}
