// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package signals

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// scanSource is the source the signal scan stamps on what it writes.
const scanSource = "signal-scan"

// ScanSummaryReword is one stored scan summary and the sentence that replaces it.
type ScanSummaryReword struct {
	Signal ids.UUID
	From   string
	To     string
}

// RewordScanSummaryTx replaces a summary the signal scan wrote in an older
// wording. It changes the row only while it still holds From, so a summary a
// human wrote or changed is left alone. Audit-only, like
// NarrowDerivedForActivity: the event catalog has no signal.updated.
func RewordScanSummaryTx(ctx context.Context, tx pgx.Tx, in ScanSummaryReword) (bool, error) {
	if in.To == "" || in.To == in.From {
		return false, nil
	}
	lock, err := storekit.LockRow(ctx, tx, "signal", in.Signal, storekit.IncludeArchived)
	if errors.Is(err, apperrors.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var summary, source string
	if err := tx.QueryRow(ctx, `SELECT summary, source FROM signal WHERE id = $1`, in.Signal).Scan(&summary, &source); err != nil {
		return false, fmt.Errorf("signals: reading the summary to reword: %w", err)
	}
	if source != scanSource || summary != in.From {
		return false, nil
	}
	patch := storekit.NewPatch()
	patch.Set("summary", summary, in.To)
	if err := patch.ApplyLocked(ctx, tx, lock); err != nil {
		return false, fmt.Errorf("signals: rewording a scan summary: %w", err)
	}
	if _, err := storekit.Audit(ctx, tx, "update", "signal", in.Signal, patch.Before(), patch.After()); err != nil {
		return false, fmt.Errorf("signals: auditing a reworded scan summary: %w", err)
	}
	return true, nil
}
