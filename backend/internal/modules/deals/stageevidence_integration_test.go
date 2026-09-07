// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

// The evidence ledger against a real database.
//
// The claims are about constraints and a refusal that has to fire at the
// WRITE: a seller-authored row satisfying a buyer milestone is a false fact in
// the trail, and no read-side filter fixes it once it is there. Every row is
// seeded through the real writers.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// evidenceFixture seeds a deal on an open stage carrying one criterion of the
// given kind, and answers both ids.
func evidenceFixture(t *testing.T, e *configEnv, kind CriterionKind) (ids.DealID, ids.ExitCriterionID) {
	t.Helper()
	ctx := e.as()
	stageID := openStage(t, e, SemanticOpen)
	criterion, err := e.store.CreateStageExitCriterion(ctx, CreateCriterionInput{
		StageID: stageID, Key: "the_thing", Label: "The thing", Kind: string(kind),
	})
	if err != nil {
		t.Fatalf("seeding a %s criterion: %v", kind, err)
	}
	var pipelineID ids.PipelineID
	if err := e.owner.QueryRow(t.Context(),
		`SELECT pipeline_id FROM stage WHERE id = $1`, stageID).Scan(&pipelineID); err != nil {
		t.Fatalf("reading the stage's pipeline: %v", err)
	}
	// Through the real writer: a hand-built INSERT would have to keep up with
	// every column deal acquires, and a row it shaped differently from
	// production proves nothing about production.
	owner := ids.From[ids.UserKind](e.admin)
	deal, err := e.store.CreateDeal(e.asDealWriter(), CreateDealInput{
		Name: "Warehouse rollout", PipelineID: pipelineID, StageID: stageID,
		OwnerID: &owner, OwnerExact: true,
	})
	if err != nil {
		t.Fatalf("seeding a deal: %v", err)
	}
	return ids.From[ids.DealKind](ids.UUID(deal.Id)), ids.From[ids.ExitCriterionKind](ids.UUID(criterion.Id))
}

func claim(dealID ids.DealID, criterionID ids.ExitCriterionID, side AuthorSide) EvidenceInput {
	return EvidenceInput{
		DealID: dealID, CriterionID: criterionID,
		SourceType: SourceActivity, SourceID: ids.NewV7(),
		AuthorSide: side, Commitment: CommitmentAgreed, Met: true,
		ObservedAt: time.Now().UTC(), ExtractedBy: ExtractedByDeterministic,
	}
}

// The rule the ledger exists for, held at the WRITE. A rep's own message
// saying the buyer confirmed something is our claim about them, not theirs.
func TestSellerAuthoredEvidenceCannotSettleABuyerMilestone(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, criterionID := evidenceFixture(t, e, CriterionBuyerConfirmed)

	for _, side := range []AuthorSide{AuthorSeller, AuthorUnknown} {
		_, err := e.store.RecordStageEvidence(e.asDealWriter(), claim(dealID, criterionID, side))
		var parse *values.ParseError
		if !errors.As(err, &parse) || parse.Code != "buyer_milestone_needs_buyer_evidence" {
			t.Errorf("%s-authored evidence settled buyer_confirmed (%v)", side, err)
		}
	}

	// The positive control: the buyer's own word is accepted, so the refusals
	// above are about authorship and not about a fixture that refuses
	// everything.
	if _, err := e.store.RecordStageEvidence(e.asDealWriter(), claim(dealID, criterionID, AuthorBuyer)); err != nil {
		t.Fatalf("buyer-authored evidence was refused: %v", err)
	}
}

// Observing that a criterion is NOT met is something anybody may report.
// Refusing it would leave the ledger able to record only good news.
func TestOurOwnSideMayRecordThatACriterionIsUnmet(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, criterionID := evidenceFixture(t, e, CriterionBuyerConfirmed)

	unmet := claim(dealID, criterionID, AuthorSeller)
	unmet.Met = false
	if _, err := e.store.RecordStageEvidence(e.asDealWriter(), unmet); err != nil {
		t.Fatalf("our own side could not record that a buyer milestone is unmet: %v", err)
	}
}

// A criterion NOT naming something the buyer did takes either side's word: we
// learn who the economic buyer is from our own notes as readily as from theirs.
func TestANonBuyerMilestoneTakesOurOwnObservation(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, criterionID := evidenceFixture(t, e, CriterionRoleIdentified)

	if _, err := e.store.RecordStageEvidence(e.asDealWriter(), claim(dealID, criterionID, AuthorSeller)); err != nil {
		t.Fatalf("seller-authored evidence was refused for role_identified: %v", err)
	}
}

// The bus is at-least-once and the trigger fires per delivery, so the same
// contract turning active arrives more than once. A second row would
// double-count one fact for every reader that counts met criteria.
func TestTheSameSourceIsRecordedOnce(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, criterionID := evidenceFixture(t, e, CriterionRoleIdentified)

	first := claim(dealID, criterionID, AuthorSeller)
	written, err := e.store.RecordStageEvidence(e.asDealWriter(), first)
	if err != nil {
		t.Fatalf("the first delivery: %v", err)
	}
	again, err := e.store.RecordStageEvidence(e.asDealWriter(), first)
	if err != nil {
		t.Fatalf("a redelivery errored instead of answering the standing row: %v", err)
	}
	if again.Id != written.Id {
		t.Fatalf("a redelivery wrote a second row (%s beside %s); the fact is now counted twice",
			again.Id, written.Id)
	}

	var rows int
	if err := e.owner.QueryRow(t.Context(),
		`SELECT count(*) FROM deal_stage_evidence WHERE deal_id = $1`, dealID.UUID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Errorf("the ledger holds %d rows for one redelivered fact", rows)
	}

	// And the trail carries ONE create, not one per delivery.
	var creates int
	if err := e.owner.QueryRow(t.Context(), `
		SELECT count(*) FROM audit_log
		 WHERE entity_type = 'deal_stage_evidence' AND action = 'create'
		   AND entity_id = $1`, ids.UUID(written.Id)).Scan(&creates); err != nil {
		t.Fatal(err)
	}
	if creates != 1 {
		t.Errorf("a redelivered fact wrote %d create audit rows", creates)
	}
}

// A refuted claim STAYS. Why a stage move was reversed is a question asked
// later, and a deleted row answers it with silence.
func TestRefutingAClaimKeepsItAndMarksIt(t *testing.T) {
	e := setupConfigEnv(t)
	ctx := e.asDealWriter()
	dealID, criterionID := evidenceFixture(t, e, CriterionRoleIdentified)

	written, err := e.store.RecordStageEvidence(ctx, claim(dealID, criterionID, AuthorSeller))
	if err != nil {
		t.Fatalf("recording: %v", err)
	}
	refuted, err := e.store.RefuteStageEvidence(ctx, dealID, ids.UUID(written.Id))
	if err != nil {
		t.Fatalf("refuting: %v", err)
	}
	if refuted.RefutedAt == nil || refuted.RefutedBy == nil {
		t.Fatal("the refutation recorded neither a moment nor an author")
	}

	// It is still LISTED — a reader asking what a stage move rested on needs
	// the withdrawn claim as much as the ones that stood.
	all, err := e.store.ListStageEvidence(ctx, dealID)
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(all) != 1 || all[0].RefutedAt == nil {
		t.Fatalf("the refuted claim is gone from the ledger (%d rows); the reversal cannot be explained", len(all))
	}
}

// The first word on a refutation is the one worth keeping: re-refuting would
// overwrite who first said the claim was wrong.
func TestASecondRefutationLeavesTheFirstStanding(t *testing.T) {
	e := setupConfigEnv(t)
	ctx := e.asDealWriter()
	dealID, criterionID := evidenceFixture(t, e, CriterionRoleIdentified)
	written, err := e.store.RecordStageEvidence(ctx, claim(dealID, criterionID, AuthorSeller))
	if err != nil {
		t.Fatalf("recording: %v", err)
	}
	first, err := e.store.RefuteStageEvidence(ctx, dealID, ids.UUID(written.Id))
	if err != nil {
		t.Fatalf("the first refutation: %v", err)
	}
	second, err := e.store.RefuteStageEvidence(ctx, dealID, ids.UUID(written.Id))
	if err != nil {
		t.Fatalf("the second refutation errored: %v", err)
	}
	if !second.RefutedAt.Equal(*first.RefutedAt) {
		t.Errorf("a second refutation moved the moment from %v to %v", first.RefutedAt, second.RefutedAt)
	}
}

// A deterministic writer restates what a record says, so a probability beside
// it is theatre. The column refuses the pairing; this answers in the caller's
// own vocabulary rather than surfacing a constraint violation.
func TestADeterministicClaimCarriesNoConfidence(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, criterionID := evidenceFixture(t, e, CriterionRoleIdentified)

	certain := claim(dealID, criterionID, AuthorSeller)
	confidence := 0.8
	certain.Confidence = &confidence
	_, err := e.store.RecordStageEvidence(e.asDealWriter(), certain)
	var parse *values.ParseError
	if !errors.As(err, &parse) || parse.Code != "deterministic_evidence_is_certain" {
		t.Fatalf("a deterministic claim carried a confidence (%v)", err)
	}
}

// The deterministic writer resolves the criteria itself, so a deal whose stage
// asks for nothing of the kind gets no row — the common case, not an error.
func TestADealWhoseStageAsksForNothingOfTheKindGetsNoEvidence(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, _ := evidenceFixture(t, e, CriterionRoleIdentified)

	written, err := e.store.RecordDeterministicEvidence(e.asDealWriter(), DeterministicClaim{
		DealID: dealID, Kind: CriterionDocumentSigned,
		SourceType: SourceContract, SourceID: ids.NewV7(),
		AuthorSide: AuthorBuyer, ObservedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("a stage asking for nothing of the kind errored: %v", err)
	}
	if written != 0 {
		t.Errorf("%d rows were written against criteria that do not exist", written)
	}
}

// And when the stage DOES ask for it, the writer finds the criterion without
// the caller naming it.
func TestTheDeterministicWriterFindsTheCriterionByKind(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, criterionID := evidenceFixture(t, e, CriterionDocumentSigned)

	written, err := e.store.RecordDeterministicEvidence(e.asDealWriter(), DeterministicClaim{
		DealID: dealID, Kind: CriterionDocumentSigned,
		SourceType: SourceContract, SourceID: ids.NewV7(),
		AuthorSide: AuthorBuyer, ObservedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("recording: %v", err)
	}
	if written != 1 {
		t.Fatalf("the writer recorded %d rows against one matching criterion", written)
	}
	all, err := e.store.ListStageEvidence(e.asDealWriter(), dealID)
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(all) != 1 || all[0].CriterionId != openapiUUID(criterionID.UUID) {
		t.Fatalf("the row did not land on the stage's document_signed criterion")
	}
	if all[0].ExtractedBy != ExtractedByDeterministic {
		t.Errorf("the writer named itself %q, not %q", all[0].ExtractedBy, ExtractedByDeterministic)
	}
}

// Evidence is a record handed back, so it carries the deal's own read rule —
// which in this product is workspace-wide: `deal` is an identity table, and
// every seat reads every deal. The boundary here is the OBJECT grant, not row
// scope, so that is what this asserts.
//
// A reader with no deal:read is refused, and one with it is served. Asserting
// a row-scope boundary instead would encode a rule the product does not have
// and would fail the day somebody reads tableclass.go and believes the test.
func TestEvidenceCarriesTheDealsOwnReadRule(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, criterionID := evidenceFixture(t, e, CriterionRoleIdentified)
	if _, err := e.store.RecordStageEvidence(e.asDealWriter(), claim(dealID, criterionID, AuthorSeller)); err != nil {
		t.Fatalf("recording: %v", err)
	}

	if _, err := e.store.ListStageEvidence(e.asDealless(), dealID); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a reader holding no deal:read read the ledger (%v)", err)
	}

	// The positive control: a colleague who is not the owner IS served, which
	// is the product's rule and what makes the refusal above about the grant.
	got, err := e.store.ListStageEvidence(e.asScopedStranger(), dealID)
	if err != nil {
		t.Fatalf("a colleague holding deal:read was refused: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("a colleague read %d rows of a ledger holding 1", len(got))
	}
}

// A deal nobody may see at all still answers 404 rather than an empty list:
// an unknown id and a hidden one are the same answer, by design.
func TestEvidenceForAnUnknownDealIsNotFound(t *testing.T) {
	e := setupConfigEnv(t)
	_, err := e.store.ListStageEvidence(e.asDealWriter(), ids.From[ids.DealKind](ids.NewV7()))
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("an unknown deal answered %v, not not-found", err)
	}
}

var _ = crmcontracts.StageEvidence{}

// asScopedStranger is a reader who holds deal:read but only over their OWN
// deals — the ordinary rep. The fixture's deal belongs to somebody else, so
// this is the principal a row-scope gate must withhold it from.
func (e *configEnv) asScopedStranger() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman,
		ID:   "human:" + ids.NewV7().String(), UserID: ids.NewV7(),
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			RowScope: principal.RowScopeOwn,
			Objects:  map[string]principal.ObjectGrant{"deal": {Read: true}},
		},
	})
}

// asDealWriter holds the deal verbs the fixture needs, beside the pipeline
// grant the criteria need.
func (e *configEnv) asDealWriter() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.admin.String(), UserID: e.admin,
		Permissions: principal.Permissions{
			RoleKeys: []string{"admin"},
			RowScope: principal.RowScopeAll,
			Objects: map[string]principal.ObjectGrant{
				"deal":     {Create: true, Read: true, Update: true},
				"pipeline": {Create: true, Read: true, Update: true},
			},
		},
	})
}

// asDealless holds pipeline grants but no deal verb at all — the reader the
// object gate must refuse.
func (e *configEnv) asDealless() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.admin.String(), UserID: e.admin,
		Permissions: principal.Permissions{
			RoleKeys: []string{"member"},
			Objects:  map[string]principal.ObjectGrant{"pipeline": {Read: true}},
		},
	})
}

// Nothing in the table ties an evidence row's criterion to its deal: the two
// are separate foreign keys, and the unique index on
// (deal, criterion, source) is satisfied whether or not they agree. So a
// caller could hang evidence for one deal on ANOTHER deal's criterion, and
// every reader counting a stage's met criteria would count a row belonging to
// somebody else's stage.
//
// ErrNotFound rather than a field fault: naming the mismatch would confirm
// that the other stage's criterion exists.
func TestEvidenceCannotNameACriterionFromAnotherDealsStage(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, _ := evidenceFixture(t, e, CriterionEventHeld)
	_, otherCriterion := evidenceFixture(t, e, CriterionEventHeld)

	_, err := e.store.RecordStageEvidence(e.asDealWriter(), claim(dealID, otherCriterion, AuthorBuyer))
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("evidence naming another stage's criterion answered %v, want not-found; "+
			"it would be counted against a stage it does not belong to", err)
	}
}

// The binding holds for a NOT-met claim too. The buyer-milestone rule exempts
// those — "the buyer has not confirmed" is an observation anybody may make —
// but a misfiled row is misfiled whichever way it reads.
func TestAnUnmetClaimIsAlsoBoundToItsDealsStage(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, _ := evidenceFixture(t, e, CriterionEventHeld)
	_, otherCriterion := evidenceFixture(t, e, CriterionEventHeld)

	in := claim(dealID, otherCriterion, AuthorBuyer)
	in.Met = false
	if _, err := e.store.RecordStageEvidence(e.asDealWriter(), in); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("an unmet claim on another stage's criterion answered %v, want not-found", err)
	}
}

// Evidence lands on the stage the deal was on WHEN THE THING HAPPENED, not the
// stage it has reached by the time the event is handled.
//
// The two differ whenever delivery lags a stage move — an outage, a backlog, a
// retry. Resolving against the current stage made the answer depend on queue
// latency: the same meeting settled a different criterion on a replay, and a
// criterion on a stage the deal had already left could never be settled at all.
func TestEvidenceLandsOnTheStageTheDealWasOnWhenItHappened(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, criterionID := evidenceFixture(t, e, CriterionDocumentSigned)
	observedAt := time.Now().UTC()

	// The deal moves on to a later stage that asks for the SAME kind, so a
	// writer resolving against the current stage would still find a criterion
	// — just the wrong one. A later stage with no criterion at all would let
	// this pass by writing nothing.
	later := laterStageAsking(t, e, dealID, CriterionDocumentSigned)

	written, err := e.store.RecordDeterministicEvidence(e.asDealWriter(), DeterministicClaim{
		DealID: dealID, Kind: CriterionDocumentSigned,
		SourceType: SourceContract, SourceID: ids.NewV7(),
		AuthorSide: AuthorUnknown, ObservedAt: observedAt,
	})
	if err != nil {
		t.Fatalf("recording a late-delivered signature: %v", err)
	}
	if written != 1 {
		t.Fatalf("the writer recorded %d rows", written)
	}

	all, err := e.store.ListStageEvidence(e.asDealWriter(), dealID)
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("the ledger holds %d rows", len(all))
	}
	if all[0].CriterionId == openapiUUID(later.UUID) {
		t.Fatal("the evidence landed on the criterion of the stage the deal " +
			"moved to, so which criterion it settles depends on delivery timing")
	}
	if all[0].CriterionId != openapiUUID(criterionID.UUID) {
		t.Errorf("the evidence landed on %v, want the criterion of the stage "+
			"the deal was on when the contract was signed (%v)",
			all[0].CriterionId, criterionID.UUID)
	}
}

// laterStageAsking advances the deal to a NEW stage on its own pipeline that
// carries a criterion of the same kind, and answers that criterion's id.
func laterStageAsking(
	t *testing.T, e *configEnv, dealID ids.DealID, kind CriterionKind,
) ids.ExitCriterionID {
	t.Helper()
	ctx := e.as()
	var pipelineID ids.PipelineID
	if err := e.owner.QueryRow(t.Context(), `
		SELECT s.pipeline_id FROM deal d JOIN stage s ON s.id = d.stage_id
		 WHERE d.id = $1`, dealID.UUID).Scan(&pipelineID); err != nil {
		t.Fatalf("reading the deal's pipeline: %v", err)
	}
	stage, err := e.store.CreateStage(ctx, CreateStageInput{
		PipelineID: pipelineID, Name: "Negotiation", Position: 1,
		Semantic: string(SemanticOpen),
	})
	if err != nil {
		t.Fatalf("seeding a later stage: %v", err)
	}
	stageID := ids.From[ids.StageKind](ids.UUID(stage.Id))
	criterion, err := e.store.CreateStageExitCriterion(ctx, CreateCriterionInput{
		StageID: stageID, Key: "the_thing", Label: "The thing", Kind: string(kind),
	})
	if err != nil {
		t.Fatalf("seeding the later stage's criterion: %v", err)
	}
	if _, err := e.store.AdvanceDeal(e.asDealWriter(), dealID,
		AdvanceDealInput{ToStageID: stageID}); err != nil {
		t.Fatalf("advancing the deal: %v", err)
	}
	return ids.From[ids.ExitCriterionKind](ids.UUID(criterion.Id))
}

// Evidence older than the deal itself lands on the stage the deal was CREATED
// on, rather than being discarded.
//
// This is the ordinary case, not an edge: a meeting happens, and the rep
// creates the deal because of it. Refusing evidence that predates the record
// would throw away exactly the evidence that motivated creating it.
func TestEvidenceOlderThanTheDealLandsOnTheStageItWasCreatedOn(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, criterionID := evidenceFixture(t, e, CriterionDocumentSigned)

	written, err := e.store.RecordDeterministicEvidence(e.asDealWriter(), DeterministicClaim{
		DealID: dealID, Kind: CriterionDocumentSigned,
		SourceType: SourceContract, SourceID: ids.NewV7(),
		AuthorSide: AuthorUnknown,
		// A week before the deal was created, which is the whole point.
		ObservedAt: time.Now().UTC().Add(-7 * 24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("recording evidence older than the deal: %v", err)
	}
	if written != 1 {
		t.Fatalf("evidence predating the deal wrote %d rows; the meeting that "+
			"prompted the deal would settle nothing", written)
	}
	all, err := e.store.ListStageEvidence(e.asDealWriter(), dealID)
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(all) != 1 || all[0].CriterionId != openapiUUID(criterionID.UUID) {
		t.Errorf("the row did not land on the criterion of the deal's first stage")
	}
}

// The SQL that counts a transcript's lines and the Go that states the
// addressing must agree, or a cited line number points a reader at different
// text than the one the evidence was read from.
//
// ReadActivityAuthorship measures the transcript in SQL deliberately: it needs
// to know a recording EXISTS, not what was said in it, and projecting the body
// would make it a content reader of a table whose audience decides who may read
// that text. So the count lives in two languages, and this holds them together.
//
// It calls the REAL reader rather than re-running a copy of its SQL. A test
// that pastes the expression it is checking proves the paste correct and says
// nothing about production — the version of this test that did exactly that is
// what Codex caught.
func TestTheTranscriptLineCountAgreesBetweenSQLAndGo(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, _ := evidenceFixture(t, e, CriterionEventHeld)

	for name, body := range map[string]string{
		"three turns":               "Dana: hello\nRep: hi\nDana: bye",
		"a trailing newline":        "Dana: hello\nRep: hi\n",
		"one line":                  "Dana: hello",
		"a blank line inside":       "Dana: hello\n\nRep: hi",
		"several trailing newlines": "Dana: hello\nRep: hi\n\n",
	} {
		activityID := seedTranscribedMeeting(t, e, dealID, body)
		var got ActivityAuthorship
		if err := e.store.Tx(e.asDealWriter(), func(tx pgx.Tx) error {
			var err error
			got, err = ReadActivityAuthorship(e.asDealWriter(), tx, activityID, noDomains{})
			return err
		}); err != nil {
			t.Fatalf("%s: reading the activity: %v", name, err)
		}
		if !got.HasTranscript {
			t.Errorf("%s: the reader did not see a transcript on a "+
				"source_system=transcript activity with a body", name)
		}
		if want := transcriptLineCount(body); got.TranscriptLines != want {
			t.Errorf("%s: the reader counted %d lines and Go counts %d; a "+
				"citation would point at different text than the reader sees",
				name, got.TranscriptLines, want)
		}
	}
}

// An activity with no transcript reports none, so HasTranscript is a real
// answer rather than a column that is always true.
func TestAnActivityWithNoTranscriptReportsNone(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, _ := evidenceFixture(t, e, CriterionEventHeld)
	activityID := seedTranscribedMeeting(t, e, dealID, "")

	var got ActivityAuthorship
	if err := e.store.Tx(e.asDealWriter(), func(tx pgx.Tx) error {
		var err error
		got, err = ReadActivityAuthorship(e.asDealWriter(), tx, activityID, noDomains{})
		return err
	}); err != nil {
		t.Fatalf("reading the activity: %v", err)
	}
	if got.HasTranscript || got.TranscriptLines != 0 {
		t.Errorf("an activity with no body reported a %d-line transcript", got.TranscriptLines)
	}
}

// seedTranscribedMeeting writes a meeting linked to the deal, carrying the
// given body as a transcript. An empty body is written as NULL, which is what
// a meeting logged without one holds.
func seedTranscribedMeeting(
	t *testing.T, e *configEnv, dealID ids.DealID, body string,
) ids.UUID {
	t.Helper()
	var id ids.UUID
	var storedBody, sourceSystem *string
	if body != "" {
		marker := TranscriptSourceSystem
		storedBody, sourceSystem = &body, &marker
	}
	if err := e.owner.QueryRow(t.Context(), `
		INSERT INTO activity (kind, subject, occurred_at, source, captured_by,
		                      source_system, body)
		VALUES ('meeting', 'Demo', now(), 'manual', 'human:test', $1, $2)
		RETURNING id`, sourceSystem, storedBody).Scan(&id); err != nil {
		t.Fatalf("seeding a transcribed meeting: %v", err)
	}
	if _, err := e.owner.Exec(t.Context(), `
		INSERT INTO activity_link (activity_id, entity_type, deal_id)
		VALUES ($1, 'deal', $2)`, id, dealID.UUID); err != nil {
		t.Fatalf("linking the meeting to the deal: %v", err)
	}
	return id
}

// noDomains is an installation that has declared no own domains, which is the
// safe direction for these reads: every address then looks external, so a
// transcript count can never be hidden by a domain judgement.
type noDomains struct{}

func (noDomains) Domains(context.Context, pgx.Tx) ([]string, error) { return nil, nil }
