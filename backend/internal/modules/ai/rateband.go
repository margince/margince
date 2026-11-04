// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// What a rate write inherits from the sheet: the model's lane and its
// long-context band.
//
// Both belong to the model rather than to one dated price. So a write that
// names neither keeps what the model carries. Only a write that says so drops
// the band: a base-price edit that lost it would under-price every long call.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ModelRateBand is a long-context band in the sheet's USD-per-MTok strings.
type ModelRateBand struct {
	AboveTokens                                      int
	InputUsd, OutputUsd, CacheReadUsd, CacheWriteUsd string
}

// bandUSD renders a stored band, or nil for a flat price.
func bandUSD(b *ContextBand) *ModelRateBand {
	if b == nil {
		return nil
	}
	return &ModelRateBand{
		AboveTokens: b.AboveTokens,
		InputUsd:    MicroUSDToUsdPerMTok(b.InputPerMTokMicroUSD), OutputUsd: MicroUSDToUsdPerMTok(b.OutputPerMTokMicroUSD),
		CacheReadUsd: MicroUSDToUsdPerMTok(b.CacheReadPerMTokMicroUSD), CacheWriteUsd: MicroUSDToUsdPerMTok(b.CacheWritePerMTokMicroUSD),
	}
}

// bandMicro validates a written band, as modelRateMicroUSD validates the base.
func bandMicro(b *ModelRateBand) (*ContextBand, error) {
	if b == nil {
		return nil, nil //nolint:nilnil // no band is a flat price, not an error
	}
	if b.AboveTokens <= 0 {
		return nil, rateInvalid("long_context.above_tokens", "rate_band_line", "above_tokens must be positive")
	}
	in, out, cacheRead, cacheWrite, err := modelRateMicroUSD(SetModelRateInput{
		InputUsd: b.InputUsd, OutputUsd: b.OutputUsd, CacheReadUsd: b.CacheReadUsd, CacheWriteUsd: b.CacheWriteUsd,
	})
	if err != nil {
		return nil, err
	}
	return &ContextBand{
		AboveTokens:          b.AboveTokens,
		InputPerMTokMicroUSD: in, OutputPerMTokMicroUSD: out,
		CacheReadPerMTokMicroUSD: cacheRead, CacheWritePerMTokMicroUSD: cacheWrite,
	}, nil
}

// sameBand reports whether two bands price alike.
func sameBand(a, b *ModelRateBand) bool {
	am, errA := bandMicro(a)
	bm, errB := bandMicro(b)
	if errA != nil || errB != nil || (am == nil) != (bm == nil) {
		return false
	}
	return am == nil || *am == *bm
}

// filedBand resolves the band this write stores: the caller's, none when the
// caller cleared it, otherwise the band the model's latest row carries.
//
// Read under the model's write-identity lock, as filedLane is.
func filedBand(ctx context.Context, tx pgx.Tx, p preparedModelRate) (*ContextBand, error) {
	if p.band != nil || p.clearBand {
		return p.band, nil
	}
	var band scannedBand
	err := tx.QueryRow(ctx, `
		SELECT `+longContextColumns("r")+` FROM ai_model_rate r
		WHERE r.provider = $1 AND r.model_id = $2
		ORDER BY r.effective_date DESC LIMIT 1`,
		p.provider, p.modelID).Scan(&band.above, &band.input, &band.output, &band.cacheRead, &band.cacheWrite)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // a model the sheet has never seen has no band
	}
	if err != nil {
		return nil, fmt.Errorf("read filed long-context band: %w", err)
	}
	return band.band(), nil
}

// bandImage adds a band to an audit image, so a band that moved shows in the diff.
func bandImage(image map[string]any, b *ContextBand) map[string]any {
	if b != nil {
		image["long_context_above_tokens"] = b.AboveTokens
		image["long_context_input_microusd"] = b.InputPerMTokMicroUSD
		image["long_context_output_microusd"] = b.OutputPerMTokMicroUSD
		image["long_context_cache_read_microusd"] = b.CacheReadPerMTokMicroUSD
		image["long_context_cache_write_microusd"] = b.CacheWritePerMTokMicroUSD
	}
	return image
}

// filedLane resolves what this write files the model as. That is the caller's
// lane when they named one, otherwise the lane of the model's latest row. A
// model the sheet has never seen is a chat model, as all but a handful are.
//
// Read under the model's write-identity lock, so a concurrent write cannot file
// the model differently before the upsert.
func filedLane(ctx context.Context, tx pgx.Tx, p preparedModelRate) (Lane, error) {
	if p.lane != "" {
		return p.lane, nil
	}
	var lane Lane
	err := tx.QueryRow(ctx, `
		SELECT lane FROM ai_model_rate
		WHERE provider = $1 AND model_id = $2
		ORDER BY effective_date DESC LIMIT 1`,
		p.provider, p.modelID).Scan(&lane)
	if errors.Is(err, pgx.ErrNoRows) {
		return LaneChat, nil
	}
	if err != nil {
		return "", fmt.Errorf("read filed lane: %w", err)
	}
	return lane, nil
}
