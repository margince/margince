// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auditattribution"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// WithCreators stamps on each row of a page who recorded it, read from its
// create audit row the way a contact page reads its own. captured_by names only
// an agent's passport. Without this, a note logged through an agent names
// neither the colleague it worked for nor the tool.
func WithCreators(ctx context.Context, tx pgx.Tx, page []crmcontracts.Activity) error {
	activityIDs := make([]ids.UUID, len(page))
	for i := range page {
		activityIDs[i] = ids.UUID(page[i].Id)
	}
	creators, err := auditattribution.CreatorsTx(ctx, tx, "activity", activityIDs)
	if err != nil {
		return err
	}
	for i := range page {
		if creator, found := creators[activityIDs[i]]; found {
			page[i].CreatedBy = creator.Wire()
		}
	}
	return nil
}
