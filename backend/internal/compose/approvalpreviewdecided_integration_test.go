// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// hiddenMailSecret names the company only Rep1 opens, which the agent's mail
// is filed under.
const hiddenMailSecret = "Undisclosed Account"

// renewalIdentity is the logical key a newer member mail supersedes an older by.
var renewalIdentity = json.RawMessage(`{"subject":"Renewal terms"}`)

// stageHiddenMail stages Rep1's agent's mail filed under a company only Rep1
// opens. Rep1 decides as an admin; Rep2, a rep, cannot open the company.
func (s *promotionSight) stageHiddenMail(company ids.UUID, in approvals.StageInput, body string) ids.ApprovalID {
	return s.stageMail(s.agent, company, in, body)
}

// stageMail stages the same mail as proposer: the agent, or a member, whose
// proposal may carry an identity a newer one supersedes.
func (s *promotionSight) stageMail(proposer context.Context, company ids.UUID, in approvals.StageInput, body string) ids.ApprovalID {
	in.Kind, in.TargetType = previewSendCompanyEmail, "activity"
	return s.stageCall(proposer, in, fmt.Sprintf(`{
		"to":["buyer@example.test"],"subject":"Renewal terms","body":%q,
		"communication_context":"requested_followup","consent_purpose":"transactional","operator_reason":"Buyer asked.",
		"links":[{"entity_type":"company","entity_id":%q}]}`, body, company))
}

func (s *promotionSight) rep1() context.Context { return storedMemberSeat(s.t, s.e, s.e.Rep1) }
func (s *promotionSight) rep2() context.Context { return storedMemberSeat(s.t, s.e, s.e.Rep2) }

// requirePlanHidden holds that reader gets the approval's metadata and none of
// its plan, from the single read, the listing and read_approval alike.
func (s *promotionSight) requirePlanHidden(reader context.Context, id ids.ApprovalID, secrets ...string) {
	s.t.Helper()
	approval, err := s.svc.GetWire(reader, id)
	if err != nil {
		s.t.Fatal(err)
	}
	requireCarriesNoPlan(s.t, approval, secrets...)
	listed, _, err := s.svc.ListWire(reader, approvals.ListInput{})
	if err != nil {
		s.t.Fatal(err)
	}
	at := slices.IndexFunc(listed, func(a crmcontracts.Approval) bool { return a.Id == approval.Id })
	if at < 0 {
		s.t.Fatalf("the %s approval is not listed for its reader", approval.Status)
	}
	requireCarriesNoPlan(s.t, listed[at], secrets...)
	tool, err := (approvalInbox{svc: s.svc}).ReadApproval(reader, id.UUID)
	if err != nil {
		s.t.Fatal(err)
	}
	if tool.Summary != "" || tool.ProposedChange != nil || tool.Evidence != nil {
		s.t.Fatalf("read_approval still carries the plan: %+v", tool)
	}
}

func requireCarriesNoPlan(t *testing.T, approval crmcontracts.Approval, secrets ...string) {
	t.Helper()
	if approval.ProposedChange != nil || approval.Summary != nil || approval.SummaryParts != nil || approval.Evidence != nil {
		t.Fatalf("a %s approval still carries its plan: change=%v summary=%v", approval.Status, approval.ProposedChange, approval.Summary)
	}
	if approval.Preview == nil || approval.Preview.State != approvals.PreviewNeedsWiderSight {
		t.Fatalf("a hidden plan reads as %+v, want the sentence a pending one shows", approval.Preview)
	}
	raw, err := json.Marshal(approval)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range secrets {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("the %s approval carries %q: %s", approval.Status, secret, raw)
		}
	}
}

func (s *promotionSight) requirePlanShown(reader context.Context, id ids.ApprovalID) {
	s.t.Helper()
	approval, err := s.svc.GetWire(reader, id)
	if err != nil {
		s.t.Fatal(err)
	}
	if approval.ProposedChange == nil {
		s.t.Fatalf("a reader who opens everything lost the %s plan: %+v", approval.Status, approval)
	}
}

// The admin who decided keeps the plan; the rep who cannot open the company
// it is filed under reads the decision without it.
func TestADecidedPlanStaysHiddenFromAReaderWhoCannotOpenWhatItTouched(t *testing.T) {
	s := newMailSight(t)
	company := s.privateCompany(hiddenMailSecret, s.e.Rep1)
	id := s.stageHiddenMail(company, approvals.StageInput{}, "Renewal terms attached.")
	if _, err := s.svc.Decide(s.rep1(), id, true, nil); err != nil {
		t.Fatal(err)
	}

	s.requirePlanHidden(s.rep2(), id, company.String(), "Renewal terms")
	s.requirePlanShown(s.rep1(), id)
	stored := s.e.WsScalar(t, `SELECT plan_footprint::text FROM approval WHERE id = $1`, id.UUID)
	if !strings.Contains(stored, company.String()) || strings.Contains(stored, "Renewal terms") || strings.Contains(stored, "buyer@example.test") {
		t.Fatalf("the stored footprint is not references and field names only: %s", stored)
	}
}

// Every way a plan stops being pending leaves the rep without it. A writer
// that stores no footprint leaves the decider without it too.
func TestEveryEndOfAPlanKeepsItFromAReaderWhoCannotOpenWhatItTouched(t *testing.T) {
	for _, tc := range []struct {
		name        string
		end         func(s *promotionSight, id ids.ApprovalID, company ids.UUID)
		deciderSees bool
	}{
		{"rejected", func(s *promotionSight, id ids.ApprovalID, _ ids.UUID) {
			if _, err := s.svc.Decide(s.rep1(), id, false, nil); err != nil {
				s.t.Fatal(err)
			}
		}, true},
		{"expired by the sweep", func(s *promotionSight, id ids.ApprovalID, _ ids.UUID) {
			s.e.WsExec(s.t, `UPDATE approval SET expires_at = now() - interval '1 day' WHERE id = $1`, id.UUID)
			if _, err := s.svc.ExpireDue(expiryCtx(s.e)); err != nil {
				s.t.Fatal(err)
			}
			if s.e.WsCount(s.t, `SELECT count(*) FROM approval WHERE id = $1 AND status = 'expired'`, id.UUID) != 1 {
				s.t.Fatal("the sweep left the approval pending")
			}
		}, true},
		{"lapsed before the sweep", func(s *promotionSight, id ids.ApprovalID, _ ids.UUID) {
			s.e.WsExec(s.t, `UPDATE approval SET expires_at = now() - interval '1 day' WHERE id = $1`, id.UUID)
		}, true},
		{"superseded", func(s *promotionSight, id ids.ApprovalID, company ids.UUID) {
			s.stageMail(s.rep1(), company, approvals.StageInput{Identity: renewalIdentity, JoinPending: true}, "Revised renewal terms.")
			if s.e.WsCount(s.t, `SELECT count(*) FROM approval WHERE id = $1 AND status = 'expired'`, id.UUID) != 1 {
				s.t.Fatal("the newer proposal did not supersede the first")
			}
		}, false},
		{"withdrawn", func(s *promotionSight, id ids.ApprovalID, _ ids.UUID) {
			if err := s.e.DB().Tx(s.e.Admin(), func(tx pgx.Tx) error {
				_, err := s.svc.WithdrawInTx(s.e.Admin(), tx, id, "the question went away")
				return err
			}); err != nil {
				s.t.Fatal(err)
			}
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newMailSight(t)
			company := s.privateCompany(hiddenMailSecret, s.e.Rep1)
			id := s.stageHiddenMail(company, approvals.StageInput{}, "Renewal terms attached.")
			if tc.name == "superseded" {
				id = s.stageMail(s.rep1(), company, approvals.StageInput{Identity: renewalIdentity, JoinPending: true}, "Renewal terms attached.")
			}
			tc.end(s, id, company)

			s.requirePlanHidden(s.rep2(), id, company.String(), "Renewal terms")
			if tc.deciderSees {
				s.requirePlanShown(s.rep1(), id)
			} else {
				s.requirePlanHidden(s.rep1(), id)
			}
		})
	}
}

// Rep2 opens every record the plan touched, and their role masks the address
// it is sent to. The mask, not the sight, keeps the decided plan from them.
func TestADecidedPlanStaysHiddenFromAReaderWhoseRoleMasksAFieldItNamed(t *testing.T) {
	s := newMailSight(t)
	company := s.company("Open Account", s.e.Rep2, "")
	id := s.stageHiddenMail(company, approvals.StageInput{}, "Renewal terms attached.")
	if _, err := s.svc.Decide(s.rep1(), id, false, nil); err != nil {
		t.Fatal(err)
	}
	viewer, _ := principal.Actor(s.rep2())
	viewer.Permissions.FieldMasks = []principal.FieldMask{{Object: "contact", Field: "email", Condition: principal.MaskAlways}}

	s.requirePlanHidden(principal.WithActor(s.rep2(), viewer), id, "buyer@example.test")
	s.requirePlanShown(s.rep2(), id)
}

// A rep may decline a plan they cannot open. What the decision hands back to
// them, one at a time or as a bundle, carries none of it.
func TestADeclineByAReaderWhoCannotOpenThePlanAnswersWithoutIt(t *testing.T) {
	s := newMailSight(t)
	company := s.privateCompany(hiddenMailSecret, s.e.Rep1)
	single := s.stageHiddenMail(company, approvals.StageInput{}, "Renewal terms attached.")
	declined, err := (approvalInbox{svc: s.svc}).DecideApproval(s.rep2(), single.UUID, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if declined.Summary != "" || declined.ProposedChange != nil || declined.Preview == nil || declined.Preview.State != approvals.PreviewNeedsWiderSight {
		t.Fatalf("read_approval's decline answers with the plan: %+v", declined)
	}

	bundle := ids.NewV7()
	s.stageHiddenMail(company, approvals.StageInput{BundleID: bundle}, "First renewal terms.")
	s.stageHiddenMail(company, approvals.StageInput{BundleID: bundle}, "Second renewal terms.")
	members, err := s.svc.DecideBundleWire(s.rep2(), bundle, false, nil)
	if err != nil || len(members) != 2 {
		t.Fatalf("declining the bundle: %d members, %v", len(members), err)
	}
	for _, member := range members {
		requireCarriesNoPlan(t, member.Approval, company.String(), "renewal terms")
		s.requirePlanShown(s.rep1(), ids.From[ids.ApprovalKind](ids.UUID(member.Approval.Id)))
	}
}

// A row decided before footprints were stored proves nothing about its
// reader. An agent's plan then shows nobody its payload; a kind with no
// preview shows every decider what its pending card showed.
func TestAPlanDecidedWithoutAFootprintShowsOnlyWhatNoPreviewGuarded(t *testing.T) {
	s := newMailSight(t)
	agentPlan := s.stageHiddenMail(s.company("Plain Account", s.e.Rep1, ""), approvals.StageInput{}, "Renewal terms attached.")
	typed, err := s.svc.Stage(s.e.Admin(), approvals.StageInput{
		Kind: "fx_rate_proposal", ProposedChange: json.RawMessage(`{"note":"a rate to confirm"}`), DiffHash: "legacy-" + ids.NewV7().String(), Summary: "Confirm the month's rate",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []ids.ApprovalID{agentPlan, typed} {
		if _, err := s.svc.Decide(s.e.Admin(), id, false, nil); err != nil {
			t.Fatal(err)
		}
	}
	s.e.WsExec(t, `UPDATE approval SET plan_footprint = NULL WHERE id = ANY($1)`, []ids.ApprovalID{agentPlan, typed})

	s.requirePlanHidden(s.rep1(), agentPlan, "Renewal terms")
	kept, err := s.svc.GetWire(s.e.Admin(), typed)
	if err != nil || kept.ProposedChange == nil || kept.Summary == nil || *kept.Summary != "Confirm the month's rate" {
		t.Fatalf("a decided kind with no preview lost its payload: %+v, %v", kept, err)
	}
}

// A contact the plan touched, named nowhere in its payload or summary, is
// found through the footprint by both the export and the erasure.
func TestAContactOnlyADecidedPlansFootprintNamesIsExportedAndErased(t *testing.T) {
	s := newMailSight(t)
	buyer := s.e.SeedContact(t, "Open Buyer", &s.e.Rep1)
	contact := s.e.SeedContact(t, "Footprint Subject", &s.e.Rep1)
	note, _, err := s.e.Activities.LogActivity(s.e.Admin(), activities.LogActivityInput{
		Kind: "note", Subject: new("Buyer call notes"), Source: "manual", Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: buyer}},
	})
	if err != nil {
		t.Fatal(err)
	}
	activity := ids.UUID(note.Id)
	id := s.stageCall(s.agent, approvals.StageInput{Kind: singleRelinkTool, TargetType: "activity", TargetID: activity},
		fmt.Sprintf(`{"activity_id":%q,"entity_type":"contact","entity_id":%q}`, activity, contact))
	if _, err := s.svc.Decide(s.rep1(), id, false, nil); err != nil {
		t.Fatal(err)
	}
	if s.e.WsCount(t, `SELECT count(*) FROM approval WHERE id = $1 AND plan_footprint->'refs'->'contact' @> to_jsonb($2::uuid)`, id.UUID, contact) != 1 {
		t.Fatal("the decided plan's footprint does not name the contact it touched, so this test proves nothing")
	}
	subject := ids.From[ids.ContactKind](contact)

	pkg, err := privacy.AssembleSAR(s.e.Admin(), s.e.DB(), subject)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(pkg.StagedMessages, func(row map[string]any) bool { return fmt.Sprint(row["id"]) == id.String() }) {
		t.Fatalf("the export lists %d staged proposals and not the one whose plan touched the subject", len(pkg.StagedMessages))
	}
	if err := eraserWithGraph(s.e.DB()).EraseContact(s.e.Admin(), contact, "subject request"); err != nil {
		t.Fatal(err)
	}
	if s.e.WsCount(t, `SELECT count(*) FROM approval WHERE id = $1 AND proposed_change = '{}'::jsonb AND plan_footprint IS NULL AND status = 'rejected'`, id.UUID) != 1 {
		t.Fatal("the erasure left the decided plan that touched the subject as it was")
	}
}
