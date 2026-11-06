// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// clearRequestAddresses empties the addresses a contact merge bound the
// subject's recorded requests to. Both acts run it, because the list holds the
// addresses both acts remove. Empty, not NULL: NULL would mean the request
// covers every address the record holds.
func clearRequestAddresses(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	if _, err := tx.Exec(ctx, `
		UPDATE consent_qualifying_event SET recorded_for_addresses = '{}'
		 WHERE contact_id = $1 AND recorded_for_addresses IS NOT NULL`, id); err != nil {
		return fmt.Errorf("clear the addresses the subject's requests were bound to: %w", err)
	}
	return nil
}
