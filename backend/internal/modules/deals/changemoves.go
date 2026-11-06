// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// How much value one change Margince made moved, in the base currency. Each
// move is valued from the deal as it stood then. That comes from the change's
// audit images, its stage history row, or the valuation its evidence stored.
// A change with none of these is counted as unvalued.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The evidence a probability change stores so its move can be valued later:
// the deal's amount and currency, and the win probability it had before.
const (
	valuedAmountKey      = "valued_amount_minor"
	valuedCurrencyKey    = "valued_currency"
	valuedProbabilityKey = "valued_probability_before"
)

// valueKinds are the corrections that move value.
var valueKinds = []string{
	VerdictAmountCorrection, CloseLostCorrection, DealOpenCorrection, DealReviveCorrection,
	VerdictProbabilityCorrection, VerdictStageMoveCorrection,
}

// changeMovedSQL is one change's value moved, in minor units of its currency,
// and that currency. The value is NULL when the change cannot be valued, or
// the reader's mask withholds the amount. It reads the stage history row of
// the change as h.
func changeMovedSQL(ctx context.Context, arg func(any) int) (moved, currency string, err error) {
	var amounts [4]string
	for i, expr := range []string{
		"(a.before->>'amount_minor')::bigint", "(a.after->>'amount_minor')::bigint",
		"h.amount_minor_at_change", "(a.evidence->>'" + valuedAmountKey + "')::bigint",
	} {
		if amounts[i], err = auth.MaskedExpressionSQL(ctx, maskObject, "amount_minor", "d", expr, arg); err != nil {
			return "", "", err
		}
	}
	before, after, atChange, valued := amounts[0], amounts[1], amounts[2], amounts[3]
	const beforeOverride = "(a.before->>'win_probability_override')::int"
	stageBefore := "coalesce(" + beforeOverride + `,
		(SELECT p.win_probability_at_change::int FROM deal_stage_history p
		  WHERE p.deal_id = h.deal_id AND p.to_stage_id = h.from_stage_id AND p.changed_at < h.changed_at
		  ORDER BY p.changed_at DESC LIMIT 1),
		(SELECT fs.win_probability FROM stage fs WHERE fs.id = h.from_stage_id))`
	amountKind, lostKind, addKinds := arg(VerdictAmountCorrection), arg(CloseLostCorrection), arg([]string{DealReviveCorrection})
	openKind, probabilityKind, stageKind := arg(DealOpenCorrection), arg(VerdictProbabilityCorrection), arg(VerdictStageMoveCorrection)
	moved = storekit.SQLf(`CASE
		WHEN c.correction = $%[1]d THEN %[7]s - coalesce(%[8]s, 0)
		WHEN c.correction = $%[2]d THEN -(%[9]s)
		WHEN c.correction = ANY($%[3]d) THEN %[9]s
		WHEN c.correction = $%[4]d THEN %[7]s
		WHEN c.correction = $%[5]d THEN round(%[10]s * ((a.after->>'win_probability_override')::int
		                                  - (a.evidence->>'%[12]s')::int) / 100.0)::bigint
		WHEN c.correction = $%[6]d THEN round(%[9]s * (h.win_probability_at_change - %[11]s) / 100.0)::bigint
		ELSE 0 END`,
		amountKind, lostKind, addKinds, openKind, probabilityKind, stageKind,
		after, before, atChange, valued, stageBefore, valuedProbabilityKey)
	currency = storekit.SQLf(`CASE
		WHEN c.correction = $%[1]d THEN a.evidence->>'amount_currency'
		WHEN c.correction = $%[2]d THEN a.after->>'currency'
		WHEN c.correction = $%[3]d THEN a.evidence->>'%[4]s'
		ELSE h.currency_at_change::text END`, amountKind, openKind, probabilityKind, valuedCurrencyKey)
	return moved, currency, nil
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
			out[moves[i].key] = p.Minor
		}
	}
	return out, nil
}

// groupKey names one owner's changes of one kind.
type groupKey struct {
	owner ids.UUID
	kind  string
}

// dealMove is one deal within a group; deal numbers the rows so deals stay
// apart without naming them.
type dealMove struct {
	group groupKey
	deal  int
}

// groupMoves is what one group's changes moved, and how many of its deals
// could not be valued.
type groupMoves struct {
	moved    int64
	unvalued int
}

// teamMoves sums the value moved per owner and kind. A deal's changes of one
// kind net out first. The group adds the size of each deal's net move, so a
// raise on one deal and a cut on another do not cancel. An undone change
// moved nothing. A deal with a change that cannot be valued adds nothing and
// is counted as unvalued.
func teamMoves(ctx context.Context, tx pgx.Tx, rates *FXRates, where func(arg func(any) int) (string, error)) (map[groupKey]groupMoves, error) {
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
	// One row per deal: its changes' moves and currencies, in step.
	query := storekit.SQLf(`
		SELECT d.owner_id, c.correction, array_agg(%s), array_agg(%s) FROM deal_correction c JOIN deal d ON d.id = c.deal_id JOIN audit_log a ON a.id = c.audit_log_id
	LEFT JOIN deal_stage_history h ON h.id = c.stage_history_id%s
		   AND c.reversed_at IS NULL AND c.correction = ANY($%d)
		 GROUP BY d.owner_id, c.correction, c.deal_id`, moved, currency, filter, arg(valueKinds))
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("deals: reading the value the team's changes moved: %w", err)
	}
	moves, keys, unvalued, err := scanTeamMoves(rows)
	if err != nil {
		return nil, err
	}
	priced, err := priceMoves(ctx, tx, rates, moves)
	if err != nil {
		return nil, err
	}
	net := map[dealMove]int64{}
	for _, m := range moves {
		value, ok := priced[m.key]
		if !ok {
			unvalued[keys[m.key]] = true
		}
		net[keys[m.key]] += value
	}
	return sumGroups(net, unvalued), nil
}

// scanTeamMoves reads the rows of teamMoves and closes them. It answers each
// nonzero move, the deal each move belongs to, and the deals that cannot be
// valued.
func scanTeamMoves(rows pgx.Rows) ([]movedAmount[int], map[int]dealMove, map[dealMove]bool, error) {
	var moves []movedAmount[int]
	keys := map[int]dealMove{}
	unvalued := map[dealMove]bool{}
	for deal := 0; rows.Next(); deal++ {
		key := dealMove{deal: deal}
		var minors []*int64
		var currencies []*string
		if err := rows.Scan(&key.group.owner, &key.group.kind, &minors, &currencies); err != nil {
			rows.Close()
			return nil, nil, nil, err
		}
		for i, minor := range minors {
			switch {
			case minor != nil && *minor == 0:
			case minor == nil || currencies[i] == nil:
				unvalued[key] = true
			default:
				keys[len(moves)] = key
				moves = append(moves, movedAmount[int]{key: len(moves), minor: *minor, currency: *currencies[i]})
			}
		}
	}
	rows.Close()
	return moves, keys, unvalued, rows.Err()
}

// sumGroups adds each valued deal's net move to its group.
func sumGroups(net map[dealMove]int64, unvalued map[dealMove]bool) map[groupKey]groupMoves {
	out := map[groupKey]groupMoves{}
	for key, value := range net {
		if unvalued[key] {
			continue
		}
		g := out[key.group]
		g.moved += abs64(value)
		out[key.group] = g
	}
	for key := range unvalued {
		g := out[key.group]
		g.unvalued++
		out[key.group] = g
	}
	return out
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
		sums := moved[groupKey{owner: g.OwnerID, kind: g.Kind}]
		report.Groups[i].ValueMovedBaseMinor, report.Groups[i].Unvalued = sums.moved, sums.unvalued
	}
	report.Moved, err = rowMoves(ctx, tx, rates, where, report.Rows)
	return err
}

// rowMoves is the value each listed change moved, keyed by its audit row,
// read under the same filter as the list. An undone change moved nothing and
// has no entry.
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
		SELECT c.audit_log_id, %s, %s FROM deal_correction c JOIN deal d ON d.id = c.deal_id JOIN audit_log a ON a.id = c.audit_log_id
	LEFT JOIN deal_stage_history h ON h.id = c.stage_history_id%s
		   AND c.audit_log_id = ANY($%d) AND c.reversed_at IS NULL AND c.correction = ANY($%d)`,
		moved, currency, filter, arg(auditIDs), arg(valueKinds))
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
