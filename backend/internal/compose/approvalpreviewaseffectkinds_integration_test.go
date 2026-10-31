// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// stageCall stages one call as proposer: the agent through its gate, or a
// member in person, whose proposal carries no passport.
func (s *promotionSight) stageCall(proposer context.Context, in approvals.StageInput, payload string) ids.ApprovalID {
	s.t.Helper()
	change, hash, err := diffhash.Canonical([]byte(payload))
	if err != nil {
		s.t.Fatal(err)
	}
	in.ProposedChange, in.DiffHash = change, hash
	if proposer != s.agent {
		id, err := s.svc.Stage(proposer, in)
		if err != nil {
			s.t.Fatal(err)
		}
		return id
	}
	id, _, err := s.svc.StageAgentCall(s.agent, in)
	if err != nil {
		s.t.Fatal(err)
	}
	return id
}

// privateCompany is a company only owner and an admin can open.
func (s *promotionSight) privateCompany(name string, owner ids.UUID) ids.UUID {
	company := s.company(name, owner, "")
	s.hide(company)
	return company
}

func (s *promotionSight) employ(contact, company ids.UUID) {
	s.t.Helper()
	_, err := s.e.Contacts.CreateRelationship(s.e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: new(ids.From[ids.ContactKind](contact)), CompanyID: new(ids.From[ids.CompanyKind](company)), Source: "manual",
	})
	if err != nil {
		s.t.Fatal(err)
	}
}

func (s *promotionSight) deal(name string, owner ids.UUID) ids.UUID {
	s.t.Helper()
	pipeline, stage, _ := integration.DealFixture(s.t, s.e)
	deal, err := deals.NewStore(s.e.DB(), DealsInstallation()).CreateDeal(s.e.Admin(), deals.CreateDealInput{
		Name: name, PipelineID: pipeline, StageID: stage, Source: "manual", OwnerID: new(ids.From[ids.UserKind](owner)),
	})
	if err != nil {
		s.t.Fatal(err)
	}
	return ids.UUID(deal.Id)
}

// newKindSight seats Rep1's agent, and Rep1 in person as the approver who
// opens every record the agent can. Rep2 is the colleague who cannot.
func newKindSight(t *testing.T) *promotionSight {
	s := newPromotionSight(t, "admin", approverIsRep2, "rep")
	s.approver = storedMemberSeat(t, s.e, s.e.Rep1)
	return s
}

// requireAgentsPlanIsShown holds that an approver who opens every record sees
// the plan, and that it names text.
func requireAgentsPlanIsShown(t *testing.T, preview crmcontracts.ApprovalPreview, text string) {
	t.Helper()
	if preview.State != previewReadyState {
		t.Fatalf("preview for an approver who opens everything = %s, want ready: %+v", preview.State, preview)
	}
	if !previewMentions(preview, text) {
		t.Fatalf("the agent's plan does not show %q: %+v", text, preview)
	}
}

func previewMentions(preview crmcontracts.ApprovalPreview, text string) bool {
	for _, change := range preview.Changes {
		if change.Before.Text == text || change.After.Text == text {
			return true
		}
	}
	if preview.Details != nil {
		for _, detail := range *preview.Details {
			if detail.Value.Text == text {
				return true
			}
		}
	}
	return false
}

// requireWaitsForWiderSight switches the approver to Rep2, who cannot open
// secret's record, and holds that the plan is hidden and cannot be decided.
func (s *promotionSight) requireWaitsForWiderSight(id ids.ApprovalID, secret string) {
	s.t.Helper()
	s.approver = storedMemberSeat(s.t, s.e, s.e.Rep2)
	requireShowsNothing(s.t, s.preview(id), secret)
	_, err := s.svc.Decide(s.approver, id, true, nil)
	requireRefusedForSight(s.t, err)
	if s.e.WsCount(s.t, `SELECT count(*) FROM approval WHERE id = $1 AND status = 'pending'`, id.UUID) != 1 {
		s.t.Fatal("a refused decision moved the approval")
	}
}

func TestAnAgentsRecordUpdateWaitsForAnApproverWhoCanOpenItsNewCompany(t *testing.T) {
	s := newKindSight(t)
	deal := s.deal("Harbor renewal", s.e.Rep2)
	company := s.privateCompany("Quiet Holdings", s.e.Rep1)
	id := s.stageCall(s.agent, approvals.StageInput{Kind: toolUpdateRecord, TargetType: "deal", TargetID: deal},
		fmt.Sprintf(`{"record_type":"deal","id":%q,"fields":{"company_id":%q}}`, deal, company))

	requireAgentsPlanIsShown(t, s.preview(id), "Quiet Holdings")
	s.requireWaitsForWiderSight(id, "Quiet Holdings")
}

// An admin approver reads the plan while the employer is open to everyone,
// and waits once it is private to Rep1.
func TestAnAgentsArchiveWaitsForAnApproverWhoCanOpenWhatItDetaches(t *testing.T) {
	s := newKindSight(t)
	s.e.GrantRole(t, s.e.AdminUser, "admin")
	s.approver = storedMemberSeat(t, s.e, s.e.AdminUser)
	contact := s.e.SeedContact(t, "Ada Archive", &s.e.Rep2)
	employer := s.company("Hidden Employer", s.e.Rep1, "")
	s.employ(contact, employer)
	id := s.stageCall(s.agent, approvals.StageInput{Kind: toolArchiveRecord, TargetType: "contact", TargetID: contact},
		fmt.Sprintf(`{"record_type":"contact","id":%q}`, contact))

	requireAgentsPlanIsShown(t, s.preview(id), "Hidden Employer")
	s.hide(employer)
	requireShowsNothing(t, s.preview(id), "Hidden Employer")
	_, err := s.svc.Decide(s.approver, id, true, nil)
	requireRefusedForSight(t, err)
}

func TestAnAgentsRelinkWaitsForAnApproverWhoCanOpenItsDestination(t *testing.T) {
	s := newKindSight(t)
	buyer := s.e.SeedContact(t, "Open Buyer", &s.e.Rep2)
	destination := s.e.SeedContact(t, "Private Destination", &s.e.Rep1)
	s.e.WsExec(t, `UPDATE contact SET visibility = 'owner' WHERE id = $1`, destination)
	note, _, err := s.e.Activities.LogActivity(s.e.Admin(), activities.LogActivityInput{
		Kind: "note", Subject: new("Buyer call notes"), Source: "manual", Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: buyer}},
	})
	if err != nil {
		t.Fatal(err)
	}
	activity := ids.UUID(note.Id)
	id := s.stageCall(s.agent, approvals.StageInput{Kind: singleRelinkTool, TargetType: "activity", TargetID: activity},
		fmt.Sprintf(`{"activity_id":%q,"entity_type":"contact","entity_id":%q}`, activity, destination))

	requireAgentsPlanIsShown(t, s.preview(id), "Private Destination")
	s.requireWaitsForWiderSight(id, "Private Destination")
}

func TestAnAgentsCompanyMailWaitsForAnApproverWhoCanOpenWhereItIsFiled(t *testing.T) {
	s := newKindSight(t)
	company := s.privateCompany("Discreet Account", s.e.Rep1)
	id := s.stageCall(s.agent, approvals.StageInput{Kind: previewSendCompanyEmail, TargetType: "activity"}, fmt.Sprintf(`{
		"to":["buyer@example.test"],"subject":"Delivery plan","body":"Delivery on Friday.",
		"communication_context":"requested_followup","consent_purpose":"transactional","operator_reason":"Buyer asked for the plan.",
		"links":[{"entity_type":"company","entity_id":%q}]}`, company))

	requireAgentsPlanIsShown(t, s.preview(id), "Discreet Account")
	s.requireWaitsForWiderSight(id, "Discreet Account")
}

func TestAnAgentsMergeWaitsForAnApproverWhoCanOpenWhatMoves(t *testing.T) {
	s := newKindSight(t)
	source := s.e.SeedContact(t, "Merge Source", &s.e.Rep2)
	survivor := s.e.SeedContact(t, "Merge Survivor", &s.e.Rep2)
	employer := s.company("Sealed Employer", s.e.Rep1, "")
	s.employ(source, employer)
	s.hide(employer)
	info, err := agents.StageSubject(s.agent, agents.NewMergeCall(NewProviderFor(s.e.DB()), nil, agents.MergeCommand{RecordType: "contact", SourceID: source, TargetID: survivor}))
	if err != nil {
		t.Fatal(err)
	}
	id := s.stageCall(s.agent, approvals.StageInput{
		Kind: toolMergeRecords, TargetType: info.TargetType, TargetID: info.TargetID, CoTargetType: info.CoTargetType, CoTargetID: info.CoTargetID,
	}, fmt.Sprintf(`{"record_type":"contact","source_id":%q,"target_id":%q}`, source, survivor))

	requireAgentsPlanIsShown(t, s.preview(id), "Sealed Employer")
	s.requireWaitsForWiderSight(id, "Sealed Employer")
}

// The approver opens a company the agent cannot. The agent's update could not
// run, so it is not shown as ready. The same update proposed by its member in
// person runs as its decider, and is.
func TestAnUpdateIsPreviewedAsWhoeverRunsIt(t *testing.T) {
	s := newPromotionSight(t, "rep", approverIsAdmin, "admin")
	deal := s.deal("Harbor expansion", s.e.Rep1)
	company := s.privateCompany("Admin Only Ltd", s.e.AdminUser)
	payload := fmt.Sprintf(`{"record_type":"deal","id":%q,"fields":{"company_id":%q}}`, deal, company)
	in := approvals.StageInput{Kind: toolUpdateRecord, TargetType: "deal", TargetID: deal}

	staged := s.stageCall(s.agent, in, payload)
	if state := s.preview(staged).State; state == previewReadyState {
		t.Fatal("an update the agent cannot run reads as ready")
	}
	if _, err := s.svc.Decide(s.approver, staged, false, nil); err != nil {
		t.Fatalf("declining the agent's update: %v", err)
	}
	proposed := s.stageCall(storedMemberSeat(t, s.e, s.e.Rep1), in, payload)
	requireAgentsPlanIsShown(t, s.preview(proposed), "Admin Only Ltd")
}
