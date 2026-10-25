// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// StampActivitiesWritable sets Writable on a page of activities from the
// question the update path asks. That is the object grant, then the row
// authority auth.ActivityWritableSubset answers for the whole page at once.
// The list, the single read and the contact timeline each call it.
func StampActivitiesWritable(ctx context.Context, tx pgx.Tx, page []crmcontracts.Activity) error {
	may := make(map[ids.UUID]bool)
	if mayMutate(ctx) && auth.Allows(ctx, "activity", principal.ActionUpdate) && len(page) > 0 {
		named := make([]ids.UUID, len(page))
		for i := range page {
			named[i] = ids.UUID(page[i].Id)
		}
		var err error
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

// mayMutate is the seat ceiling the mutation admission applies before any
// grant: a read seat changes nothing, whatever its role says. The system
// principal holds no seat and is trusted by construction, as in auth.Require.
func mayMutate(ctx context.Context) bool {
	p, ok := principal.Actor(ctx)
	return ok && (p.Type == principal.PrincipalSystem || p.SeatType.CanMutate())
}
