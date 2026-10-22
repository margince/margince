// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestCommercialPreviewReadsThePinnedRecordAndRefusesStaleBeforeValues(t *testing.T) {
	e := integration.Setup(t)
	actor, ok := principal.Actor(e.Admin())
	if !ok {
		t.Fatal("missing fixture actor")
	}
	owner := ids.From[ids.UserKind](actor.UserID)
	company, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: "Harbor", Source: "manual", OwnerID: &owner})
	if err != nil {
		t.Fatal(err)
	}
	raw, hash, err := diffhash.Canonical([]byte(fmt.Sprintf(`{"record_type":"company","id":%q,"fields":{"owner_id":null}}`, company.Id)))
	if err != nil {
		t.Fatal(err)
	}
	svc := NewApprovalsService(e.DB())
	id, err := svc.Stage(e.Admin(), approvals.StageInput{Kind: "update_record", TargetType: "company", TargetID: ids.UUID(company.Id), ProposedChange: raw, DiffHash: hash})
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.GetWire(e.Admin(), id)
	if err != nil {
		t.Fatal(err)
	}
	if result.Preview == nil || result.Preview.State != "ready" || len(result.Preview.Changes) != 1 {
		t.Fatalf("preview=%+v", result.Preview)
	}
	change := result.Preview.Changes[0]
	if change.Field != "owner_id" || change.Before.Kind != "owner" || change.Before.Text == "" || change.After.Kind != "empty" {
		t.Fatalf("change=%+v", change)
	}
	rows, _, err := svc.ListWire(e.Admin(), approvals.ListInput{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range rows {
		if row.Id == result.Id {
			found = row.Preview != nil && row.Preview.State == "ready"
		}
	}
	if !found {
		t.Fatal("list did not carry the ready preview")
	}
	name := "Harbor renamed"
	if _, err := e.Contacts.UpdateCompany(e.Admin(), ids.From[ids.CompanyKind](ids.UUID(company.Id)), contacts.UpdateCompanyInput{DisplayName: &name}); err != nil {
		t.Fatal(err)
	}
	result, err = svc.GetWire(e.Admin(), id)
	if err != nil {
		t.Fatal(err)
	}
	if result.Preview == nil || result.Preview.State != "stale" || len(result.Preview.Changes) != 0 {
		t.Fatalf("stale preview=%+v", result.Preview)
	}
	if _, err := svc.Decide(e.Admin(), id, true, nil); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("approving stale preview: %v", err)
	}
	rejected, err := svc.DecideWire(e.Admin(), id, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Status != "rejected" || rejected.Preview != nil {
		t.Fatalf("rejected decision still asks for a new proposal: %+v", rejected)
	}
}

func TestCommercialPreviewDoesNotCallWithheldMoneyEmpty(t *testing.T) {
	e := integration.Setup(t)
	pipeline, open, _ := integration.DealFixture(t, e)
	id := e.SeedDeal(t, "Harbor renewal", pipeline, open, &e.Rep3)
	amount, currency := int64(250000), "EUR"
	if _, err := e.Deals.UpdateDeal(e.Admin(), ids.From[ids.DealKind](id), deals.UpdateDealInput{AmountMinor: &amount, Currency: &currency}); err != nil {
		t.Fatal(err)
	}
	raw, hash, err := diffhash.Canonical([]byte(fmt.Sprintf(`{"record_type":"deal","id":%q,"fields":{"owner_id":null,"amount_minor":300000,"currency":"JPY"}}`, id)))
	if err != nil {
		t.Fatal(err)
	}
	svc := NewApprovalsService(e.DB())
	approval, err := svc.Stage(e.Admin(), approvals.StageInput{Kind: "update_record", TargetType: "deal", TargetID: id, ProposedChange: raw, DiffHash: hash})
	if err != nil {
		t.Fatal(err)
	}
	staged, err := svc.GetWire(e.Admin(), approval)
	if err != nil {
		t.Fatal(err)
	}
	if staged.Preview == nil || staged.Preview.State != "ready" {
		t.Fatalf("priced preview=%+v", staged.Preview)
	}
	money := false
	for _, change := range staged.Preview.Changes {
		if change.Field != "amount_minor" {
			continue
		}
		money = change.Before.Kind == "money" && change.After.Kind == "money" && change.Before.Currency != nil && *change.Before.Currency == "EUR" && change.After.Currency != nil && *change.After.Currency == "JPY"
	}
	if !money {
		t.Fatalf("currency change was not explained: %+v", staged.Preview)
	}

	if staged.Preview == nil || staged.Preview.State != "ready" {
		t.Fatalf("admin preview=%+v", staged.Preview)
	}
	perms := principal.Permissions{RoleKeys: []string{"rep"}, RowScope: principal.RowScopeTeam}
	perms.Objects = map[string]principal.ObjectGrant{"deal": {Read: true}, "pipeline": {Read: true}}
	perms.FieldMasks = []principal.FieldMask{{Object: "deal", Field: "amount_minor", Condition: principal.MaskAlways}}
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, perms)
	preview, err := newApprovalPreviewReader(e.DB()).read(reader, staged)
	if err != nil {
		t.Fatal(err)
	}
	if preview == nil || preview.State != "incomplete" || len(preview.Changes) != 0 {
		t.Fatalf("withheld preview=%+v", preview)
	}
}

func TestDealProgressPreviewNamesBothStagesAndTheSeparateNote(t *testing.T) {
	e := integration.Setup(t)
	pipeline, open, won := integration.DealFixture(t, e)
	deal := e.SeedDeal(t, "Harbor renewal", pipeline, open, &e.Rep1)
	raw, hash, err := diffhash.Canonical([]byte(fmt.Sprintf(`{"deal_id":%q,"to_stage_id":%q,"note":"Record the buyer confirmation","won_without_contract_reason":"verbal"}`, deal, won)))
	if err != nil {
		t.Fatal(err)
	}
	svc := NewApprovalsService(e.DB())
	id, err := svc.Stage(e.Admin(), approvals.StageInput{Kind: "progress_deal", TargetType: "deal", TargetID: deal, CoTargetType: "stage", CoTargetID: won.UUID, ProposedChange: raw, DiffHash: hash})
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.GetWire(e.Admin(), id)
	if err != nil {
		t.Fatal(err)
	}
	unchanged, err := e.Deals.GetDeal(e.Admin(), ids.From[ids.DealKind](deal), storekit.LiveOnly)
	if err != nil || unchanged.StageId == nil || ids.UUID(*unchanged.StageId) != open.UUID || unchanged.ClosedAt != nil {
		t.Fatalf("preview changed deal: %+v, %v", unchanged, err)
	}
	if result.TargetLabel == nil || *result.TargetLabel != "Harbor renewal" {
		t.Fatalf("target=%+v", result.TargetLabel)
	}
	if result.Preview == nil || result.Preview.State != "ready" || len(result.Preview.Changes) != 5 {
		t.Fatalf("preview=%+v", result.Preview)
	}
	for _, change := range result.Preview.Changes {
		switch change.Field {
		case "closed_at":
			if change.Before.Kind != "empty" || change.After.Kind != "execution" {
				t.Fatalf("close time=%+v", change)
			}
		case "won_without_contract_detail":
			if change.Before.Kind != "empty" || change.After.Kind != "empty" {
				t.Fatalf("win detail=%+v", change)
			}
		case "status":
			if change.Before.Text != "open" || change.After.Text != "won" {
				t.Fatalf("status=%+v", change)
			}
		case "stage_id":
			if change.Before.Kind != "stage" || change.After.Kind != "stage" || change.Before.Text == "" || change.After.Text == "" || change.Before.Text == change.After.Text {
				t.Fatalf("stage=%+v", change)
			}
		case "won_without_contract_reason":
			if change.Before.Kind != "empty" || change.After.Text != "verbal" {
				t.Fatalf("win reason=%+v", change)
			}
		case "note":
			if change.Before.Kind != "empty" || change.After.Text != "Record the buyer confirmation" {
				t.Fatalf("note=%+v", change)
			}
		default:
			t.Fatalf("unexpected effect=%+v", change)
		}
	}
}

func TestAutomationOwnerStagingUsesTheSharedPreviewReader(t *testing.T) {
	e := integration.Setup(t)
	seats := identity.NewServiceFor(e.DB())
	if _, err := seats.SaveMyDisplayName(e.Admin(), "Ada"); err != nil {
		t.Fatal(err)
	}
	if _, err := seats.SaveMyDisplayName(e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms), "Ben"); err != nil {
		t.Fatal(err)
	}

	company := e.SeedCompany(t, "Harbor", &e.AdminUser)
	raw, hash, err := diffhash.Canonical([]byte(fmt.Sprintf(`{"owner_id":%q}`, e.Rep1)))
	if err != nil {
		t.Fatal(err)
	}
	svc := NewApprovalsService(e.DB())
	id, err := (automationApprovalsAdapter{svc: svc}).Stage(e.Admin(), automation.StageRequest{Kind: "assign_owner", TargetType: "company", TargetID: company, ProposedChange: raw, DiffHash: hash})
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.GetWire(e.Admin(), id)
	if err != nil {
		t.Fatal(err)
	}
	if result.Preview == nil || result.Preview.State != "ready" || len(result.Preview.Changes) != 1 {
		t.Fatalf("preview=%+v", result.Preview)
	}
	change := result.Preview.Changes[0]
	if change.Field != "owner_id" || change.Before.Kind != "owner" || change.After.Kind != "owner" || change.Before.Text == "" || change.After.Text == "" || change.Before.Text != "Ada" || change.After.Text != "Ben" {
		t.Fatalf("owner preview=%+v", change)
	}
}

func TestOpenStagePreviewDoesNotRequireUnrelatedMaskedMoney(t *testing.T) {
	e := integration.Setup(t)
	pipeline, open, won := integration.DealFixture(t, e)
	stages, err := e.Deals.DefaultPipeline(e.Admin())
	if err != nil {
		t.Fatal(err)
	}
	var next ids.StageID
	for _, stage := range *stages.Stages {
		if stage.Semantic == "open" && ids.UUID(stage.Id) != open.UUID {
			next = ids.From[ids.StageKind](ids.UUID(stage.Id))
			break
		}
	}
	if next.IsZero() {
		t.Fatal("fixture has no second open stage")
	}
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Harbor", pipeline, open, &e.Rep3))
	if _, err := e.Deals.UpdateDeal(e.Admin(), deal, deals.UpdateDealInput{AmountMinor: new(int64(250000)), Currency: new("EUR")}); err != nil {
		t.Fatal(err)
	}
	perms := principal.Permissions{
		RoleKeys: []string{"rep"}, RowScope: principal.RowScopeAll,
		Objects:    map[string]principal.ObjectGrant{"deal": {Read: true}, "pipeline": {Read: true}},
		FieldMasks: []principal.FieldMask{{Object: "deal", Field: "amount_minor", Condition: principal.MaskAlways}},
	}
	ctx := e.As(e.Rep1, nil, perms)
	preview, err := e.Deals.PreviewAdvanceDeal(ctx, deal, deals.AdvanceDealInput{ToStageID: next})
	if err != nil || len(preview.After) == 0 {
		t.Fatalf("open move: %+v, %v", preview, err)
	}
	if _, err := e.Deals.PreviewAdvanceDeal(ctx, deal, deals.AdvanceDealInput{ToStageID: won, WonWithoutContractReason: new("verbal")}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("masked close: %v", err)
	}
}
