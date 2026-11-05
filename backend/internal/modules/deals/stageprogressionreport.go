// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// What a transition has earned, counted from the ledger.
//
// This is the launch gate's evidence: a transition may only move deals by
// itself once these numbers say contacts agreed with it often enough, for long
// enough, without having to correct it. Nothing here DECIDES that — the policy
// that reads these rates is its own file — because a report that also enforced
// would be a number nobody could check against the thing it authorized.
//
// Every rate is a fraction of REVIEWED proposals, and reviewed is the word
// doing the work: a proposal nobody answered is not evidence that anyone
// agreed with it. See reviewedOutcomes.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// TransitionRates is one from→to transition's record.
type TransitionRates struct {
	PipelineID  ids.PipelineID
	FromStageID ids.StageID
	ToStageID   ids.StageID
	FromName    string
	ToName      string
	// Reviewed is how many proposals a human actually answered — the
	// denominator of every rate below.
	Reviewed int
	// Proposed, Expired and Superseded are counted but are NOT reviewed. They
	// are reported because their absence would make the reviewed count look
	// like the whole story: a transition whose cards are mostly ignored has an
	// acceptance rate computed over the few somebody happened to open.
	Proposed   int
	Expired    int
	Superseded int
	// AcceptedClean is an approval that changed nothing. It is the number the
	// launch gate is about: a card a human read and simply agreed with.
	AcceptedClean int
	// AcceptedEdited is an approval the human changed before releasing.
	// Counted apart from clean because an edit is agreement with a correction,
	// which is a weaker claim about the proposal than agreement without one.
	AcceptedEdited int
	Rejected       int
	AutoApplied    int
	// Unsafe is the count of reviewed proposals that were reversed OR whose
	// evidence a human corrected. ONE outcome counts once even when both are
	// true: the pair is a single "this move should not have happened", and
	// double-counting it would make one bad move look like two.
	Unsafe int
	// ObservationDays is the span from the first reviewed proposal to the
	// last, in whole days. A transition with a high acceptance rate earned
	// over two days has not been observed, however many proposals it saw.
	ObservationDays int
	// EvidenceKinds is the same record cut by what the moves rested on, so an
	// installation can see it accepts a signed contract and refuses a reading
	// of a conversation rather than only that it accepts 80% of everything.
	EvidenceKinds []EvidenceKindRates
}

// EvidenceKindRates is one criterion kind's slice of a transition's record.
type EvidenceKindRates struct {
	Kind          string
	Reviewed      int
	AcceptedClean int
	Unsafe        int
}

// CleanAcceptanceRate is approvals that changed nothing, over everything a
// human answered.
//
// An EDIT IS NOT A CLEAN ACCEPTANCE, and that is the whole point of the
// number: the question it answers is "when the product proposes this move, is
// it right as it stands", and a proposal a human had to fix before releasing
// is one they did not find right as it stood.
//
// Answers 0 for a transition nobody has reviewed, and the caller must not read
// that as a bad rate — Reviewed is reported beside it precisely so a zero can
// be told from a failure.
func (r TransitionRates) CleanAcceptanceRate() float64 {
	return rateOf(r.AcceptedClean, r.Reviewed)
}

// RejectionRate is what a human read and said no to.
func (r TransitionRates) RejectionRate() float64 {
	return rateOf(r.Rejected, r.Reviewed)
}

// EditRate is what a human agreed with only after changing it.
func (r TransitionRates) EditRate() float64 {
	return rateOf(r.AcceptedEdited, r.Reviewed)
}

// UnsafeRate is the safety number: moves undone, or resting on evidence a
// human went back and marked wrong.
//
// This is the one the launch gate holds to a CEILING rather than a floor, and
// it is deliberately a single number over a union rather than two rates added
// up: a move that was reversed AND whose evidence was corrected is one mistake
// described twice, and summing two rates would report it as two.
func (r TransitionRates) UnsafeRate() float64 {
	return rateOf(r.Unsafe, r.Reviewed)
}

func rateOf(part, whole int) float64 {
	if whole <= 0 {
		return 0
	}
	return float64(part) / float64(whole)
}

// reviewedOutcomes are the outcomes that mean a CONTACT answered.
//
// `proposed` is not here: the card is still open. `expired` is not either, and
// that is the subtle one — the window closing IS a refusal in the approvals
// module's own vocabulary, but it is a refusal by nobody. Counting it as a
// rejection would let a transition nobody has time to read look like one
// contacts actively disagree with, and the fix for those two is opposite: one
// needs a better proposal, the other needs somebody to look. `superseded` is
// not an answer at all — a fresher card replaced it.
//
// `auto_applied` IS NOT HERE EITHER, and that one decides whether this whole
// report can be trusted. An automatic move is the autopilot agreeing with
// itself; putting it in the denominator lets it dilute every rate the launch
// gate reads, so a transition running on automatic would report a falling
// clean-acceptance rate as its own volume grew — one human approval among nine
// automatic moves reads as 10% agreement when every contact who looked agreed.
// The autopilot cannot be allowed to vote on whether it should be running. It
// is counted and reported on its own line instead.
//
// `reversed` stays: somebody undid the move, which is an answer, and the one
// that matters most.
var reviewedOutcomes = []string{
	ProgressionApprovedClean, ProgressionApprovedEdited,
	ProgressionRejected, ProgressionReversed,
}

// ReadStageAutomationReport answers every transition's record in one pipeline.
//
// Pipeline-scoped like the criteria read beside it: this is configuration
// evidence every rep can see on the board, not a record with an owner.
//
// NOT PAGINATED, and the bound is the writer's rather than a LIMIT here. The
// only thing that opens a ledger row is the proposer, which proposes a deal's
// CURRENT stage to the next open one (stageprogressionfacts.go's
// nextOpenStage) — so the distinct pairs a pipeline can accumulate are its
// adjacent stages, not every from×to combination. A pipeline with fifty stages
// reports at most fifty rows.
//
// If a later writer ever proposes a skip, this becomes a real bound to add.
// The count is a fact about who writes the table, so it is stated here beside
// the read rather than assumed.
func (s *Store) ReadStageAutomationReport(
	ctx context.Context, pipelineID ids.PipelineID, window time.Duration,
) ([]TransitionRates, error) {
	if err := auth.Require(ctx, "pipeline", principal.ActionRead); err != nil {
		return nil, err
	}
	var out []TransitionRates
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		// The pipeline must exist, so an unknown id is 404 rather than an
		// empty report — which a reader would take for "this pipeline has
		// proposed nothing yet".
		if err := requirePipeline(ctx, tx, pipelineID); err != nil {
			return err
		}
		var err error
		since := s.clock().Add(-window)
		out, err = readTransitionRates(ctx, tx, pipelineID, since)
		if err != nil {
			return err
		}
		return attachEvidenceKinds(ctx, tx, pipelineID, since, out)
	})
	return out, err
}

func readTransitionRates(
	ctx context.Context, tx pgx.Tx, pipelineID ids.PipelineID, since time.Time,
) ([]TransitionRates, error) {
	rows, err := tx.Query(ctx, `
		SELECT o.from_stage_id, o.to_stage_id,
		       coalesce(f.name, ''), coalesce(t.name, ''),
		       count(*) FILTER (WHERE o.outcome = ANY($3)) AS reviewed,
		       count(*) FILTER (WHERE o.outcome = 'proposed')   AS still_open,
		       count(*) FILTER (WHERE o.outcome = 'expired')    AS expired,
		       count(*) FILTER (WHERE o.outcome = 'superseded') AS superseded,
		       count(*) FILTER (WHERE o.outcome = 'approved_clean')  AS clean,
		       count(*) FILTER (WHERE o.outcome = 'approved_edited') AS edited,
		       count(*) FILTER (WHERE o.outcome = 'rejected')        AS rejected,
		       count(*) FILTER (WHERE o.outcome = 'auto_applied')    AS auto_applied,
		       -- ONE row counts once however many ways it went wrong. A move
		       -- reversed AND corrected is a single mistake, and OR is what
		       -- keeps it that way; two counts added would report it twice and
		       -- push a transition past a ceiling it had not actually crossed.
		       --
		       -- The OUTCOME is an arm of the OR, not only the timestamp. The
		       -- column is nullable and its constraint only forbids a contact
		       -- with no instant, so a row standing at reversed with a null
		       -- reversed_at is legal — and reading only the timestamp would
		       -- count that undone move as a safe one, which is the single
		       -- direction this number must never fail in.
		       count(*) FILTER (
		           WHERE o.outcome = ANY($3)
		             AND (o.outcome = 'reversed'
		                  OR o.reversed_at IS NOT NULL
		                  OR o.evidence_corrected)
		       ) AS unsafe,
		       -- The span actually OBSERVED, from the first answered proposal
		       -- to the last. Not the window: a window of thirty days over
		       -- which everything arrived on one afternoon has observed one
		       -- afternoon.
		       min(o.decided_at) FILTER (WHERE o.outcome = ANY($3)),
		       max(o.decided_at) FILTER (WHERE o.outcome = ANY($3))
		  FROM stage_progression_outcome o
		  LEFT JOIN stage f ON f.id = o.from_stage_id
		  LEFT JOIN stage t ON t.id = o.to_stage_id
		 -- decided_at, not created_at. The report is about DECISIONS, and a
		 -- card proposed five weeks ago and answered this morning is a decision
		 -- from this morning: windowing on creation drops it, so a rate
		 -- computed over a slow-moving transition silently loses its most
		 -- recent answers. An open row carries its creation time here (the
		 -- column is NOT NULL and defaults to now() at insert), so this
		 -- windows every row on the last thing that happened to it.
		 WHERE o.pipeline_id = $1 AND o.decided_at >= $2
		 GROUP BY o.from_stage_id, o.to_stage_id, f.name, t.name, f."position", t."position"
		 ORDER BY f."position", t."position"`,
		pipelineID, since, reviewedOutcomes)
	if err != nil {
		return nil, fmt.Errorf("read the transitions' record: %w", err)
	}
	defer rows.Close()

	var out []TransitionRates
	for rows.Next() {
		r := TransitionRates{PipelineID: pipelineID}
		var first, last *time.Time
		if err := rows.Scan(&r.FromStageID, &r.ToStageID, &r.FromName, &r.ToName,
			&r.Reviewed, &r.Proposed, &r.Expired, &r.Superseded,
			&r.AcceptedClean, &r.AcceptedEdited, &r.Rejected, &r.AutoApplied,
			&r.Unsafe, &first, &last); err != nil {
			return nil, fmt.Errorf("scan a transition's record: %w", err)
		}
		r.ObservationDays = observationDays(first, last)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read the transitions' record: %w", err)
	}
	return out, nil
}

// observationDays is the whole days between the first answered proposal and
// the last.
//
// Whole days ROUNDED DOWN, so a transition observed for twenty-seven days and
// twenty-three hours reports 27 and does not clear a 28-day bar. The bar is a
// floor on how long contacts have had to notice a problem, and rounding up would
// let it be cleared by an hour.
func observationDays(first, last *time.Time) int {
	if first == nil || last == nil {
		return 0
	}
	return int(last.Sub(*first).Hours() / 24)
}

// requirePipeline answers nil when the pipeline exists, ErrNotFound otherwise.
//
// An ARCHIVED pipeline still answers: its transitions' record is what says
// whether retiring it was right, and a report that went blank on archive would
// lose exactly the history somebody archiving it would want to check.
func requirePipeline(ctx context.Context, tx pgx.Tx, pipelineID ids.PipelineID) error {
	var exists bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM pipeline WHERE id = $1)`, pipelineID).Scan(&exists); err != nil {
		return fmt.Errorf("resolve the pipeline: %w", err)
	}
	if !exists {
		return apperrors.ErrNotFound
	}
	return nil
}

// attachEvidenceKinds fills each transition's per-kind cut.
//
// A SECOND query rather than a wider first one, because unnesting the kinds
// array multiplies the rows: a move resting on two criteria would be counted
// twice in every total beside it, and the transition's own reviewed count —
// the denominator of every rate — would silently inflate with the number of
// criteria its stage happens to ask for.
func attachEvidenceKinds(
	ctx context.Context, tx pgx.Tx, pipelineID ids.PipelineID,
	since time.Time, into []TransitionRates,
) error {
	if len(into) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx, `
		SELECT o.from_stage_id, o.to_stage_id, k.kind,
		       count(*) AS reviewed,
		       count(*) FILTER (WHERE o.outcome = 'approved_clean') AS clean,
		       count(*) FILTER (
		           WHERE o.outcome = 'reversed'
		              OR o.reversed_at IS NOT NULL
		              OR o.evidence_corrected
		       ) AS unsafe
		  FROM stage_progression_outcome o
		  CROSS JOIN LATERAL unnest(o.evidence_kinds) AS k(kind)
		 WHERE o.pipeline_id = $1 AND o.decided_at >= $2 AND o.outcome = ANY($3)
		 GROUP BY o.from_stage_id, o.to_stage_id, k.kind
		 ORDER BY k.kind`,
		pipelineID, since, reviewedOutcomes)
	if err != nil {
		return fmt.Errorf("read the transitions' record by evidence kind: %w", err)
	}
	defer rows.Close()

	at := map[[2]ids.StageID]int{}
	for i, r := range into {
		at[[2]ids.StageID{r.FromStageID, r.ToStageID}] = i
	}
	for rows.Next() {
		var from, to ids.StageID
		var k EvidenceKindRates
		if err := rows.Scan(&from, &to, &k.Kind, &k.Reviewed, &k.AcceptedClean, &k.Unsafe); err != nil {
			return fmt.Errorf("scan a transition's record by evidence kind: %w", err)
		}
		i, ok := at[[2]ids.StageID{from, to}]
		if !ok {
			// A transition the totals query did not see. The two statements run
			// under Read Committed, so a decision committed between them shows
			// up here and not there — a race, not a defect, and one that ends
			// the moment the report is asked again.
			//
			// Skipped rather than faulted: this is a READ, and answering 500
			// to somebody who asked for a report because a colleague approved
			// a card mid-query is the worse failure. The row it belongs to is
			// simply not in this snapshot, and its own evidence cut arrives
			// with it next time.
			continue
		}
		into[i].EvidenceKinds = append(into[i].EvidenceKinds, k)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read the transitions' record by evidence kind: %w", err)
	}
	return nil
}
