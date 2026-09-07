// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The stage progression, driven the way production drives it.
//
// The module's own tests call the ledger writers directly with an approval id
// they minted themselves, which proves the writers' guards and NOTHING about
// the wiring. Three defects lived happily underneath a green suite of them:
// the proposer staged through an entry point that refuses the input it was
// given, nothing constructed the proposer at all, and no decision ever reached
// the ledger — so every card would have stood at `proposed` forever.
//
// So this file mints nothing. It writes evidence through the real writer,
// proposes through the real proposer, decides through the real approvals
// service, and reads the ledger back. Each of the three defects fails it.

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// stageProgressionClock is the instant the fixtures below are read at. Fixed,
// so a criterion's record-age window is a property of the fixture rather than
// of when the suite happens to run.
var stageProgressionClock = time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)

// proposeOnMetCriteria seeds a deal whose current stage asks for one thing,
// records the evidence that settles it, and runs the real proposer.
//
// Answers the deal and whether a card was staged.
func proposeOnMetCriteria(t *testing.T, e *Env) (ids.DealID, bool) {
	t.Helper()
	admin := e.Admin()
	pipeline, open, _ := DealFixture(t, e)
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Progression e2e", pipeline, open, &e.Rep1))

	// The stage has to ASK for something. A stage with no criteria settles
	// nothing — every check is a loop over the list, so an empty one would pass
	// them all vacuously.
	if _, err := e.Deals.CreateStageExitCriterion(admin, deals.CreateCriterionInput{
		StageID: open,
		Key:     "signed",
		Label:   "The agreement is signed",
		Kind:    string(deals.CriterionDocumentSigned),
	}); err != nil {
		t.Fatalf("configuring the stage's exit criterion: %v", err)
	}

	// Through the REAL writer. A hand-inserted row would test a shape the
	// product never writes.
	written, err := e.Deals.RecordDeterministicEvidence(admin, deals.DeterministicClaim{
		DealID:     deal,
		Kind:       deals.CriterionDocumentSigned,
		SourceType: "contract",
		SourceID:   ids.NewV7(),
		AuthorSide: deals.AuthorBuyer,
		ObservedAt: stageProgressionClock.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("recording the evidence: %v", err)
	}
	if written == 0 {
		t.Fatal("the evidence writer wrote NO rows, so the criterion below is unmet for a " +
			"fixture reason rather than a product one and every assertion here is vacuous")
	}

	proposer := compose.NewStageProgressionProposer(e.Pool, e.Deals,
		approvals.NewService(e.DB()),
		func() time.Time { return stageProgressionClock },
		slog.New(slog.DiscardHandler))
	staged, err := proposer.Propose(admin, deal)
	if err != nil {
		t.Fatalf("proposing the move: %v", err)
	}
	return deal, staged
}

// The context a BUS-DRIVEN caller has, which is the one the evidence lanes
// actually pass: an actor and a correlation id, and NO workspace, because an
// envelope carries no tenant.
//
// The proposer has to resolve the installation itself. Without this the
// deterministic lane's every proposal opens a transaction bound to the zero
// workspace and is refused — and the lane swallows the error, so the feature is
// inert on exactly the installations that have no model configured. An earlier
// version of this suite missed it by calling Propose with an admin context,
// which carries a workspace the real caller does not have.
func busDrivenCtx(t *testing.T) context.Context {
	t.Helper()
	return principal.WithCorrelationID(
		principal.WithActor(t.Context(), principal.Principal{
			Type: principal.PrincipalSystem, ID: "system:stage-evidence",
		}), ids.NewV7())
}

func TestAProposalFromTheBusResolvesItsOwnWorkspace(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	pipeline, open, _ := DealFixture(t, e)
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Bus-driven", pipeline, open, &e.Rep1))
	if _, err := e.Deals.CreateStageExitCriterion(admin, deals.CreateCriterionInput{
		StageID: open, Key: "signed", Label: "The agreement is signed",
		Kind: string(deals.CriterionDocumentSigned),
	}); err != nil {
		t.Fatalf("configuring the criterion: %v", err)
	}
	if _, err := e.Deals.RecordDeterministicEvidence(admin, deals.DeterministicClaim{
		DealID: deal, Kind: deals.CriterionDocumentSigned, SourceType: "contract",
		SourceID: ids.NewV7(), AuthorSide: deals.AuthorBuyer,
		ObservedAt: stageProgressionClock.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("recording the evidence: %v", err)
	}

	proposer := compose.NewStageProgressionProposer(e.Pool, e.Deals,
		approvals.NewService(e.DB()),
		func() time.Time { return stageProgressionClock },
		slog.New(slog.DiscardHandler))
	staged, err := proposer.Propose(busDrivenCtx(t), deal)
	if err != nil {
		t.Fatalf("a proposal from the bus errored: %v", err)
	}
	if !staged {
		t.Fatal("a proposal from the bus staged NOTHING while the same deal stages from an " +
			"admin context: the proposer is reading a workspace off a context that has none")
	}
}

// The whole chain, in one pass: evidence lands, a card appears, and the ledger
// carries the proposal before anybody has answered it.
//
// This is the test the three wiring defects each fail. It cannot pass while
// the proposer stages through an entry point that refuses its input.
func TestEvidenceLandingStagesACardAndOpensTheLedger(t *testing.T) {
	e := Setup(t)
	deal, staged := proposeOnMetCriteria(t, e)
	if !staged {
		t.Fatal("the evidence settled the stage's only criterion and NO card was staged")
	}

	// The card is really in the inbox — read from the approval table itself
	// rather than from what the proposer said it did.
	var approvalID ids.UUID
	if err := e.Pool.QueryRow(t.Context(), `
		SELECT id FROM approval
		 WHERE kind = $1 AND target_entity_id = $2 AND status = 'pending'`,
		deals.StageProgressionKind, deal).Scan(&approvalID); err != nil {
		t.Fatalf("reading the staged card: %v", err)
	}

	// And the ledger opened in the same act, so the report counts a proposal
	// that was really put to somebody.
	var outcome string
	if err := e.Pool.QueryRow(t.Context(),
		`SELECT outcome FROM stage_progression_outcome WHERE approval_id = $1`,
		approvalID).Scan(&outcome); err != nil {
		t.Fatalf("reading the ledger row: %v", err)
	}
	if outcome != deals.ProgressionProposed {
		t.Fatalf("a card nobody has answered reads as %q on the ledger", outcome)
	}
}

// A second reading whose evidence has MOVED supersedes the first card rather
// than adding one beside it. That is what the staged Identity buys, and it is
// the case the identity exists for: two proposals of the same move whose
// payloads differ.
//
// The payload has to differ, or this proves nothing. Two byte-identical
// proposals share a diff_hash, and the plain join already collapses those —
// a version of this test that re-proposed unchanged passed with the identity
// keyed on the REASON TEXT, which identifies no move at all.
func TestFreshEvidenceSupersedesTheCardItReplaces(t *testing.T) {
	e := Setup(t)
	deal, staged := proposeOnMetCriteria(t, e)
	if !staged {
		t.Fatal("the first proposal did not stage")
	}

	// A SECOND signature on the same deal: the criterion was already met, so
	// the decision is unchanged, but the card now cites two rows rather than
	// one and its payload is a different document.
	written, err := e.Deals.RecordDeterministicEvidence(e.Admin(), deals.DeterministicClaim{
		DealID:     deal,
		Kind:       deals.CriterionDocumentSigned,
		SourceType: "contract",
		SourceID:   ids.NewV7(),
		AuthorSide: deals.AuthorBuyer,
		ObservedAt: stageProgressionClock.Add(-30 * time.Minute),
	})
	if err != nil {
		t.Fatalf("recording the second claim: %v", err)
	}
	if written == 0 {
		t.Fatal("the second claim wrote no rows, so both proposals carry the SAME payload " +
			"and this test is back to the case the plain join already handles")
	}
	proposer := compose.NewStageProgressionProposer(e.Pool, e.Deals,
		approvals.NewService(e.DB()),
		func() time.Time { return stageProgressionClock },
		slog.New(slog.DiscardHandler))
	if _, err := proposer.Propose(e.Admin(), deal); err != nil {
		t.Fatalf("the second proposal errored: %v", err)
	}

	// The GUARD on this test's own premise, checked before the assertion it
	// guards. If both proposals hash the same the plain join collapsed them and
	// the identity was never asked — which is exactly the hole an earlier
	// version of this test fell into, passing while the identity was keyed on
	// the reason sentence.
	var hashes int
	if err := e.Pool.QueryRow(t.Context(), `
		SELECT count(DISTINCT diff_hash) FROM approval
		 WHERE kind = $1 AND target_entity_id = $2`,
		deals.StageProgressionKind, deal).Scan(&hashes); err != nil {
		t.Fatalf("counting the distinct payloads: %v", err)
	}
	if hashes < 2 {
		t.Fatalf("both proposals carried the SAME payload (%d distinct diff_hash), so the "+
			"plain join handled them and this test never exercised the identity", hashes)
	}

	var live int
	if err := e.Pool.QueryRow(t.Context(), `
		SELECT count(*) FROM approval
		 WHERE kind = $1 AND target_entity_id = $2 AND status = 'pending'`,
		deals.StageProgressionKind, deal).Scan(&live); err != nil {
		t.Fatalf("counting the live cards: %v", err)
	}
	if live != 1 {
		t.Fatalf("two readings of one deal left %d live cards in the inbox, want 1", live)
	}

	// And the SURVIVOR is the fresh one. Counting to one is not enough on its
	// own: superseding the new card with the old would also leave exactly one,
	// and a rep would then approve a move citing evidence the product had
	// already moved past.
	var liveHash, newestHash string
	if err := e.Pool.QueryRow(t.Context(), `
		SELECT diff_hash FROM approval
		 WHERE kind = $1 AND target_entity_id = $2 AND status = 'pending'`,
		deals.StageProgressionKind, deal).Scan(&liveHash); err != nil {
		t.Fatalf("reading the surviving card: %v", err)
	}
	if err := e.Pool.QueryRow(t.Context(), `
		SELECT diff_hash FROM approval
		 WHERE kind = $1 AND target_entity_id = $2
		 ORDER BY created_at DESC LIMIT 1`,
		deals.StageProgressionKind, deal).Scan(&newestHash); err != nil {
		t.Fatalf("reading the newest card: %v", err)
	}
	if liveHash != newestHash {
		t.Fatal("the card left standing is the STALE one: the fresh reading was superseded " +
			"by the proposal it should have replaced")
	}
}

// A decision reaches the ledger through the EVENT, not through a call the
// decide path makes. Nothing in the approvals module knows this ledger exists,
// so the consumer is what closes the row — and without it every proposal ever
// made stands at `proposed` and every rate the launch gate reads is computed
// over an empty set.
func TestApprovingACardClosesItsLedgerRow(t *testing.T) {
	e := Setup(t)
	deal, staged := proposeOnMetCriteria(t, e)
	if !staged {
		t.Fatal("the proposal did not stage")
	}
	admin := e.As(e.Rep1, []ids.UUID{e.Team1}, AdminPerms)

	var approvalID ids.ApprovalID
	if err := e.Pool.QueryRow(t.Context(), `
		SELECT id FROM approval
		 WHERE kind = $1 AND target_entity_id = $2 AND status = 'pending'`,
		deals.StageProgressionKind, deal).Scan(&approvalID); err != nil {
		t.Fatalf("reading the staged card: %v", err)
	}

	// The decision the product makes, through the same service the HTTP
	// surface decides on.
	svc := approvals.NewService(e.DB())
	if _, err := svc.Decide(admin, approvalID, false, nil); err != nil {
		t.Fatalf("rejecting the card: %v", err)
	}

	// Then the consumer, fed the event the decision wrote. Driving it by hand
	// rather than through Redis keeps this a test of the MAPPING and the
	// write; that the consumer is subscribed at all is held by
	// TestEveryEventConsumerIsSubscribed.
	deliverApprovalDecided(t, e, approvalID)

	var outcome string
	if err := e.Pool.QueryRow(t.Context(),
		`SELECT outcome FROM stage_progression_outcome WHERE approval_id = $1`,
		approvalID).Scan(&outcome); err != nil {
		t.Fatalf("reading the ledger row: %v", err)
	}
	if outcome != deals.ProgressionRejected {
		t.Fatalf("a rejected card reads as %q on the ledger, want %q",
			outcome, deals.ProgressionRejected)
	}
}

// The card cites the RECORD each claim was read from, not the ledger row that
// records the reading.
//
// A citation naming a deal_stage_evidence id under source_type "deal" resolves
// to nothing — no reader can open a deal by an evidence id — and a citation a
// reviewer cannot follow is worse than none, because it looks checkable.
func TestTheCardCitesSomethingAReviewerCanOpen(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	pipeline, open, _ := DealFixture(t, e)
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Citations", pipeline, open, &e.Rep1))
	if _, err := e.Deals.CreateStageExitCriterion(admin, deals.CreateCriterionInput{
		StageID: open, Key: "signed", Label: "The agreement is signed",
		Kind: string(deals.CriterionDocumentSigned),
	}); err != nil {
		t.Fatalf("configuring the criterion: %v", err)
	}
	contractID := ids.NewV7()
	if _, err := e.Deals.RecordDeterministicEvidence(admin, deals.DeterministicClaim{
		DealID: deal, Kind: deals.CriterionDocumentSigned, SourceType: "contract",
		SourceID: contractID, AuthorSide: deals.AuthorBuyer,
		ObservedAt: stageProgressionClock.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("recording the evidence: %v", err)
	}
	proposer := compose.NewStageProgressionProposer(e.Pool, e.Deals,
		approvals.NewService(e.DB()),
		func() time.Time { return stageProgressionClock },
		slog.New(slog.DiscardHandler))
	if staged, err := proposer.Propose(admin, deal); err != nil || !staged {
		t.Fatalf("proposing: staged=%v err=%v", staged, err)
	}

	var kind string
	var source ids.UUID
	if err := e.Pool.QueryRow(t.Context(), `
		SELECT ev->>'source_type', (ev->>'source_id')::uuid
		  FROM approval a, jsonb_array_elements(a.evidence) AS ev
		 WHERE a.kind = $1 AND a.target_entity_id = $2`,
		deals.StageProgressionKind, deal).Scan(&kind, &source); err != nil {
		t.Fatalf("reading the card's citation: %v", err)
	}
	if kind != "contract" {
		t.Fatalf("the card cites source_type %q, want the record the claim was read "+
			"from (contract)", kind)
	}
	if source != contractID {
		t.Fatalf("the card cites %s, want the contract %s the claim was read from — "+
			"an evidence-row id under this type resolves to nothing", source, contractID)
	}
}

// A superseded card does not sit on the ledger forever.
//
// Staging expires the stale approval WITHOUT emitting approval.decided, so the
// outcome consumer never hears about it. Left alone the row stands at
// `proposed` for good, and every rate the launch gate reads is computed over a
// denominator that only grows.
func TestASupersededCardIsClosedOnTheLedger(t *testing.T) {
	e := Setup(t)
	deal, staged := proposeOnMetCriteria(t, e)
	if !staged {
		t.Fatal("the first proposal did not stage")
	}
	if _, err := e.Deals.RecordDeterministicEvidence(e.Admin(), deals.DeterministicClaim{
		DealID: deal, Kind: deals.CriterionDocumentSigned, SourceType: "contract",
		SourceID: ids.NewV7(), AuthorSide: deals.AuthorBuyer,
		ObservedAt: stageProgressionClock.Add(-30 * time.Minute),
	}); err != nil {
		t.Fatalf("recording the second claim: %v", err)
	}
	proposer := compose.NewStageProgressionProposer(e.Pool, e.Deals,
		approvals.NewService(e.DB()),
		func() time.Time { return stageProgressionClock },
		slog.New(slog.DiscardHandler))
	if _, err := proposer.Propose(e.Admin(), deal); err != nil {
		t.Fatalf("the second proposal errored: %v", err)
	}

	var open int
	if err := e.Pool.QueryRow(t.Context(), `
		SELECT count(*) FROM stage_progression_outcome
		 WHERE deal_id = $1 AND outcome = $2`,
		deal, deals.ProgressionProposed).Scan(&open); err != nil {
		t.Fatalf("counting the open ledger rows: %v", err)
	}
	if open != 1 {
		t.Fatalf("%d ledger rows still read `proposed` after a supersede, want 1 — a row "+
			"nothing can ever close inflates every rate the launch gate computes", open)
	}
	var superseded int
	if err := e.Pool.QueryRow(t.Context(), `
		SELECT count(*) FROM stage_progression_outcome
		 WHERE deal_id = $1 AND outcome = $2`,
		deal, deals.ProgressionSuperseded).Scan(&superseded); err != nil {
		t.Fatalf("counting the superseded rows: %v", err)
	}
	if superseded != 1 {
		t.Fatalf("the replaced card was closed as %d superseded rows, want 1", superseded)
	}
}

// An ACCEPTED card does not protect the deal against the next one.
//
// readProtection excludes a stage move carrying an approval_id, on the ground
// that accepting a proposal is agreeing with the product rather than overruling
// it. That exclusion is only real if the advance actually records the id — and
// it did not: the guard was written, commented, and inert, so every approved
// card would have silenced the following fortnight of proposals on that deal.
func TestAnAcceptedCardDoesNotProtectTheDealAgainstTheNextOne(t *testing.T) {
	e := Setup(t)
	deal, staged := proposeOnMetCriteria(t, e)
	if !staged {
		t.Fatal("the proposal did not stage")
	}
	admin := e.As(e.Rep1, []ids.UUID{e.Team1}, AdminPerms)

	var approvalID ids.ApprovalID
	if err := e.Pool.QueryRow(t.Context(), `
		SELECT id FROM approval
		 WHERE kind = $1 AND target_entity_id = $2 AND status = 'pending'`,
		deals.StageProgressionKind, deal).Scan(&approvalID); err != nil {
		t.Fatalf("reading the staged card: %v", err)
	}
	// The REAL registry, so approving here runs the effect a rep's click runs.
	if _, err := compose.StageProgressionDecisions(e.Pool).Decide(
		admin, approvalID, true, nil); err != nil {
		t.Fatalf("approving the card: %v", err)
	}

	// The move the approval caused names the card it came from.
	var carried *ids.UUID
	if err := e.Pool.QueryRow(t.Context(), `
		SELECT approval_id FROM deal_stage_history
		 WHERE deal_id = $1 AND from_stage_id IS NOT NULL
		 ORDER BY changed_at DESC LIMIT 1`, deal).Scan(&carried); err != nil {
		t.Fatalf("reading the stage history: %v", err)
	}
	if carried == nil {
		t.Fatal("the accepted move recorded NO approval_id, so readProtection reads it as a " +
			"rep overruling the product and the deal is protected for a fortnight")
	}
	if *carried != approvalID.UUID {
		t.Fatalf("the move names approval %s, want the card %s that caused it", carried, approvalID)
	}
}

// deliverApprovalDecided hands the consumer the approval.decided envelope the
// decision ACTUALLY wrote.
//
// Read from event_outbox rather than composed here. A hand-built envelope is a
// test supplying its own version of production: it would carry whatever fields
// this file thinks the payload has, and would go on passing after the decide
// path stopped writing one of them — which is precisely how a consumer keyed
// on `kind` could quietly match nothing.
func deliverApprovalDecided(t *testing.T, e *Env, approvalID ids.ApprovalID) {
	t.Helper()
	var raw []byte
	if err := e.Pool.QueryRow(t.Context(), `
		SELECT envelope FROM event_outbox
		 WHERE envelope->>'type' = 'approval.decided'
		   AND envelope->'entity'->>'id' = $1::text
		 ORDER BY created_at DESC LIMIT 1`, approvalID).Scan(&raw); err != nil {
		t.Fatalf("no approval.decided event was written for the decision: %v", err)
	}
	var env kevents.Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decoding the outbox envelope: %v", err)
	}
	consumer := compose.NewStageProgressionOutcome(
		e.Pool, e.Deals, identity.NewService(e.Pool), slog.New(slog.DiscardHandler))
	if err := consumer.HandleEvent(t.Context(), env); err != nil {
		t.Fatalf("the outcome consumer refused the decision it was given: %v", err)
	}
}
