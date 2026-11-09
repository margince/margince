// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

// Controller receipts past their delivery deadline, read for the sweep that
// tells the privacy operators about them. These reads join the notice table by
// dedupe key, as sendlane.go joins activity_link. Each pass then skips what was
// already raised or settled, so a long backlog cannot starve it.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// defaultReceiptPage bounds one receipt read.
const defaultReceiptPage = 200

// WithReceiptPage sets how many receipts one read answers.
func WithReceiptPage(n int) StoreOption {
	return func(s *Store) { s.receiptPage = n }
}

func (s *Store) receiptPageSize() int {
	if s.receiptPage > 0 {
		return s.receiptPage
	}
	return defaultReceiptPage
}

// overdueReceiptSQL is an overdue receipt: a controller message past its
// deadline, not sent, and not scrubbed by a privacy erasure or retention.
const overdueReceiptSQL = `o.sender_kind = 'controller' AND o.status <> 'sent'
	AND cardinality(o.redacted_fields) = 0 AND o.delivery_due_at < `

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

// UnraisedOverdueReceipts answers the overdue receipts with no notice yet under
// raisedPrefix plus their message id, oldest deadline first, bounded.
func (s *Store) UnraisedOverdueReceipts(ctx context.Context, now time.Time, raisedPrefix string) ([]OverdueReceipt, error) {
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
			 WHERE ` + overdueReceiptSQL + arg(now) + `
			   AND NOT EXISTS (SELECT 1 FROM notice n WHERE n.dedupe_key = ` + arg(raisedPrefix) + ` || o.message_id)
			 ORDER BY o.delivery_due_at, o.id
			 LIMIT ` + arg(s.receiptPageSize())
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, scanOverdueReceipt)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("comms: reading overdue controller receipts: %w", err)
	}
	return out, nil
}

func scanOverdueReceipt(row pgx.CollectableRow) (OverdueReceipt, error) {
	var r OverdueReceipt
	var subjectType *string
	var subjectID *ids.UUID
	err := row.Scan(&r.DeliveryID, &r.MessageID, &r.Status, &r.DueAt, &subjectType, &subjectID)
	if subjectType != nil && subjectID != nil {
		r.SubjectType, r.SubjectID = *subjectType, *subjectID
	}
	return r, err
}

// OverdueReceiptBacklog counts every overdue receipt, raised or not, for the log.
func (s *Store) OverdueReceiptBacklog(ctx context.Context, now time.Time) (int, error) {
	if err := requireSystemReader(ctx); err != nil {
		return 0, err
	}
	var count int
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		args := []any{now}
		query := `SELECT count(*) FROM comms_outbound o WHERE ` + overdueReceiptSQL + storekit.Placeholders(args)
		return tx.QueryRow(ctx, query, args...).Scan(&count)
	})
	if err != nil {
		return 0, fmt.Errorf("comms: counting overdue controller receipts: %w", err)
	}
	return count, nil
}

// SentReceiptsWithOpenNotice answers the message ids whose receipt has been
// sent while a notice under raisedPrefix plus that id is still open, bounded.
// Settling those notices removes them from the next read.
func (s *Store) SentReceiptsWithOpenNotice(ctx context.Context, raisedPrefix string) ([]string, error) {
	if err := requireSystemReader(ctx); err != nil {
		return nil, err
	}
	var sent []string
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var args []any
		arg := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
		prefix := arg(raisedPrefix)
		query := `
			SELECT DISTINCT o.message_id
			  FROM notice n
			  JOIN comms_outbound o ON o.message_id = substr(n.dedupe_key, length(` + prefix + `) + 1)
			 WHERE starts_with(n.dedupe_key, ` + prefix + `) AND n.retracted_at IS NULL
			   AND o.sender_kind = 'controller' AND o.status = 'sent'
			 ORDER BY o.message_id
			 LIMIT ` + arg(s.receiptPageSize())
		rows, err := tx.Query(ctx, query, args...)
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
// None of these reads exposes message contents.
func requireSystemReader(ctx context.Context) error {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalSystem {
		return apperrors.ErrPermissionDenied
	}
	return auth.Require(ctx, "activity", principal.ActionRead)
}
