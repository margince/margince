// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package briefs is the Morning-Brief orchestration (E05) — a compose
// subpackage because it is a cross-module composition, never a module:
// deal facts (deals),
// relationship warmth (contacts §4), and the overnight activity signal
// (activities) rank into the persisted run the home surface reads.
// The deterministic ranker (this file) implements formulas-and-rules
// §10/§10.1 over the rows briefreads.go gathers; the pure fold it feeds
// is briefscore.go, the persisted read model briefstore.go, the advisory
// model re-order briefl2.go, and the contract transport briefhandlers.go. The composite is the
// fallback rank when the L2 layer is unavailable and the evidence basis
// every ranked item exposes (B-E05.12).
package briefs

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
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
	// FactorsOmitted names the ranking factors this run could not read. Empty
	// is the ordinary answer and is not the same as a factor that scored zero:
	// a floored factor the reader is not told about makes every deal rank lower
	// than it is, with nothing marking why.
	FactorsOmitted []string
}

// briefStrengthSource is the compose-injected §4 warmth seam —
// contacts.Store satisfies it; the brief never reaches into contacts's SQL.
type briefStrengthSource interface {
	ContactStrength(ctx context.Context, contactID ids.ContactID, now time.Time) (contacts.RelationshipStrength, error)
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
	// seatsReadable is whether the caller holds the edge grant the stakeholder
	// read needs. False is NOT "this rep's deals have no stakeholders": it
	// floors the warmth factor for every deal, which reorders the queue.
	seatsReadable bool
	// today is the installation-zone calendar day the timing factor measures
	// "days until expected close" from — the SAME day brief_run.local_day is
	// stamped in, resolved once in the gather transaction so the score reads the
	// morning the run belongs to.
	today time.Time
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

		// The installation-zone day the timing factor measures against,
		// resolved in this transaction so it is the same day the run is stamped.
		out.today, err = localDay(ctx, tx, now)
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
		out.seatsReadable, err = briefEvidenceRows(ctx, tx, lastView, now, out.facts, out.order, out.stakeholders)
		if err != nil {
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

	warmthReadable, err := e.resolveWarmth(ctx, now, facts, stakeholders)
	if err != nil {
		return BriefRanking{}, err
	}

	scored := make([]BriefQueueItem, 0, len(order))
	for _, dealID := range order {
		item := briefScore(facts[dealID], revenueNorm, gathered.today)
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
		FactorsOmitted:      omittedFactors(gathered, warmthReadable),
	}, nil
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
