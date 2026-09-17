// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package briefs

// The rows the ranker folds. Every statement behind one pass of §10/§10.1 is
// here, and the arithmetic that turns them into a queue is in briefrank.go
// beside briefscore.go's pure fold.
//
// The seam is the mask, which is the reason these are worth reading together:
// three of the four reads below touch a deal's money, and they answer the same
// question in two different shapes — the norm FILTERS the rows it may not
// price, because an aggregate that counted a withheld figure discloses it; the
// candidate query NULLS it, because a card the reader has work on must still be
// drawn. Split by file they read as two policies. Here they read as one.

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// briefUnnarrowed is the predicate that narrows nothing — what a clause becomes
// when the caller's scope, mask or grant places no bound on it. A name rather
// than a bare TRUE in the middle of a WHERE, where the reader has to work out
// whether the statement meant "everything" or "this part is unfinished".
const briefUnnarrowed = "TRUE"

// briefRevenueNorm computes REVENUE_NORM: the P90 base deal value over the
// live deals whose amount this reader may see, or the fixed fallback below ten
// deals of history.
//
// The basis is a bind parameter, and the workspace join that used to supply it
// is gone with it: it earned its place only by carrying base_currency, which
// is now one installation-wide value rather than a column on a joinable row.
//
// THE READER'S deals and not the workspace's, because this figure is SERVED —
// `revenue_norm_minor` on the morning brief — and a percentile is a reading of
// the amounts it was taken over. Filtered rather than nulled, which is the
// opposite of the candidate query one page down and for the reason the two
// shapes differ: an aggregate that counted a withheld figure discloses it, a
// row that hides one still has a card to draw. A masked reader with nothing
// left to take the percentile over falls to the same fallback a young
// installation gets, which is already the "not enough history to normalize
// against" answer rather than a new one.
func briefRevenueNorm(ctx context.Context, tx pgx.Tx, now time.Time, base string) (int64, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	asOfPos := arg(now.UTC())
	basePos := arg(base)
	maskClause, masked, err := auth.MaskExcludedClause(ctx, "deal", "amount_minor", "d", arg)
	if err != nil {
		return 0, err
	}
	summable := briefUnnarrowed
	if masked {
		summable = maskClause
	}
	var valued int
	var p90 *float64
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		WITH sized AS (
			SELECT %s AS base_value
			FROM deal d
			WHERE d.archived_at IS NULL AND %s
		)
		SELECT count(*), percentile_cont(%v) WITHIN GROUP (ORDER BY base_value::double precision)
		FROM sized WHERE base_value IS NOT NULL`,
		briefBaseValueSQL(fmt.Sprintf("$%d", asOfPos), fmt.Sprintf("$%d", basePos), "d"),
		summable, briefRevenueNormPercentile), args...).Scan(&valued, &p90)
	if err != nil {
		return 0, err
	}
	if valued < briefRevenueNormMinDeals || p90 == nil || *p90 <= 0 {
		return briefRevenueNormFallbackMinor, nil
	}
	return int64(math.Round(*p90)), nil
}

// briefCandidates gathers the open, row-scoped candidate deals, minus
// the ones this user acted on or dismissed with no linked activity since
// the mark (B-E05.13: a dismissed deal reappears only when it materially
// changed; an unchanged one stays out — across ALL previous runs, not
// just the last). A snoozed item suppresses its deal on time alone
// (A77/AC-home-6): out while snoozed_until lies ahead, back once it
// passes — no material change required.
func briefCandidates(ctx context.Context, tx pgx.Tx, userID ids.UUID, now time.Time,
	base string, facts map[ids.UUID]briefDealFacts, order *[]ids.UUID,
) error {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	asOfPos := arg(now.UTC())
	userPos := arg(userID)
	basePos := arg(base)

	scope, err := auth.ScopeClauseFor(ctx, "deal", "d", arg)
	if err != nil {
		return err
	}
	// Only an activity the rep may read can bring a dismissed deal back —
	// the lineage line then names it, and the two must agree.
	readable, err := briefActivityClause(ctx, "a", arg)
	if err != nil {
		return err
	}
	// A masked amount must not score. The revenue factor is min(1, value/NORM)
	// and it is SERVED — MorningBriefFeatureVector.Revenue on the wire — so a
	// figure the deal list withholds would come back as a ratio a reader can
	// invert once they know the norm from a deal of their own.
	//
	// Nulled rather than filtered: the deal is still the rep's to act on — a
	// colleague's deal carrying an open task assigned to them reaches this
	// query through the responsibility clause below — and dropping it would
	// take a card off the brief to hide a number. A nil base value is a shape
	// this fold already has, and scores its floor for the same reason a missing
	// FX rate does: the factor floors rather than guessing.
	baseValue, err := auth.MaskedExpressionSQL(ctx, "deal", "amount_minor", "d",
		briefBaseValueSQL(fmt.Sprintf("$%d", asOfPos), fmt.Sprintf("$%d", basePos), "d"), arg)
	if err != nil {
		return err
	}
	q := fmt.Sprintf(`
		SELECT d.id, s.win_probability, %s, d.expected_close_date
		FROM deal d
		JOIN stage s ON s.id = d.stage_id
		WHERE d.archived_at IS NULL AND d.status = 'open'
		  AND NOT EXISTS (
			SELECT 1 FROM brief_item bi
			JOIN brief_run br ON br.id = bi.brief_run_id
			WHERE br.user_id = $%d AND bi.deal_id = d.id AND bi.state <> 'new'
			  AND CASE WHEN bi.state = 'snoozed'
			      -- Still suppressed while the snooze holds. A time snooze
			      -- holds until its moment; the other two hold until the
			      -- shared predicate says the world moved.
			      THEN CASE WHEN bi.reopen_on = 'time'
			           THEN bi.snoozed_until > $%d
			           ELSE NOT %s END
			      ELSE NOT EXISTS (
				SELECT 1 FROM activity a
				JOIN activity_link l ON l.activity_id = a.id AND l.deal_id = d.id
				WHERE a.archived_at IS NULL AND a.occurred_at > bi.state_at
				  -- Not after this instant. A future-dated activity has not
				  -- happened, so treating it as "the deal moved" brings a
				  -- dismissed deal back for something still to come — and the
				  -- lineage read bounds itself the same way, so an unbounded
				  -- one here would return deals whose card can say nothing.
				  AND a.occurred_at <= $%d
				  AND %s) END)`,
		baseValue, userPos, asOfPos,
		briefSnoozeLiftedSQL("d.id", "bi.reopen_on", "bi.reopen_ref", "bi.state_at", fmt.Sprintf("$%d", asOfPos)),
		asOfPos, readable)
	if scope != "" {
		q += " AND " + scope
	}
	// RESPONSIBILITY, not merely visibility — and it belongs HERE rather than
	// after the ranking, which is the whole defect.
	//
	// A deal is workspace-readable in this product: auth.ScopeClauseFor renders
	// no predicate for a rep on `deal`, so every seat that may read one may read
	// them all. The overnight queue then takes the top seven by score and the
	// worklist narrows to "mine" afterwards — so a rep whose colleagues carry
	// larger deals watched all seven slots fill with deals that were never
	// theirs to act on, and their own work never entered the ranking at all.
	// One observed morning selected six colleague deals out of seven.
	//
	// Applied before the cap, the ranking competes among the deals this contact
	// can actually move. Access to a colleague's deal is not responsibility for
	// it; the team view is where breadth belongs.
	q += fmt.Sprintf(`
		  AND (d.owner_id = $%d
		       OR EXISTS (
			SELECT 1 FROM activity a
			JOIN activity_link l ON l.activity_id = a.id AND l.deal_id = d.id
			WHERE a.kind = 'task' AND NOT a.is_done
			  AND a.archived_at IS NULL AND a.assignee_id = $%d))`, userPos, userPos)
	q += " ORDER BY d.id"

	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var f briefDealFacts
		if err := rows.Scan(&f.dealID, &f.winProbability, &f.baseValueMinor, &f.expectedClose); err != nil {
			return err
		}
		facts[f.dealID] = f
		*order = append(*order, f.dealID)
	}
	return rows.Err()
}

// briefEvidenceRows gathers each candidate's overnight activities (the
// momentum evidence) and stakeholder contacts, after the candidate rows
// are drained (one connection, one active query).
//
// It reports whether the seat evidence was READABLE, because that is not the
// same as a deal having no stakeholders. A refused caller gets a floored warmth
// factor on every deal, which reorders the queue; the caller has to be told, or
// they read an order that is wrong rather than one that is short.
func briefEvidenceRows(
	ctx context.Context, tx pgx.Tx, lastView *time.Time, asOf time.Time,
	facts map[ids.UUID]briefDealFacts, order []ids.UUID, stakeholders map[ids.UUID][]ids.UUID,
) (seatsReadable bool, err error) {
	// The seat edge's admission is resolved ONCE, ahead of the loop: it is a
	// property of the caller, not of the deal being read, and asking per deal
	// would put a grant lookup inside a per-row loop for an answer that cannot
	// change. A refused caller runs no stakeholder query at all.
	edgeArgs, edgeBound, mayReadSeats, err := seatEvidenceBound(ctx)
	if err != nil {
		return false, err
	}
	// One statement for every deal, rendered once: the activity clause is a
	// property of the caller, like the seat edge above. The deal's slot is
	// registered first and rebound per deal, so the clause's own parameters
	// keep the positions it rendered against.
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	dealPos, sincePos, asOfPos, capPos := arg(ids.Nil), arg(lastView), arg(asOf.UTC()), arg(briefOvernightEvidenceCap)
	readable, err := briefActivityClause(ctx, "a", arg)
	if err != nil {
		return false, err
	}
	overnightSQL := fmt.Sprintf(`
		SELECT a.id FROM activity a
		JOIN activity_link l ON l.activity_id = a.id AND l.deal_id = $%[1]d
		WHERE a.archived_at IS NULL
		  AND ($%[2]d::timestamptz IS NULL OR a.occurred_at > $%[2]d)
		  -- Bounded at the cutoff, the way the dismissal filter above is: a
		  -- future-dated row has not happened, so counting it as overnight
		  -- movement claims the deal moved for something still to come.
		  AND a.occurred_at <= $%[3]d
		  AND %[5]s
		ORDER BY a.occurred_at DESC, a.id DESC
		LIMIT $%[4]d`, dealPos, sincePos, asOfPos, capPos, readable)
	for _, dealID := range order {
		f := facts[dealID]
		args[dealPos-1] = dealID
		overnight, err := collectIDList(tx.Query(ctx, overnightSQL, args...))
		if err != nil {
			return false, err
		}
		f.overnightActivityIDs = overnight
		facts[dealID] = f

		if !mayReadSeats {
			continue
		}
		contacts, err := collectIDList(tx.Query(ctx, fmt.Sprintf(`
			SELECT r.contact_id FROM relationship r
			WHERE r.kind = 'deal_stakeholder' AND r.deal_id = $1 AND r.archived_at IS NULL
			  AND (%s)
			ORDER BY r.contact_id`, edgeBound), append([]any{dealID}, edgeArgs...)...))
		if err != nil {
			return false, err
		}
		stakeholders[dealID] = contacts
	}
	return mayReadSeats, nil
}

// collectIDList drains a single-uuid-column result set (the compose
// spelling of the modules' collectIDs helpers).
func collectIDList(rows pgx.Rows, err error) ([]ids.UUID, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ids.UUID
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
