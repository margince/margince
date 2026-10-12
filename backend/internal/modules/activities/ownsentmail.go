// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ClaimOwnSentMailTx rewrites a stored message as the sender's own attested
// outbound mail to recipient: a received reading from sender is turned round,
// and an unattested outbound reading to recipient gains the attestation. Capture calls it when the sender's own
// mailbox delivers, from its sent filing, a message a colleague's mailbox filed
// first as received from sender. Without it the message keeps its first
// reading, and nothing downstream sees our reply or whom we wrote to.
//
// It changes nothing unless the row still reads one of those two ways, so a
// replay, a concurrent claim or a row somebody changed meanwhile is a
// no-op rather than a second rewrite.
func ClaimOwnSentMailTx(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, sender, recipient string) error {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return err
	}
	if _, err := storekit.LockRow(ctx, tx, "activity", activityID.UUID, storekit.LiveOnly); err != nil {
		return err
	}
	inbound, outbound := string(crmcontracts.ActivityDirectionInbound), string(crmcontracts.ActivityDirectionOutbound)
	var wasDirection string
	err := tx.QueryRow(ctx, `
		UPDATE activity a
		   SET direction = $4, counterparty_email = $3, counterparty_outbound_attested = true
		  FROM (SELECT direction FROM activity WHERE id = $1) was
		 WHERE a.id = $1 AND a.kind = 'email' AND a.archived_at IS NULL
		   AND ((a.direction = $5 AND a.counterparty_email = $2)
		     OR (a.direction = $4 AND a.counterparty_email = $3 AND NOT a.counterparty_outbound_attested))
		RETURNING was.direction`,
		activityID, correspondence.Fold(sender), correspondence.Fold(recipient), outbound, inbound).Scan(&wasDirection)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("activities: reading %s as the seat's own sent mail: %w", activityID, err)
	}
	before := map[string]any{"direction": wasDirection, "counterparty_outbound_attested": false}
	if wasDirection == inbound {
		before["counterparty_email"] = correspondence.Fold(sender)
	}
	after := map[string]any{
		"direction":                      outbound,
		"counterparty_email":             correspondence.Fold(recipient),
		"counterparty_outbound_attested": true,
	}
	auditID, err := storekit.Audit(ctx, tx, "update", "activity", activityID.UUID, before, after)
	if err != nil {
		return fmt.Errorf("activities: auditing %s as the seat's own sent mail: %w", activityID, err)
	}
	attested := true
	changed := crmcontracts.PublicEventActivityChangedFields{OutboundAttested: &attested}
	if wasDirection == inbound {
		became := crmcontracts.DirectionBecameOutbound
		changed.Direction = &became
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, activityID.UUID, crmcontracts.PublicEventActivityUpdated{
		ChangedFields: changed,
	}); err != nil {
		return fmt.Errorf("activities: emitting %s as the seat's own sent mail: %w", activityID, err)
	}
	return nil
}
