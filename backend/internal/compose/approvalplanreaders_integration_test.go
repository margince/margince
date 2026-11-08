// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// panelApproval is approval id as the record panel lists it for reader.
func (s *promotionSight) panelApproval(reader context.Context, targetType string, target ids.UUID, id ids.ApprovalID) crmcontracts.Approval {
	s.t.Helper()
	var panel approvals.PendingOnTarget
	if err := s.e.DB().Tx(reader, func(tx pgx.Tx) (err error) {
		panel, err = s.svc.PendingForTarget(reader, tx, targetType, target)
		return err
	}); err != nil {
		s.t.Fatal(err)
	}
	for _, approval := range panel.Approvals {
		if ids.UUID(approval.Id) == id.UUID {
			return approval
		}
	}
	s.t.Fatalf("the record panel does not list the proposal for its reader: %+v", panel.Approvals)
	return crmcontracts.Approval{}
}

// The record panel answers the same plan check the inbox does. The rep who
// cannot open the company the agent's update names sees the proposal without it.
func TestARecordPanelKeepsBackAPlanItsReaderCannotOpen(t *testing.T) {
	s := newKindSight(t)
	deal := s.deal("Harbor renewal", s.e.Rep2)
	company := s.privateCompany("Quiet Holdings", s.e.Rep1)
	id := s.stageCall(s.agent, approvals.StageInput{Kind: toolUpdateRecord, TargetType: "deal", TargetID: deal},
		fmt.Sprintf(`{"record_type":"deal","id":%q,"fields":{"company_id":%q}}`, deal, company))

	requireCarriesNoPlan(t, s.panelApproval(s.rep2(), "deal", deal, id), company.String(), "Quiet Holdings")
	if shown := s.panelApproval(s.rep1(), "deal", deal, id); shown.ProposedChange == nil {
		t.Fatal("the record panel lost the plan for a reader who opens everything")
	}
}

// failedDecision is approval id on reader's own list of failed decisions.
func (s *promotionSight) failedDecision(reader context.Context, id ids.ApprovalID) approvals.FailedDecision {
	s.t.Helper()
	failed, err := s.svc.OwnFailedDecisions(reader, 20)
	if err != nil {
		s.t.Fatal(err)
	}
	for _, decision := range failed {
		if ids.UUID(decision.Approval.Id) == id.UUID {
			return decision
		}
	}
	s.t.Fatalf("the decider's failed decisions do not list %s", id)
	return approvals.FailedDecision{}
}

// Rep2 approved an update whose work then failed. Once their role masks the
// field it changes, neither their failed list nor Home's lane repeats the plan.
func TestAFailedDecisionKeepsItsPlanBackOnceItsDeciderMayNotSeeIt(t *testing.T) {
	s := newKindSight(t)
	s.svc.WithEffect(toolUpdateRecord, func(context.Context, ids.ApprovalID, json.RawMessage, string) error {
		return errors.New("the record store refused the update")
	})
	deal := s.deal("Harbor expansion", s.e.Rep2)
	company := s.company("Visible Holdings", s.e.Rep2, "")
	id := s.stageCall(s.rep2(), approvals.StageInput{Kind: toolUpdateRecord, TargetType: "deal", TargetID: deal, Summary: "Move Harbor expansion to Visible Holdings"},
		fmt.Sprintf(`{"record_type":"deal","id":%q,"fields":{"company_id":%q}}`, deal, company))
	if _, err := s.svc.Decide(s.rep2(), id, true, nil); err == nil {
		t.Fatal("the failing work was reported as done")
	}
	if shown := s.failedDecision(s.rep2(), id); shown.Approval.Summary == nil || shown.Approval.ProposedChange == nil {
		t.Fatalf("the decider lost the plan of their own failed decision: %+v", shown.Approval)
	}

	viewer, _ := principal.Actor(s.rep2())
	viewer.Permissions.FieldMasks = []principal.FieldMask{{Object: "deal", Field: "company_id", Condition: principal.MaskAlways}}
	masked := principal.WithActor(s.rep2(), viewer)
	requireCarriesNoPlan(t, s.failedDecision(masked, id).Approval, "Visible Holdings", company.String())
	lane, err := attentionFailedEffects{svc: s.svc}.Failed(masked, 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, failed := range lane {
		if failed.ID == id.UUID && failed.Summary != nil {
			t.Fatalf("Home's failed lane repeats the summary: %s", *failed.Summary)
		}
	}
}

// A decided mail's file is later withheld. The stored plan is then checked by
// the rule a send applies, which refuses a withheld file, as the pending plan was.
func TestADecidedPlanWhoseFileIsWithheldStaysBack(t *testing.T) {
	e := integration.Setup(t)
	company := e.SeedCompany(t, "Harbor account", &e.AdminUser)
	files := activities.NewStore(e.DB()).WithBlobstore(blobstore.NewMemory())
	file, err := files.UploadAttachment(e.Admin(), activities.AttachmentInput{
		EntityType: "company", EntityID: company, Filename: "terms.txt", ContentType: "text/plain", Content: strings.NewReader("Delivery terms."),
	})
	if err != nil {
		t.Fatal(err)
	}
	canonical, hash, err := diffhash.Canonical(fmt.Appendf(nil, `{"to":["buyer@example.test"],"subject":"Delivery terms","body":"Terms attached.",
		"communication_context":"requested_followup","consent_purpose":"transactional","operator_reason":"Buyer asked.",
		"attachment_ids":[%q],"links":[{"entity_type":"company","entity_id":%q}]}`, file.Id, company))
	if err != nil {
		t.Fatal(err)
	}
	svc := NewApprovalsService(e.DB())
	id, err := svc.Stage(e.Admin(), approvals.StageInput{Kind: previewSendCompanyEmail, TargetType: "activity", ProposedChange: canonical, DiffHash: hash, Summary: "Send the delivery terms"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Decide(e.Admin(), id, false, nil); err != nil {
		t.Fatal(err)
	}
	if shown, err := svc.GetWire(e.Admin(), id); err != nil || shown.ProposedChange == nil {
		t.Fatalf("the decider lost the plan while its file is whole: %v", err)
	}

	key := e.WsScalar(t, `SELECT storage_key FROM attachment WHERE id = $1`, file.Id)
	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		return files.WithholdStoredFilesTx(e.Admin(), tx, []activities.StoredMessageFile{{ID: ids.UUID(file.Id), Key: key}})
	}); err != nil {
		t.Fatal(err)
	}
	hidden, err := svc.GetWire(e.Admin(), id)
	if err != nil {
		t.Fatal(err)
	}
	requireCarriesNoPlan(t, hidden, "Terms attached.")
}
