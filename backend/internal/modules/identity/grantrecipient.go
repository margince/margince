// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// ValidateRecordGrantRecipientTx checks the recipient even when a bulk share has no eligible records.
func ValidateRecordGrantRecipientTx(ctx context.Context, tx pgx.Tx, in CreateGrantInput) error {
	if err := grantRecipientExists(ctx, tx, in); err != nil {
		return err
	}
	return refuseWriteGrantToReadSeat(ctx, tx, in)
}

func grantRecipientExists(ctx context.Context, tx pgx.Tx, in CreateGrantInput) error {
	subjectTable := "app_user"
	if in.SubjectType == grantSubjectTeam {
		subjectTable = grantSubjectTeam
	}
	var subjectExists bool
	if err := tx.QueryRow(ctx,
		storekit.SQLf(`SELECT EXISTS (SELECT 1 FROM %s WHERE id = $1)`, subjectTable),
		in.SubjectID).Scan(&subjectExists); err != nil {
		return err
	}
	if !subjectExists {
		return apperrors.ErrNotFound
	}
	return nil
}
