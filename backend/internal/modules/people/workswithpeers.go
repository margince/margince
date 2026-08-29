// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// WorksWithPeers reports which contacts already hold a LIVE works_with edge
// with the anchor, whichever column either of them landed in.
//
// It exists so a suggestion surface can say "not yet recorded" without lying:
// that sentence discloses which edges exist, so the read carries the full
// relationship edge gate — object grant and endpoint row scope — and a caller
// refused there gets the refusal, which the surface turns into "no
// suggestions" rather than into a claim about records it may not see.
func (s *Store) WorksWithPeers(ctx context.Context, tx pgx.Tx, anchor ids.PersonID) (map[ids.UUID]bool, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	scope, err := auth.EdgeReadScope(ctx, "r", arg)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = scopeAllRows
	}
	anchorPos := arg(anchor)
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT CASE WHEN r.person_id = $%d THEN r.counterparty_person_id ELSE r.person_id END
		  FROM relationship r
		 WHERE r.kind = 'works_with' AND r.archived_at IS NULL
		   AND (r.person_id = $%d OR r.counterparty_person_id = $%d)
		   AND (%s)`, anchorPos, anchorPos, anchorPos, scope), args...)
	if err != nil {
		return nil, fmt.Errorf("people: reading who is already recorded as working with a contact: %w", err)
	}
	defer rows.Close()
	out := map[ids.UUID]bool{}
	for rows.Next() {
		var peer ids.UUID
		if err := rows.Scan(&peer); err != nil {
			return nil, err
		}
		out[peer] = true
	}
	return out, rows.Err()
}
