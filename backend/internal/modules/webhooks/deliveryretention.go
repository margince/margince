// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webhooks

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A delivery's rendered body is a copy of the record it was about. The row
// stays for the delivery log and the (subscription, event) dedupe; only the
// body goes, and payload_pruned_at says when.

// erasedSubjectReason is what an undelivered row says once its subject was
// erased. Fixed text, because last_error is shown to the operator.
const erasedSubjectReason = "the record this delivery was about was erased, so it was not sent"

// DeliveryBody is one stored body and the record its delivery was about. The
// record is empty on a row written before deliveries named one.
type DeliveryBody struct {
	ID         ids.UUID
	EntityType string
	EntityID   ids.UUID
}

// DeliveryBodiesPastTx is up to limit bodies after the cursor whose delivery
// ended before cutoff: delivered, dead-lettered or revoked. A pending or
// retrying delivery keeps its body, because it still has to be sent.
func DeliveryBodiesPastTx(ctx context.Context, tx pgx.Tx, cutoff time.Time, after ids.UUID, limit int) ([]DeliveryBody, error) {
	rows, err := tx.Query(ctx, `SELECT id, coalesce(entity_type, ''), coalesce(entity_id, '00000000-0000-0000-0000-000000000000')
		FROM webhook_delivery
		WHERE payload_pruned_at IS NULL AND payload <> ''
		  AND status IN ('delivered', 'dead_lettered', 'visibility_revoked')
		  AND CASE status WHEN 'delivered' THEN delivered_at WHEN 'dead_lettered' THEN dead_lettered_at
		      ELSE updated_at END < @cutoff
		  AND id > @after
		ORDER BY id LIMIT @limit`, pgx.StrictNamedArgs{"cutoff": cutoff, "after": after, "limit": limit})
	if err != nil {
		return nil, fmt.Errorf("webhooks: reading delivery bodies past their window: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[DeliveryBody])
}

// ClearDeliveryBodiesTx empties the bodies of ended deliveries. A delivery
// that is pending or retrying again by now keeps its body.
func ClearDeliveryBodiesTx(ctx context.Context, tx pgx.Tx, deliveries []ids.UUID) error {
	if _, err := tx.Exec(ctx, `UPDATE webhook_delivery SET payload = '', payload_pruned_at = now()
		WHERE id = ANY(@deliveries) AND payload_pruned_at IS NULL
		  AND status IN ('delivered', 'dead_lettered', 'visibility_revoked')`,
		pgx.StrictNamedArgs{"deliveries": deliveries}); err != nil {
		return fmt.Errorf("webhooks: clearing delivery bodies past their window: %w", err)
	}
	return nil
}

// ScrubSubjectDeliveriesTx empties every body about the erased records, and
// cancels the deliveries still to be sent: a retry would otherwise send the
// erased subject's data after the erasure certified it destroyed.
func ScrubSubjectDeliveriesTx(ctx context.Context, tx pgx.Tx, entityType string, records []ids.UUID) error {
	if _, err := tx.Exec(ctx, `UPDATE webhook_delivery
		SET payload = '', payload_pruned_at = coalesce(payload_pruned_at, now()),
		    status = CASE WHEN status IN ('pending', 'retrying') THEN 'visibility_revoked' ELSE status END,
		    last_error = CASE WHEN status IN ('pending', 'retrying') THEN @reason ELSE last_error END,
		    next_retry_at = CASE WHEN status IN ('pending', 'retrying') THEN NULL ELSE next_retry_at END,
		    claimed_until = CASE WHEN status IN ('pending', 'retrying') THEN NULL ELSE claimed_until END
		WHERE entity_type = @type AND entity_id = ANY(@records)
		  AND (payload <> '' OR status IN ('pending', 'retrying'))`,
		pgx.StrictNamedArgs{"type": entityType, "records": records, "reason": erasedSubjectReason}); err != nil {
		return fmt.Errorf("webhooks: clearing the erased subject's delivery bodies: %w", err)
	}
	return nil
}
