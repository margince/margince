// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

// A legal review against a real database. The stage it gates, the request
// and the answer that open the gate, and who may do which.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/config/configtest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// reviewEnv is a config env whose store prices in EUR and lists the documents
// the fixture hands it, plus a second seat that reviews.
type reviewEnv struct {
	*configEnv
	reviewer ids.UUID
	docs     []PacketDocument
	openable map[ids.UUID]DocumentName
}

// ReviewDocuments stands in for the activities reader, which this package may
// not import; compose's own integration suite runs the real one.
func (e *reviewEnv) ReviewDocuments(context.Context, ids.UUID) ([]PacketDocument, error) {
	return e.docs, nil
}

// OpenableDocuments answers the files the fixture marked openable; the real
// attachment gate is held in compose's suite.
func (e *reviewEnv) OpenableDocuments(_ context.Context, attachmentIDs []ids.UUID) (map[ids.UUID]DocumentName, error) {
	out := map[ids.UUID]DocumentName{}
	for _, id := range attachmentIDs {
		if name, ok := e.openable[id]; ok {
			out[id] = name
		}
	}
	return out, nil
}

func setupReviewEnv(t *testing.T) *reviewEnv {
	t.Helper()
	e := &reviewEnv{configEnv: setupConfigEnv(t), reviewer: ids.NewV7()}
	if _, err := e.owner.Exec(t.Context(),
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Lena Legal')`,
		e.reviewer, "legal-"+e.reviewer.String()+"@config.test"); err != nil {
		t.Fatal(err)
	}
	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)), eurInstallation())
	store.reviewDocs = e
	e.store = store
	return e
}

// asRequester is the deal's owner: every deal verb and the offer read the
// pack needs, and no review grant.
func (e *reviewEnv) asRequester() context.Context {
	return humanWith(e.ws, e.admin, map[string]principal.ObjectGrant{
		"deal":     {Create: true, Read: true, Update: true},
		"offer":    {Read: true},
		"pipeline": {Create: true, Read: true, Update: true},
	})
}

// asReviewer holds the review grant and nothing on deals.
func (e *reviewEnv) asReviewer(grant principal.ObjectGrant) context.Context {
	return humanWith(e.ws, e.reviewer, map[string]principal.ObjectGrant{legalReviewEntity: grant})
}

func humanWith(ws, user ids.UUID, objects map[string]principal.ObjectGrant) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{
			RoleKeys: []string{"custom"}, RowScope: principal.RowScopeAll, Objects: objects,
		},
	})
}

var decide = principal.ObjectGrant{Read: true, Update: true}

// gatedDeal seeds a two-stage pipeline whose first stage requires legal's
// approval, with a deal on it.
func gatedDeal(t *testing.T, e *reviewEnv) (ids.DealID, ids.StageID, ids.StageID) {
	t.Helper()
	dealID, fromID, toID := twoStagePipeline(t, e.configEnv)
	if _, err := e.store.CreateStageExitCriterion(e.as(), CreateCriterionInput{
		StageID: fromID, Key: "legal", Label: "Legal approved the terms",
		Kind: string(CriterionLegalReviewed),
	}); err != nil {
		t.Fatalf("adding the legal criterion: %v", err)
	}
	return dealID, fromID, toID
}

func (e *reviewEnv) advance(dealID ids.DealID, to ids.StageID) error {
	_, err := e.store.AdvanceDeal(e.asRequester(), dealID, AdvanceDealInput{ToStageID: to})
	return err
}

func refusedFor(t *testing.T, err error, want LegalReviewStatus) {
	t.Helper()
	var required *LegalReviewRequiredError
	if !errors.As(err, &required) {
		t.Fatalf("the advance answered %v, want the legal review refusal", err)
	}
	if required.ReviewStatus != want {
		t.Fatalf("the refusal named a %q review, want %q", required.ReviewStatus, want)
	}
}

func (e *reviewEnv) request(t *testing.T, dealID ids.DealID, previous *ids.UUID) ids.UUID {
	t.Helper()
	out, err := e.store.RequestLegalReview(e.asRequester(), dealID, RequestLegalReviewInput{PreviousReviewID: previous})
	if err != nil {
		t.Fatalf("requesting a legal review: %v", err)
	}
	return ids.UUID(out.Id)
}

func (e *reviewEnv) answer(t *testing.T, id ids.UUID, outcome LegalReviewStatus) {
	t.Helper()
	comments := "Cap the liability at the contract value."
	if _, err := e.store.DecideLegalReview(e.asReviewer(decide), id, DecideLegalReviewInput{
		Outcome: outcome, Comments: &comments,
	}); err != nil {
		t.Fatalf("answering %s: %v", outcome, err)
	}
}

// The gate end to end. No review, a waiting one and one that asked for
// changes each hold the deal at its stage. Only an approval lets it pass.
func TestAStageThatNeedsLegalIsPassedOnlyAfterLegalApproves(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, toID := gatedDeal(t, e)

	refusedFor(t, e.advance(dealID, toID), "")
	first := e.request(t, dealID, nil)
	refusedFor(t, e.advance(dealID, toID), ReviewRequested)
	e.answer(t, first, ReviewChangesNeeded)
	refusedFor(t, e.advance(dealID, toID), ReviewChangesNeeded)

	again := e.request(t, dealID, &first)
	e.answer(t, again, ReviewApproved)
	if err := e.advance(dealID, toID); err != nil {
		t.Fatalf("an approved deal was held at its stage: %v", err)
	}
}

// Asking again sets the earlier approval aside: legal answers the new request
// before the gated stage opens.
func TestANewRequestSetsTheEarlierApprovalAside(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, toID := gatedDeal(t, e)
	e.answer(t, e.request(t, dealID, nil), ReviewApproved)
	e.request(t, dealID, nil)
	refusedFor(t, e.advance(dealID, toID), ReviewRequested)
}

// A move back and a move to a lost stage do not pass the stage legal guards.
func TestALostDealLeavesAGatedStageWithoutLegal(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, fromID, _ := gatedDeal(t, e)
	var pipelineID ids.PipelineID
	if err := e.owner.QueryRow(t.Context(), `SELECT pipeline_id FROM stage WHERE id = $1`, fromID).Scan(&pipelineID); err != nil {
		t.Fatal(err)
	}
	zero := 0
	lost, err := e.store.CreateStage(e.as(), CreateStageInput{
		PipelineID: pipelineID, Name: "Lost", Position: 2, Semantic: string(SemanticLost), WinProbability: &zero,
	})
	if err != nil {
		t.Fatalf("seeding a lost stage: %v", err)
	}
	reason := "budget cut"
	if _, err := e.store.AdvanceDeal(e.asRequester(), dealID, AdvanceDealInput{
		ToStageID: ids.From[ids.StageKind](ids.UUID(lost.Id)), LostReason: &reason, LostReasonCode: new("other"),
	}); err != nil {
		t.Fatalf("losing a deal on a gated stage was refused: %v", err)
	}
}

// Switched off, the gate stands aside and a request is refused as absent.
func TestSwitchingLegalReviewOffReleasesTheGate(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, toID := gatedDeal(t, e)
	switchLegalReview(t, e, false)
	t.Cleanup(func() { switchLegalReview(t, e, true) })

	_, err := e.store.RequestLegalReview(e.asRequester(), dealID, RequestLegalReviewInput{})
	var off *LegalReviewOffError
	if !errors.As(err, &off) || !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("a request while off answered %v, want the 404 legal_review_off", err)
	}
	if err := e.advance(dealID, toID); err != nil {
		t.Fatalf("the gate held a deal while legal review is off: %v", err)
	}
}

func switchLegalReview(t *testing.T, e *reviewEnv, on bool) {
	t.Helper()
	store := configtest.StoredOver(t, e.pool, ConfigSettings())
	admin := humanWith(e.ws, e.admin, map[string]principal.ObjectGrant{"installation_settings": {Read: true, Update: true}})
	if err := config.Save(admin, store, LegalReviewEnabled, on); err != nil {
		t.Fatalf("switching legal review to %v: %v", on, err)
	}
}

// A review is a second pair of eyes. The requester cannot answer their own.
// A seat without the read grant cannot answer blind. A connector is no seat.
func TestOnlyAnotherSeatHoldingTheReviewGrantAnswers(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, _ := gatedDeal(t, e)
	id := e.request(t, dealID, nil)

	own := humanWith(e.ws, e.admin, map[string]principal.ObjectGrant{legalReviewEntity: decide})
	_, err := e.store.DecideLegalReview(own, id, DecideLegalReviewInput{Outcome: ReviewApproved})
	var parse *values.ParseError
	if !errors.As(err, &parse) || parse.Code != "own_request" {
		t.Fatalf("the requester answering their own review got %v, want own_request", err)
	}

	_, err = e.store.DecideLegalReview(e.asReviewer(principal.ObjectGrant{Update: true}), id,
		DecideLegalReviewInput{Outcome: ReviewApproved})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("an update-only seat answered %v, want 403", err)
	}

	connector := principal.WithActor(principal.WithWorkspaceID(context.Background(), e.ws), principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:test", UserID: e.reviewer,
		Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{legalReviewEntity: decide}},
	})
	_, err = e.store.DecideLegalReview(connector, id, DecideLegalReviewInput{Outcome: ReviewApproved})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a connector answered %v, want 403", err)
	}
}

// One open request per deal, and a repeat links only a changes_needed review.
func TestADealWaitsForOneAnswerAtATime(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, _ := gatedDeal(t, e)
	first := e.request(t, dealID, nil)
	_, err := e.store.RequestLegalReview(e.asRequester(), dealID, RequestLegalReviewInput{})
	var open *LegalReviewOpenError
	if !errors.As(err, &open) || !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("a second open request answered %v, want 409 legal_review_already_open", err)
	}
	if _, err := e.store.WithdrawLegalReview(e.asRequester(), first); err != nil {
		t.Fatalf("withdrawing: %v", err)
	}
	_, err = e.store.RequestLegalReview(e.asRequester(), dealID, RequestLegalReviewInput{PreviousReviewID: &first})
	var parse *values.ParseError
	if !errors.As(err, &parse) || parse.Code != "invalid_previous_review" {
		t.Fatalf("linking a withdrawn review answered %v, want invalid_previous_review", err)
	}
}

// Only legal's decision settles the kind: the ledger refuses a rep's evidence.
func TestEvidenceCannotSettleALegalReviewCriterion(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, criterionID := evidenceFixture(t, e.configEnv, CriterionLegalReviewed)
	_, err := e.store.RecordStageEvidence(e.asDealWriter(), claim(dealID, criterionID, AuthorBuyer))
	var parse *values.ParseError
	if !errors.As(err, &parse) || parse.Code != "legal_review_settles_this" {
		t.Fatalf("evidence on a legal review criterion answered %v, want legal_review_settles_this", err)
	}
}

// The queue puts the largest value first, then the nearest close, and a
// review with neither last; a page boundary neither repeats nor skips.
func TestTheQueueSortsByValueThenCloseDate(t *testing.T) {
	e := setupReviewEnv(t)
	_, stageID, _ := twoStagePipeline(t, e.configEnv)
	soon, later := clocktest.Now(t).AddDate(0, 1, 0), clocktest.Now(t).AddDate(0, 3, 0)
	tag := " " + ids.NewV7().String()
	for _, seed := range []struct {
		name   string
		amount *int64
		close  *time.Time
	}{
		{"big later", new(int64(900_00)), &later},
		{"small", new(int64(100_00)), &soon},
		{"no value", nil, &soon},
		{"big soon", new(int64(900_00)), &soon},
	} {
		e.request(t, queueDeal(t, e, stageID, seed.name+tag, seed.amount, seed.close), nil)
	}
	want := []string{"big soon" + tag, "big later" + tag, "small" + tag, "no value" + tag}
	reader := humanWith(e.ws, e.reviewer, map[string]principal.ObjectGrant{legalReviewEntity: {Read: true}, "deal": {Read: true}})
	got := queueNames(reader, t, e, tag)
	if len(got) != len(want) {
		t.Fatalf("the queue paged out %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("the queue read %v, want %v", got, want)
		}
	}
}

// queueNames pages through the whole open queue one row at a time. It keeps
// the deals this test seeded, which carry tag, because other tests share it.
func queueNames(reader context.Context, t *testing.T, e *reviewEnv, tag string) []string {
	t.Helper()
	var got []string
	var cursor *string
	one := 1
	for {
		page, next, err := e.store.ListLegalReviewQueue(reader, QueueOpen, cursor, &one)
		if err != nil {
			t.Fatalf("reading the queue: %v", err)
		}
		for _, item := range page {
			if strings.HasSuffix(item.DealName, tag) {
				got = append(got, item.DealName)
			}
		}
		if !next.HasMore {
			return got
		}
		cursor = &next.NextCursor
	}
}

func queueDeal(t *testing.T, e *reviewEnv, stageID ids.StageID, name string, amount *int64, closeOn *time.Time) ids.DealID {
	t.Helper()
	var pipelineID ids.PipelineID
	if err := e.owner.QueryRow(t.Context(), `SELECT pipeline_id FROM stage WHERE id = $1`, stageID).Scan(&pipelineID); err != nil {
		t.Fatal(err)
	}
	in := CreateDealInput{Name: name, PipelineID: pipelineID, StageID: stageID, AmountMinor: amount, ExpectedClose: closeOn}
	if amount != nil {
		in.Currency = new("EUR")
	}
	deal, err := e.store.CreateDeal(e.asRequester(), in)
	if err != nil {
		t.Fatalf("seeding deal %q: %v", name, err)
	}
	return ids.From[ids.DealKind](ids.UUID(deal.Id))
}

// The pack is the deal as it stood, read with the reviewer's own grants. A
// document the reviewer cannot open stays listed, unavailable and unnamed.
func TestTheReviewerReadsThePackWithoutADealGrant(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, _ := gatedDeal(t, e)
	openable, removed, mailed := ids.NewV7(), ids.NewV7(), ids.NewV7()
	e.docs = []PacketDocument{
		{AttachmentID: openable, Category: "contract", DocState: "draft", AddedAt: clocktest.Now(t).UTC()},
		{AttachmentID: removed, Category: "legal", DocState: "final", AddedAt: clocktest.Now(t).UTC()},
		{AttachmentID: mailed, Category: "offer", DocState: "current", AddedAt: clocktest.Now(t).UTC(), FromMessage: true},
	}
	e.openable = map[ids.UUID]DocumentName{openable: {Filename: "msa.pdf"}}
	id := e.request(t, dealID, nil)

	got, err := e.store.GetLegalReview(e.asReviewer(principal.ObjectGrant{Read: true}), id)
	if err != nil {
		t.Fatalf("the reviewer reading the pack: %v", err)
	}
	// Without deal.read the deal reads as its name, and only openable files show.
	if got.Packet.Deal.Name != "Warehouse rollout" || got.Packet.Deal.StageName != nil || len(got.Packet.History) != 0 {
		t.Fatalf("a reviewer with no deal grant read stage %v and %d history rows", got.Packet.Deal.StageName, len(got.Packet.History))
	}
	if len(got.Packet.Documents) != 1 || ids.UUID(got.Packet.Documents[0].AttachmentId) != openable {
		t.Fatalf("a reviewer with no deal grant was shown %+v, want the openable file alone", got.Packet.Documents)
	}
	dealReader := humanWith(e.ws, e.reviewer, map[string]principal.ObjectGrant{legalReviewEntity: {Read: true}, "deal": {Read: true}})
	full, err := e.store.GetLegalReview(dealReader, id)
	if err != nil || full.Packet.Deal.StageName == nil || *full.Packet.Deal.StageName != "Discovery" || len(full.Packet.History) == 0 {
		t.Fatalf("a reviewer who reads deals saw stage %v and %d history rows (%v)", full.Packet.Deal.StageName, len(full.Packet.History), err)
	}
	// A deal upload removed since reads as unavailable and nameless; a file
	// from mail outside the reader's audience is not listed at all.
	shown := map[ids.UUID]crmcontracts.LegalReviewPacketDocument{}
	for _, doc := range full.Packet.Documents {
		shown[ids.UUID(doc.AttachmentId)] = doc
	}
	if _, listed := shown[mailed]; listed || len(shown) != 2 {
		t.Fatalf("the deal reader was shown %+v; a mailed file they cannot open must not be listed", full.Packet.Documents)
	}
	if gone := shown[removed]; gone.Available || gone.Filename != nil || gone.Title != nil {
		t.Fatalf("a removed deal upload read available=%v with name %v", gone.Available, gone.Filename)
	}
	if !shown[openable].Available || shown[openable].Filename == nil {
		t.Fatal("the openable file was not available with its name")
	}
	if _, err := e.store.GetLegalReview(e.asRequester(), id); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a seat without the review grant read the pack: %v", err)
	}
}

// threeStageDeal seeds Discovery, a gated Legal stage and Contract, with a
// deal on Discovery.
func threeStageDeal(t *testing.T, e *reviewEnv) (ids.DealID, ids.StageID) {
	t.Helper()
	dealID, _, gated := twoStagePipeline(t, e.configEnv)
	if _, err := e.store.CreateStageExitCriterion(e.as(), CreateCriterionInput{
		StageID: gated, Key: "legal", Label: "Legal approved the terms", Kind: string(CriterionLegalReviewed),
	}); err != nil {
		t.Fatalf("adding the legal criterion: %v", err)
	}
	var pipelineID ids.PipelineID
	if err := e.owner.QueryRow(t.Context(), `SELECT pipeline_id FROM stage WHERE id = $1`, gated).Scan(&pipelineID); err != nil {
		t.Fatal(err)
	}
	open := 0
	last, err := e.store.CreateStage(e.as(), CreateStageInput{
		PipelineID: pipelineID, Name: "Contract", Position: 2, Semantic: string(SemanticOpen), WinProbability: &open,
	})
	if err != nil {
		t.Fatalf("seeding the third stage: %v", err)
	}
	return dealID, ids.From[ids.StageKind](ids.UUID(last.Id))
}

// A jump over the gated stage passes it as surely as a step off it does.
func TestAMoveCannotJumpOverAStageThatNeedsLegal(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, last := threeStageDeal(t, e)
	refusedFor(t, e.advance(dealID, last), "")
}

// Legal approving the deal before it reaches the gated stage settles that
// stage too: the approval is of the deal, not of one step.
func TestAnEarlierApprovalOpensALaterGatedStage(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, last := threeStageDeal(t, e)
	e.answer(t, e.request(t, dealID, nil), ReviewApproved)
	if err := e.advance(dealID, last); err != nil {
		t.Fatalf("an approved deal could not pass the gated stage: %v", err)
	}
}

// A deal cannot be born in the future of a gated stage it never passed.
func TestADealCannotBeCreatedPastAStageThatNeedsLegal(t *testing.T) {
	e := setupReviewEnv(t)
	_, last := threeStageDeal(t, e)
	var pipelineID ids.PipelineID
	if err := e.owner.QueryRow(t.Context(), `SELECT pipeline_id FROM stage WHERE id = $1`, last).Scan(&pipelineID); err != nil {
		t.Fatal(err)
	}
	_, err := e.store.CreateDeal(e.asRequester(), CreateDealInput{Name: "Skipped legal", PipelineID: pipelineID, StageID: last})
	if _, refused := errors.AsType[*LegalGateOnCreateError](err); !refused {
		t.Fatalf("a deal born past the gated stage answered %v, want legal_review_required", err)
	}
}

// An approval holds for the terms legal read. Changing the amount afterwards
// sends the deal back to legal.
func TestAnApprovalLapsesWhenTheAmountChanges(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, toID := gatedDeal(t, e)
	e.answer(t, e.request(t, dealID, nil), ReviewApproved)
	amount, currency := int64(50_000_00), "EUR"
	if _, err := e.store.UpdateDeal(e.asRequester(), dealID, UpdateDealInput{AmountMinor: &amount, Currency: &currency}); err != nil {
		t.Fatalf("changing the amount: %v", err)
	}
	var required *LegalReviewRequiredError
	if err := e.advance(dealID, toID); !errors.As(err, &required) || !required.TermsChanged {
		t.Fatalf("a deal whose amount changed after approval answered %v, want the terms-changed refusal", err)
	}
}
