// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
)

// DoneRows reads the rows the acting reader marked done among these, beside
// the pins the same store keeps.
func (p attentionPins) DoneRows(ctx context.Context, among []attention.RowRef) (map[attention.RowRef]time.Time, error) {
	var out map[attention.RowRef]time.Time
	err := database.WithWorkspaceTx(ctx, p.pool, func(tx pgx.Tx) error {
		done, err := p.store.DoneRows(ctx, tx, worklistRows(among))
		if err != nil {
			return err
		}
		out = make(map[attention.RowRef]time.Time, len(done))
		for ref, at := range done {
			out[attention.RowRef{Source: ref.Source, RowID: ref.RowID}] = at
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// MarkDone writes the Done the attention service has already checked.
func (p attentionPins) MarkDone(ctx context.Context, rows []attention.RowRef) error {
	return p.store.MarkWorklistRowsDone(ctx, worklistRows(rows))
}

// ReopenDone takes the reader's own Done back.
func (p attentionPins) ReopenDone(ctx context.Context, rows []attention.RowRef) error {
	return p.store.ReopenWorklistRows(ctx, worklistRows(rows))
}

func worklistRows(rows []attention.RowRef) []activities.WorklistRowRef {
	out := make([]activities.WorklistRowRef, 0, len(rows))
	for _, row := range rows {
		out = append(out, activities.WorklistRowRef{Source: row.Source, RowID: row.RowID})
	}
	return out
}
