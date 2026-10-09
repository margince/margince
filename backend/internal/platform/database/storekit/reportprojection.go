// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// FenceReportProjection serializes publication with erasure and authority changes. Under
// repeatable read, a publisher with a stale snapshot must retry before reading facts.
func FenceReportProjection(ctx context.Context, tx pgx.Tx) error {
	result, err := tx.Exec(ctx, "UPDATE report_projection_fence SET generation=generation+1 WHERE singleton")
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return errors.New("report projection fence is unavailable")
	}
	return nil
}
