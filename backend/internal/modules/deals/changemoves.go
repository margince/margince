// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// How much value one change Margince made moved, in the base currency.
//
// An amount change moves its difference. A close as lost takes the deal's
// value out of the pipeline, and an opened or revived deal puts it in. A
// probability change or a stage move moves the weighted value: the amount
// times the change in win probability. A new close date moves no value.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// changeMovedSQL is one change's value moved, in minor units of its currency,
// and that currency. It is NULL for a change that moves no value, and when
// the reader's mask withholds the amount.
func changeMovedSQL(ctx context.Context, arg func(any) int) (moved, currency string, err error) {
	masked := func(expr string) (string, error) {
		return auth.MaskedExpressionSQL(ctx, maskObject, "amount_minor", "d", expr, arg)
	}
	deal, err := masked("d.amount_minor")
	if err != nil {
		return "", "", err
	}
	before, err := masked("(a.before->>'amount_minor')::bigint")
	if err != nil {
		return "", "", err
	}
	after, err := masked("(a.after->>'amount_minor')::bigint")
	if err != nil {
		return "", "", err
	}
	const beforeOverride = "(a.before->>'win_probability_override')::int"
	weighted := func(from, to string) string {
		return storekit.SQLf("round(%s * (%s - %s) / 100.0)::bigint", deal, to, from)
	}
	moved = storekit.SQLf(`CASE
		WHEN c.correction = $%d THEN %s - coalesce(%s, 0)
		WHEN c.correction = $%d THEN -(%s)
		WHEN c.correction = ANY($%d) THEN %s
		WHEN c.correction = $%d THEN %s
		WHEN c.correction = $%d THEN %s
		END`,
		arg(VerdictAmountCorrection), after, before,
		arg(CloseLostCorrection), deal,
		arg([]string{DealOpenCorrection, DealReviveCorrection}), deal,
		arg(VerdictProbabilityCorrection), weighted(
			"coalesce("+beforeOverride+", "+StageWinProbabilitySQL("d")+")",
			"(a.after->>'win_probability_override')::int"),
		arg(VerdictStageMoveCorrection), weighted(
			"coalesce("+beforeOverride+", (SELECT fs.win_probability FROM stage fs WHERE fs.id::text = a.before->>'stage_id'))",
			"coalesce((SELECT h.win_probability_at_change::int FROM deal_stage_history h WHERE h.id = c.stage_history_id),"+
				" (SELECT ts.win_probability FROM stage ts WHERE ts.id::text = a.after->>'stage_id'))"))
	return moved, "coalesce(a.evidence->>'amount_currency', d.currency::text)", nil
}

// movedAmount is a value moved in its own currency, before conversion.
type movedAmount[K comparable] struct {
	key      K
	minor    int64
	currency string
}

// priceMoves converts each moved amount to the base currency through the one
// FX engine. An amount with no rate, or one too large to state, is left out.
func priceMoves[K comparable](ctx context.Context, tx pgx.Tx, rates *FXRates, moves []movedAmount[K]) (map[K]int64, error) {
	amounts := make([]CurrencyAmount, len(moves))
	for i, m := range moves {
		amounts[i] = CurrencyAmount{Minor: m.minor, Currency: m.currency}
	}
	priced, err := PriceAll(ctx, tx, rates, amounts)
	if err != nil {
		return nil, fmt.Errorf("deals: pricing the value Margince's changes moved: %w", err)
	}
	out := map[K]int64{}
	for i, p := range priced {
		if p.Priced {
			out[moves[i].key] += p.Minor
		}
	}
	return out, nil
}

// groupKey names one owner's changes of one kind.
type groupKey struct {
	owner ids.UUID
	kind  string
}

// dealMove is one deal's net move within a group; n tells deals apart.
type dealMove struct {
	group groupKey
	n     int
}

// teamMoves sums the value moved per owner and kind. A deal's changes of one
// kind net out first. The group adds the size of each deal's net move, so a
// raise on one deal and a cut on another do not cancel. An undone change
// moved nothing.
func teamMoves(ctx context.Context, tx pgx.Tx, rates *FXRates, where func(arg func(any) int) (string, error)) (map[groupKey]int64, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	moved, currency, err := changeMovedSQL(ctx, arg)
	if err != nil {
		return nil, err
	}
	filter, err := where(arg)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, storekit.SQLf(`
		SELECT d.owner_id, c.correction, sum(%s)::bigint, %s
		  FROM deal_correction c JOIN deal d ON d.id = c.deal_id JOIN audit_log a ON a.id = c.audit_log_id%s
		   AND c.reversed_at IS NULL
		 GROUP BY 1, 2, c.deal_id, 4
		HAVING sum(%s) IS NOT NULL`, moved, currency, filter, moved), args...)
	if err != nil {
		return nil, fmt.Errorf("deals: reading the value the team's changes moved: %w", err)
	}
	var moves []movedAmount[dealMove]
	for rows.Next() {
		var key dealMove
		var minor int64
		var cur *string
		if err := rows.Scan(&key.group.owner, &key.group.kind, &minor, &cur); err != nil {
			rows.Close()
			return nil, err
		}
		if cur != nil {
			key.n = len(moves)
			moves = append(moves, movedAmount[dealMove]{key: key, minor: minor, currency: *cur})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	perDeal, err := priceMoves(ctx, tx, rates, moves)
	if err != nil {
		return nil, err
	}
	out := map[groupKey]int64{}
	for key, net := range perDeal {
		out[key.group] += abs64(net)
	}
	return out, nil
}

// addMoves sets the value moved on each group and on each listed row.
func addMoves(
	ctx context.Context, tx pgx.Tx, rates *FXRates, where func(arg func(any) int) (string, error), report *TeamChangeReport,
) error {
	moved, err := teamMoves(ctx, tx, rates, where)
	if err != nil {
		return err
	}
	for i, g := range report.Groups {
		report.Groups[i].ValueMovedBaseMinor = moved[groupKey{owner: g.OwnerID, kind: g.Kind}]
	}
	report.Moved, err = rowMoves(ctx, tx, rates, where, report.Rows)
	return err
}

// rowMoves is the value each listed change moved, keyed by its audit row,
// read under the same filter as the list.
func rowMoves(
	ctx context.Context, tx pgx.Tx, rates *FXRates, where func(arg func(any) int) (string, error), listed []CorrectionReceipt,
) (map[ids.UUID]int64, error) {
	auditIDs := make([]ids.UUID, 0, len(listed))
	for _, row := range listed {
		auditIDs = append(auditIDs, row.AuditLogID)
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	moved, currency, err := changeMovedSQL(ctx, arg)
	if err != nil {
		return nil, err
	}
	filter, err := where(arg)
	if err != nil {
		return nil, err
	}
	query := storekit.SQLf(`
		SELECT c.audit_log_id, %s, %s
		  FROM deal_correction c JOIN deal d ON d.id = c.deal_id JOIN audit_log a ON a.id = c.audit_log_id%s
		   AND c.audit_log_id = ANY($%d)`, moved, currency, filter, arg(auditIDs))
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("deals: reading the value each change moved: %w", err)
	}
	var moves []movedAmount[ids.UUID]
	for rows.Next() {
		var id ids.UUID
		var minor *int64
		var cur *string
		if err := rows.Scan(&id, &minor, &cur); err != nil {
			rows.Close()
			return nil, err
		}
		if minor != nil && cur != nil {
			moves = append(moves, movedAmount[ids.UUID]{key: id, minor: *minor, currency: *cur})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return priceMoves(ctx, tx, rates, moves)
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
