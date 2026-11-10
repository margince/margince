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

// ActivityProjection is the SELECT list every activity read scans, rendered
// with this query's own audience test.
//
// contentArm is auth.ActivityAudienceArm rendered for the caller's arguments.
// It lands in the final column, content_available, which is what decides
// whether a row's content reaches the caller or only its markers — so a reader
// that composed a different test there would be answering a different question
// about the same rows.
func ActivityProjection(ctx context.Context, contentArm string, arg func(any) int) (string, error) {
	return activityColumns(ctx, contentArm, arg)
}

// ScanActivityRow reads one row of the projection and answers the finished record.
func ScanActivityRow(ctx context.Context, rows pgx.Rows) (crmcontracts.Activity, error) {
	return scanActivity(ctx, rows)
}
