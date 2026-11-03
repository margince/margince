// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func (e *Eraser) purgePartyProjections(ctx context.Context, tx pgx.Tx, emails []string) error {
	if len(emails) == 0 {
		return nil
	}
	if e.purgeRawCaptures != nil {
		return e.purgeRawCaptures.PurgePartySourcesForEmailsTx(ctx, tx, emails)
	}
	var held bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM capture_party_source WHERE recipients && ARRAY(SELECT lower(btrim(address)) FROM unnest(@emails::text[]) address) OR policy_addresses && ARRAY(SELECT lower(btrim(address)) FROM unnest(@emails::text[]) address))`, pgx.NamedArgs{"emails": emails}).Scan(&held); err != nil {
		return err
	}
	if held {
		return ErrRetentionSeamMissing
	}
	return nil
}
