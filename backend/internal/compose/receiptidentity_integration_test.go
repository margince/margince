// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestNativeStageReceiptHasTheSameIdentityOnBothHomeReaders(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	now := clocktest.Now(t)
	clock := func() time.Time { return now }
	pipeline, open, _ := integration.DealFixture(t, e)
	owner := ids.From[ids.UserKind](e.AdminUser)
	deal, err := e.Deals.CreateDeal(ctx, deals.CreateDealInput{Name: "Agreed project", PipelineID: pipeline, StageID: open, OwnerID: &owner, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	dealID := ids.From[ids.DealKind](ids.UUID(deal.Id))
	if _, err := e.Deals.CreateStageExitCriterion(ctx, deals.CreateCriterionInput{StageID: open, Key: "signed", Label: "Agreement signed", Kind: string(deals.CriterionDocumentSigned)}); err != nil {
		t.Fatal(err)
	}
	company := e.SeedCompany(t, "Agreement buyer", &e.AdminUser)
	contract, err := e.Contracts.CreateContract(ctx, contracts.CreateContractInput{CompanyID: ids.From[ids.CompanyKind](company), Title: "Agreed project", ValueBasis: contracts.BasisTotal, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if count, err := e.Deals.RecordDeterministicEvidence(ctx, deals.DeterministicClaim{DealID: dealID, Kind: deals.CriterionDocumentSigned, SourceType: "contract", SourceID: ids.UUID(contract.Id), AuthorSide: deals.AuthorBuyer, ObservedAt: now.Add(-time.Hour)}); err != nil || count == 0 {
		t.Fatalf("evidence: count=%d err=%v", count, err)
	}
	svc := StageProgressionDecisions(e.Pool)
	proposer := NewStageProgressionProposer(e.Pool, e.Deals, svc, clock, slog.New(slog.DiscardHandler))
	if staged, err := proposer.Propose(ctx, dealID); err != nil || !staged {
		t.Fatalf("proposal: staged=%t err=%v", staged, err)
	}
	rows, _, err := svc.List(ctx, approvals.ListInput{Status: new("pending"), Kind: new(deals.StageProgressionKind), TargetID: &dealID.UUID})
	if err != nil || len(rows) != 1 {
		t.Fatalf("pending: count=%d err=%v", len(rows), err)
	}
	approvalID := ids.From[ids.ApprovalKind](rows[0].ID.UUID)
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("fixture has no owner")
	}
	actor.Type, actor.ID = principal.PrincipalAgent, "agent:receipt-review"
	actor.OnBehalfOf = e.AdminUser
	actor.Scopes = principal.NewScopeSet(principal.ScopeRead, principal.ScopeWrite)
	agent := principal.WithActor(ctx, actor)
	if _, err := svc.ApplyUnderPolicy(agent, approvalID); err != nil {
		t.Fatal(err)
	}
	handled, err := newAttentionService(e.Pool, svc, clock).HandledForYou(ctx)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := newMagicService(e.Pool, svc, clock).Read(ctx, nil, 100)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range handled.Receipts {
		if ids.UUID(r.Id) != approvalID.UUID {
			continue
		}
		for _, line := range receipt.Done {
			if line.ReceiptIds != nil && slices.Contains(*line.ReceiptIds, r.Id) {
				if line.Id == r.Id {
					t.Fatal("audit identity was replaced by the approval identity")
				}
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("no shared receipt identity: handled=%+v done=%+v", handled.Receipts, receipt.Done)
	}
	// One population: every receipt the worklist reads stands behind exactly
	// one line of the done lane.
	for _, r := range handled.Receipts {
		standing := 0
		for _, line := range receipt.Done {
			if line.ReceiptIds != nil && slices.Contains(*line.ReceiptIds, r.Id) {
				standing++
			}
		}
		if standing != 1 {
			t.Errorf("receipt %s (%s) stands behind %d done lines, want exactly 1", r.Id, r.Kind, standing)
		}
	}
}
