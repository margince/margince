// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// An approval whose window has closed, though its stored status has not been
// written yet, is no work to decide. The count already leaves it out, so every
// list of work must too.
func TestAnExpiredButStoredPendingApprovalIsNoWorkOnAnySurface(t *testing.T) {
	e := integration.Setup(t)
	pipeline, open, _ := integration.DealFixture(t, e)
	svc := NewApprovalsService(e.DB())
	stage := func() ids.ApprovalID {
		deal := e.SeedDeal(t, "Review date", pipeline, open, &e.AdminUser)
		proposal, err := json.Marshal(deals.CloseDateCorrection{DealID: ids.From[ids.DealKind](deal), ExpectedCloseDate: "2026-12-01", Basis: "Review the date"})
		if err != nil {
			t.Fatal(err)
		}
		id, err := svc.Stage(e.Admin(), approvals.StageInput{Kind: deals.CloseDateCorrectionKind, TargetType: "deal", TargetID: deal, ProposedChange: proposal, DiffHash: deal.String()})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	live, lapsed := stage(), stage()
	e.WsExec(t, `UPDATE approval SET expires_at = now() - interval '1 day' WHERE id = $1`, lapsed)

	status := string(crmcontracts.ApprovalStatusPending)
	count, err := svc.CountPending(e.Admin(), approvals.ListInput{Status: &status, RequestedForReader: true})
	if err != nil || count.Count != 1 {
		t.Fatalf("pending count = %d, err %v, want 1", count.Count, err)
	}

	lane, err := (attentionApprovals{svc: svc}).ListWire(e.Admin(), attention.ApprovalQuery{Status: status, Limit: 9, RequestedForReader: true})
	if err != nil {
		t.Fatal(err)
	}
	magic, err := (magicPendingDecisions{svc: svc}).PendingApprovals(e.Admin(), 9)
	if err != nil {
		t.Fatal(err)
	}
	tool, err := (approvalInbox{svc: svc}).ListApprovals(e.Admin(), agents.ApprovalQuery{Limit: 9})
	if err != nil {
		t.Fatal(err)
	}
	surfaces := map[string][]ids.UUID{}
	for _, a := range lane {
		surfaces["Home"] = append(surfaces["Home"], ids.UUID(a.Id))
	}
	for _, a := range magic {
		surfaces["Magic"] = append(surfaces["Magic"], ids.UUID(a.Id))
	}
	for _, a := range tool.Approvals {
		surfaces["MCP"] = append(surfaces["MCP"], a.StagedActionID)
	}
	for name, listed := range surfaces {
		if len(listed) != 1 || listed[0] != live.UUID {
			t.Errorf("%s lists %v, want only the live approval %v, and never the expired %v", name, listed, live.UUID, lapsed.UUID)
		}
	}
	if len(surfaces) != 3 {
		t.Errorf("only %d of the three surfaces listed anything, so the check proves nothing for the others", len(surfaces))
	}
}
