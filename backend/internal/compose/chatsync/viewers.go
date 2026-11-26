// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// Viewers is chatlinks.Viewers: each live human member is judged by the read
// admission the record's own page passes (auth.AdmitReaders), under that
// member's live grants. Members are read once and judged a hundred to a
// statement, so a large team costs a few statements, not one per member.
func Viewers(ctx context.Context, tx pgx.Tx, kind string, id ids.UUID) ([]chatlinks.Viewer, error) {
	rows, err := tx.Query(ctx, `SELECT u.id, u.email FROM app_user u
		WHERE `+workforce.LiveMemberSQL("u")+` AND NOT u.is_agent ORDER BY u.id`)
	if err != nil {
		return nil, fmt.Errorf("chat links: reading the members: %w", err)
	}
	members, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (chatlinks.Viewer, error) {
		var v chatlinks.Viewer
		return v, row.Scan(&v.ID, &v.Email)
	})
	if err != nil {
		return nil, fmt.Errorf("chat links: reading the members: %w", err)
	}
	memberIDs := make([]ids.UUID, len(members))
	for i, m := range members {
		memberIDs[i] = m.ID
	}
	principals, err := identity.MemberPrincipalsTx(ctx, tx, memberIDs)
	if err != nil {
		return nil, err
	}
	var judged []chatlinks.Viewer
	var ps []principal.Principal
	for _, m := range members {
		if p, live := principals[m.ID]; live {
			judged, ps = append(judged, m), append(ps, p)
		}
	}
	refusals, err := auth.AdmitReaders(ctx, tx, kind, id, ps)
	if err != nil {
		return nil, err
	}
	var out []chatlinks.Viewer
	for i, refusal := range refusals {
		admitted, err := auth.Admitted(refusal)
		if err != nil {
			return nil, err
		}
		if admitted {
			out = append(out, judged[i])
		}
	}
	return out, nil
}
