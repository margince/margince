// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package briefs

// The brief read model (B-E05.3b): a ranking pass persists as one
// brief_run plus its brief_item rows so the home open re-reads the
// latest run instead of re-ranking, and the acted/dismissed/snoozed
// marks (B-E05.13, A77) live on the items as the per-rep queue state
// the next run's candidate filter honors. A brief is strictly personal: every
// read and mark resolves through run.user_id = the acting principal —
// another rep's brief reads as not-found, never as forbidden.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Brief item states (data-model §12.5). A snooze (A77/AC-home-6) hides
// the item until `snoozed_until` passes, then it re-surfaces as
// actionable — unlike acted/dismissed, whose return needs a material
// change on the deal.
const (
	briefStateNew       = "new"
	briefStateActed     = "acted"
	briefStateDismissed = "dismissed"
	briefStateSnoozed   = "snoozed"
)

// BriefRun is one persisted brief for a rep: the queue snapshot plus the
// metadata that reproduces it (candidate count, the revenue norm the
// composite folded with, the as-of cutoff the next run reads "overnight"
// from).
type BriefRun struct {
	ID               ids.UUID
	UserID           ids.UUID
	GeneratedAt      time.Time
	AsOf             time.Time
	LocalDay         time.Time
	CandidateCount   int
	RevenueNormMinor int64
	// RevenueNormCurrency is what RevenueNormMinor is in. Empty on a run stored
	// before the column existed, which the wire omits rather than guessing at.
	RevenueNormCurrency string
	// Narrative is the overnight agent's sentence about the night, empty when
	// no pass has written one — which AnnotatedAt is what distinguishes from a
	// pass that ran and had nothing to say.
	Narrative   string
	AnnotatedAt *time.Time
	Items       []BriefRunItem
}

// BriefRunItem is one persisted queue entry with its per-rep state.
type BriefRunItem struct {
	ID           ids.UUID
	DealID       ids.UUID
	Rank         int
	Composite    float64
	Features     BriefFeatureVector
	EvidenceIDs  []ids.UUID
	State        string
	StateAt      *time.Time
	SnoozedUntil *time.Time
	// Finding is what the overnight agent found about this deal, empty when no
	// pass has annotated the run it belongs to.
	Finding string
	// Lineage is set when this deal is back after the rep dismissed it. Nil is
	// the ordinary case.
	Lineage *ItemLineage
}

// SnapshotRun ranks and persists one brief run for the acting rep at the
// given instant. The write is audited in the run's own transaction; the
// events.md §5 catalog defines no brief.* type, so the run — like voice
// DNA and lists — is audit-only by the closed-verb law (see the
// writeshape gate's ratified waivers).
//
// A rep has ONE run per local day, and uq_brief_run_user_day is what makes
// that true rather than hoped for: the overnight dispatcher and the boot pass
// that backfills a missed night can reach the same rep in the same morning,
// and so can a rep pressing refresh while the night's job is still running.
// The loser of that race READS the winner instead of failing — a duplicate is
// not an error the caller should see, it is the constraint doing its job — so
// the job that lost still reports success and the rep still gets a brief.
func (e *BriefEngine) SnapshotRun(ctx context.Context, now time.Time) (BriefRun, error) {
	run, _, err := e.SnapshotRunForDay(ctx, now)
	return run, err
}

// SnapshotRunForDay is SnapshotRun, additionally reporting whether this call is
// the one that assembled the day's run. The transport needs the distinction to
// answer 201 versus 200; nothing else does.
//
// The day's existing run is looked for BEFORE ranking, not after. Ranking is
// the expensive half — the candidate SQL plus, on the api role, a model call —
// and once the night has assembled the morning that work is thrown away by the
// insert's conflict clause. Paying for it to discard it also made the answer
// worse than useless on the api role: the caller waited for a model re-order
// and was then served the deterministic run the worker had already stored.
func (e *BriefEngine) SnapshotRunForDay(ctx context.Context, now time.Time) (BriefRun, bool, error) {
	existing, err := e.LatestRun(ctx, now)
	switch {
	case err == nil:
		return existing, false, nil
	case !errors.Is(err, apperrors.ErrNotFound):
		return BriefRun{}, false, err
	}

	ranking, err := e.Rank(ctx, now)
	if err != nil {
		return BriefRun{}, false, err
	}
	userID, err := briefUser(ctx)
	if err != nil {
		return BriefRun{}, false, err
	}

	run := BriefRun{
		ID:                  ids.NewV7(),
		UserID:              userID,
		GeneratedAt:         now,
		AsOf:                ranking.AsOf,
		CandidateCount:      ranking.CandidateCount,
		RevenueNormMinor:    ranking.RevenueNormMinor,
		RevenueNormCurrency: ranking.RevenueNormCurrency,
	}
	queueDeals := make([]ids.UUID, 0, len(ranking.Queue))
	var joinedExisting bool
	err = database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		day, err := localDay(ctx, tx, now)
		if err != nil {
			return err
		}
		run.LocalDay = day
		// This, not the read above, is what makes one run per day true: the read
		// is an optimisation that skips the ranking when the answer is already
		// there, and two callers can still pass it together.
		//
		// DO NOTHING rather than DO UPDATE: the run already there was assembled
		// from the same day's facts and the rep may already have marked its
		// items, so replacing it would silently discard those marks.
		inserted, err := insertRunIfDayFree(ctx, tx, run)
		if err != nil {
			return err
		}
		if !inserted {
			joinedExisting = true
			return nil
		}
		run.Items, err = insertRunItems(ctx, tx, run.ID, ranking.Queue)
		if err != nil {
			return err
		}
		for _, item := range run.Items {
			queueDeals = append(queueDeals, item.DealID)
		}
		_, err = storekit.Audit(ctx, tx, "create", "brief_run", run.ID, nil, map[string]any{
			"user_id":               run.UserID,
			"as_of":                 run.AsOf,
			"local_day":             run.LocalDay.Format(time.DateOnly),
			"candidate_count":       run.CandidateCount,
			"revenue_norm_minor":    run.RevenueNormMinor,
			"revenue_norm_currency": run.RevenueNormCurrency,
			"queue_deal_ids":        queueDeals,
		})
		return err
	})
	if err != nil {
		return BriefRun{}, false, err
	}
	if joinedExisting {
		// The winner is read through the ordinary day read, so the caller gets a
		// run scoped and snooze-resolved exactly as an on-open read would give
		// it — not the half-built one this call was assembling.
		existing, err := e.LatestRun(ctx, now)
		return existing, false, err
	}
	return run, true, nil
}

// insertRunItems writes the ranked queue as this run's items, in rank order,
// and returns them as persisted.
//
// Rank comes from the queue's position rather than from the composite: the L2
// re-order is allowed to disagree with the deterministic score, and what the
// rep sees is the order the queue arrived in.
func insertRunItems(ctx context.Context, tx pgx.Tx, runID ids.UUID, queue []BriefQueueItem) ([]BriefRunItem, error) {
	items := make([]BriefRunItem, 0, len(queue))
	for i, item := range queue {
		features, err := json.Marshal(item.Features)
		if err != nil {
			return nil, err
		}
		persisted := BriefRunItem{
			ID:          ids.NewV7(),
			DealID:      item.DealID,
			Rank:        i + 1,
			Composite:   item.Composite,
			Features:    item.Features,
			EvidenceIDs: item.EvidenceIDs,
			State:       briefStateNew,
			Lineage:     item.Lineage,
		}
		// Both halves or neither — brief_item_lineage_whole says so, and a
		// sentence with one half is one the screen cannot finish.
		var dismissedOn, returnedWith *time.Time
		if item.Lineage != nil {
			dismissedOn = &item.Lineage.DismissedOn
			returnedWith = &item.Lineage.ReturnedWith
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO brief_item (id, brief_run_id, deal_id, rank, composite, feature_vector, evidence_ids, state,
			                        returned_after_dismissal_on, returned_with_activity_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			persisted.ID, runID, persisted.DealID, persisted.Rank,
			persisted.Composite, features, persisted.EvidenceIDs, briefStateNew,
			dismissedOn, returnedWith); err != nil {
			return nil, err
		}
		items = append(items, persisted)
	}
	return items, nil
}

// LatestRun re-reads the acting rep's brief FOR THE CURRENT LOCAL DAY — the
// on-open path that must not re-rank. No run for today reads as not-found.
//
// Today's, not the newest: a rep back from a week's holiday would otherwise
// open Home to last Monday's ranking presented as this morning's, with an
// as-of line she has no reason to disbelieve. A brief that is stale is worse
// than one that is absent, because absence is visible and staleness is not.
// This read is where a snooze resolves (A77/AC-home-6): an expired
// snooze flips back to actionable inside the read's own transaction,
// and a still-running one keeps its item hidden — so the returned run
// is always what the rep should see NOW, without a refresh.
//
// The queue may therefore be shorter than the run it re-reads, and the
// answer says nothing about that on purpose: a count of what the row
// scope removed is the side channel existence-hiding closes. The agent
// door already states the query-level fact for both of them —
// agents.noteRowScope raises BYO-RES-2's warning on every tool call by
// a bounded actor, which is exactly when an item can drop here.
func (e *BriefEngine) LatestRun(ctx context.Context, now time.Time) (BriefRun, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return BriefRun{}, err
	}
	userID, err := briefUser(ctx)
	if err != nil {
		return BriefRun{}, err
	}

	var run BriefRun
	err = database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		day, err := localDay(ctx, tx, now)
		if err != nil {
			return err
		}
		run, err = scanRun(tx.QueryRow(ctx, runSelect+`
			WHERE user_id = $1 AND local_day = $2`, userID, day))
		if err != nil {
			return err
		}

		if err := resurfaceExpiredSnoozes(ctx, tx, run.ID, now); err != nil {
			return err
		}

		run.Items, err = readRunItems(ctx, tx, run.ID)
		return err
	})
	if err != nil {
		return BriefRun{}, err
	}
	return run, nil
}

// runSelect is the run's own columns, in one place.
//
// Shared by the day read above and the mail claim's read of one run by id
// (briefmail.go): two spellings of a column list is how one of them comes to
// miss a column the other added, and the miss is silent — the struct simply
// carries a zero.
const runSelect = `
	SELECT id, user_id, generated_at, as_of, local_day, candidate_count, revenue_norm_minor,
	       revenue_norm_currency, coalesce(narrative, ''), annotated_at
	FROM brief_run`

// scanRun reads one runSelect row, answering ErrNotFound for no row.
func scanRun(row pgx.Row) (BriefRun, error) {
	var run BriefRun
	err := row.Scan(&run.ID, &run.UserID, &run.GeneratedAt, &run.AsOf, &run.LocalDay,
		&run.CandidateCount, &run.RevenueNormMinor, &run.RevenueNormCurrency,
		&run.Narrative, &run.AnnotatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return BriefRun{}, apperrors.ErrNotFound
	}
	if err != nil {
		return BriefRun{}, err
	}
	return run, nil
}

// readRunItems reads back one run's visible queue in rank order.
//
// The join is the point: a brief item is a REFERENCE to a deal, persisted
// when the ranking queued it and served for as long as the run lives, so
// the deal's row scope is re-applied HERE rather than inherited from the
// snapshot that wrote it. A deal archived or otherwise gone from the
// rep's read since then leaves the queue, which is the same answer the
// deal's own read gives on that id (deals.Store.GetDeal: the object gate,
// then auth.EnsureVisible).
//
// Unexpired snoozes stay hidden; everything else, including the rows the
// caller just re-surfaced, reads back.
func readRunItems(ctx context.Context, tx pgx.Tx, runID ids.UUID) ([]BriefRunItem, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	runPos := arg(runID)

	scope, err := auth.ScopeClauseFor(ctx, "deal", "d", arg)
	if err != nil {
		return nil, err
	}
	q := fmt.Sprintf(`
		SELECT bi.id, bi.deal_id, bi.rank, bi.composite, bi.feature_vector, bi.evidence_ids, bi.state, bi.state_at, bi.snoozed_until, coalesce(bi.finding, ''),
		       bi.returned_after_dismissal_on, bi.returned_with_activity_at
		FROM brief_item bi
		JOIN deal d ON d.id = bi.deal_id
		WHERE bi.brief_run_id = $%d AND bi.state <> 'snoozed'`, runPos)
	if scope != "" {
		q += " AND " + scope
	}
	q += " ORDER BY bi.rank"

	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []BriefRunItem
	for rows.Next() {
		item, err := scanBriefItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// resurfaceExpiredSnoozes flips a run's expired snoozes back to
// actionable — the A77 re-surface half of the snooze contract. Each flip
// is a state change on per-rep queue data, so it is audited like the
// rep's own marks (brief rows are audit-only; no brief.* event exists in
// the events.md catalog).
func resurfaceExpiredSnoozes(ctx context.Context, tx pgx.Tx, runID ids.UUID, now time.Time) error {
	resurfaced, err := collectIDList(tx.Query(ctx, `
		UPDATE brief_item
		SET state = 'new', state_at = NULL, snoozed_until = NULL
		WHERE brief_run_id = $1 AND state = 'snoozed' AND snoozed_until <= $2
		RETURNING id`, runID, now.UTC()))
	if err != nil {
		return err
	}
	for _, itemID := range resurfaced {
		before := map[string]any{auditFieldState: briefStateSnoozed}
		after := map[string]any{
			auditFieldState: briefStateNew, auditFieldStateAt: nil,
			auditFieldSnoozedUntil: nil,
		}
		if _, err := storekit.Audit(ctx, tx, "update", "brief_item", itemID, before, after); err != nil {
			return err
		}
	}
	return nil
}

// scanBriefItem reads one brief_item row in the LatestRun column order.
func scanBriefItem(rows pgx.Rows) (BriefRunItem, error) {
	var item BriefRunItem
	var featuresRaw []byte
	var dismissedOn *time.Time
	var returnedWith *time.Time
	if err := rows.Scan(&item.ID, &item.DealID, &item.Rank, &item.Composite,
		&featuresRaw, &item.EvidenceIDs, &item.State, &item.StateAt, &item.SnoozedUntil,
		&item.Finding, &dismissedOn, &returnedWith); err != nil {
		return BriefRunItem{}, err
	}
	if err := json.Unmarshal(featuresRaw, &item.Features); err != nil {
		return BriefRunItem{}, fmt.Errorf("brief: item %s carries an unreadable feature vector: %w", item.ID, err)
	}
	// The CHECK keeps the pair whole, so one non-null half means both are.
	if dismissedOn != nil && returnedWith != nil {
		item.Lineage = &ItemLineage{DismissedOn: *dismissedOn, ReturnedWith: *returnedWith}
	}
	return item, nil
}
