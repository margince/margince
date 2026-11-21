// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"errors"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// CountOwnReview is how many duplicate cards the reader's Mine page holds:
// pairs of records they own and contact cases one of their contacts is in.
// It asks the lane's own stores with the lane's own scope. A refused read
// counts none.
func CountOwnReview(ctx context.Context, duplicates Duplicates, cases ContactCases) (int, error) {
	reader := readerOf(ctx)
	if reader.IsZero() {
		return 0, nil
	}
	pairs, err := duplicates.CountOpen(ctx, &PairOwners{IDs: []ids.UUID{reader}})
	if err != nil && !errors.Is(err, apperrors.ErrPermissionDenied) {
		return 0, err
	}
	grouped, err := cases.CountCases(ctx, CaseQuery{Scope: CaseScopeMine, Recipient: reader})
	if err != nil && !errors.Is(err, apperrors.ErrPermissionDenied) {
		return 0, err
	}
	return pairs + grouped, nil
}
