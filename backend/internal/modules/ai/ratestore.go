// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
)

// RateStore is the ai_model_rate price sheet — the fx_rate-style
// as-of-date lookup (ADR-0067). It rides the workspace GUC transaction
// like every tenant read: RLS alone decides which workspace's rates a
// caller can see, the same as fx_rate.
type RateStore struct {
	// db binds the workspace this store runs for (ADR-0091 §9 step 3).
	db *database.DB
	// clock is the "today" source for effective-dated writes; injected so
	// append-forward date validation is deterministic in tests.
	clock func() time.Time
}

// NewRateStore constructs the RateStore over pool.
func NewRateStore(db *database.DB) *RateStore {
	return &RateStore{db: db, clock: time.Now}
}

// SetClock overrides the "today" source (tests only). Returns the store
// for chaining.
func (s *RateStore) SetClock(clock func() time.Time) *RateStore {
	s.clock = clock
	return s
}

// RateFor resolves the rate effective on day for (provider, modelID) —
// the latest row whose effective_date is on or before day, mirroring
// fx_rate's as-of-date resolution (deal_advance.go). No matching row is
// not an error: it means the call is UNPRICED, a materially different
// signal from a 0 price (price-on-read; never fabricate a price), so the
// caller gets (nil, nil) and decides what "unpriced" means to it.
func (s *RateStore) RateFor(ctx context.Context, provider, modelID string, day time.Time) (*ModelRate, error) {
	var rate *ModelRate
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var e error
		rate, e = rateForInTx(ctx, tx, provider, modelID, day)
		return e
	})
	if err != nil {
		return nil, err
	}
	return rate, nil
}

func rateForInTx(ctx context.Context, tx pgx.Tx, provider, modelID string, day time.Time) (*ModelRate, error) {
	var rate ModelRate
	var band scannedBand
	from, to := rateFallbacks()
	err := tx.QueryRow(ctx, `
		SELECT r.provider, r.model_id, r.input_per_mtok_microusd, r.output_per_mtok_microusd,
		       r.cache_read_per_mtok_microusd, r.cache_write_per_mtok_microusd, r.effective_date,
		       `+longContextColumns("r")+`
		FROM (`+rateMatch("$1", "$2", "$3", 4, 5)+`) r`,
		provider, modelID, day, from, to).Scan(
		&rate.Provider, &rate.ModelID, &rate.InputPerMTokMicroUSD, &rate.OutputPerMTokMicroUSD,
		&rate.CacheReadPerMTokMicroUSD, &rate.CacheWritePerMTokMicroUSD, &rate.EffectiveDate,
		&band.above, &band.input, &band.output, &band.cacheRead, &band.cacheWrite,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // no matching rate row IS the "unpriced" answer, not an error — price-on-read never fabricates a price
	}
	if err != nil {
		return nil, fmt.Errorf("ai: rate lookup: %w", err)
	}
	rate.LongContext = band.band()
	return &rate, nil
}

// CostReport prices the [from, to) window's ai_call rows against their
// as-of-date rate and sums per (calendar day, task, tier). It is the one money
// computation that runs at read time; the router, meter and adapters never
// compute a cost.
//
// It is one SQL statement. A LATERAL join picks each row's as-of-date rate, as
// RateFor does, so the window prices in one query. The arithmetic mirrors
// PriceCall: the same floor, zero-cache fallback, long-context band and
// truncating /1000000. GROUP BY day, task and tier matches the /ai/usage wire
// grain, so the handler attaches each line to one row. A shared task total
// spread over every tier would double-count when a client sums the rows.
//
// Two kinds of row spend nothing and are never counted unpriced, because
// they are free BY CONSTRUCTION, not merely unrated: a cache_hit (served
// from the router's result cache, no provider call happened) and a row
// with zero provider usage (tokens_in = 0 AND tokens_out = 0 — a call
// that failed before the provider was ever reached). Every other row
// with no matching rate row counts into its (day, task, tier)'s
// UnpricedCalls — visible, never a silent 0 (global constraint: cost is
// transparency, never a gate).
func (s *RateStore) CostReport(ctx context.Context, from, to time.Time) ([]DayCost, error) {
	var report []DayCost
	fallbackFrom, fallbackTo := rateFallbacks()
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT
			  ac.occurred_at::date AS day,
			  ac.task,
			  ac.tier,
			  COALESCE(SUM(`+callCostMicroUSD("ac")+`), 0) AS cost_microusd,
			  COUNT(*) FILTER (WHERE `+callUnpriced("ac")+`) AS unpriced_calls
			FROM ai_call ac
			LEFT JOIN LATERAL (`+pricedRate("ac", 3, 4)+`) r ON true
			WHERE ac.occurred_at >= $1 AND ac.occurred_at < $2
			GROUP BY ac.occurred_at::date, ac.task, ac.tier
			ORDER BY day, ac.task, ac.tier`,
			from, to, fallbackFrom, fallbackTo)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var dc DayCost
			var task, tier string
			if err := rows.Scan(&dc.Day, &task, &tier, &dc.CostMicroUSD, &dc.UnpricedCalls); err != nil {
				return err
			}
			dc.Task = Task(task)
			dc.Tier = Tier(tier)
			report = append(report, dc)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("ai: cost report: %w", err)
	}
	return report, nil
}

// callCostMicroUSD is what one ai_call row (aliased alias, joined to its rate
// as r through pricedRate) cost, in micro-USD. Every reader that prices a call
// in SQL takes it from here, so they price alike.
func callCostMicroUSD(alias string) string {
	return fmt.Sprintf(`CASE
	      WHEN %[1]s.cache_hit OR (%[1]s.tokens_in = 0 AND %[1]s.tokens_out = 0) THEN 0
	      WHEN r.id IS NULL THEN 0
	      ELSE (GREATEST(%[1]s.tokens_in - %[1]s.cached_tokens - %[1]s.cache_write_tokens, 0) * %[2]s
	           + %[1]s.cached_tokens * COALESCE(NULLIF(%[3]s, 0), %[2]s)
	           + %[1]s.cache_write_tokens * COALESCE(NULLIF(%[4]s, 0), %[2]s)
	           + %[1]s.tokens_out * %[5]s) / 1000000
	    END`, alias,
		promptPrice(alias, "input_per_mtok_microusd"), promptPrice(alias, "cache_read_per_mtok_microusd"),
		promptPrice(alias, "cache_write_per_mtok_microusd"), promptPrice(alias, "output_per_mtok_microusd"))
}

// promptPrice is one of rate r's four prices for the ai_call row aliased alias.
// Above the band's line it is the band's price, otherwise the row's own.
// ModelRate.forPrompt is the same rule in Go.
func promptPrice(alias, column string) string {
	return fmt.Sprintf(`CASE WHEN %[1]s.tokens_in > r.long_context_above_tokens
	        THEN r.long_context_%[2]s ELSE r.%[2]s END`, alias, column)
}

// longContextColumns are the five long-context band columns of the rate row
// aliased alias, in the order scannedBand scans them.
func longContextColumns(alias string) string {
	return fmt.Sprintf(`%[1]s.long_context_above_tokens, %[1]s.long_context_input_per_mtok_microusd,
	       %[1]s.long_context_output_per_mtok_microusd, %[1]s.long_context_cache_read_per_mtok_microusd,
	       %[1]s.long_context_cache_write_per_mtok_microusd`, alias)
}

// scannedBand holds the five nullable band columns as read. The schema keeps
// them all set or all empty (ai_model_rate_long_context_complete_check).
type scannedBand struct {
	above                                *int
	input, output, cacheRead, cacheWrite *int64
}

// band answers the scanned band, or nil for a row with one flat price.
func (b scannedBand) band() *ContextBand {
	if b.above == nil || b.input == nil || b.output == nil || b.cacheRead == nil || b.cacheWrite == nil {
		return nil
	}
	return &ContextBand{
		AboveTokens:          *b.above,
		InputPerMTokMicroUSD: *b.input, OutputPerMTokMicroUSD: *b.output,
		CacheReadPerMTokMicroUSD: *b.cacheRead, CacheWritePerMTokMicroUSD: *b.cacheWrite,
	}
}

// callUnpriced is whether a row spent something no rate prices. A cache hit and
// a call that reached no provider are free by construction, never unpriced.
func callUnpriced(alias string) string {
	return fmt.Sprintf(`NOT %[1]s.cache_hit AND NOT (%[1]s.tokens_in = 0 AND %[1]s.tokens_out = 0) AND r.id IS NULL`, alias)
}
