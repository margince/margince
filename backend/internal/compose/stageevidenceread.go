// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What one reading of one deal's criteria does, from gathering the text to
// writing what it found.
//
// The division of labour with stageevidenceextract.go is: that file owns the
// question put to the model and what may come back, this one owns the run —
// which text is offered, which claims survive, and what is written.
//
// It writes to the SAME ledger the deterministic writers use, through the same
// entry point, so a model's claim carries a confidence and a rep's contract
// carries none and both are one row shape a reader can count. The ledger's own
// rules apply unchanged: a criterion naming something the BUYER did is refused
// where our own side authored the text it was read from, whatever the model
// concluded, and that refusal lives at the write rather than here.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// stageEvidenceReadActor is what the audit trail names as the writer of these
// rows. A model produced them and no human asked, so binding the last person to
// touch the deal would put their name on a reading they never made.
const stageEvidenceReadActor = "system:stage-evidence-read"

// stageEvidenceExtractedBy marks a claim a model made, as against the
// deterministic writers' own name. The two are told apart by this column
// everywhere a reader counts what a stage rests on.
const stageEvidenceExtractedBy = "stage_evidence_extract"

// StageEvidenceReader reads a deal's exit criteria against what was said.
type StageEvidenceReader struct {
	pool  *pgxpool.Pool
	deals *deals.Store
	own   deals.OwnDomainReader
	brain completer
	// propose asks whether the reading just recorded completes a stage's
	// checklist. Nil is a legal composition and skips silently, as it does on
	// the deterministic lane beside this one.
	propose stageProposer
	now     func() time.Time
	log     *slog.Logger
}

// NewStageEvidenceReader builds the engine over the pool and one model lane.
func NewStageEvidenceReader(
	pool *pgxpool.Pool, store *deals.Store, own deals.OwnDomainReader,
	brain completer, propose stageProposer, now func() time.Time, log *slog.Logger,
) *StageEvidenceReader {
	return &StageEvidenceReader{
		pool: pool, deals: store, own: own, brain: brain,
		propose: propose, now: now, log: log,
	}
}

// Read asks about one activity against the criteria of the deal it reaches,
// and records what the text settles.
//
// ONE ACTIVITY rather than the whole conversation. The activity is what just
// landed, so it is the only text that can have changed the answer, and reading
// a deal's whole history on every message would re-ask the same question of the
// same words for as long as the deal is open.
func (r *StageEvidenceReader) Read(
	ctx context.Context, dealID ids.DealID, activityID ids.UUID,
) (int, error) {
	actorCtx := stageEvidenceReadCtx(ctx)
	facts, criteria, err := r.gather(actorCtx, dealID, activityID)
	if err != nil {
		// ErrNotFound alongside the internal sentinel: between the enqueue and
		// this reading the activity can be archived, or lose the deal link the
		// reading was queued for, and ReadActivityAuthorship answers not-found
		// for both. Neither is a fault — the text this job was queued to read
		// is gone, and retrying re-asks the same question of the same absence
		// until the attempts run out.
		if errors.Is(err, errNothingToRead) || errors.Is(err, apperrors.ErrNotFound) {
			return 0, nil
		}
		return 0, err
	}
	claims, err := r.ask(actorCtx, criteria, facts.spans)
	if err != nil {
		return 0, err
	}
	written, err := r.record(actorCtx, dealID, facts, claims)
	if err != nil || written == 0 {
		// Nothing new landed, so the checklist stands where it did and there is
		// no fresh answer to propose on. The common case by far: most
		// conversations settle nothing.
		return written, err
	}
	// A failure to PROPOSE fails the reading, so the job retries.
	//
	// It costs a second model call, which is the reason to want to swallow it.
	// But nothing else ever comes back: the claims are committed, and the next
	// claim on this deal may be months away or never — so a swallowed error
	// loses the card on a deal whose criteria are all met right now. The
	// re-read is idempotent (the claim write finds its own row, the staging
	// joins the pending card), so the retry costs a call and changes nothing
	// else.
	if r.propose != nil {
		if _, err := r.propose.Propose(actorCtx, dealID); err != nil {
			return written, fmt.Errorf(
				"propose the stage move this reading completes: %w", err)
		}
	}
	return written, nil
}

// errNothingToRead is the ordinary outcome, not a failure: a deal whose stage
// asks for nothing, or an activity carrying no text, has no question to put.
var errNothingToRead = errors.New("compose: this reading has nothing to ask about")

// stageEvidenceReadFacts is what one reading needs about the activity it was
// given, gathered in one transaction.
type stageEvidenceReadFacts struct {
	activityID ids.UUID
	authorship deals.ActivityAuthorship
	spans      []stageEvidenceSpan
	// byKey resolves the key a claim names back to the criterion it was read
	// from, taken from the SAME read that built the prompt. Resolving it again
	// at write time would let a criterion archived in between turn a valid
	// claim into a refusal, or worse, match a key somebody had reused.
	byKey map[string]ids.ExitCriterionID
}

// gather reads the deal's criteria and the activity's text in ONE transaction,
// so the criteria a claim is validated against are the criteria that were
// offered to the model.
func (r *StageEvidenceReader) gather(
	ctx context.Context, dealID ids.DealID, activityID ids.UUID,
) (stageEvidenceReadFacts, []stageEvidenceCriterion, error) {
	var facts stageEvidenceReadFacts
	var criteria []stageEvidenceCriterion
	err := database.WithWorkspaceTx(ctx, r.pool, func(tx pgx.Tx) error {
		authorship, err := deals.ReadActivityAuthorship(ctx, tx, activityID, r.own)
		if err != nil {
			return err
		}
		if authorship.DealID != dealID {
			// The activity was relinked between the trigger and this reading.
			// Its text belongs to another deal's criteria now, and reading it
			// against this one's would file a claim about the wrong record.
			return errNothingToRead
		}
		facts.authorship = authorship
		text, err := readActivityText(ctx, tx, activityID)
		if err != nil {
			return err
		}
		if len(text) == 0 {
			return errNothingToRead
		}
		facts.activityID = activityID
		facts.spans = []stageEvidenceSpan{{SourceID: activityID.String(), Lines: text}}
		criteria, facts.byKey, err = r.criteriaOf(ctx, tx, dealID)
		return err
	})
	if err != nil {
		return facts, nil, err
	}
	if len(criteria) == 0 {
		return facts, nil, errNothingToRead
	}
	return facts, criteria, nil
}

// criteriaOf answers the live criteria on the deal's current stage, in the
// shape the prompt offers them.
func (r *StageEvidenceReader) criteriaOf(
	ctx context.Context, tx pgx.Tx, dealID ids.DealID,
) ([]stageEvidenceCriterion, map[string]ids.ExitCriterionID, error) {
	var stageID ids.StageID
	if err := tx.QueryRow(ctx,
		`SELECT stage_id FROM deal WHERE id = $1 AND archived_at IS NULL`,
		dealID).Scan(&stageID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, errNothingToRead
		}
		return nil, nil, fmt.Errorf("read the deal's stage: %w", err)
	}
	rows, err := r.deals.ListStageExitCriteria(ctx, stageID, storekit.LiveOnly)
	if err != nil {
		return nil, nil, err
	}
	out := make([]stageEvidenceCriterion, 0, len(rows))
	byKey := make(map[string]ids.ExitCriterionID, len(rows))
	for _, c := range rows {
		criterion := stageEvidenceCriterion{Key: c.Key, Label: c.Label}
		if c.Hint != nil {
			criterion.Hint = *c.Hint
		}
		out = append(out, criterion)
		byKey[c.Key] = ids.From[ids.ExitCriterionKind](ids.UUID(c.Id))
	}
	return out, byKey, nil
}

// ask puts the one question this site asks and returns the claims that came
// back, or none when the reply could not be used.
//
// A refused reply is NOT an error here. The records are still on disk, a later
// reading of a later message can settle the same criterion, and faulting the
// job would re-ask the same question of the same words on every retry.
func (r *StageEvidenceReader) ask(
	ctx context.Context, criteria []stageEvidenceCriterion, spans []stageEvidenceSpan,
) ([]stageEvidenceClaim, error) {
	req := stageEvidenceRequest(criteria, spans, identity.BaseLanguageForPrompt(ctx, r.pool))
	validate := stageEvidenceValid(criteria, spans)
	resp, err := ai.Ask(ctx, r.brain, req, validate)
	if err != nil {
		if errors.Is(err, ai.ErrOutputRejected) {
			r.log.InfoContext(ctx, "stage evidence read: refusing the model's reading",
				"error", clampToken(err.Error()))
			return nil, nil
		}
		// A provider or budget failure is not this reading's fault, so it is
		// returned and the job retries — unlike a refused reply, which would
		// ask the same question of the same words and be refused again.
		return nil, err
	}
	// Re-parsed and re-validated even when CompleteValidated already ran the
	// validator: a bare completer (the offline fake, a role wired without the
	// structured lane) does not, and this is the only floor those paths have.
	var payload stageEvidencePayload
	if err := json.Unmarshal([]byte(ai.Unfence(resp.Text)), &payload); err != nil {
		r.log.InfoContext(ctx, "stage evidence read: unparseable reading",
			"error", clampToken(err.Error()))
		return nil, nil
	}
	if err := validate(resp.Text); err != nil {
		r.log.InfoContext(ctx, "stage evidence read: refusing the model's reading",
			"error", clampToken(err.Error()))
		return nil, nil
	}
	return payload.claims(), nil
}

// record writes the claims that clear the floor, and answers how many landed.
//
// A claim BELOW the floor is dropped rather than written with a low
// confidence. The ledger is read by a policy function that counts met
// criteria, and a hedged claim sitting in it is a claim that will be counted —
// the floor is where an unsure reading stops, not where it is annotated.
func (r *StageEvidenceReader) record(
	ctx context.Context, dealID ids.DealID,
	facts stageEvidenceReadFacts, claims []stageEvidenceClaim,
) (int, error) {
	written := 0
	for _, claim := range claims {
		if claim.Confidence < stageEvidenceFloor {
			continue
		}
		landed, err := r.write(ctx, dealID, facts, claim)
		if err != nil {
			return written, err
		}
		if landed {
			written++
		}
	}
	if written > 0 {
		r.log.InfoContext(ctx, "stage evidence read", "deal", dealID.String(), "rows", written)
	}
	return written, nil
}

// write records one claim, translating the model's answer into the ledger's
// own vocabulary.
//
// A REFUSAL IS AN ORDINARY OUTCOME. The ledger refuses a claim about what the
// buyer did where our own side wrote the text, and refuses a criterion that is
// not on this deal's stage — both are the model reading something it should
// not have, and neither is a reason to fail the job and re-ask.
func (r *StageEvidenceReader) write(
	ctx context.Context, dealID ids.DealID,
	facts stageEvidenceReadFacts, claim stageEvidenceClaim,
) (bool, error) {
	criterionID, known := facts.byKey[claim.CriterionKey]
	if !known {
		// Unreachable: the validator refuses a key that was not offered, and
		// the offer came from this same map. Answered rather than assumed,
		// because a silent skip here would be a reading producing nothing for
		// a reason nobody could see.
		return false, fmt.Errorf(
			"the reading claims criterion %q, which this call did not offer", claim.CriterionKey)
	}
	confidence := float64(claim.Confidence)
	quote := claim.Quote
	// Bounded before the narrowing rather than trusting the validator to have
	// done it upstream: these are numbers a model chose, the column is int32,
	// and a wrapped conversion would file a citation pointing at a line that
	// does not exist. The validator refuses anything outside the span, so this
	// is unreachable — and a guard that costs one comparison is cheaper than
	// reasoning about whether it stays unreachable.
	lines := make([]int32, 0, len(claim.SourceLines))
	for _, line := range claim.SourceLines {
		if line < 1 || line > math.MaxInt32 {
			return false, fmt.Errorf(
				"the reading cites line %d, which is not a line number", line)
		}
		lines = append(lines, int32(line))
	}
	_, err := r.deals.RecordStageEvidence(ctx, deals.EvidenceInput{
		DealID:      dealID,
		CriterionID: criterionID,
		SourceType:  deals.SourceActivity,
		SourceID:    facts.activityID,
		SourceLines: lines,
		Snippet:     &quote,
		AuthorSide:  facts.authorship.AuthorSide,
		Commitment:  claim.Commitment,
		Met:         claim.Met == stageEvidenceMet,
		Confidence:  &confidence,
		ObservedAt:  facts.authorship.OccurredAt,
		ExtractedBy: stageEvidenceExtractedBy,
	})
	if err != nil {
		// The ledger's own refusals are ORDINARY outcomes of a reading, not
		// faults: a criterion naming something the buyer did is refused where
		// our own side wrote the text, and one that is not on this deal's
		// stage answers not-found. Both mean the model read something it
		// should not have, and failing the job would re-ask the same question
		// of the same words forever.
		var parse *values.ParseError
		if errors.As(err, &parse) || errors.Is(err, apperrors.ErrNotFound) {
			r.log.InfoContext(ctx, "stage evidence read: the ledger refused a claim",
				"criterion", claim.CriterionKey, "reason", clampToken(err.Error()))
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// stageEvidenceReadCtx binds the system principal every read and write in this
// pass runs under.
//
// Nobody asked for this reading — an activity landing on a deal is what starts
// it — so captured_by names the system rather than the last human to touch the
// record. The correlation id is minted HERE rather than carried, because this
// pass is its own unit of work: the job that runs it is enqueued by a trigger
// whose own trace ended when the activity was written.
func stageEvidenceReadCtx(ctx context.Context) context.Context {
	actorCtx := principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem,
		ID:   stageEvidenceReadActor,
	})
	return principal.WithCorrelationID(actorCtx, ids.NewV7())
}

// readActivityText answers the lines of an activity that its OWN sender wrote.
//
// THE QUOTED THREAD IS CUT OFF. A stored mail body keeps the conversation
// beneath the reply — mailmap caps it, it does not strip it — and this site
// records every claim under the author side of the activity as a whole. So a
// buyer's reply quoting our own earlier message would let a claim quote OUR
// words and land in the ledger as buyer-authored, settling the very milestones
// that exist to require the buyer's own word. textlang.CurrentMessage is the
// cut the correspondence gate already makes against the same hazard, and its
// own doc comment states the rule: a forwarded original stays in the activity
// and is not presented as the forwarding person's statement.
//
// The line numbers a claim cites are the numbers of THIS text, which is what
// reaches the model — so a citation still resolves against exactly what was
// offered.
//
// Split the way ADR-0058's canonical form addresses a transcript — on
// newlines, 1-indexed, trailing newlines being punctuation rather than final
// empty turns — because a claim cites a line NUMBER and every other reader of
// the same body must land on the same text.
func readActivityText(ctx context.Context, tx pgx.Tx, activityID ids.UUID) ([]string, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idPos := arg(activityID)
	// This one really does project the text — it is the text the model reads —
	// so the timeline's own rule applies in full: who may read an activity is
	// decided by the records it links to and by its audience. The pass runs as
	// the system principal, where the clause is the discovery arm alone, and
	// composing it rather than waiving it is what keeps the reader correct if a
	// seat-bound caller is ever given this seam.
	scope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	if scope != "" {
		scope = " AND " + scope
	}
	var body *string
	if err := tx.QueryRow(ctx, fmt.Sprintf(
		`SELECT a.body FROM activity a WHERE a.id = $%d AND a.archived_at IS NULL%s`,
		idPos, scope), args...).Scan(&body); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errNothingToRead
		}
		return nil, fmt.Errorf("read the activity's text: %w", err)
	}
	if body == nil {
		return nil, nil
	}
	own := textlang.CurrentMessage(*body)
	trimmed := strings.TrimRight(own, "\n")
	if trimmed == "" {
		return nil, nil
	}
	return strings.Split(trimmed, "\n"), nil
}
