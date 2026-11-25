// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// replacedModelRate reads the price this write would overwrite, if any. The
// second admission check needs insert-vs-overwrite, and an overwrite's audit
// owes the ledger the prices it displaced. The before image has the after
// image's keys, so the two diff to the buckets that moved. It runs under the
// model's write-identity lock, so no writer inserts the row before the upsert.
func replacedModelRate(ctx context.Context, tx pgx.Tx, p preparedModelRate, effDate time.Time) (before map[string]any, replacing bool, err error) {
	var (
		in, out, cacheRead, cacheWrite int64
		lane                           Lane
		source                         RateSource
		band                           scannedBand
	)
	err = tx.QueryRow(ctx, `
		SELECT r.input_per_mtok_microusd, r.output_per_mtok_microusd,
		       r.cache_read_per_mtok_microusd, r.cache_write_per_mtok_microusd, r.lane, r.source,
		       `+longContextColumns("r")+`
		FROM ai_model_rate r
		WHERE r.provider = $1 AND r.model_id = $2 AND r.effective_date = $3`,
		p.provider, p.modelID, effDate).Scan(&in, &out, &cacheRead, &cacheWrite, &lane, &source,
		&band.above, &band.input, &band.output, &band.cacheRead, &band.cacheWrite)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read replaced ai_model_rate: %w", err)
	}
	prior := preparedModelRate{
		provider: p.provider, modelID: p.modelID,
		input: in, output: out, cacheRead: cacheRead, cacheWrite: cacheWrite,
		lane: lane, source: source, band: band.band(),
	}
	return modelRateImage(prior, effDate), true, nil
}

// modelRateImage renders one audit image of a price row. Before and after
// share it, so the pair diffs to the buckets that moved, not to spelling.
func modelRateImage(r preparedModelRate, effDate time.Time) map[string]any {
	return bandImage(map[string]any{
		"provider": r.provider, "model_id": r.modelID,
		"input_microusd": r.input, "output_microusd": r.output,
		"cache_read_microusd": r.cacheRead, "cache_write_microusd": r.cacheWrite,
		"date": effDate, "lane": string(r.lane), rateSourceField: string(r.source),
	}, r.band)
}
