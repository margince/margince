// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package briefs is the Morning-Brief orchestration (E05) — a compose
// subpackage because it is a cross-module composition, never a module:
// deal facts (deals),
// relationship warmth (people §4), and the overnight activity signal
// (activities) rank into the persisted run the home surface reads.
// The deterministic ranker (this file) implements formulas-and-rules
// §10/§10.1; the pure fold it feeds is briefscore.go, the persisted
// read model briefstore.go, the advisory model re-order briefl2.go,
// and the contract transport briefhandlers.go. The composite is the
// fallback rank when the L2 layer is unavailable and the evidence basis
// every ranked item exposes (B-E05.12).
package briefs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// BriefRanking is one deterministic ranking pass: the honest-short queue
// plus the reproducibility metadata a persisted run snapshots.
type BriefRanking struct {
	Queue            []BriefQueueItem
	CandidateCount   int
	RevenueNormMinor int64
	// RevenueNormCurrency is what that figure is in — the installation's base
	// currency as it stood when the rank ran. Carried with the figure because a
	// proportion is only checkable against a NAMED base, and because the base
	// can still change: a norm computed against EUR must not later be read as
	// the USD in force by then.
	RevenueNormCurrency string
	AsOf                time.Time
}

// briefStrengthSource is the compose-injected §4 warmth seam —
// people.Store satisfies it; the brief never reaches into people's SQL.
type briefStrengthSource interface {
	PersonStrength(ctx context.Context, personID ids.PersonID, now time.Time) (people.RelationshipStrength, error)
}

// BriefEngine ranks a rep's open deals and owns the brief_run/brief_item
// read model (B-E05.3b/.13). The L2 ranker (B-E05.2) is optional: without
// one the queue is the deterministic §10.1 composite order, which is also
// the AI-off fallback rank.
type BriefEngine struct {
	pool     *pgxpool.Pool
	strength briefStrengthSource
	ranker   *briefL2Ranker
	log      *slog.Logger
}

func NewBriefEngine(pool *pgxpool.Pool, strength briefStrengthSource) *BriefEngine {
	return &BriefEngine{pool: pool, strength: strength, log: slog.Default()}
}

// WithL2Ranker enables the model-bound re-order over the deterministic
// candidate set. The api role wires it from the brief_ranking model lane;
// without it the engine stays fully functional on the deterministic floor.
func (e *BriefEngine) WithL2Ranker(brain briefBrain, log *slog.Logger) *BriefEngine {
	if log == nil {
		log = slog.Default()
	}
	e.log = log
	e.ranker = &briefL2Ranker{brain: brain, log: log}
	return e
}

// briefBaseValueSQL renders the §6 base-currency value of d (joined to
// its workspace w): native amount when already in base currency, the
// frozen amount_minor_base (0065's GENERATED column — round(amount_minor
// x fx_rate_to_base) computed once at write time) for closed deals, the
// latest daily rate on or before the as-of date for open ones. A missing
// rate yields NULL — the revenue factor floors rather than guessing (a
// wrong number is worse than a missing one). asOfPos is the bind position
// of the as-of date.
// THE SECOND SPELLING, AND WHY. compose.BaseValueSQL is the same expression.
// This package cannot call it — compose imports briefs, so the reverse is a
// cycle — so the two are held character-identical by
// TestOneSpellingOfADealsBaseValue rather than left to drift.
func briefBaseValueSQL(asOfSQL, baseSQL, alias string) string {
	return fmt.Sprintf(`CASE
		WHEN %[3]s.amount_minor IS NULL THEN NULL
		WHEN %[3]s.currency IS NULL OR %[3]s.currency = %[2]s THEN %[3]s.amount_minor
		WHEN %[3]s.fx_rate_to_base IS NOT NULL THEN %[3]s.amount_minor_base
		ELSE (SELECT round(%[3]s.amount_minor * fr.rate)::bigint FROM fx_rate fr
		      WHERE fr.from_currency = %[3]s.currency AND fr.to_currency = %[2]s
		        AND fr.rate_date <= %[1]s::date
		      ORDER BY fr.rate_date DESC LIMIT 1)
	END`, asOfSQL, baseSQL, alias)
}

// briefFacts is everything ONE transaction gathers for a rank: the candidates
// and what is known about them, plus the basis every one of them was measured
// against.
//
// One struct rather than six out-parameters, because they are one read: the
// revenue norm and every candidate's base value have to be measured against the
// same basis, and two reads of one installation-wide value is two chances to
// disagree.
type briefFacts struct {
	facts        map[ids.UUID]briefDealFacts
	order        []ids.UUID
	stakeholders map[ids.UUID][]ids.UUID
	lineage      map[ids.UUID]dealLineage
	// revenueNorm is the base value the revenue factor divides by, and
	// revenueNormCurrency is what that value is in.
	revenueNorm         int64
	revenueNormCurrency string
}

// gather reads one transaction's worth of ranking facts.
func (e *BriefEngine) gather(ctx context.Context, now time.Time, userID ids.UUID) (briefFacts, error) {
	out := briefFacts{
		facts:        map[ids.UUID]briefDealFacts{},
		stakeholders: map[ids.UUID][]ids.UUID{},
		lineage:      map[ids.UUID]dealLineage{},
		revenueNorm:  int64(briefRevenueNormFallbackMinor),
	}
	err := database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		// The rep's last brief view: the previous run's data cutoff. No
		// previous run → the overnight window is all-time.
		lastView, err := briefLastView(ctx, tx, userID)
		if err != nil {
			return err
		}

		// Resolved ONCE for the whole rank: the revenue norm and every
		// candidate's base value must be measured against the same basis, and
		// two reads of one installation-wide value is two chances to disagree.
		base, err := identity.BaseCurrencyOf(ctx, tx)
		if err != nil {
			return err
		}
		norm, err := briefRevenueNorm(ctx, tx, now, base)
		if err != nil {
			return err
		}
		out.revenueNorm = norm
		out.revenueNormCurrency = base

		if err := briefCandidates(ctx, tx, userID, now, base, out.facts, &out.order); err != nil {
			return err
		}
		if err := briefEvidenceRows(ctx, tx, lastView, out.facts, out.order, out.stakeholders); err != nil {
			return err
		}
		// Why each returning deal is back, for the whole candidate set at once.
		// It reads AFTER the candidates because it is asked about them: a deal
		// the suppression rule is still holding out has no lineage to tell.
		out.lineage, err = briefLineage(ctx, tx, userID, out.order, now)
		return err
	})
	if err != nil {
		return briefFacts{}, err
	}
	return out, nil
}

// Rank computes the deterministic §10.1 queue for the acting rep at one
// instant. It is a read: nothing is persisted (SnapshotRun does that),
// and the candidate set is bounded by the caller's own row scope — a
// rep's brief only ranks deals they can see.
func (e *BriefEngine) Rank(ctx context.Context, now time.Time) (BriefRanking, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return BriefRanking{}, err
	}
	userID, err := briefUser(ctx)
	if err != nil {
		return BriefRanking{}, err
	}

	gathered, err := e.gather(ctx, now, userID)
	if err != nil {
		return BriefRanking{}, err
	}
	facts, order := gathered.facts, gathered.order
	revenueNorm := gathered.revenueNorm
	stakeholders := gathered.stakeholders
	lineage := gathered.lineage

	if err := e.resolveWarmth(ctx, now, facts, stakeholders); err != nil {
		return BriefRanking{}, err
	}

	scored := make([]BriefQueueItem, 0, len(order))
	for _, dealID := range order {
		item := briefScore(facts[dealID], revenueNorm, now)
		// Attached AFTER scoring, never inside it. briefScore is a pure
		// function of the ranking facts and is tested as one; lineage explains
		// why a deal is in the queue and must not be able to change where it
		// sits, or "you dismissed this" would become a reason to rank it.
		if back, returning := lineage[dealID]; returning {
			item.Lineage = &ItemLineage{
				DismissedOn:  back.dismissedOn,
				ReturnedWith: back.returnedWith,
			}
		}
		scored = append(scored, item)
	}

	// The deterministic floor first: the full §10.1 candidate set, ordered
	// and evidence-gated. The L2 layer re-orders WITHIN it (never below the
	// cutoff), then the honest-short truncation and the post-L2 gate close
	// over the result.
	candidates := briefCandidateOrder(scored, facts)
	if err := validateBriefCandidates(candidates); err != nil {
		return BriefRanking{}, err
	}
	ordered := candidates
	if e.ranker != nil {
		ordered = e.ranker.reorder(ctx, candidates)
	}
	queue := ordered
	if len(queue) > briefQueueTarget {
		queue = queue[:briefQueueTarget]
	}
	if err := validateBriefQueue(queue, candidates); err != nil {
		return BriefRanking{}, err
	}
	return BriefRanking{
		Queue:               queue,
		CandidateCount:      len(candidates),
		RevenueNormMinor:    revenueNorm,
		RevenueNormCurrency: gathered.revenueNormCurrency,
		AsOf:                now,
	}, nil
}

// briefUser resolves the human the brief belongs to. The brief is a
// personal lens — a principal without a user identity (the system actor)
// has no "my morning" to rank.
func briefUser(ctx context.Context) (ids.UUID, error) {
	p, ok := principal.Actor(ctx)
	if !ok {
		return ids.Nil, errors.New("brief: no actor bound to context")
	}
	if p.UserID.IsZero() {
		return ids.Nil, apperrors.ErrPermissionDenied
	}
	return p.UserID, nil
}

// briefLastView reads the previous run's data cutoff for this user; nil
// when the user never had a brief.
func briefLastView(ctx context.Context, tx pgx.Tx, userID ids.UUID) (*time.Time, error) {
	var lastView *time.Time
	err := tx.QueryRow(ctx, `
		SELECT as_of FROM brief_run
		WHERE user_id = $1
		ORDER BY generated_at DESC, id DESC
		LIMIT 1`, userID).Scan(&lastView)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return lastView, nil
}

// briefRevenueNorm computes REVENUE_NORM: the workspace P90 base deal
// value over live deals with an evidencable amount, or the fixed
// fallback below ten deals of history.
//
// The basis is a bind parameter, and the workspace join that used to supply it
// is gone with it: it earned its place only by carrying base_currency, which
// is now one installation-wide value rather than a column on a joinable row.
func briefRevenueNorm(ctx context.Context, tx pgx.Tx, now time.Time, base string) (int64, error) {
	var valued int
	var p90 *float64
	err := tx.QueryRow(ctx, fmt.Sprintf(`
		WITH sized AS (
			SELECT %s AS base_value
			FROM deal d
			WHERE d.archived_at IS NULL
		)
		SELECT count(*), percentile_cont(%v) WITHIN GROUP (ORDER BY base_value::double precision)
		FROM sized WHERE base_value IS NOT NULL`,
		briefBaseValueSQL("$1", "$2", "d"), briefRevenueNormPercentile), now.UTC(), base).Scan(&valued, &p90)
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
			      THEN bi.snoozed_until > $%d
			      ELSE NOT EXISTS (
				SELECT 1 FROM activity a
				JOIN activity_link l ON l.activity_id = a.id AND l.deal_id = d.id
				WHERE a.archived_at IS NULL AND a.occurred_at > bi.state_at
				  -- Not after this instant. A future-dated activity has not
				  -- happened, so treating it as "the deal moved" brings a
				  -- dismissed deal back for something still to come — and the
				  -- lineage read bounds itself the same way, so an unbounded
				  -- one here would return deals whose card can say nothing.
				  AND a.occurred_at <= $%d) END)`,
		briefBaseValueSQL(fmt.Sprintf("$%d", asOfPos), fmt.Sprintf("$%d", basePos), "d"), userPos, asOfPos, asOfPos)
	if scope != "" {
		q += " AND " + scope
	}
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
// momentum evidence) and stakeholder persons, after the candidate rows
// are drained (one connection, one active query).
func briefEvidenceRows(ctx context.Context, tx pgx.Tx, lastView *time.Time, facts map[ids.UUID]briefDealFacts, order []ids.UUID, stakeholders map[ids.UUID][]ids.UUID) error {
	// The seat edge's admission is resolved ONCE, ahead of the loop: it is a
	// property of the caller, not of the deal being read, and asking per deal
	// would put a grant lookup inside a per-row loop for an answer that cannot
	// change. A refused caller runs no stakeholder query at all — the brief
	// simply carries no seat evidence, which is the same shape as a deal with
	// no stakeholders on it.
	edgeArgs, edgeBound, mayReadSeats, err := seatEvidenceBound(ctx)
	if err != nil {
		return err
	}
	for _, dealID := range order {
		f := facts[dealID]
		overnight, err := collectIDList(tx.Query(ctx, `
			SELECT a.id FROM activity a
			JOIN activity_link l ON l.activity_id = a.id AND l.deal_id = $1
			WHERE a.archived_at IS NULL
			  AND ($2::timestamptz IS NULL OR a.occurred_at > $2)
			ORDER BY a.occurred_at DESC, a.id DESC
			LIMIT $3`, dealID, lastView, briefOvernightEvidenceCap))
		if err != nil {
			return err
		}
		f.overnightActivityIDs = overnight
		facts[dealID] = f

		if !mayReadSeats {
			continue
		}
		persons, err := collectIDList(tx.Query(ctx, fmt.Sprintf(`
			SELECT r.person_id FROM relationship r
			WHERE r.kind = 'deal_stakeholder' AND r.deal_id = $1 AND r.archived_at IS NULL
			  AND (%s)
			ORDER BY r.person_id`, edgeBound), append([]any{dealID}, edgeArgs...)...))
		if err != nil {
			return err
		}
		stakeholders[dealID] = persons
	}
	return nil
}

// seatEvidenceBound resolves the seat edge's admission for the stakeholder
// evidence read: the arguments its clause binds, the clause itself, and whether
// the caller may run the read at all.
//
// The registrar returns positions offset by one because the statement it feeds
// already spends $1 on the deal id. Getting that wrong would bind the deal id
// to a scope predicate, which is why the offset lives here with the statement
// it belongs to rather than at the call site.
func seatEvidenceBound(ctx context.Context) (args []any, clause string, admitted bool, err error) {
	clause, err = auth.EdgeReadScope(ctx, "r", func(v any) int {
		args = append(args, v)
		return len(args) + 1
	})
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil, "", false, nil
	}
	if err != nil {
		return nil, "", false, err
	}
	if clause == "" {
		clause = "TRUE"
	}
	return args, clause, true, nil
}

// resolveWarmth fills each deal's warmth from its strongest visible
// stakeholder through the injected §4 seam. A stakeholder outside the
// caller's row scope — or a caller with no person grant at all —
// contributes nothing: the warmth factor floors instead of out-seeing
// the people list.
func (e *BriefEngine) resolveWarmth(ctx context.Context, now time.Time, facts map[ids.UUID]briefDealFacts, stakeholders map[ids.UUID][]ids.UUID) error {
	cache := map[ids.UUID]people.RelationshipStrength{}
	for dealID, persons := range stakeholders {
		f := facts[dealID]
		for _, personID := range persons {
			st, ok := cache[personID]
			if !ok {
				var err error
				st, err = e.strength.PersonStrength(ctx, ids.From[ids.PersonKind](personID), now)
				switch {
				case errors.Is(err, apperrors.ErrNotFound), errors.Is(err, apperrors.ErrPermissionDenied):
					// Invisible to this caller: no strength to disclose.
					st = people.RelationshipStrength{}
				case err != nil:
					return err
				}
				cache[personID] = st
			}
			if st.Strength > f.warmthStrength {
				f.warmthStrength = st.Strength
				f.warmthEvidence = make([]ids.UUID, len(st.ContributingIDs))
				for i, activityID := range st.ContributingIDs {
					f.warmthEvidence[i] = activityID.UUID
				}
			}
		}
		facts[dealID] = f
	}
	return nil
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
