// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// frozenBar is one element of weekly_review.outlook_movement.
type frozenBar struct {
	PeriodKind string `json:"period_kind"`
	Bar        string `json:"bar"`
	DeltaMinor int64  `json:"delta_minor"`
}

// wireExactMinor is the largest amount a JSON reader gets back unchanged.
const wireExactMinor = 1<<53 - 1

// frozenBars is the bars as the review stores them. The column is one jsonb
// value with no CHECK on its elements, so the vocabularies are held here.
// A second bar for one horizon is dropped: the bridge sums what it is given.
func frozenBars(movements []Movement) ([]frozenBar, error) {
	bars := make([]frozenBar, 0, len(movements))
	for _, move := range movements {
		if _, known := forecasting.PeriodKindOf(move.PeriodKind); !known || move.PeriodKind == "" {
			return nil, fmt.Errorf("weekly: %q is not a horizon a bar can belong to", move.PeriodKind)
		}
		if !slices.Contains(barOrder, move.Bar) {
			return nil, fmt.Errorf("weekly: %q is not a bar of the bridge", move.Bar)
		}
		if move.DeltaMinor > wireExactMinor || move.DeltaMinor < -wireExactMinor {
			return nil, fmt.Errorf("weekly: the %s bar moved more than a reader can hold", move.Bar)
		}
		taken := slices.ContainsFunc(bars, func(bar frozenBar) bool {
			return bar.PeriodKind == move.PeriodKind && bar.Bar == move.Bar
		})
		if !taken {
			bars = append(bars, frozenBar{PeriodKind: move.PeriodKind, Bar: move.Bar, DeltaMinor: move.DeltaMinor})
		}
	}
	return bars, nil
}

// freezeMovement stores the bars on a review that has none. A retried job
// finds them there and writes nothing, so no bar is counted twice.
func freezeMovement(ctx context.Context, tx pgx.Tx, reviewID ids.UUID, movements []Movement) error {
	if len(movements) == 0 {
		return nil
	}
	bars, err := frozenBars(movements)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(bars)
	if err != nil {
		return fmt.Errorf("weekly: encoding the bridge: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE weekly_review SET outlook_movement = $2::jsonb
		 WHERE id = $1 AND outlook_movement IS NULL`, reviewID, string(encoded)); err != nil {
		return fmt.Errorf("weekly: freezing the bridge: %w", err)
	}
	return nil
}

// readFrozenBars reads the bars a review froze. A review with no bridge has none.
func readFrozenBars(ctx context.Context, tx pgx.Tx, reviewID ids.UUID) ([]frozenBar, error) {
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT outlook_movement FROM weekly_review WHERE id = $1`, reviewID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && raw == nil) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("weekly: reading the frozen bridge: %w", err)
	}
	var bars []frozenBar
	if err := json.Unmarshal(raw, &bars); err != nil {
		return nil, fmt.Errorf("weekly: decoding the frozen bridge: %w", err)
	}
	return bars, nil
}
