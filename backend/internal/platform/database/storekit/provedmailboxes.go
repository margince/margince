// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/mailidentity"
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

// ProvedMailSeatIdentitiesSQL binds each seat's provider-attested mailbox
// addresses as a jsonb map from seat id to addresses. It reads
// ProvedMailboxLabels, the one set SeatProvedAddressTx trusts, so a declared
// alias or a delivered-to sighting proves nothing here either. An address two
// seats proved belongs to neither.
func ProvedMailSeatIdentitiesSQL(ctx context.Context, tx pgx.Tx, bind func(any) int) (string, error) {
	labels, err := ProvedMailboxLabels(ctx, tx)
	if err != nil {
		return "", err
	}
	holders := map[string]map[ids.UUID]bool{}
	for seat, values := range labels {
		for _, label := range values {
			address := mailidentity.ReaderAddress(label, "")
			if address == "" {
				continue
			}
			if holders[address] == nil {
				holders[address] = map[ids.UUID]bool{}
			}
			holders[address][seat] = true
		}
	}
	out := map[string][]string{}
	for address, seats := range holders {
		if len(seats) != 1 {
			continue
		}
		for seat := range seats {
			out[seat.String()] = append(out[seat.String()], address)
		}
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("$%d::jsonb", bind(encoded)), nil
}
