// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

// What the week CHANGED, as opposed to what it held.
//
// Split from the assembly because the two answer different questions and fail
// differently. Every tally in countWeek is always answerable; the money here is
// a conversion, which has a third outcome — not computable — and the prior week
// is a pointer into the rep's own history rather than a measurement of this one.

import (
	"context"
	"errors"
	"fmt"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// countWeekMoney sums what the week added to and took out of the pipeline, in
// the installation's base currency — or reports that it could not.
//
// A SEPARATE read from countWeek because its failure mode is different in kind.
// Every count there is a tally that is always answerable; this one is a
// conversion, and a conversion has a third outcome beside "some" and "none":
// not computable. Folding it into the same row would make one unconvertible
// deal look like a week with no pipeline movement.
//
// The conversion is deals.ConvertToBase over deals.FXRates, NOT arithmetic in
// SQL. Two implementations of one conversion agree until they do not, and the
// first thing to diverge is the rounding: ConvertToBase rounds half away from
// zero in exact decimal arithmetic over the rate's stored digits, where a
// round() in SQL over a float can land a minor unit away. A one-cent
// disagreement between two pages about the same week is the kind of defect
// nobody can reproduce on demand.
//
// WHY IT DOES NOT SUM amount_minor_base for the created figure. That column is
// null on every OPEN deal by design — a deal freezes its rate on
// CLOSE (deal_closed_fx) — so summing it over deals created in the week returns
// null whenever the week actually created pipeline. Closed deals do carry it,
// and the won and lost sums below use it for exactly that reason: their rate is
// the one that applied at close, which is the honest figure for money that has
// already moved.
//
// A deal whose currency has no usable rate makes the WHOLE figure unknown
// rather than being skipped: a total covering three of four deals is a
// confident number that is quietly short. Nothing is ever converted at an
// invented rate of 1, which would report ¥5,000,000 as €5,000,000.
func countWeekMoney(
	ctx context.Context, tx pgx.Tx, userID ids.UUID, start, end time.Time,
) (Money, error) {
	base, err := baseCurrency(ctx, tx)
	if err != nil || base == "" {
		// No base currency named: nothing converts, and the week says so.
		return Money{}, err
	}
	opened, err := openedInWeek(ctx, tx, userID, start, end)
	if err != nil {
		return Money{}, err
	}
	// Rates as of the week's END, frozen into the figure written below. The
	// lookup is per-read by design — a rate is appended forward, and a
	// longer-lived cache would price this week at a rate corrected since.
	rates := deals.NewFXRates(base, end)
	var created int64
	for _, row := range opened {
		rate, ok, err := rates.For(ctx, tx, row.currency)
		if err != nil {
			return Money{}, err
		}
		if !ok {
			// One unconvertible deal, and the whole figure is unknown.
			return Money{}, nil
		}
		amount, err := deals.ConvertToBase(row.amountMinor, rate.Rate, row.currency, base)
		if err != nil {
			return Money{}, fmt.Errorf("weekly: converting the week's pipeline: %w", err)
		}
		created += amount
	}
	won, lost, err := closedInWeek(ctx, tx, userID, start, end)
	if err != nil {
		return Money{}, err
	}
	return Money{
		CreatedMinor: created, WonMinor: won, LostMinor: lost,
		Currency: base, Known: true,
	}, nil
}

// baseCurrency is the currency the installation reports in, or empty when it
// names none.
func baseCurrency(ctx context.Context, tx pgx.Tx) (string, error) {
	var code string
	err := tx.QueryRow(ctx, `
		SELECT (value #>> '{}')::text FROM setting
		 WHERE key = 'installation.base_currency'`).Scan(&code)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("weekly: reading the base currency: %w", err)
	}
	return code, nil
}

// pricedDeal is one priced deal the week opened, in its own currency.
type pricedDeal struct {
	amountMinor int64
	currency    string
}

// openedInWeek reads the priced deals this rep opened in the week.
//
// Rows rather than a sum, because the conversion happens in Go: see
// countWeekMoney on why it is not arithmetic in SQL.
func openedInWeek(
	ctx context.Context, tx pgx.Tx, userID ids.UUID, start, end time.Time,
) ([]pricedDeal, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	startPos, endPos, userPos := arg(start), arg(end), arg(userID)
	scope, err := auth.ScopeClauseFor(ctx, "deal", "d", arg)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = "true"
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT d.amount_minor, d.currency FROM deal d
		 WHERE d.owner_id = $%[3]d AND d.archived_at IS NULL
		   AND d.created_at >= $%[1]d AND d.created_at < $%[2]d
		   AND d.amount_minor IS NOT NULL AND d.currency IS NOT NULL
		   AND (%[4]s)`, startPos, endPos, userPos, scope), args...)
	if err != nil {
		return nil, fmt.Errorf("weekly: reading the week's new deals: %w", err)
	}
	defer rows.Close()
	var out []pricedDeal
	for rows.Next() {
		var row pricedDeal
		if err := rows.Scan(&row.amountMinor, &row.currency); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// closedInWeek sums the deals won and lost in the week.
//
// amount_minor_base is right HERE and wrong for the opened figure above: a
// closed deal froze its own rate at close (deal_closed_fx guarantees one for
// every closed deal that has an amount), and that frozen rate is the honest
// figure for money that has already moved. An amountless deal's null is skipped
// by SUM — an honest zero, not an invented rate.
func closedInWeek(
	ctx context.Context, tx pgx.Tx, userID ids.UUID, start, end time.Time,
) (won, lost int64, err error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	startPos, endPos, userPos := arg(start), arg(end), arg(userID)
	scope, err := auth.ScopeClauseFor(ctx, "deal", "d", arg)
	if err != nil {
		return 0, 0, err
	}
	if scope == "" {
		scope = "true"
	}
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT
		  (SELECT coalesce(sum(d.amount_minor_base), 0)::bigint FROM deal d
		    WHERE d.status = 'won' AND d.owner_id = $%[3]d
		      AND d.closed_at >= $%[1]d AND d.closed_at < $%[2]d AND (%[4]s)),
		  (SELECT coalesce(sum(d.amount_minor_base), 0)::bigint FROM deal d
		    WHERE d.status = 'lost' AND d.owner_id = $%[3]d
		      AND d.closed_at >= $%[1]d AND d.closed_at < $%[2]d AND (%[4]s))`,
		startPos, endPos, userPos, scope), args...).Scan(&won, &lost)
	if err != nil {
		return 0, 0, fmt.Errorf("weekly: summing the week's closed deals: %w", err)
	}
	return won, lost, nil
}

// deref reads a nullable sum as zero. A week in which nothing closed sums to
// NULL, and that IS zero — unlike an unconvertible week, which countWeekMoney
// has already turned into an absent Money above.
func deref(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

// priorReview names the rep's most recent review before this week, if any.
//
// Read inside the same transaction that writes this one, so a rep whose first
// two weeks are backfilled in one run chains correctly: the second finds the
// first because the first is already committed to this transaction.
func priorReview(
	ctx context.Context, tx pgx.Tx, userID ids.UUID, weekStart time.Time,
) (*ids.UUID, error) {
	var id ids.UUID
	err := tx.QueryRow(ctx, `
		SELECT id FROM weekly_review
		 WHERE user_id = $1 AND local_week_start < $2
		 ORDER BY local_week_start DESC LIMIT 1`, userID, weekStart).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		// Their first week. Null rather than an error: a review with nothing to
		// compare against is the ordinary start of a history.
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("weekly: finding the prior review: %w", err)
	}
	return &id, nil
}

// readPriorWeek loads the frozen figures of the week this one is compared to.
//
// Scoped to the same rep, so a prior_review_id that somehow named another
// person's review reads as no prior week rather than as a comparison against
// somebody else's numbers. Nothing writes such a row — the pointer is set from
// the rep's own history — and that is exactly why the read states it rather
// than relying on it.
//
// Its own Prior is deliberately not loaded: a review carries ONE step of
// history, and chaining would make a rep's fiftieth week read forty-nine rows
// to render one comparison.
func readPriorWeek(
	ctx context.Context, tx pgx.Tx, priorID *ids.UUID, userID ids.UUID,
) (*PriorWeek, error) {
	if priorID == nil {
		return nil, nil
	}
	prior, err := readReviewTx(ctx, tx, *priorID, userID)
	if errors.Is(err, apperrors.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &PriorWeek{
		LocalWeekStart: prior.LocalWeekStart,
		Counts:         prior.Counts,
		Money:          prior.Money,
	}, nil
}
