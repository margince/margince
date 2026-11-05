// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestDealCreationApprovalMatchesTheWriter(t *testing.T) {
	for _, rest := range []bool{false, true} {
		t.Run(fmt.Sprintf("rest=%t", rest), func(t *testing.T) {
			e := integration.Setup(t)
			e.GrantRole(t, e.AdminUser, "admin")
			perms := integration.AdminPerms
			perms.Objects = maps.Clone(perms.Objects)
			perms.Objects["custom_field"] = principal.ObjectGrant{Read: true}
			ctx := e.As(e.AdminUser, nil, perms)
			store := deals.NewStore(e.DB(), DealsInstallation())
			pipeline, err := store.CreatePipeline(ctx, deals.CreatePipelineInput{Name: "Enterprise sales"})
			if err != nil {
				t.Fatal(err)
			}
			stage, err := store.CreateStage(ctx, deals.CreateStageInput{PipelineID: ids.From[ids.PipelineKind](ids.UUID(pipeline.Id)), Name: "Discovery", Semantic: "open"})
			if err != nil {
				t.Fatal(err)
			}
			fields := map[string]any{"name": "Buyer expansion", "source": "manual", "pipeline_id": pipeline.Id, "stage_id": stage.Id, "amount_minor": 250000, "expected_arr_minor": 120000, "currency": "EUR"}
			payload := map[string]any{"record_type": "deal", "fields": fields}
			if rest {
				payload = map[string]any{"operation": "createDeal", "body": fields}
			}
			encoded, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			raw, hash, err := diffhash.Canonical(encoded)
			if err != nil {
				t.Fatal(err)
			}
			svc := NewApprovalsService(e.DB())
			id, err := svc.Stage(ctx, approvals.StageInput{Kind: "create_record", TargetType: "deal", ProposedChange: raw, DiffHash: hash})
			if err != nil {
				t.Fatal(err)
			}
			proposal, err := svc.GetWire(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			preview := proposal.Preview
			if preview == nil || preview.State != "ready" || preview.Details == nil || preview.Effects == nil || !slices.Contains(*preview.Effects, "create_deal_record") {
				t.Fatalf("preview: %+v", preview)
			}
			details := map[string]crmcontracts.ApprovalPreviewValue{}
			for _, detail := range *preview.Details {
				details[detail.Field] = detail.Value
			}
			if details["pipeline_id"].Text != pipeline.Name || details["stage_id"].Text != stage.Name || details["name"].Text != "Buyer expansion" {
				t.Fatalf("missing named context: %+v", details)
			}
			for _, field := range []string{"amount_minor", "expected_arr_minor"} {
				value := details[field]
				if value.Kind != "money" || value.Currency == nil || *value.Currency != "EUR" {
					t.Fatalf("untyped money: %+v", value)
				}
			}
			if count := e.WsCount(t, `SELECT count(*) FROM deal`); count != 0 {
				t.Fatalf("preview wrote %d deals", count)
			}
			input, err := dealCreationRequest(proposal)
			if err != nil {
				t.Fatal(err)
			}
			actual, err := store.CreateDeal(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			if actual.Name != details["name"].Text || (actual.StageId == nil || *actual.StageId != stage.Id) || actual.AmountMinor == nil || *actual.AmountMinor != 250000 || actual.ExpectedArrMinor == nil || *actual.ExpectedArrMinor != 120000 {
				t.Fatalf("writer differs: %+v", actual)
			}
		})
	}
}
