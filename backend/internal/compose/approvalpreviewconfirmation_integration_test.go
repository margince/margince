// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

func TestAutomationConfirmationShowsTheQuestionAndOnlyItsActualEffect(t *testing.T) {
	e := integration.Setup(t)
	contact := e.SeedContact(t, "Review account", nil)
	svc := NewApprovalsService(e.DB())
	question := "Has the account review been completed?"
	reason := strings.Repeat("Review the agreed scope.\n", 30)
	raw, err := json.Marshal(automation.ConfirmationRequest{Question: question, Reason: reason})
	if err != nil {
		t.Fatal(err)
	}
	_, err = automation.ApplyActions(e.Admin(), automation.Executors{Approvals: automationApprovalsAdapter{svc: svc}}, workflow.Effect{Actions: []workflow.Action{{Kind: workflow.ActionEmitFlowEvent, Target: datasource.EntityRef{Type: datasource.EntityContact, ID: contact}, Args: raw}}})
	var staged *workflow.StagedApprovalError
	if !errors.As(err, &staged) {
		t.Fatalf("staging: %v", err)
	}
	proposal, err := svc.GetWire(e.Admin(), staged.ApprovalID)
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Preview == nil || proposal.Preview.State != "ready" || proposal.Preview.Details == nil || proposal.Preview.Effects == nil {
		t.Fatalf("missing confirmation: %+v", proposal.Preview)
	}
	if proposal.Summary == nil || !strings.Contains(*proposal.Summary, question) {
		t.Fatalf("generic summary: %v", proposal.Summary)
	}
	details := *proposal.Preview.Details
	if len(details) != 2 || details[0].Value.Text != question || details[1].Value.Text != reason {
		t.Fatalf("lost context: %+v", details)
	}
	effects := *proposal.Preview.Effects
	if len(effects) != 1 || effects[0] != "record_confirmation_only" {
		t.Fatalf("wrong effect: %v", effects)
	}
	for _, payload := range []string{`{}`, `{"to_stage_id":"destination"}`, `{"note":"Confirm the account review"}`} {
		canonical, hash, err := diffhash.Canonical([]byte(payload))
		if err != nil {
			t.Fatal(err)
		}
		id, err := svc.Stage(e.Admin(), approvals.StageInput{Kind: "emit_flow_event", TargetType: "contact", TargetID: contact, ProposedChange: canonical, DiffHash: hash})
		if err != nil {
			t.Fatal(err)
		}
		legacy, err := svc.GetWire(e.Admin(), id)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(payload, "note") {
			if legacy.Preview == nil || legacy.Preview.State != "ready" {
				t.Fatalf("legacy note lost: %+v", legacy.Preview)
			}
			continue
		}
		if legacy.Preview == nil || legacy.Preview.State != "incomplete" || legacy.Preview.Details != nil {
			t.Fatalf("ambiguous confirmation exposed: %+v", legacy.Preview)
		}
		if _, err := svc.Decide(e.Admin(), id, true, nil); err == nil {
			t.Fatal("ambiguous confirmation accepted")
		}
	}
}
