// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Usage is one model call's metering record (§6): who spent, on what,
// how much, and whether the cache saved the call.
type Usage struct {
	Task      Task
	Tier      Tier
	TokensIn  int
	TokensOut int
	Cached    bool
	// CachedTokens / ReasoningTokens are the itemized counts a native provider
	// reports (prompt-cache reads, reasoning/thinking tokens); 0 when the
	// provider reports neither.
	CachedTokens    int
	ReasoningTokens int
	// CacheWriteTokens is the cache-creation bucket a native provider reports
	// (e.g. Anthropic's cache_creation_input_tokens) — disjoint from
	// CachedTokens (a read), already counted inside TokensIn, 0 when the
	// provider reports none. The pricer (pricing.go) reads it, and the call's
	// own row in ai_call keeps it.
	CacheWriteTokens int
}

// usageStore is what the router needs from metering; the interface
// exists so router unit tests run without Postgres while the Meter
// below is the one real implementation.
type usageStore interface {
	Record(ctx context.Context, u Usage) error
	MonthTokens(ctx context.Context) (int64, error)
	// Workspace names the company a call is served for: its result cache is
	// partitioned by it, so an answer never crosses to another company.
	Workspace(ctx context.Context) (ids.WorkspaceID, error)
}

// Workspace names the installation's live company.
func (m *Meter) Workspace(ctx context.Context) (ids.WorkspaceID, error) {
	return m.db.Workspace(ctx)
}

// Meter accumulates ai_usage counters per (workspace, day, task, tier).
// It rides the workspace GUC transaction like every tenant write; a
// call outside workspace context is a programming error and fails.
type Meter struct {
	// db binds the workspace this store runs for (ADR-0091 §9 step 3).
	db *database.DB
	// now is injectable so tests pin the day/month boundaries.
	now func() time.Time
	// log carries the warning for a negative provider count.
	log *slog.Logger
}

// NewMeter binds the AI usage meter to the pool its counters are folded in.
func NewMeter(db *database.DB) *Meter {
	return &Meter{db: db, now: time.Now, log: slog.Default()}
}

// WithLogger points the meter's warnings at a caller's logger.
func (m *Meter) WithLogger(log *slog.Logger) *Meter {
	if log != nil {
		m.log = log
	}
	return m
}

// billingContext is ctx for writing what a provider already billed. A caller
// that stopped waiting does not unspend the tokens. So the write keeps the
// context's values, drops its cancellation, and is bounded as the trace write is.
func billingContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), traceWriteTimeout)
}

// recordUsage meters u on the billing context.
func (r *Router) recordUsage(ctx context.Context, u Usage) error {
	billing, done := billingContext(ctx)
	defer done()
	return r.meter.Record(billing, u)
}

func (m *Meter) Record(ctx context.Context, u Usage) error {
	// A provider response has already spent these tokens, even if its job lost ownership.
	ctx = database.WithoutTransactionGuard(ctx)
	day := m.now().UTC().Format("2006-01-02")
	cachedHit := 0
	if u.Cached {
		cachedHit = 1
	}
	source := usageSource{task: u.Task, tier: u.Tier}
	tokensIn := storedTokenCount(ctx, m.log, source, "tokens_in", u.TokensIn)
	tokensOut := storedTokenCount(ctx, m.log, source, "tokens_out", u.TokensOut)
	return m.db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO ai_usage (day, task, tier, calls, cached_hits, tokens_in, tokens_out)
			VALUES ($1, $2, $3, 1, $4, $5, $6)
			ON CONFLICT (day, task, tier) DO UPDATE SET
			  calls       = ai_usage.calls + 1,
			  cached_hits = ai_usage.cached_hits + EXCLUDED.cached_hits,
			  tokens_in   = ai_usage.tokens_in + EXCLUDED.tokens_in,
			  tokens_out  = ai_usage.tokens_out + EXCLUDED.tokens_out`,
			day, string(u.Task), string(u.Tier), cachedHit, tokensIn, tokensOut)
		if err != nil {
			return fmt.Errorf("ai: metering: %w", err)
		}
		return nil
	})
}

// MonthTokens sums the workspace's current calendar-month spend — the
// input to the §1.3 utilization bands.
func (m *Meter) MonthTokens(ctx context.Context) (int64, error) {
	var total int64
	err := m.db.Tx(ctx, func(tx pgx.Tx) error { var err error; total, err = monthTokensTx(ctx, tx, m.now()); return err })
	return total, err
}

func monthTokensTx(ctx context.Context, tx pgx.Tx, now time.Time) (int64, error) {
	args := []any{now.UTC().Format("2006-01") + "-01"}
	var total int64
	query := fmt.Sprintf(`SELECT COALESCE(SUM(tokens_in+tokens_out),0) FROM ai_usage WHERE day >= $%d::date`, len(args))
	if err := tx.QueryRow(ctx, query, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("ai: month tokens: %w", err)
	}
	return total, nil
}

// PremiumShare is the costly-cloud fraction of tokens over the trailing
// window — the §1.3 routing-fix alarm input. It measures every rung billed
// above the cheap cloud rate, not premium alone: a rung priced higher than
// premium that counted only toward the denominator would push the reported
// share DOWN as a workspace spent more on it, which is backwards for an alarm
// whose whole job is to notice expensive routing.
func (m *Meter) PremiumShare(ctx context.Context, window time.Duration) (share float64, alarm bool, err error) {
	since := m.now().UTC().Add(-window).Format("2006-01-02")
	costly := make([]string, 0, len(costlyCloudTiers))
	for _, tier := range costlyCloudTiers {
		costly = append(costly, string(tier))
	}
	var premium, total int64
	err = m.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT
			  COALESCE(SUM(tokens_in + tokens_out) FILTER (WHERE tier = ANY($1)), 0),
			  COALESCE(SUM(tokens_in + tokens_out), 0)
			FROM ai_usage WHERE day >= $2::date`, costly, since).Scan(&premium, &total)
	})
	if err != nil {
		return 0, false, fmt.Errorf("ai: premium share: %w", err)
	}
	if total == 0 {
		return 0, false, nil
	}
	share = float64(premium) / float64(total)
	return share, share > premiumShareAlarmThreshold, nil
}
