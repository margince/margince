// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// overdueReceiptBatch bounds one read of overdue receipts.
const overdueReceiptBatch = 200

// OverdueReceipt is one controller receipt past its delivery deadline and not
// sent. It names the stop act by message id and the subject the receipt is
// filed under, and carries no message content.
type OverdueReceipt struct {
	DeliveryID  ids.UUID
	MessageID   string
	Status      string
	DueAt       time.Time
	SubjectType string
	SubjectID   ids.UUID
}

// OverdueControllerReceipts answers the controller receipts past their
// deadline and not sent, oldest deadline first, bounded.
//
// The subject is read off the receipt's activity link. activity_link belongs to
// the activities module, and this system read joins it as sendlane.go does.
func (s *Store) OverdueControllerReceipts(ctx context.Context, now time.Time) ([]OverdueReceipt, error) {
	if err := requireSystemReader(ctx); err != nil {
		return nil, err
	}
	var out []OverdueReceipt
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var args []any
		arg := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
		query := `
			SELECT o.id, o.message_id, o.status, o.delivery_due_at, l.entity_type, l.subject_id
			  FROM comms_outbound o
			  LEFT JOIN LATERAL (
			    SELECT al.entity_type, COALESCE(al.contact_id, al.lead_id) AS subject_id
			      FROM activity_link al
			     WHERE al.activity_id = o.activity_id AND al.entity_type IN ('contact', 'lead')
			     ORDER BY al.entity_type LIMIT 1
			  ) l ON true
			 WHERE o.sender_kind = 'controller' AND o.status <> 'sent'
			   AND o.delivery_due_at < ` + arg(now) + `
			 ORDER BY o.delivery_due_at, o.id
			 LIMIT ` + arg(overdueReceiptBatch)
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (OverdueReceipt, error) {
			var r OverdueReceipt
			var subjectType *string
			var subjectID *ids.UUID
			err := row.Scan(&r.DeliveryID, &r.MessageID, &r.Status, &r.DueAt, &subjectType, &subjectID)
			if subjectType != nil && subjectID != nil {
				r.SubjectType, r.SubjectID = *subjectType, *subjectID
			}
			return r, err
		})
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("comms: reading overdue controller receipts: %w", err)
	}
	return out, nil
}

// SentControllerReceipts answers which of these message ids name a controller
// receipt that has been sent.
func (s *Store) SentControllerReceipts(ctx context.Context, messageIDs []string) ([]string, error) {
	if err := requireSystemReader(ctx); err != nil {
		return nil, err
	}
	if len(messageIDs) == 0 {
		return nil, nil
	}
	var sent []string
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT message_id FROM comms_outbound
			 WHERE sender_kind = 'controller' AND status = 'sent' AND message_id = ANY($1)`, messageIDs)
		if err != nil {
			return err
		}
		sent, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("comms: reading sent controller receipts: %w", err)
	}
	return sent, nil
}

// requireSystemReader admits the system actor that runs the receipt sweep.
// Neither read exposes message contents.
func requireSystemReader(ctx context.Context) error {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalSystem {
		return apperrors.ErrPermissionDenied
	}
	return auth.Require(ctx, "activity", principal.ActionRead)
}
