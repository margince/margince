// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// SeatHistoryBaselines is what one seat's own import of a captured message proves.
type SeatHistoryBaselines struct {
	// Baselines has one entry per mailbox proven to have delivered the message
	// to this seat. A nil entry is a mailbox without a baseline.
	Baselines []*HistoryBaseline
	// SourceAt is the sent time this seat's own copy stated, nil when it stated none.
	SourceAt *time.Time
	// FirstCapture is true when this seat's capture created the activity row,
	// so the stored original is this seat's copy.
	FirstCapture bool
}

// Excludes reports whether every proven mailbox withholds a message dated at.
// With no proven mailbox the baseline does not apply.
func (s SeatHistoryBaselines) Excludes(at time.Time) (bool, error) {
	return allExclude(s.Baselines, at)
}

// SeatHistoryBaselinesTx reads one seat's import of a message and the baselines
// of the mailboxes that delivered it.
//
// A mailbox is named by provider and generation, so a rebind to another account
// never claims mail the previous account delivered.
func SeatHistoryBaselinesTx(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, seat ids.UUID) (SeatHistoryBaselines, error) {
	var out SeatHistoryBaselines
	var delivered []string
	var importedAt time.Time
	var legacyProvider string
	var legacy bool
	err := tx.QueryRow(ctx, `
		SELECT ci.source_occurred_at, ci.delivered_by, ci.imported_at,
		       a.captured_by = 'connector:' || split_part(a.captured_by, ':', 2) || ':' || ci.user_id::text,
		       split_part(a.captured_by, ':', 2),
		       NOT EXISTS (SELECT 1 FROM unnest(ci.delivered_by) AS d(mailbox)
		                    WHERE split_part(d.mailbox, ':', 1) = split_part(a.captured_by, ':', 2))
		  FROM capture_import ci
		  JOIN activity a ON a.id = ci.activity_id AND a.restricted_at IS NULL
		 WHERE ci.activity_id = $1 AND ci.user_id = $2`, activityID, seat).
		Scan(&out.SourceAt, &delivered, &importedAt, &out.FirstCapture, &legacyProvider, &legacy)
	if errors.Is(err, pgx.ErrNoRows) {
		return SeatHistoryBaselines{}, nil
	}
	if err != nil {
		return SeatHistoryBaselines{}, fmt.Errorf("capture: reading %s's import of %s: %w", seat, activityID, err)
	}
	if out.Baselines, err = deliveredBaselinesTx(ctx, tx, seat, delivered); err != nil {
		return SeatHistoryBaselines{}, err
	}
	if out.FirstCapture && legacy {
		legacy, err := legacyBaselineTx(ctx, tx, seat, legacyProvider, importedAt)
		if err != nil {
			return SeatHistoryBaselines{}, err
		}
		out.Baselines = append(out.Baselines, legacy...)
	}
	return out, nil
}

func deliveredBaselinesTx(ctx context.Context, tx pgx.Tx, seat ids.UUID, delivered []string) ([]*HistoryBaseline, error) {
	rows, err := tx.Query(ctx, `
		SELECT c.history_baseline_date, c.history_baseline_tz
		  FROM unnest($2::text[]) AS d(mailbox)
		  LEFT JOIN capture_connection c ON c.user_id = $1 AND c.provider || ':' || c.generation = d.mailbox`, seat, delivered)
	if err != nil {
		return nil, fmt.Errorf("capture: reading the history baselines of %s: %w", seat, err)
	}
	baselines, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*HistoryBaseline, error) {
		var c historyBaselineColumns
		err := row.Scan(&c.date, &c.timezone)
		return c.baseline(), err
	})
	if err != nil {
		return nil, fmt.Errorf("capture: reading the history baselines of %s: %w", seat, err)
	}
	return baselines, nil
}

// legacyBaselineTx answers for an import recorded before mailboxes were named on
// it. The connection counts only when no rebind happened after the import; a nil
// entry is a mailbox that excludes nothing.
func legacyBaselineTx(ctx context.Context, tx pgx.Tx, seat ids.UUID, provider string, importedAt time.Time) ([]*HistoryBaseline, error) {
	var c historyBaselineColumns
	var sameAccount bool
	err := tx.QueryRow(ctx, `
		SELECT history_baseline_date, history_baseline_tz, account_bound_at IS NULL OR account_bound_at <= $3
		  FROM capture_connection WHERE user_id = $1 AND provider = $2`, seat, provider, importedAt).
		Scan(&c.date, &c.timezone, &sameAccount)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !sameAccount) {
		return []*HistoryBaseline{nil}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("capture: reading the %s baseline of %s: %w", provider, seat, err)
	}
	return []*HistoryBaseline{c.baseline()}, nil
}

func allExclude(baselines []*HistoryBaseline, at time.Time) (bool, error) {
	if len(baselines) == 0 {
		return false, nil
	}
	for _, b := range baselines {
		if b == nil {
			return false, nil
		}
		excluded, err := b.Excludes(at)
		if err != nil || !excluded {
			return false, err
		}
	}
	return true, nil
}
