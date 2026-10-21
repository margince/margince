// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestStageDecisionCarriesAuthorizedCommercialContext(t *testing.T) {
	e := integration.Setup(t)
	pipeline, open, won := integration.DealFixture(t, e)
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Harbor account", &e.Rep1))
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Harbor renewal", pipeline, open, &e.Rep1))
	if _, err := e.Deals.UpdateDeal(e.Admin(), deal, deals.UpdateDealInput{CompanyID: &company, AmountMinor: new(int64(12500000)), Currency: new("EUR")}); err != nil {
		t.Fatal(err)
	}
	raw, hash, err := diffhash.Canonical([]byte(fmt.Sprintf(`{"deal_id":%q,"to_stage_id":%q,"won_without_contract_reason":"verbal"}`, deal, won)))
	if err != nil {
		t.Fatal(err)
	}
	svc := NewApprovalsService(e.DB())
	id, err := svc.Stage(e.Admin(), approvals.StageInput{Kind: previewAdvanceDeal, TargetType: "deal", TargetID: deal.UUID, CoTargetType: "stage", CoTargetID: won.UUID, ProposedChange: raw, DiffHash: hash})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := svc.GetWire(e.Admin(), id)
	if err != nil {
		t.Fatal(err)
	}
	details := decisionDetails(t, proposal.Preview)
	if details["company_id"].Text != "Harbor account" || details["owner_id"].Text == "" || details["owner_id"].EntityId == nil || ids.UUID(*details["owner_id"].EntityId) != e.Rep1 {
		t.Fatalf("responsibility context=%+v", details)
	}
	amount := details["amount_minor"]
	if amount.Kind != "money" || amount.Text != "12500000" || amount.Currency == nil || *amount.Currency != "EUR" {
		t.Fatalf("amount=%+v", amount)
	}
	perms := principal.Permissions{RoleKeys: []string{"rep"}, RowScope: principal.RowScopeTeam, Objects: map[string]principal.ObjectGrant{"deal": {Read: true}, "pipeline": {Read: true}, "installation_settings": {Read: true}}}
	perms.FieldMasks = []principal.FieldMask{{Object: "deal", Field: "amount_minor", Condition: principal.MaskAlways}}
	masked, err := newApprovalPreviewReader(e.DB()).read(e.As(e.Rep1, []ids.UUID{e.Team1}, perms), proposal)
	if err != nil {
		t.Fatal(err)
	}
	if masked == nil || masked.State != previewIncompleteState || masked.Details != nil || len(masked.Changes) != 0 {
		t.Fatalf("masked closure must withhold the decision: %+v", masked)
	}
	perms.FieldMasks = nil
	limited, err := newApprovalPreviewReader(e.DB()).read(e.As(e.Rep1, []ids.UUID{e.Team1}, perms), proposal)
	if err != nil {
		t.Fatal(err)
	}
	visible := decisionDetails(t, limited)
	if _, exposed := visible["company_id"]; exposed {
		t.Fatalf("unreadable company context leaked: %+v", visible)
	}
	if visible["owner_id"].Text == "" || visible["currency"].Text != "EUR" || visible["amount_minor"].Text != "12500000" {
		t.Fatalf("readable context lost: %+v", visible)
	}
	if _, err := e.Deals.UpdateDeal(e.Admin(), deal, deals.UpdateDealInput{AmountMinor: new(int64(14000000))}); err != nil {
		t.Fatal(err)
	}
	stale, err := svc.GetWire(e.Admin(), id)
	if err != nil || stale.Preview == nil || stale.Preview.State != previewStaleState || stale.Preview.Details != nil {
		t.Fatalf("stale context=%+v, %v", stale.Preview, err)
	}
}

func decisionDetails(t *testing.T, preview *crmcontracts.ApprovalPreview) map[string]crmcontracts.ApprovalPreviewValue {
	t.Helper()
	if preview == nil || preview.State != previewReadyState || preview.Version != 2 || preview.Details == nil {
		t.Fatalf("preview=%+v", preview)
	}
	out := map[string]crmcontracts.ApprovalPreviewValue{}
	for _, detail := range *preview.Details {
		out[detail.Field] = detail.Value
	}
	return out
}
