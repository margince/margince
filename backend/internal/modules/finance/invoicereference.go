// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package finance

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (s *Store) InvoiceReference(ctx context.Context, id ids.UUID) (ids.CompanyID, string, error) {
	if err := auth.Require(ctx, "finance", principal.ActionRead); err != nil {
		return ids.CompanyID{}, "", err
	}
	var company ids.CompanyID
	var number string
	err := s.tx(ctx, func(tx pgx.Tx) error {
		args := []any{id}
		err := tx.QueryRow(ctx, fmt.Sprintf(`SELECT company_id, coalesce(number, '') FROM finance_invoice WHERE id = $%d AND archived_at IS NULL`, len(args)), args...).Scan(&company, &number)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return err
		}
		return auth.EnsureVisible(ctx, tx, "company", company.UUID)
	})
	if err != nil {
		return ids.CompanyID{}, "", err
	}
	return company, number, nil
}
