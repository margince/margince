// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func releasedReviewPlan(ctx context.Context, tx pgx.Tx, id RunID, version int) (int, bool, error) {
	expected, released := auth.ReleasedRecordVersion(ctx, ImportRunObject, id)
	if !released {
		return version, false, nil
	}
	if version != 0 && int64(version) != expected {
		return 0, false, apperrors.ErrVersionSkew
	}
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT report FROM import_run WHERE id=@id FOR UPDATE`, pgx.StrictNamedArgs{"id": id}).Scan(&raw); err != nil {
		return 0, false, err
	}
	var report Report
	if err := json.Unmarshal(raw, &report); err != nil {
		return 0, false, apperrors.ErrVersionSkew
	}
	if report.ReviewPlan == nil || report.ReviewPlan.Version != 1 || !report.ReviewPlan.Complete {
		return 0, false, apperrors.ErrVersionSkew
	}
	actor, ok := principal.Actor(ctx)
	if !ok || report.ReviewPlan.ActorID != actor.ID {
		return 0, false, apperrors.ErrVersionSkew
	}
	return int(expected), true, nil
}
