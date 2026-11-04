// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The report of what Margince changed: per owner, per deal and per team.
//
// One deal is one report unit. Several changes Margince made to one deal count
// as one deal, and its Undo takes them all back together.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ChangeBundle is every change Margince made to one deal that its owner has
// neither accepted nor undone.
type ChangeBundle struct {
	DealID   ids.DealID
	DealName string
	Version  int64
	// AuditLogIDs name the changes, newest first: the order an Undo takes
	// them back in.
	AuditLogIDs []ids.UUID
	Kinds       []string
	// ValueBaseMinor is the deal's value in the base currency; nil when the
	// deal has no amount or no rate.
	ValueBaseMinor *int64
	LatestAt       time.Time
}

// ChangeSummary counts the owner's changed deals by kind. A deal with two
// kinds counts once in Deals and once under each kind.
type ChangeSummary struct {
	Deals, Closed, Redated, Revalued, Probabilities int
	BaseCurrency                                    string
	// Bundles is ordered by value, the largest first. Truncated says
	// bundleCap cut it; the counts above are never cut.
	Bundles   []ChangeBundle
	Truncated bool
}

// bundleCap bounds the rows of one summary. The counts come from SQL over
// every changed deal, so a cut list still reports the true total.
const bundleCap = 500

// PendingChangesOwnedBy reads every change Margince made to the reader's own
// deals that they have not accepted or undone, one bundle per deal. A since
// keeps only the changes made from then on.
func (s *Store) PendingChangesOwnedBy(ctx context.Context, since *time.Time) (ChangeSummary, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return ChangeSummary{}, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID == ids.Nil {
		return ChangeSummary{}, nil
	}
	var out ChangeSummary
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		base, err := s.installation.BaseCurrencyApplied(ctx, tx)
		if err != nil {
			return err
		}
		out.BaseCurrency = base
		args := []any{}
		arg := func(v any) int { args = append(args, v); return len(args) }
		heldPos, ownerPos, basePos, capPos := arg(heldCorrections), arg(actor.UserID), arg(base), arg(bundleCap+1)
		sincePos := arg(since)
		closedPos, redatedPos := arg(CloseLostCorrection), arg(VerdictCloseDateCorrection)
		revaluedPos, probabilityPos := arg(VerdictAmountCorrection), arg(VerdictProbabilityCorrection)
		scope, err := auth.ScopeClauseFor(ctx, dealTable, "d", arg)
		if err != nil {
			return err
		}
		where := ""
		if scope != "" {
			where = " AND " + scope
		}
		value, err := auth.MaskedExpressionSQL(ctx, maskObject, "amount_minor", "d",
			BaseValueSQL("CURRENT_DATE", fmt.Sprintf("$%d", basePos), "d"), arg)
		if err != nil {
			return err
		}
		query := storekit.SQLf(`
			WITH bundle AS (
			  SELECT c.deal_id,
			         array_agg(c.audit_log_id ORDER BY c.applied_at DESC, c.audit_log_id DESC) AS audits,
			         array_agg(DISTINCT c.correction) AS kinds,
			         max(c.applied_at) AS latest
			    FROM deal_correction c
			    JOIN deal d ON d.id = c.deal_id
			   WHERE c.correction = ANY($%d) AND c.accepted_at IS NULL AND c.reversed_at IS NULL
			     AND ($%d::timestamptz IS NULL OR c.applied_at >= $%d)
			     AND d.owner_id = $%d AND d.archived_at IS NULL%s
			   GROUP BY c.deal_id)
			SELECT b.deal_id, d.name, d.version, b.audits, b.kinds, b.latest, %s,
			       count(*) OVER (),
			       count(*) FILTER (WHERE $%d = ANY(b.kinds)) OVER (),
			       count(*) FILTER (WHERE $%d = ANY(b.kinds)) OVER (),
			       count(*) FILTER (WHERE $%d = ANY(b.kinds)) OVER (),
			       count(*) FILTER (WHERE $%d = ANY(b.kinds)) OVER ()
			  FROM bundle b JOIN deal d ON d.id = b.deal_id
			 ORDER BY 7 DESC NULLS LAST, b.latest DESC, b.deal_id
			 LIMIT $%d`,
			heldPos, sincePos, sincePos, ownerPos, where, value,
			closedPos, redatedPos, revaluedPos, probabilityPos, capPos)
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("deals: reading the changes Margince made: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var b ChangeBundle
			if err := rows.Scan(&b.DealID, &b.DealName, &b.Version, &b.AuditLogIDs, &b.Kinds, &b.LatestAt, &b.ValueBaseMinor,
				&out.Deals, &out.Closed, &out.Redated, &out.Revalued, &out.Probabilities); err != nil {
				return err
			}
			out.Bundles = append(out.Bundles, b)
		}
		return rows.Err()
	})
	if err != nil {
		return ChangeSummary{}, err
	}
	if len(out.Bundles) > bundleCap {
		out.Bundles, out.Truncated = out.Bundles[:bundleCap], true
	}
	return out, nil
}

// ChangeCursor is where the next page of one deal's changes starts.
type ChangeCursor struct {
	AppliedAt  time.Time
	AuditLogID ids.UUID
}

// DealChanges reads every change Margince made to one deal, newest first and
// paged, whether accepted, undone or still open. It answers 404 for a deal
// the reader may not see.
func (s *Store) DealChanges(ctx context.Context, dealID ids.DealID, after *ChangeCursor, limit int) ([]CorrectionReceipt, bool, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, false, err
	}
	var out []CorrectionReceipt
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisible(ctx, tx, dealTable, dealID.UUID); err != nil {
			return err
		}
		args := []any{dealID}
		arg := func(v any) int { args = append(args, v); return len(args) }
		query, err := correctionReceiptSelect(ctx, arg)
		if err != nil {
			return err
		}
		query += " WHERE c.deal_id = $1"
		if after != nil {
			query += storekit.SQLf(" AND (c.applied_at, c.audit_log_id) < ($%d, $%d)", arg(after.AppliedAt), arg(after.AuditLogID))
		}
		query += storekit.SQLf(" ORDER BY c.applied_at DESC, c.audit_log_id DESC LIMIT $%d", arg(limit+1))
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("deals: reading the deal's changes: %w", err)
		}
		out, err = pgx.CollectRows(rows, scanCorrectionReceipt)
		return err
	})
	if err != nil {
		return nil, false, err
	}
	if len(out) > limit {
		return out[:limit], true, nil
	}
	return out, false, nil
}

// TeamChangeGroup counts one owner's changes of one kind.
type TeamChangeGroup struct {
	OwnerID ids.UUID
	Kind    string
	Deals   int
	Undone  int
	// ValueBaseMinor sums the changed deals' value in the base currency over
	// the deals that have one.
	ValueBaseMinor int64
}

// TeamChangeReport is what Margince changed on a team's deals in a window.
type TeamChangeReport struct {
	BaseCurrency string
	Groups       []TeamChangeGroup
	Rows         []CorrectionReceipt
	Truncated    bool
}

// teamRowCap bounds the rows of one team report. The groups are counted in
// SQL over every change, so a cut list never shrinks a count.
const teamRowCap = 200

// TeamChanges reads the changes Margince made since an instant on deals the
// named owners hold, under the reader's own row scope.
func (s *Store) TeamChanges(ctx context.Context, owners []ids.UUID, since time.Time) (TeamChangeReport, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return TeamChangeReport{}, err
	}
	var out TeamChangeReport
	if len(owners) == 0 {
		return out, nil
	}
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		base, err := s.installation.BaseCurrencyApplied(ctx, tx)
		if err != nil {
			return err
		}
		out.BaseCurrency = base
		// Each read binds its own arguments; both filter alike.
		filter := func(arg func(any) int) (string, error) {
			where := storekit.SQLf(" WHERE c.applied_at >= $%d AND d.owner_id = ANY($%d) AND d.archived_at IS NULL", arg(since), arg(owners))
			scope, err := auth.ScopeClauseFor(ctx, dealTable, "d", arg)
			if scope != "" {
				where += " AND " + scope
			}
			return where, err
		}
		var countArgs []any
		countArg := func(v any) int { countArgs = append(countArgs, v); return len(countArgs) }
		where, err := filter(countArg)
		if err != nil {
			return err
		}
		value, err := auth.MaskedExpressionSQL(ctx, maskObject, "amount_minor", "d",
			BaseValueSQL("CURRENT_DATE", fmt.Sprintf("$%d", countArg(base)), "d"), countArg)
		if err != nil {
			return err
		}
		// One row per deal and kind first, so a deal changed twice in one kind
		// counts and sums once. A value the reader's mask withholds adds nothing.
		groups, err := tx.Query(ctx, storekit.SQLf(`
			SELECT owner_id, correction, count(*), count(*) FILTER (WHERE undone), coalesce(sum(value), 0)::bigint
			  FROM (SELECT d.owner_id, c.correction, c.deal_id,
			               bool_or(c.reversed_at IS NOT NULL) AS undone, max(%s) AS value
			          FROM deal_correction c JOIN deal d ON d.id = c.deal_id%s
			         GROUP BY d.owner_id, c.correction, c.deal_id) per_deal
			 GROUP BY owner_id, correction
			 ORDER BY owner_id, correction`, value, where), countArgs...)
		if err != nil {
			return fmt.Errorf("deals: counting the team's changes: %w", err)
		}
		out.Groups, err = pgx.CollectRows(groups, func(row pgx.CollectableRow) (TeamChangeGroup, error) {
			var g TeamChangeGroup
			return g, row.Scan(&g.OwnerID, &g.Kind, &g.Deals, &g.Undone, &g.ValueBaseMinor)
		})
		if err != nil {
			return err
		}
		var rowArgs []any
		rowArg := func(v any) int { rowArgs = append(rowArgs, v); return len(rowArgs) }
		selected, err := correctionReceiptSelect(ctx, rowArg)
		if err != nil {
			return err
		}
		if where, err = filter(rowArg); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, selected+where+
			storekit.SQLf(" ORDER BY c.applied_at DESC, c.audit_log_id DESC LIMIT %d", teamRowCap+1), rowArgs...)
		if err != nil {
			return fmt.Errorf("deals: reading the team's changes: %w", err)
		}
		out.Rows, err = pgx.CollectRows(rows, scanCorrectionReceipt)
		return err
	})
	if err != nil {
		return TeamChangeReport{}, err
	}
	if len(out.Rows) > teamRowCap {
		out.Rows, out.Truncated = out.Rows[:teamRowCap], true
	}
	return out, nil
}

// readerOf is the seat reading; a caller with no seat reads no reason.
func readerOf(ctx context.Context) ids.UUID {
	if actor, ok := principal.Actor(ctx); ok {
		return actor.UserID
	}
	return ids.Nil
}
