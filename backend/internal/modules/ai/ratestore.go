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

// WithClock overrides the "today" source (tests only). Returns the store
// for chaining.
func (s *RateStore) WithClock(clock func() time.Time) *RateStore {
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
	from, to := rateFallbacks()
	err := tx.QueryRow(ctx, `
		SELECT r.provider, r.model_id, r.input_per_mtok_microusd, r.output_per_mtok_microusd,
		       r.cache_read_per_mtok_microusd, r.cache_write_per_mtok_microusd, r.effective_date
		FROM (`+rateMatch("$1", "$2", "$3", 4, 5)+`) r`,
		provider, modelID, day, from, to).Scan(
		&rate.Provider, &rate.ModelID, &rate.InputPerMTokMicroUSD, &rate.OutputPerMTokMicroUSD,
		&rate.CacheReadPerMTokMicroUSD, &rate.CacheWritePerMTokMicroUSD, &rate.EffectiveDate,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // no matching rate row IS the "unpriced" answer, not an error — price-on-read never fabricates a price
	}
	if err != nil {
		return nil, fmt.Errorf("ai: rate lookup: %w", err)
	}
	return &rate, nil
}

// CostReport prices the [from, to) window's ai_call rows against their
// as-of-date rate and sums per (calendar day, task, tier) — THE one
// money computation that runs at read time (price-on-read: the
// router/meter/adapters never compute a cost). One SQL statement: a
// LATERAL join picks each row's as-of-date rate (RateFor's same
// resolution, inlined so the whole window prices in one query instead of
// one round-trip per call), the four-bucket arithmetic mirrors PriceCall
// exactly (same floor, same zero-cache-price fallback to the input rate, same
// truncating /1000000), and GROUP BY the
// call's UTC calendar day + task + tier rolls the window up to exactly
// AIRT-WIRE-1's /ai/usage grain — one report line per wire row, so the
// handler attaches each line to its one matching (day, task, tier) row
// instead of broadcasting a shared task total across every tier that
// task ran on (which double-counts whenever a client sums cost_est_minor
// across rows).
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
			LEFT JOIN LATERAL (`+rateMatch("ac.provider", "ac.model_id", "ac.occurred_at::date", 3, 4)+`) r ON true
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
// as r through rateMatch) cost, in micro-USD. Every reader that prices a call
// in SQL takes it from here, so they price alike.
func callCostMicroUSD(alias string) string {
	return fmt.Sprintf(`CASE
	      WHEN %[1]s.cache_hit OR (%[1]s.tokens_in = 0 AND %[1]s.tokens_out = 0) THEN 0
	      WHEN r.id IS NULL THEN 0
	      ELSE (GREATEST(%[1]s.tokens_in - %[1]s.cached_tokens - %[1]s.cache_write_tokens, 0) * r.input_per_mtok_microusd
	           + %[1]s.cached_tokens * COALESCE(NULLIF(r.cache_read_per_mtok_microusd, 0), r.input_per_mtok_microusd)
	           + %[1]s.cache_write_tokens * COALESCE(NULLIF(r.cache_write_per_mtok_microusd, 0), r.input_per_mtok_microusd)
	           + %[1]s.tokens_out * r.output_per_mtok_microusd) / 1000000
	    END`, alias)
}

// callUnpriced is whether a row spent something no rate prices. A cache hit and
// a call that reached no provider are free by construction, never unpriced.
func callUnpriced(alias string) string {
	return fmt.Sprintf(`NOT %[1]s.cache_hit AND NOT (%[1]s.tokens_in = 0 AND %[1]s.tokens_out = 0) AND r.id IS NULL`, alias)
}
