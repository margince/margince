// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

func TestOwnerPreviewAgreesAcrossRESTAndTheHumanEditToolStager(t *testing.T) {
	q := setupQueue(t)
	var me struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if status := q.Call(t, http.MethodGet, "/v1/me", nil, nil, &me); status != http.StatusOK {
		t.Fatalf("reader identity: %d", status)
	}
	token := q.mintPassport(t, "Owner preview agent", "read", "write")
	invoke := q.invoker(t, token)
	var previews []crmcontracts.ApprovalPreviewChange
	for _, door := range []string{"REST", "MCP"} {
		company := createdID(t, q.AppEnv, "/v1/companies", AnyMap{"display_name": door + " account", "source": "manual", "owner_id": me.User.ID})
		var approvalID string
		if door == "REST" {
			var problem struct {
				Detail string `json:"detail"`
			}
			if status := q.Call(t, http.MethodPatch, "/v1/companies/"+company, AnyMap{"owner_id": nil}, map[string]string{"Authorization": "Bearer " + token}, &problem); status != http.StatusForbidden {
				t.Fatalf("REST owner staging: %d", status)
			}
			approvalID = ExtractStagedApprovalID(t, problem.Detail)
		} else {
			raw, err := json.Marshal(AnyMap{"record_type": "company", "id": company, "fields": AnyMap{"owner_id": nil}})
			if err != nil {
				t.Fatal(err)
			}
			_, err = invoke("update_record", string(raw))
			var staged *workflow.StagedApprovalError
			if !errors.As(err, &staged) {
				t.Fatalf("MCP owner staging: %v", err)
			}
			approvalID = staged.ApprovalID.String()
		}
		changes := readCommercialPreview(t, q.AppEnv, approvalID)
		if len(changes) != 1 {
			t.Fatalf("%s changes=%+v", door, changes)
		}
		change := changes[0]
		if change.Field != "owner_id" || change.Before.Kind != "owner" || change.Before.Text == "" || change.After.Kind != "empty" {
			t.Fatalf("%s owner change: %+v", door, change)
		}
		if previews != nil && !reflect.DeepEqual(previews, changes) {
			t.Fatalf("doors disagree: %+v / %+v", previews, changes)
		}
		previews = changes
	}
}

func readCommercialPreview(t *testing.T, e *apptest.AppEnv, id string) []crmcontracts.ApprovalPreviewChange {
	t.Helper()
	var approval crmcontracts.Approval
	if status := e.Call(t, http.MethodGet, "/v1/approvals/"+id, nil, nil, &approval); status != http.StatusOK {
		t.Fatalf("preview read: %d", status)
	}
	if approval.Preview == nil || approval.Preview.State != "ready" || approval.TargetLabel == nil || *approval.TargetLabel == "" {
		t.Fatalf("approval=%+v", approval)
	}
	return approval.Preview.Changes
}

func TestChangedDestinationStageInvalidatesTheDecisionAndItsRedemption(t *testing.T) {
	for _, approved := range []bool{false, true} {
		name := "pending"
		if approved {
			name = "approved"
		}
		t.Run(name, func(t *testing.T) {
			d := newTwoDoors(t, "stage-pin", "read", "write")
			stages := apptest.DiscoverSeededPipeline(t, d.e)
			deal := openDeal(t, d.e, stages, "Stage-bound renewal")
			win := AnyMap{"to_stage_id": stages.Won, "won_without_contract_reason": "verbal"}
			var refusal fileRefusal
			if status := d.e.Call(t, "POST", "/v1/deals/"+deal+"/progress", win, d.bearer, &refusal); status != http.StatusForbidden {
				t.Fatalf("stage move: %d", status)
			}
			approvalID := ExtractStagedApprovalID(t, refusal.Detail)
			readCommercialPreview(t, d.e, approvalID)
			if approved {
				mustCall(t, d.e, "POST", "/v1/approvals/"+approvalID+"/approve", AnyMap{}, http.StatusOK, nil)
			}
			mustCall(t, d.e, "PATCH", "/v1/stages/"+stages.Won, AnyMap{"name": "Won after legal review"}, http.StatusOK, nil)
			var proposal crmcontracts.Approval
			mustCall(t, d.e, "GET", "/v1/approvals/"+approvalID, nil, http.StatusOK, &proposal)
			if !approved && (proposal.Preview == nil || proposal.Preview.State != "stale" || len(proposal.Preview.Changes) != 0) {
				t.Fatalf("changed stage preview: %+v", proposal.Preview)
			}
			if approved {
				if proposal.Preview != nil {
					t.Fatalf("decided proposal has a fresh decision preview: %+v", proposal.Preview)
				}
				headers := map[string]string{"Authorization": d.bearer["Authorization"], "X-Approval-Token": approvalID}
				if status := d.e.Call(t, "POST", "/v1/deals/"+deal+"/progress", win, headers, nil); status != http.StatusConflict {
					t.Fatalf("stale redemption: %d", status)
				}
			} else {
				mustCall(t, d.e, "POST", "/v1/approvals/"+approvalID+"/approve", AnyMap{}, http.StatusConflict, nil)
			}
			if current := dealStage(t, d.e, deal); current != stages.Open {
				t.Fatalf("stale decision moved deal to %s", current)
			}
		})
	}
}
