// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package finance

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type MergeReference struct {
	CompanyID  ids.UUID
	Number     string
	PaidAt     *time.Time
	ArchivedAt *time.Time
}

func (s *Store) MergeReference(ctx context.Context, kind string, id ids.UUID) (MergeReference, error) {
	var out MergeReference
	if err := auth.Require(ctx, "finance", principal.ActionRead); err != nil {
		return out, err
	}
	query := ""
	switch kind {
	case "invoice":
		query = "SELECT company_id, coalesce(number, ''), NULL::timestamptz, archived_at FROM finance_invoice WHERE id = @id"
	case "payment":
		query = "SELECT p.company_id, coalesce(i.number, ''), p.paid_at, p.archived_at FROM finance_payment p LEFT JOIN finance_invoice i ON i.id = p.invoice_id AND i.company_id = p.company_id WHERE p.id = @id"
	case "customer_link":
		if err := auth.Require(ctx, objectIntegrations, principal.ActionRead); err != nil {
			return out, err
		}
		query = "SELECT l.company_id, coalesce(c.display_name, l.external_customer_id), NULL::timestamptz, l.archived_at FROM finance_customer_link l LEFT JOIN finance_external_customer c ON c.connection_id = l.connection_id AND c.external_customer_id = l.external_customer_id WHERE l.id = @id"
	default:
		return out, apperrors.ErrInvalidArgument
	}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, query, pgx.StrictNamedArgs{"id": id}).Scan(&out.CompanyID, &out.Number, &out.PaidAt, &out.ArchivedAt); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperrors.ErrNotFound
			}
			return fmt.Errorf("read finance merge reference: %w", err)
		}
		return auth.EnsureVisible(ctx, tx, "company", out.CompanyID)
	})
	if err != nil {
		return MergeReference{}, err
	}
	return out, nil
}
