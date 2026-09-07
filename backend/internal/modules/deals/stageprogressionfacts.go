// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Reading one deal into the facts DecideStageMove judges.
//
// ONE TRANSACTION, because the policy is a judgement about a consistent
// picture: criteria read at one moment and evidence at another can disagree
// with each other, and the decision would then be about a deal that never
// existed in that state.
//
// The target stage is computed HERE and never taken from a caller. A proposer
// that could be told where to move a deal would be a proposer whose target a
// bug — or a request body — could choose.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// StageProgressionFacts is the whole picture one decision rests on: what the
// policy judges, plus the identities the proposal needs to name.
type StageProgressionFacts struct {
	Decision StageMoveDecision
	DealID   ids.DealID
	// FromStageID and ToStageID name the move. ToStageID is the next open
	// stage in the pipeline's own order, computed here.
	FromStageID ids.StageID
	ToStageID   ids.StageID
	FromName    string
	ToName      string
	PipelineID  ids.PipelineID
	// Criteria carries the evidence ids behind each met criterion, so the card
	// can cite what it rests on and a reviewer can open it.
	Criteria []ProgressionCriterion
	// NeedsWinReason marks a move onto a WON stage with no signed agreement on
	// the deal. Such a move is refused unless somebody says why there is no
	// paper, and nothing the product read can answer that — so the card has to
	// carry the question to the person deciding it. Without this the proposal
	// stages a card that fails on every approval, forever.
	NeedsWinReason bool
}

// ProgressionCriterion is one criterion as the card shows it: the fact the
// policy judged, plus the rows a reader can click through to.
type ProgressionCriterion struct {
	CriterionFact
	CriterionID ids.ExitCriterionID
	Label       string
	EvidenceIDs []ids.UUID
	// Sources are the records each claim was read FROM — a contract, a message
	// — so the card can cite something a reviewer can open. The evidence row's
	// own id is not that: it points at the ledger entry, not at the material.
	Sources []EvidenceSource
}

// EvidenceSource is one claim's pointer back to the record it was read from,
// with the fragment it rests on.
type EvidenceSource struct {
	SourceType string
	SourceID   ids.UUID
	Snippet    string
}

// ErrNoNextStage is the ordinary outcome for a deal already on its pipeline's
// last open stage: there is nowhere to propose, and that is not a failure.
var ErrNoNextStage = errors.New("deals: this deal is on the last open stage of its pipeline")

// ReadStageProgressionFacts assembles one deal's picture and decides on it.
//
// The autopilot facts are the CALLER's to supply: they are read from the
// installation setting, the per-transition policy row and the measured rates,
// and the caller reads all three in the transaction that will apply the move.
// Passing them in keeps this function's answer a function of what it read.
func ReadStageProgressionFacts(
	ctx context.Context, tx pgx.Tx, dealID ids.DealID, autopilot AutopilotFacts, now time.Time,
) (StageProgressionFacts, error) {
	var out StageProgressionFacts
	out.DealID = dealID

	where, err := readDealStanding(ctx, tx, dealID)
	if err != nil {
		return out, err
	}
	out.FromStageID, out.PipelineID, out.FromName = where.stageID, where.pipelineID, where.stageName

	next, err := nextOpenStage(ctx, tx, where)
	if err != nil {
		return out, err
	}
	out.ToStageID, out.ToName = next.id, next.name

	criteria, err := readProgressionCriteria(ctx, tx, where.stageID, dealID)
	if err != nil {
		return out, err
	}
	out.Criteria = criteria

	protected, reason, err := readProtection(ctx, tx, dealID, next.id, now)
	if err != nil {
		return out, err
	}
	// Asked only where it can matter. A win with paper behind it, and every
	// move that is not a win, needs no answer — and the contract read is a
	// second query this saves on the overwhelming majority of proposals.
	if next.semantic == SemanticWon {
		signed, err := hasSignedContract(ctx, tx, dealID)
		if err != nil {
			return out, err
		}
		out.NeedsWinReason = !signed
	}

	facts := StageMoveFacts{
		Criteria:        make([]CriterionFact, 0, len(criteria)),
		FromTerminal:    where.semantic.Terminal(),
		ToTerminal:      next.semantic.Terminal(),
		SingleStep:      true,
		SamePipeline:    true,
		Protected:       protected,
		ProtectedReason: reason,
		Autopilot:       autopilot,
	}
	for _, c := range criteria {
		facts.Criteria = append(facts.Criteria, c.CriterionFact)
	}
	// SingleStep and SamePipeline are true by construction: nextOpenStage
	// answers the very next open stage of THIS deal's pipeline and nothing
	// else. They are still fields on the facts rather than assumptions inside
	// the policy, because a later caller — the human proposing a specific
	// target — reaches the same decision table with them false.
	out.Decision = DecideStageMove(facts)
	return out, nil
}

// dealStanding is where a deal sits right now.
type dealStanding struct {
	stageID    ids.StageID
	pipelineID ids.PipelineID
	stageName  string
	position   int
	semantic   StageSemantic
}

func readDealStanding(ctx context.Context, tx pgx.Tx, dealID ids.DealID) (dealStanding, error) {
	var out dealStanding
	var semantic string
	err := tx.QueryRow(ctx, `
		SELECT s.id, s.pipeline_id, s.name, s.position, s.semantic
		  FROM deal d JOIN stage s ON s.id = d.stage_id
		 WHERE d.id = $1 AND d.archived_at IS NULL`, dealID).
		Scan(&out.stageID, &out.pipelineID, &out.stageName, &out.position, &semantic)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, apperrors.ErrNotFound
	}
	if err != nil {
		return out, fmt.Errorf("read the deal's standing: %w", err)
	}
	out.semantic = StageSemantic(semantic)
	return out, nil
}

// stageRef is the target of a proposed move.
type stageRef struct {
	id       ids.StageID
	name     string
	semantic StageSemantic
}

// nextOpenStage answers the stage after this one in the pipeline's own order.
//
// The pipeline's ORDER decides, not a caller: a proposer that could be told
// where to move a deal is one whose target a bug could choose. A deal already
// on the last stage answers ErrNoNextStage, which the caller treats as an
// ordinary outcome.
func nextOpenStage(ctx context.Context, tx pgx.Tx, from dealStanding) (stageRef, error) {
	var out stageRef
	var semantic string
	err := tx.QueryRow(ctx, `
		SELECT id, name, semantic
		  FROM stage
		 WHERE pipeline_id = $1 AND archived_at IS NULL
		   AND (position, id) > ($2, $3)
		 ORDER BY position, id
		 LIMIT 1`, from.pipelineID, from.position, from.stageID).
		Scan(&out.id, &out.name, &semantic)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNoNextStage
	}
	if err != nil {
		return out, fmt.Errorf("read the next stage: %w", err)
	}
	out.semantic = StageSemantic(semantic)
	return out, nil
}

// readProgressionCriteria answers the stage's live criteria and what the
// ledger says about each.
//
// A criterion with no standing evidence is NOT met, which is the default the
// row shape gives it. Refuted evidence does not count: a claim a human struck
// out is a claim they said was wrong, and counting it would make the
// correction do nothing.
//
// Where more than one row settles one criterion the strongest wins — the
// authorship rule is the bar, so a buyer-authored claim beats a seller's, and
// among equals the most recent observation. A criterion settled by one good
// claim is settled whatever weaker claims sit beside it.
func readProgressionCriteria(
	ctx context.Context, tx pgx.Tx, stageID ids.StageID, dealID ids.DealID,
) ([]ProgressionCriterion, error) {
	rows, err := tx.Query(ctx, `
		SELECT c.id, c.key, c.label, c.required, c.kind,
		       coalesce(e.met, false), coalesce(e.author_side, ''),
		       coalesce(e.commitment, ''), e.confidence,
		       coalesce(e.contradicted, false), coalesce(e.ids, '{}'),
		       coalesce(e.source_types, '{}'), coalesce(e.source_ids, '{}'),
		       coalesce(e.snippets, '{}')
		  FROM stage_exit_criterion c
		  LEFT JOIN LATERAL (
		      SELECT bool_or(v.met) AS met,
		             (array_agg(v.author_side ORDER BY v.rank, v.observed_at DESC))[1] AS author_side,
		             (array_agg(v.commitment ORDER BY v.rank, v.observed_at DESC))[1] AS commitment,
		             (array_agg(v.confidence ORDER BY v.rank, v.observed_at DESC))[1] AS confidence,
		             -- ANY row's contradiction, not the winner's. A newer
		             -- uncontradicted claim can out-rank an older contradicted
		             -- one on the same criterion, and picking only the winner
		             -- would drop the disagreement exactly when a reader most
		             -- needs telling that one exists.
		             bool_or(v.contradicted_by IS NOT NULL) AS contradicted,
		             -- ORDERED, because this list is hashed. The card's
		             -- diff_hash is what binds a proposal to its payload and
		             -- what tells one proposal from another; an unordered
		             -- aggregate lets two readings of the SAME evidence hash
		             -- differently, so a re-read would supersede its own card
		             -- with an identical one and the inbox would churn.
		             array_agg(v.id ORDER BY v.id) AS ids,
		             -- The records the claims were read FROM, so the card
		             -- cites material a reviewer can open rather than the
		             -- ledger row that records the reading.
		             array_agg(v.source_type ORDER BY v.id) AS source_types,
		             array_agg(v.source_id ORDER BY v.id) AS source_ids,
		             array_agg(coalesce(v.snippet, '') ORDER BY v.id) AS snippets
		        FROM (
		            SELECT ev.*,
		                   CASE WHEN ev.author_side = 'buyer' THEN 0 ELSE 1 END AS rank
		              FROM deal_stage_evidence ev
		             WHERE ev.criterion_id = c.id AND ev.deal_id = $2
		               AND ev.refuted_at IS NULL AND ev.met
		        ) v
		  ) e ON true
		 WHERE c.stage_id = $1 AND c.archived_at IS NULL
		 ORDER BY c."position"`, stageID, dealID)
	if err != nil {
		return nil, fmt.Errorf("read the stage's criteria and their evidence: %w", err)
	}
	defer rows.Close()

	var out []ProgressionCriterion
	for rows.Next() {
		var c ProgressionCriterion
		var kind, authorSide, commitment string
		var sourceTypes, snippets []string
		var sourceIDs []ids.UUID
		if err := rows.Scan(&c.CriterionID, &c.Key, &c.Label, &c.Required, &kind,
			&c.Met, &authorSide, &commitment, &c.Confidence,
			&c.Contradicted, &c.EvidenceIDs,
			&sourceTypes, &sourceIDs, &snippets); err != nil {
			return nil, fmt.Errorf("scan a criterion's standing: %w", err)
		}
		c.Sources = evidenceSources(sourceTypes, sourceIDs, snippets)
		c.Kind = CriterionKind(kind)
		c.AuthorSide = AuthorSide(authorSide)
		c.Commitment = commitment
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read the stage's criteria: %w", err)
	}
	return out, nil
}

// readProtection answers whether a human has recently steered this deal, and
// in which of the three ways.
//
// Each arm is a different sentence to a rep, which is why the reason travels
// with the boolean rather than being composed at the card: "you moved this
// yourself", "this move was undone before" and "you already said no to this"
// are three different things to be told.
//
// The third arm is the one that keeps the product from nagging: a proposal for
// this same target already rejected, with nothing learned since. It reopens on
// NEW evidence, because the rep said no to what was known then and a claim
// recorded afterwards is a different question.
func readProtection(
	ctx context.Context, tx pgx.Tx, dealID ids.DealID, toStageID ids.StageID, now time.Time,
) (bool, string, error) {
	var humanMove, reversal, refusedAlready bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		    -- A HUMAN's own move, named positively. captured_by carries the
		    -- principal's prefix, and "not system" would read an agent's move
		    -- as a person's — the opposite of what protection is for, since an
		    -- agent moving a deal is precisely what a human has not done.
		    --
		    -- from_stage_id IS NOT NULL excludes the row a deal's CREATION
		    -- writes. Creating a deal is not steering it, and counting it as a
		    -- human stage move would protect every new deal for a fortnight —
		    -- exactly the window in which its first criteria are met.
		    --
		    -- approval_id IS NULL keeps a move a human made THROUGH a card out
		    -- of it: accepting a proposal is agreeing with the product, not
		    -- overruling it, and protecting on it would make one accepted card
		    -- silence the next fortnight of them.
		    SELECT 1 FROM deal_stage_history
		     WHERE deal_id = $1 AND changed_at >= $2
		       AND from_stage_id IS NOT NULL
		       AND changed_by LIKE 'human:%'
		       AND approval_id IS NULL
		),
		EXISTS (
		    SELECT 1 FROM deal_stage_history WHERE deal_id = $1 AND reversal_of IS NOT NULL
		),
		EXISTS (
		    -- A rejection of THIS target that nothing has been learned since.
		    -- Newer evidence reopens it: the rep said no to what was known
		    -- then, and a claim recorded afterwards is a different question,
		    -- so re-asking is the product having something new to say rather
		    -- than nagging.
		    SELECT 1 FROM stage_progression_outcome o
		     WHERE o.deal_id = $1 AND o.to_stage_id = $3 AND o.outcome = 'rejected'
		       AND NOT EXISTS (
		           SELECT 1 FROM deal_stage_evidence e
		            WHERE e.deal_id = $1 AND e.refuted_at IS NULL
		              AND e.created_at > o.decided_at
		       )
		)`, dealID, now.Add(-ProtectionWindow), toStageID).
		Scan(&humanMove, &reversal, &refusedAlready); err != nil {
		return false, "", fmt.Errorf("read the deal's recent stage history: %w", err)
	}
	switch {
	case humanMove:
		return true, "you moved this deal yourself in the last fortnight", nil
	case reversal:
		// A move that was undone once is a move this deal has already had the
		// argument about. Proposing it again is the product not listening.
		return true, "a stage move on this deal was undone before", nil
	case refusedAlready:
		return true, "you turned this move down, and nothing new has been learned since", nil
	}
	return false, "", nil
}

// evidenceSources zips the three parallel aggregates into one list.
//
// All three are aggregated over the same rows in the same order, so they are
// the same length — but a short one is read as far as it goes rather than
// panicking, because a card citing fewer sources is a smaller answer while a
// panic in an unattended proposer is a lane that stops.
func evidenceSources(types []string, ids []ids.UUID, snippets []string) []EvidenceSource {
	n := min(len(types), min(len(ids), len(snippets)))
	if n == 0 {
		return nil
	}
	out := make([]EvidenceSource, 0, n)
	for i := range n {
		out = append(out, EvidenceSource{
			SourceType: types[i], SourceID: ids[i], Snippet: snippets[i],
		})
	}
	return out
}
