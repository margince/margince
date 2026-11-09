// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// redactLeadDeliveries scrubs the deliveries still waiting to go out to a lead
// twin this erasure anonymized. Such a delivery is filed under the lead alone,
// as a lead's opt-out receipt is, and may name an address the contact never
// held. So neither the contact's timeline nor its addresses reach it, and
// unscrubbed it would be sent to the person this erasure was asked to forget.
//
// A sent delivery is left to the timeline scrubs: what it said is a record of
// what happened. A waiting one is only an intent, and it is parked here.
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
