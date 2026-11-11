// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// LockInquiryTargetTx holds the record before its task, preserving assignment lock order.
func (s *Store) LockInquiryTargetTx(ctx context.Context, tx pgx.Tx, target datasource.EntityRef) error {
	if target.Type != "lead" && target.Type != "contact" {
		return apperrors.ErrInvalidArgument
	}
	if err := auth.Require(ctx, string(target.Type), principal.ActionRead); err != nil {
		return err
	}
	if err := auth.EnsureVisibleLive(ctx, tx, string(target.Type), target.ID); err != nil {
		return err
	}
	switch target.Type {
	case "lead":
		_, err := storekit.LockRow(ctx, tx, "lead", target.ID, storekit.LiveOnly)
		return err
	case "contact":
		_, err := storekit.LockRow(ctx, tx, "contact", target.ID, storekit.LiveOnly)
		return err
	}
	return apperrors.ErrInvalidArgument
}
