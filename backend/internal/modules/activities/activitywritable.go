// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// StampActivitiesWritable sets Writable on a page of activities from the
// question the update path asks: the object grant, then the row authority
// that auth.ActivityWritableSubset answers for the whole page in one statement.
// Every read that returns activities calls it, so the list, the single read and
// the contact timeline cannot disagree about one row.
func StampActivitiesWritable(ctx context.Context, tx pgx.Tx, page []crmcontracts.Activity) error {
	may := make(map[ids.UUID]bool)
	err := auth.Require(ctx, "activity", principal.ActionUpdate)
	if err != nil && !errors.Is(err, apperrors.ErrPermissionDenied) {
		return err
	}
	if err == nil && len(page) > 0 {
		named := make([]ids.UUID, len(page))
		for i := range page {
			named[i] = ids.UUID(page[i].Id)
		}
		if may, err = auth.ActivityWritableSubset(ctx, tx, named); err != nil {
			return err
		}
	}
	for i := range page {
		w := may[ids.UUID(page[i].Id)]
		page[i].Writable = &w
	}
	return nil
}
