// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Serving a frozen snapshot to somebody narrower than the person who froze it.
//
// The stored headline is the sum over every deal the ISSUER could see. Handing
// that number to a recipient who cannot see all of them discloses a total
// covering deals they may not read — which is the aggregate disclosure the
// audience rules exist to prevent, and it does not stop being one because the
// rows behind it are hidden.
//
// So a snapshot share never serves won_minor and its siblings. It re-sums the
// contributions the recipient may actually read, and the headline it returns
// is a headline ABOUT THEM. A recipient who can see nothing gets zeroes and a
// count of zero eligible deals, which is the honest answer to what they asked.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// SharedSnapshot is one frozen state as one recipient may read it.
type SharedSnapshot struct {
	Readings forecasting.Readings
	// Withheld says something was kept back, and never how much. A count of
	// what a reader may not see is itself a statement about how much of it
	// there is.
	Withheld bool
}

// ReadSharedSnapshot re-sums a snapshot's contributions under the CALLER's
// authority.
//
// Three narrowings compose, and each is a different question. The deal row
// scope is what the recipient may see AT ALL. The field mask on the deal's
// amount excludes a row from the money entirely rather than blanking it,
// because a total that silently omits a masked row and a total that includes
// it are both wrong answers and only exclusion reconciles against a
// drill-through. The snapshot id is which frozen state.
//
// The sums come from the STORED per-deal integers, never re-derived: rounding
// happened once, per deal, at freeze time, and that is what makes a headline
// reconcile to the cent against the rows beneath it.
func ReadSharedSnapshot(
	ctx context.Context, tx pgx.Tx, snapshotID ids.UUID,
) (SharedSnapshot, error) {
	// A snapshot that does not exist is not a snapshot with nothing in it. The
	// aggregate below would answer zeroes for both, and a recipient would read
	// a deleted state as an empty quarter.
	var exists bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM forecast_snapshot WHERE id = $1)`,
		snapshotID).Scan(&exists); err != nil {
		return SharedSnapshot{}, fmt.Errorf("compose: resolving a shared snapshot: %w", err)
	}
	if !exists {
		return SharedSnapshot{}, apperrors.ErrNotFound
	}

	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	args = append(args, snapshotID)

	visible, err := sharedVisibilityClause(ctx, arg)
	if err != nil {
		return SharedSnapshot{}, err
	}

	// The visibility predicate is a FILTER rather than a WHERE, so ONE pass
	// answers both questions: the totals over the rows this recipient may read,
	// and whether the snapshot held any they may not. Asked as two statements
	// the second could see a different set — nothing writes a frozen snapshot
	// after it is taken, but a predicate evaluated twice is still two answers
	// to one question.
	sql := fmt.Sprintf(`
		SELECT
		  COALESCE(sum(c.base_minor)     FILTER (WHERE c.in_won AND %[1]s), 0),
		  COALESCE(sum(c.base_minor)     FILTER (WHERE c.in_evidence AND %[1]s), 0),
		  COALESCE(sum(c.base_minor)     FILTER (WHERE c.in_best_case AND %[1]s), 0),
		  COALESCE(sum(c.base_minor)     FILTER (WHERE c.in_open AND %[1]s), 0),
		  COALESCE(sum(c.weighted_minor) FILTER (WHERE c.in_open AND %[1]s), 0),
		  count(*) FILTER (WHERE %[1]s),
		  count(*) FILTER (WHERE c.base_minor IS NOT NULL AND %[1]s),
		  count(*) FILTER (WHERE NOT c.close_provisional AND %[1]s),
		  count(*) FILTER (WHERE c.exclusion_reason = 'fx_missing' AND %[1]s),
		  -- Whether anything was kept back, asked of THIS snapshot rather than
		  -- inferred from a clause existing. A recipient whose row scope happens
		  -- to cover every deal in the frozen state had nothing withheld, and
		  -- saying otherwise invites them to distrust a complete number.
		  count(*) FILTER (WHERE NOT %[1]s) > 0
		FROM forecast_contribution c
		JOIN deal d ON d.id = c.deal_id
		WHERE c.snapshot_id = $1`, visible)

	var r forecasting.Readings
	var withheld bool
	if err := tx.QueryRow(ctx, sql, args...).Scan(
		&r.WonMinor, &r.EvidenceMinor, &r.BestCaseMinor, &r.OpenMinor, &r.WeightedMinor,
		&r.EligibleCount, &r.PricedCount, &r.ConfirmedDateCount, &r.FxMissingCount,
		&withheld,
	); err != nil {
		return SharedSnapshot{}, fmt.Errorf("compose: re-summing a shared snapshot: %w", err)
	}
	return SharedSnapshot{Readings: r, Withheld: withheld}, nil
}

// sharedVisibilityClause is what a share recipient may read of a frozen
// snapshot, as one SQL predicate over the contribution's deal aliased `d`.
//
// ONE function because the headline and the CSV must narrow identically. Two
// predicates built separately drift, and the way they show it is a file whose
// rows do not add up to the total printed above them.
//
// Two narrowings, and they answer different questions. The row scope is what
// this seat may see at all — on a deal it renders TRUE, because a deal is an
// identity table read by every seat (auth/tableclass.go). The field mask on the
// amount EXCLUDES a row from the money rather than blanking it, because a total
// that silently omits a masked row and one that includes it are both wrong, and
// only exclusion reconciles against the rows beneath it.
//
// An archived deal is NOT excluded: it was in the pipeline when the state was
// frozen, and dropping it now would make a frozen number change after the fact.
func sharedVisibilityClause(ctx context.Context, arg func(any) int) (string, error) {
	scopeClause, err := auth.ScopeClauseFor(ctx, tableDeal, "d", arg)
	if err != nil {
		return "", err
	}
	if scopeClause == "" {
		scopeClause = sqlUnnarrowed
	}
	maskClause, masked, err := auth.MaskExcludedClause(ctx, tableDeal, "amount_minor", "d", arg)
	if err != nil {
		return "", err
	}
	if !masked || maskClause == "" {
		maskClause = sqlUnnarrowed
	}
	return fmt.Sprintf("(%s AND %s)", scopeClause, maskClause), nil
}

// SharedContribution is one deal's row as one recipient may read it.
type SharedContribution struct {
	DealID          ids.UUID
	DealName        string
	Owner           *ids.UUID
	AmountMinor     *int64
	Currency        string
	BaseMinor       *int64
	EffectiveClose  *time.Time
	Category        string
	InWon           bool
	InOpen          bool
	ExclusionReason string
}

// SharedSnapshotRows reads the per-deal rows behind a shared headline, under
// the SAME narrowing the headline was summed under.
//
// One predicate, built once and used by both: a CSV whose rows and whose total
// disagree is worse than no CSV, because somebody reconciles it by hand and
// concludes the product is wrong about one of them. Rendering the two from
// separately-built predicates is exactly how they come to disagree.
func SharedSnapshotRows(
	ctx context.Context, tx pgx.Tx, snapshotID ids.UUID,
) ([]SharedContribution, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	args = append(args, snapshotID)

	visible, err := sharedVisibilityClause(ctx, arg)
	if err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT c.deal_id, d.name, c.owner_id, c.amount_minor, c.currency,
		       c.base_minor, c.effective_close_date, c.category,
		       c.in_won, c.in_open, COALESCE(c.exclusion_reason, '')
		FROM forecast_contribution c
		JOIN deal d ON d.id = c.deal_id
		WHERE c.snapshot_id = $1 AND %s
		ORDER BY d.name, c.deal_id`, visible), args...)
	if err != nil {
		return nil, fmt.Errorf("compose: reading a shared snapshot's rows: %w", err)
	}
	defer rows.Close()

	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (SharedContribution, error) {
		var c SharedContribution
		var currency, category *string
		err := row.Scan(&c.DealID, &c.DealName, &c.Owner, &c.AmountMinor, &currency,
			&c.BaseMinor, &c.EffectiveClose, &category,
			&c.InWon, &c.InOpen, &c.ExclusionReason)
		if currency != nil {
			c.Currency = *currency
		}
		if category != nil {
			c.Category = *category
		}
		return c, err
	})
	if err != nil {
		return nil, fmt.Errorf("compose: collecting a shared snapshot's rows: %w", err)
	}
	return out, nil
}
