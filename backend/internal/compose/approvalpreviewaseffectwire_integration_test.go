// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// requireWireWithholdsPlan holds that the approval itself, which every
// surface reads, carries none of a withheld plan.
func (s *promotionSight) requireWireWithholdsPlan(id ids.ApprovalID, secrets ...string) {
	s.t.Helper()
	approval, err := s.svc.GetWire(s.approver, id)
	if err != nil {
		s.t.Fatal(err)
	}
	if approval.ProposedChange != nil || approval.Summary != nil || approval.Evidence != nil {
		s.t.Fatalf("a withheld plan still travels on the approval: change=%v summary=%v evidence=%v", approval.ProposedChange, approval.Summary, approval.Evidence)
	}
	raw, err := json.Marshal(approval)
	if err != nil {
		s.t.Fatal(err)
	}
	for _, secret := range secrets {
		if strings.Contains(string(raw), secret) {
			s.t.Fatalf("the approval carries %q: %s", secret, raw)
		}
	}
}

// A contract the mail cites is evidence its approver opens like any record.
func TestAnAgentsMailCitingAContractIsReadyForAnApproverWhoCanOpenIt(t *testing.T) {
	s := newKindSight(t)
	company := s.company("Contract Account", s.e.Rep1, "")
	agreement, err := s.e.Contracts.CreateContract(s.e.Admin(), contracts.CreateContractInput{
		CompanyID: ids.From[ids.CompanyKind](company), Title: "Supply agreement", ValueBasis: contracts.BasisTotal, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	id := s.stageCall(s.agent, approvals.StageInput{Kind: previewSendCompanyEmail, TargetType: "activity"}, fmt.Sprintf(`{
		"to":["buyer@example.test"],"subject":"Supply plan","body":"Deliveries start Monday.",
		"communication_context":"requested_followup","consent_purpose":"transactional","operator_reason":"Buyer asked.",
		"evidence":{"contract_id":%q},"links":[{"entity_type":"company","entity_id":%q}]}`, agreement.Id, company))

	requireAgentsPlanIsShown(t, s.preview(id), "Supply agreement")
}

// The approver's role masks contact email, so the addresses a mail goes to
// show as unavailable while the rest of the plan stays readable.
func TestAnAgentsMailKeepsTheAddressesTheApproversRoleMasks(t *testing.T) {
	s := newKindSight(t)
	s.approver = storedMemberSeat(t, s.e, s.e.Rep2)
	viewer, _ := principal.Actor(s.approver)
	viewer.Permissions.FieldMasks = []principal.FieldMask{{Object: "contact", Field: "email", Condition: principal.MaskAlways}}
	s.approver = principal.WithActor(s.approver, viewer)
	company := s.company("Masked Account", s.e.Rep2, "")
	id := s.stageCall(s.agent, approvals.StageInput{Kind: previewSendCompanyEmail, TargetType: "activity"}, fmt.Sprintf(`{
		"to":["buyer@masked.test"],"subject":"Delivery plan","body":"Delivery on Friday.",
		"communication_context":"requested_followup","consent_purpose":"transactional","operator_reason":"Buyer asked.",
		"links":[{"entity_type":"company","entity_id":%q}]}`, company))

	preview := s.preview(id)
	requireAgentsPlanIsShown(t, preview, "Masked Account")
	if value := promotionDetailValue(t, preview, "to"); value.Kind != previewValueUnavailable || value.Text != "" {
		t.Fatalf("a masked address reaches the approver: %+v", value)
	}
	s.requireWireWithholdsPlan(id, "buyer@masked.test")
}

// Whatever the agent's build reads through the reader's names and records
// counts as touched, so a record named nowhere in the plan is still checked.
func TestAnAgentsBuildCountsEveryRecordItReads(t *testing.T) {
	s := newKindSight(t)
	hidden := s.privateCompany("Unnamed Holdings", s.e.Rep1)
	touched := &previewTouched{}
	agent := newApprovalPreviewReader(s.e.DB()).recording(touched)
	if _, err := agent.records.Read(s.agent, datasource.EntityRef{Type: datasource.EntityCompany, ID: hidden}); err != nil {
		t.Fatal(err)
	}
	if _, err := agent.referenceValue(s.agent, "company_id", hidden.String()); err != nil {
		t.Fatal(err)
	}
	if n := len(touched.byType[entityCompany]); n != 2 {
		t.Fatalf("the agent's reads counted %d times, want the record read and the name", n)
	}
	viewer := newApprovalPreviewReader(s.e.DB())
	rep2 := storedMemberSeat(t, s.e, s.e.Rep2)
	open, err := viewer.viewerOpensPreview(rep2, crmcontracts.Approval{}, &crmcontracts.ApprovalPreview{}, touched)
	if err != nil || open {
		t.Fatalf("a record only the agent read passes the sight check: open=%v err=%v", open, err)
	}
	open, err = viewer.viewerOpensPreview(storedMemberSeat(t, s.e, s.e.Rep1), crmcontracts.Approval{}, &crmcontracts.ApprovalPreview{}, touched)
	if err != nil || !open {
		t.Fatalf("the record's owner fails the sight check: open=%v err=%v", open, err)
	}
}
