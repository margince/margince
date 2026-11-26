// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The projection and its scan are handed to readers that build their own query.
// No reader then keeps a copy of the column list that fails to grow.

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// ActivityProjection is the SELECT list every activity read scans. contentArm
// is the audience test, passed through ActivityContentFrom so it is computed
// once per row; it fills content_available and gates body and raw. Bound a
// paged source before adding the decision.
func ActivityProjection(ctx context.Context, contentArm string, arg func(any) int) (string, error) {
	return activityColumns(ctx, contentArm, arg)
}

// ScanActivityRow reads one row of the projection and answers the finished record.
func ScanActivityRow(ctx context.Context, rows pgx.Rows) (crmcontracts.Activity, error) {
	return scanActivity(ctx, rows)
}
