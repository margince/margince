// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Which companies carry live work, asked ahead of a merge rather than during
// one.
//
// The merge refuses when both endpoints hold live projects, and the duplicates
// lane offers a Merge button on any pair whose two records the reader may
// write. So a data steward with authority over both was handed a button that
// answered 409 every time — the same dead end the authority check was added to
// remove, reached by another path.
//
// The card is where a reader should learn a verb is unavailable, not the
// refusal after the press.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// OrganizationsCarryingLiveProjects answers which of these companies hold at
// least one live project, for the lane deciding whether to offer a merge.
//
// UNSCOPED by project, exactly as the merge's own refusal is: work the reader
// cannot see must still block the merge, or the card would offer a button that
// the write then refuses on evidence the reader was never shown. It says only
// WHETHER, never which — naming a project is a read of it, and this answer is
// about the companies.
//
// A SET at a time: the lane draws ten pairs, and asking per company would be
// twenty round trips on the surface a rep opens first every morning.
func (s *Store) OrganizationsCarryingLiveProjects(
	ctx context.Context, organizationIDs []ids.UUID,
) (map[ids.UUID]bool, error) {
	if err := auth.Require(ctx, "organization", principal.ActionRead); err != nil {
		return nil, err
	}
	carrying := map[ids.UUID]bool{}
	if len(organizationIDs) == 0 {
		return carrying, nil
	}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		rows, err := tx.Query(ctx, storekit.SQLf(`
			SELECT DISTINCT c.organization_id
			  `+liveProjectEdge+`
			   AND c.organization_id = ANY($%d)`, arg(organizationIDs)), args...)
		if err != nil {
			return fmt.Errorf("people: reading which companies carry live projects: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id ids.UUID
			if err := rows.Scan(&id); err != nil {
				return err
			}
			carrying[id] = true
		}
		return rows.Err()
	})
	return carrying, err
}
