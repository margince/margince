// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ProvedMailboxLabels reads labels attested by a connected OAuth provider, never typed IMAP labels.
func ProvedMailboxLabels(ctx context.Context, tx pgx.Tx) (map[ids.UUID][]string, error) {
	rows, err := tx.Query(ctx, `SELECT c.user_id, c.account_label FROM capture_connection c WHERE `+provedMailboxClause("c"))
	if err != nil {
		return nil, fmt.Errorf("reading proved mailbox labels: %w", err)
	}
	defer rows.Close()
	out := map[ids.UUID][]string{}
	for rows.Next() {
		var seat ids.UUID
		var label string
		if err := rows.Scan(&seat, &label); err != nil {
			return nil, err
		}
		out[seat] = append(out[seat], label)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func provedMailboxClause(alias string) string {
	alias = pgx.Identifier{alias}.Sanitize()
	return fmt.Sprintf(`(%[1]s.archived_at IS NULL AND coalesce(%[1]s.account_label,'') <> ''
 AND %[1]s.status='connected' AND %[1]s.provider IN ('gmail','gcal','graph','graphcal'))`, alias)
}
