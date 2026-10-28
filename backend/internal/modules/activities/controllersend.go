// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// controllerSendSQL reads, off the delivery under alias `delivery`, whether the
// installation's relay sent it and the From it was staged with.
const controllerSendSQL = `coalesce(delivery.sender_kind = 'controller', false), delivery.from_address`

// ControllerSendOf is the wire mark of a mail the installation's relay sent for
// a member, and nil for every other row.
func ControllerSendOf(controller bool, from *string) *crmcontracts.ControllerSend {
	if !controller {
		return nil
	}
	return &crmcontracts.ControllerSend{FromAddress: from}
}

// ControllerSendsFor reads the mark for each of activityIDs the relay sent.
// The caller has already applied its content gate to the ids.
func ControllerSendsFor(ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID) (map[ids.UUID]*crmcontracts.ControllerSend, error) {
	out := make(map[ids.UUID]*crmcontracts.ControllerSend, len(activityIDs))
	if len(activityIDs) == 0 {
		return out, nil
	}
	rows, err := tx.Query(ctx, `SELECT delivery.activity_id, `+controllerSendSQL+`
 FROM comms_outbound delivery WHERE delivery.activity_id = ANY($1)`, activityIDs)
	if err != nil {
		return nil, fmt.Errorf("activities: reading which mails the relay sent: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		var controller bool
		var from *string
		if err := rows.Scan(&id, &controller, &from); err != nil {
			return nil, fmt.Errorf("activities: reading which mails the relay sent: %w", err)
		}
		if mark := ControllerSendOf(controller, from); mark != nil {
			out[id] = mark
		}
	}
	return out, rows.Err()
}
