// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

// The legal gate against the ways around it. A pipeline changed under a deal,
// the switch flipped, writes racing a move. And a reviewer who may see less
// than the requester did.

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/config/configtest"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// threeStagesBehind seeds Discovery, Negotiation and Contract with the deal
// moved to Negotiation, and no gate anywhere yet.
func threeStagesBehind(t *testing.T, e *reviewEnv) (ids.DealID, ids.StageID, ids.StageID, ids.StageID) {
	t.Helper()
	dealID, first, second := twoStagePipeline(t, e.configEnv)
	var pipelineID ids.PipelineID
	if err := e.owner.QueryRow(t.Context(), `SELECT pipeline_id FROM stage WHERE id = $1`, first).Scan(&pipelineID); err != nil {
		t.Fatal(err)
	}
	open := 0
	third, err := e.store.CreateStage(e.as(), CreateStageInput{
		PipelineID: pipelineID, Name: "Contract", Position: 2, Semantic: string(SemanticOpen), WinProbability: &open,
	})
	if err != nil {
		t.Fatalf("seeding the third stage: %v", err)
	}
	if err := e.advance(dealID, second); err != nil {
		t.Fatalf("moving the deal to the second stage: %v", err)
	}
	return dealID, first, second, ids.From[ids.StageKind](ids.UUID(third.Id))
}

func (e *reviewEnv) addLegalCriterion(t *testing.T, stage ids.StageID, required bool) ids.ExitCriterionID {
	t.Helper()
	c, err := e.store.CreateStageExitCriterion(e.as(), CreateCriterionInput{
		StageID: stage, Key: "legal", Label: "Legal approved", Kind: string(CriterionLegalReviewed), Required: &required,
	})
	if err != nil {
		t.Fatalf("adding the legal criterion: %v", err)
	}
	return ids.From[ids.ExitCriterionKind](ids.UUID(c.Id))
}

// A gate added to a stage the deal already left still holds its next move.
func TestAGateAddedBehindADealHoldsItsNextMove(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, first, _, third := threeStagesBehind(t, e)
	e.addLegalCriterion(t, first, true)
	refusedFor(t, e.advance(dealID, third), "")
}

// An optional criterion made required behind the deal holds it too.
func TestACriterionMadeRequiredBehindADealHoldsIt(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, first, _, third := threeStagesBehind(t, e)
	criterion := e.addLegalCriterion(t, first, false)
	if err := e.advance(dealID, third); err != nil {
		t.Fatalf("an optional legal criterion held the deal: %v", err)
	}
	if err := e.advance(dealID, first); err != nil {
		t.Fatalf("moving the deal back: %v", err)
	}
	if err := e.advance(dealID, third); err != nil {
		t.Fatalf("moving the deal forward again: %v", err)
	}
	required := true
	if _, err := e.store.UpdateStageExitCriterion(e.as(), first, criterion, UpdateCriterionInput{Required: &required}); err != nil {
		t.Fatalf("making the criterion required: %v", err)
	}
	var pipelineID ids.PipelineID
	if err := e.owner.QueryRow(t.Context(), `SELECT pipeline_id FROM stage WHERE id = $1`, first).Scan(&pipelineID); err != nil {
		t.Fatal(err)
	}
	refusedFor(t, e.advance(dealID, laterStage(t, e, pipelineID)), "")
}

// laterStage adds an open stage after the pipeline's last and answers it.
func laterStage(t *testing.T, e *reviewEnv, pipelineID ids.PipelineID) ids.StageID {
	t.Helper()
	var position int
	if err := e.owner.QueryRow(t.Context(), `SELECT max(position) + 1 FROM stage WHERE pipeline_id = $1`, pipelineID).Scan(&position); err != nil {
		t.Fatal(err)
	}
	open := 0
	stage, err := e.store.CreateStage(e.as(), CreateStageInput{
		PipelineID: pipelineID, Name: "Signature", Position: position, Semantic: string(SemanticOpen), WinProbability: &open,
	})
	if err != nil {
		t.Fatalf("seeding a later stage: %v", err)
	}
	return ids.From[ids.StageKind](ids.UUID(stage.Id))
}

// A gated stage reordered behind the deal holds its next move.
func TestAGatedStageReorderedBehindADealHoldsIt(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, first, second, third := threeStagesBehind(t, e)
	e.addLegalCriterion(t, third, true)
	var pipelineID ids.PipelineID
	if err := e.owner.QueryRow(t.Context(), `SELECT pipeline_id FROM stage WHERE id = $1`, first).Scan(&pipelineID); err != nil {
		t.Fatal(err)
	}
	// Contract, the gated stage, now comes first; the deal sits after it.
	if _, err := e.store.ReorderStages(e.as(), pipelineID, []ids.UUID{third.UUID, first.UUID, second.UUID}, nil); err != nil {
		t.Fatalf("reordering the stages: %v", err)
	}
	refusedFor(t, e.advance(dealID, laterStage(t, e, pipelineID)), "")
}

// Switched off, a deal passes the gate; switched on again, its next move
// needs an approval of its terms like any other. A request left open stays open.
func TestSwitchingLegalReviewOffAndOnAgain(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, second, third := threeStagesBehind(t, e)
	e.addLegalCriterion(t, second, true)
	pending := e.request(t, dealID, nil)
	switchLegalReview(t, e, false)
	t.Cleanup(func() { switchLegalReview(t, e, true) })
	if err := e.advance(dealID, third); err != nil {
		t.Fatalf("the gate held a deal while legal review was off: %v", err)
	}
	_, err := e.store.DecideLegalReview(e.asReviewer(decide), pending, DecideLegalReviewInput{Outcome: ReviewApproved})
	if _, off := errors.AsType[*LegalReviewOffError](err); !off {
		t.Fatalf("a decision while off answered %v, want legal_review_off", err)
	}
	switchLegalReview(t, e, true)
	var pipelineID ids.PipelineID
	if err := e.owner.QueryRow(t.Context(), `SELECT pipeline_id FROM stage WHERE id = $1`, third).Scan(&pipelineID); err != nil {
		t.Fatal(err)
	}
	signature := laterStage(t, e, pipelineID)
	refusedFor(t, e.advance(dealID, signature), ReviewRequested)
	e.answer(t, pending, ReviewApproved)
	if err := e.advance(dealID, signature); err != nil {
		t.Fatalf("the approved deal could not move on: %v", err)
	}
}

// Ops holds installation_settings.update for the wiring it runs; whether
// deals wait for legal is an admin's call.
func TestOnlyAnAdminSwitchesLegalReview(t *testing.T) {
	e := setupReviewEnv(t)
	store := configtest.StoredOver(t, e.pool, ConfigSettings())
	ops := principal.WithActor(principal.WithWorkspaceID(context.Background(), e.ws), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.admin.String(), UserID: e.admin,
		Permissions: principal.Permissions{
			RoleKeys: []string{"ops"}, RowScope: principal.RowScopeAll,
			Objects: map[string]principal.ObjectGrant{"installation_settings": {Read: true, Update: true}},
		},
	})
	if err := SetLegalReviewEnabled(ops, store, false); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("ops switching legal review answered %v, want 403", err)
	}
	admin := humanWith(e.ws, e.admin, map[string]principal.ObjectGrant{"installation_settings": {Read: true, Update: true}})
	admin = withRole(admin, "admin")
	if err := SetLegalReviewEnabled(admin, store, true); err != nil {
		t.Fatalf("an admin switching legal review: %v", err)
	}
}

func withRole(ctx context.Context, role string) context.Context {
	p, _ := principal.Actor(ctx)
	p.Permissions.RoleKeys = []string{role}
	return principal.WithActor(ctx, p)
}

// Two decisions on one review: the row lock lets one through and the second
// finds it decided. Two requests on one deal: one is opened, the other 409s.
func TestConcurrentDecisionsAndRequestsSettleOnce(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, _ := gatedDeal(t, e)
	var wg sync.WaitGroup
	requests := make([]error, 2)
	for i := range requests {
		wg.Go(func() {
			_, requests[i] = e.store.RequestLegalReview(e.asRequester(), dealID, RequestLegalReviewInput{})
		})
	}
	wg.Wait()
	if opened, refused := countOutcomes(requests, &LegalReviewOpenError{}); opened != 1 || refused != 1 {
		t.Fatalf("two simultaneous requests: %v, want one opened and one legal_review_already_open", requests)
	}
	newest, err := newestReviewID(e, dealID)
	if err != nil {
		t.Fatal(err)
	}
	decisions := make([]error, 2)
	for i := range decisions {
		wg.Go(func() {
			_, decisions[i] = e.store.DecideLegalReview(e.asReviewer(decide), newest, DecideLegalReviewInput{Outcome: ReviewApproved})
		})
	}
	wg.Wait()
	if decided, refused := countOutcomes(decisions, &LegalReviewDecidedError{}); decided != 1 || refused != 1 {
		t.Fatalf("two simultaneous decisions: %v, want one recorded and one legal_review_decided", decisions)
	}
}

func countOutcomes[T error](errs []error, _ T) (ok, refused int) {
	for _, err := range errs {
		if err == nil {
			ok++
		} else if _, is := errors.AsType[T](err); is {
			refused++
		}
	}
	return ok, refused
}

func newestReviewID(e *reviewEnv, dealID ids.DealID) (ids.UUID, error) {
	var id ids.UUID
	err := e.owner.QueryRow(context.Background(),
		`SELECT id FROM legal_review WHERE deal_id = $1 AND status = 'requested'`, dealID).Scan(&id)
	return id, err
}

// A change to the deal's amount is still in flight when the advance starts.
// The advance waits on the deal row, then decides on the new amount.
func TestAnAdvanceWaitsForAnAmountChangeInFlight(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, toID := gatedDeal(t, e)
	e.answer(t, e.request(t, dealID, nil), ReviewApproved)

	ctx := t.Context()
	writer, err := e.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Stands in for any writer of the deal's terms: every one of them takes
	// this row lock first, which is the property under test.
	if _, err := writer.Exec(ctx, `SELECT 1 FROM deal WHERE id = $1 FOR UPDATE`, dealID); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Exec(ctx, `UPDATE deal SET amount_minor = 7700000, currency = 'EUR' WHERE id = $1`, dealID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- e.advance(dealID, toID) }()
	waitForALockWaiter(t, e)
	if err := writer.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var required *LegalReviewRequiredError
	if err := <-done; !errors.As(err, &required) || !required.TermsChanged {
		t.Fatalf("the advance answered %v, want the terms-changed refusal it sees once the write commits", err)
	}
}

// waitForALockWaiter returns once some backend waits on a row lock, which is
// the advance blocked on the writer. It fails rather than waiting forever.
func waitForALockWaiter(t *testing.T, e *reviewEnv) {
	t.Helper()
	// Bounded by tries rather than a clock: each try is a round trip.
	for range 20000 {
		var waiting int
		if err := e.owner.QueryRow(t.Context(),
			`SELECT count(*) FROM pg_stat_activity WHERE wait_event_type = 'Lock' AND datname = current_database()`).
			Scan(&waiting); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			t.Fatal(err)
		}
		if waiting > 0 {
			return
		}
	}
	t.Fatal("the advance never waited on the deal row the writer holds")
}

// A reviewer who may see less than the requester reads less of the pack. Not
// the money their field masks hide, and not a company they cannot see.
func TestThePackShowsAReviewerOnlyWhatTheyMayRead(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, _ := gatedDeal(t, e)
	company := ids.NewV7()
	if _, err := e.owner.Exec(t.Context(), `
		INSERT INTO company (id, display_name, source, captured_by, owner_id) VALUES ($1, 'Acme', 'manual', 'human:fixture', $2)`,
		company, e.admin); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(t.Context(), `UPDATE deal SET company_id = $2, amount_minor = 500000, currency = 'EUR' WHERE id = $1`,
		dealID, company); err != nil {
		t.Fatal(err)
	}
	requester := humanWith(e.ws, e.admin, map[string]principal.ObjectGrant{
		"deal": {Read: true, Update: true}, "offer": {Read: true}, "company": {Read: true},
	})
	review, err := e.store.RequestLegalReview(requester, dealID, RequestLegalReviewInput{})
	if err != nil {
		t.Fatalf("requesting: %v", err)
	}

	wide := humanWith(e.ws, e.reviewer, map[string]principal.ObjectGrant{
		legalReviewEntity: {Read: true}, "deal": {Read: true}, "company": {Read: true},
	})
	got, err := e.store.GetLegalReview(wide, ids.UUID(review.Id))
	if err != nil || got.Packet.Deal.CompanyName == nil || got.Packet.Deal.AmountMinor == nil {
		t.Fatalf("a reviewer who may read both saw company %v and amount %v (%v)",
			got.Packet.Deal.CompanyName, got.Packet.Deal.AmountMinor, err)
	}

	narrow := principal.WithActor(principal.WithWorkspaceID(context.Background(), e.ws), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.reviewer.String(), UserID: e.reviewer,
		Permissions: principal.Permissions{
			RoleKeys: []string{"legal"}, RowScope: principal.RowScopeOwn,
			Objects:    map[string]principal.ObjectGrant{legalReviewEntity: {Read: true}, "deal": {Read: true}},
			FieldMasks: []principal.FieldMask{{Object: "deal", Field: dealAmountField, Condition: principal.MaskAlways}},
		},
	})
	got, err = e.store.GetLegalReview(narrow, ids.UUID(review.Id))
	if err != nil {
		t.Fatalf("the narrow reviewer reading the pack: %v", err)
	}
	if got.Packet.Deal.AmountMinor != nil || got.Packet.ValueMinor != nil {
		t.Fatalf("a reviewer whose role masks the amount saw %v and %v", got.Packet.Deal.AmountMinor, got.Packet.ValueMinor)
	}
	if got.Packet.Deal.CompanyName != nil || got.Packet.Deal.CompanyId != nil {
		t.Fatalf("a reviewer who cannot see the company saw %v", got.Packet.Deal.CompanyName)
	}
	if got.Packet.Deal.StageName == nil {
		t.Fatal("a reviewer who reads deals lost the stage along with the amount")
	}
}

// The offer follows the reader's offer.read, and its money the deal's mask:
// the offer's total is the deal's amount by another name.
func TestThePackShowsTheOfferOnlyToAnOfferReaderAndItsMoneyUnderTheMask(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, _ := gatedDeal(t, e)
	description, price, tax := "Implementation", int64(500_000), "19.00"
	if _, err := e.store.CreateOffer(e.offerCtx(), dealID, CreateOfferInput{
		Currency: "EUR", Source: "manual",
		LineItems: []OfferLineInputRow{{Description: &description, Quantity: "1", UnitPriceMinor: &price, TaxRate: &tax}},
	}); err != nil {
		t.Fatalf("seeding an offer: %v", err)
	}
	id := e.request(t, dealID, nil)

	noOffer := humanWith(e.ws, e.reviewer, map[string]principal.ObjectGrant{legalReviewEntity: {Read: true}, "deal": {Read: true}})
	got, err := e.store.GetLegalReview(noOffer, id)
	if err != nil || got.Packet.Offer != nil {
		t.Fatalf("a reviewer without offer.read saw offer %v (%v)", got.Packet.Offer, err)
	}
	masked := principal.WithActor(principal.WithWorkspaceID(context.Background(), e.ws), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.reviewer.String(), UserID: e.reviewer,
		Permissions: principal.Permissions{
			RoleKeys: []string{"legal"}, RowScope: principal.RowScopeOwn,
			Objects:    map[string]principal.ObjectGrant{legalReviewEntity: {Read: true}, "deal": {Read: true}, "offer": {Read: true}},
			FieldMasks: []principal.FieldMask{{Object: "deal", Field: dealAmountField, Condition: principal.MaskAlways}},
		},
	})
	got, err = e.store.GetLegalReview(masked, id)
	if err != nil || got.Packet.Offer == nil {
		t.Fatalf("an offer reader saw no offer (%v)", err)
	}
	if got.Packet.Offer.GrossMinor != nil || got.Packet.Offer.Lines[0].UnitPriceMinor != nil {
		t.Fatal("a reader whose role masks the deal's amount saw the offer's money")
	}
}

// A lost deal reopened past the gate passes it as surely as a forward step,
// so the reopen needs the approval a step would.
func TestALostDealReopenedPastTheGateNeedsLegal(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, contract := threeStageDeal(t, e)
	var pipelineID ids.PipelineID
	if err := e.owner.QueryRow(t.Context(), `SELECT pipeline_id FROM stage WHERE id = $1`, contract).Scan(&pipelineID); err != nil {
		t.Fatal(err)
	}
	zero := 0
	lost, err := e.store.CreateStage(e.as(), CreateStageInput{
		PipelineID: pipelineID, Name: "Lost", Position: 3, Semantic: string(SemanticLost), WinProbability: &zero,
	})
	if err != nil {
		t.Fatalf("seeding a lost stage: %v", err)
	}
	reason := "budget cut"
	if _, err := e.store.AdvanceDeal(e.asRequester(), dealID, AdvanceDealInput{
		ToStageID: ids.From[ids.StageKind](ids.UUID(lost.Id)), LostReason: &reason, LostReasonCode: new("other"),
	}); err != nil {
		t.Fatalf("losing the deal: %v", err)
	}
	refusedFor(t, e.advance(dealID, contract), "")
}

// The autopilot and other passes run as the system principal. The gate holds
// it like any seat, because it asks about the deal, not the caller.
func TestTheSystemPrincipalIsHeldByTheGate(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, toID := gatedDeal(t, e)
	system := principal.WithActor(principal.WithWorkspaceID(context.Background(), e.ws),
		principal.Principal{Type: principal.PrincipalSystem, ID: "system:legal-review-test"})
	_, err := e.store.AdvanceDeal(system, dealID, AdvanceDealInput{ToStageID: toID})
	refusedFor(t, err, "")
}

// Withdrawing a later request leaves the deal with no standing approval: the
// newest review decides, and an older approval does not come back.
func TestAWithdrawnLaterRequestDoesNotReviveAnOlderApproval(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, toID := gatedDeal(t, e)
	e.answer(t, e.request(t, dealID, nil), ReviewApproved)
	later := e.request(t, dealID, nil)
	if _, err := e.store.WithdrawLegalReview(e.asRequester(), later); err != nil {
		t.Fatalf("withdrawing: %v", err)
	}
	refusedFor(t, e.advance(dealID, toID), ReviewWithdrawn)
}

// A reader whose role masks the amount gets the queue in close-date order.
// The cursor names a row and carries no figure of it.
func TestTheQueueDoesNotSortOrPageByAMaskedValue(t *testing.T) {
	e := setupReviewEnv(t)
	_, stageID, _ := twoStagePipeline(t, e.configEnv)
	soon, later := clocktest.Now(t).AddDate(0, 1, 0), clocktest.Now(t).AddDate(0, 3, 0)
	tag := " " + ids.NewV7().String()
	e.request(t, queueDeal(t, e, stageID, "big later"+tag, new(int64(900_00)), &later), nil)
	e.request(t, queueDeal(t, e, stageID, "small soon"+tag, new(int64(100_00)), &soon), nil)

	masked := principal.WithActor(principal.WithWorkspaceID(context.Background(), e.ws), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.reviewer.String(), UserID: e.reviewer,
		Permissions: principal.Permissions{
			RoleKeys: []string{"legal"}, RowScope: principal.RowScopeOwn,
			Objects:    map[string]principal.ObjectGrant{legalReviewEntity: {Read: true}, "deal": {Read: true}},
			FieldMasks: []principal.FieldMask{{Object: "deal", Field: dealAmountField, Condition: principal.MaskAlways}},
		},
	})
	if got := queueNames(masked, t, e, tag); len(got) != 2 || got[0] != "small soon"+tag {
		t.Fatalf("the masked reader's queue read %v, want the nearest close first", got)
	}
	one := 1
	_, next, err := e.store.ListLegalReviewQueue(masked, QueueOpen, nil, &one)
	if err != nil || !next.HasMore {
		t.Fatalf("reading the first page: %v", err)
	}
	// The token names the last row and nothing else: no value, no date.
	cursor, err := storekit.DecodeOpaque[map[string]any](next.NextCursor)
	if err != nil || len(cursor) != 1 || cursor["id"] == nil {
		t.Fatalf("the queue's cursor carried %v (%v), want the last row's id alone", cursor, err)
	}
}

// Ops holds pipeline.update. Each config change that weakens a legal gate is
// the admin's, as the switch is. Ops is refused, an admin is not.
func TestOnlyAnAdminWeakensALegalGate(t *testing.T) {
	ops := func(e *reviewEnv) context.Context {
		return withRole(humanWith(e.ws, e.admin, map[string]principal.ObjectGrant{
			"pipeline": {Create: true, Read: true, Update: true, Delete: true},
		}), "ops")
	}
	for name, weaken := range map[string]func(context.Context, *reviewEnv, ids.PipelineID, ids.StageID, ids.ExitCriterionID) error{
		"un-require the criterion": func(ctx context.Context, e *reviewEnv, _ ids.PipelineID, stage ids.StageID, c ids.ExitCriterionID) error {
			_, err := e.store.UpdateStageExitCriterion(ctx, stage, c, UpdateCriterionInput{Required: new(false)})
			return err
		},
		"archive the criterion": func(ctx context.Context, e *reviewEnv, _ ids.PipelineID, stage ids.StageID, c ids.ExitCriterionID) error {
			return e.store.ArchiveStageExitCriterion(ctx, stage, c, nil)
		},
		"archive the stage": func(ctx context.Context, e *reviewEnv, _ ids.PipelineID, stage ids.StageID, _ ids.ExitCriterionID) error {
			return e.store.ArchiveStage(ctx, stage, nil)
		},
		"reorder the stages": func(ctx context.Context, e *reviewEnv, pipeline ids.PipelineID, stage ids.StageID, _ ids.ExitCriterionID) error {
			order := stageOrder(t, e, pipeline)
			order[0], order[1] = order[1], order[0]
			_, err := e.store.ReorderStages(ctx, pipeline, order, nil)
			return err
		},
		"move the stage": func(ctx context.Context, e *reviewEnv, _ ids.PipelineID, stage ids.StageID, _ ids.ExitCriterionID) error {
			_, err := e.store.UpdateStage(ctx, stage, UpdateStageInput{Position: new(5)})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := setupReviewEnv(t)
			_, _, gated := twoStagePipeline(t, e.configEnv)
			criterion := e.addLegalCriterion(t, gated, true)
			var pipeline ids.PipelineID
			if err := e.owner.QueryRow(t.Context(), `SELECT pipeline_id FROM stage WHERE id = $1`, gated).Scan(&pipeline); err != nil {
				t.Fatal(err)
			}
			if err := weaken(ops(e), e, pipeline, gated, criterion); !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Fatalf("ops weakening the gate answered %v, want 403", err)
			}
			admin := withRole(humanWith(e.ws, e.admin, map[string]principal.ObjectGrant{
				"pipeline": {Create: true, Read: true, Update: true, Delete: true},
			}), "admin")
			if err := weaken(admin, e, pipeline, gated, criterion); err != nil {
				t.Fatalf("an admin weakening the gate: %v", err)
			}
		})
	}
}

func stageOrder(t *testing.T, e *reviewEnv, pipeline ids.PipelineID) []ids.UUID {
	t.Helper()
	rows, err := e.owner.Query(t.Context(), `SELECT id FROM stage WHERE pipeline_id = $1 AND archived_at IS NULL ORDER BY position`, pipeline)
	if err != nil {
		t.Fatal(err)
	}
	order, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		t.Fatal(err)
	}
	return order
}

// The deal's owner is no second pair of eyes on their own deal.
func TestTheDealsOwnerCannotAnswerItsReview(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, _ := gatedDeal(t, e)
	// Set in place: the owner write hands pending proposals on through the
	// approvals module, which this package may not import.
	if _, err := e.owner.Exec(t.Context(), `UPDATE deal SET owner_id = $2 WHERE id = $1`, dealID, e.reviewer); err != nil {
		t.Fatalf("handing the deal to the reviewer: %v", err)
	}
	id := e.request(t, dealID, nil)
	_, err := e.store.DecideLegalReview(e.asReviewer(decide), id, DecideLegalReviewInput{Outcome: ReviewApproved})
	var parse *values.ParseError
	if !errors.As(err, &parse) || parse.Code != "own_deal" {
		t.Fatalf("the deal's owner answering its review got %v, want own_deal", err)
	}
}

// The queue has two halves; any other status is the caller's mistake, not
// the open queue.
func TestTheQueueRefusesAStatusItDoesNotHave(t *testing.T) {
	e := setupReviewEnv(t)
	_, _, err := e.store.ListLegalReviewQueue(e.asReviewer(principal.ObjectGrant{Read: true}), "pending", nil, nil)
	var parse *values.ParseError
	if !errors.As(err, &parse) || parse.Field != "status" {
		t.Fatalf("an unknown status answered %v, want a 422 naming status", err)
	}
}

// The pack keeps ids, never names. Anonymising the company or the owner
// reaches the pack, because it reads their names at each open.
func TestThePackHoldsNoNameAnErasureCouldMiss(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, _ := gatedDeal(t, e)
	company := ids.NewV7()
	if _, err := e.owner.Exec(t.Context(), `
		INSERT INTO company (id, display_name, source, captured_by, owner_id) VALUES ($1, 'Acme Holding', 'manual', 'human:fixture', $2)`,
		company, e.admin); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(t.Context(), `
		UPDATE deal SET company_id = $2, owner_id = $3, name = 'Rollout for Acme Holding' WHERE id = $1`,
		dealID, company, e.admin); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(t.Context(), `UPDATE app_user SET display_name = 'Rosa Rep' WHERE id = $1`, e.admin); err != nil {
		t.Fatal(err)
	}
	requester := humanWith(e.ws, e.admin, map[string]principal.ObjectGrant{
		"deal": {Read: true, Update: true}, "offer": {Read: true}, "company": {Read: true},
	})
	review, err := e.store.RequestLegalReview(requester, dealID, RequestLegalReviewInput{})
	if err != nil {
		t.Fatalf("requesting: %v", err)
	}
	var stored string
	if err := e.owner.QueryRow(t.Context(), `SELECT packet::text FROM legal_review WHERE id = $1`, review.Id).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Acme Holding", "Rosa Rep"} {
		if strings.Contains(stored, name) {
			t.Fatalf("the stored pack holds the name %q: %s", name, stored)
		}
	}

	if _, err := e.owner.Exec(t.Context(), `UPDATE company SET display_name = 'Erased company' WHERE id = $1`, company); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(t.Context(), `UPDATE app_user SET display_name = 'Former member' WHERE id = $1`, e.admin); err != nil {
		t.Fatal(err)
	}
	got, err := e.store.GetLegalReview(humanWith(e.ws, e.reviewer, map[string]principal.ObjectGrant{
		legalReviewEntity: {Read: true}, "deal": {Read: true}, "company": {Read: true},
	}), ids.UUID(review.Id))
	if err != nil {
		t.Fatalf("reading the review: %v", err)
	}
	deal := got.Packet.Deal
	if deal.CompanyName == nil || *deal.CompanyName != "Erased company" || deal.OwnerName == nil || *deal.OwnerName != "Former member" {
		t.Fatalf("after the sources were anonymised the pack showed company %v and owner %v", deal.CompanyName, deal.OwnerName)
	}
}
