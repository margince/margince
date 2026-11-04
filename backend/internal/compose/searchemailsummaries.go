// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// searchEmailSummaries is the email row a search hit carries. A search page has
// no reading clock of its own, so a hit's move is judged now.
func searchEmailSummaries(
	ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID,
) (map[ids.UUID]crmcontracts.EmailSummary, error) {
	return activities.EmailSummariesByIDBatch(ctx, tx, activityIDs, time.Now())
}
