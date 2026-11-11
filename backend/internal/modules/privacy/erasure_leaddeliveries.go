// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// redactLeadDeliveries parks and scrubs queued mail whose only subject is an
// anonymized lead. Sent deliveries remain correspondence under their own
// retention rules; explicit erasure reaches them through the timeline scrub.
func redactLeadDeliveries(ctx context.Context, tx pgx.Tx, leads []ids.UUID, payloads PayloadPurger) error {
	if len(leads) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT o.activity_id
		  FROM comms_outbound o
		  JOIN activity_link l ON l.activity_id = o.activity_id
		 WHERE l.lead_id = ANY($1) AND o.status <> 'sent'`, leads)
	if err != nil {
		return fmt.Errorf("privacy: reading the deliveries waiting for an erased lead: %w", err)
	}
	waiting, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return err
	}
	return redactDeliveries(ctx, tx, waiting, erasedName, payloads)
}
