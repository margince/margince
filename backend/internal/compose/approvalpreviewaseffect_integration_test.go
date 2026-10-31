// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

// promotionSight is Rep1's agent and a member who reviews its promotions, each
// seated on a stored role the way production derives them.
type promotionSight struct {
	t               *testing.T
	e               *integration.Env
	svc             *approvals.Service
	agent, approver context.Context
}

func newPromotionSight(t *testing.T, agentRole string, approverOf func(*integration.Env) ids.UUID, approverRole string) *promotionSight {
	t.Helper()
	e := integration.Setup(t)
	approver := approverOf(e)
	e.GrantRole(t, e.Rep1, agentRole)
	e.GrantRole(t, approver, approverRole)
	passport := e.SeedPassport(t, integration.OwnerConn(t), "promotion sight")
	live, err := identity.NewService(e.Pool).AuthenticateAgentByID(sightContext(e), ids.From[ids.PassportKind](passport))
	if err != nil {
		t.Fatal(err)
	}
	return &promotionSight{
		t: t, e: e, svc: NewApprovalsService(e.DB()),
		agent:    principal.WithActor(sightContext(e), live.Principal()),
		approver: storedMemberSeat(t, e, approver),
	}
}

func sightContext(e *integration.Env) context.Context {
	return principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.WS), ids.NewV7())
}

func storedMemberSeat(t *testing.T, e *integration.Env, user ids.UUID) context.Context {
	t.Helper()
	ctx := sightContext(e)
	member, err := authz.MemberPrincipal(ctx, identity.NewService(e.Pool), e.WS, user)
	if err != nil {
		t.Fatal(err)
	}
	return principal.WithActor(ctx, member)
}

func (s *promotionSight) lead(in contacts.CreateLeadInput) ids.LeadID {
	s.t.Helper()
	lead, _, err := s.e.Contacts.CreateLead(s.e.Admin(), in)
	if err != nil {
		s.t.Fatal(err)
	}
	return ids.From[ids.LeadKind](ids.UUID(lead.Id))
}

// company seeds a company held by owner, which hide later makes private to them.
func (s *promotionSight) company(name string, owner ids.UUID, domain string) ids.UUID {
	s.t.Helper()
	in := contacts.CreateCompanyInput{DisplayName: name, Source: "manual", OwnerID: new(ids.From[ids.UserKind](owner))}
	if domain != "" {
		in.Domains = []contacts.CompanyDomainInput{{Domain: domain, IsPrimary: true}}
	}
	company, err := s.e.Contacts.CreateCompany(s.e.Admin(), in)
	if err != nil {
		s.t.Fatal(err)
	}
	return ids.UUID(company.Id)
}

func (s *promotionSight) hide(company ids.UUID) {
	s.t.Helper()
	if _, err := s.e.Contacts.UpdateCompany(s.e.Admin(), ids.From[ids.CompanyKind](company), contacts.UpdateCompanyInput{Visibility: new("owner")}); err != nil {
		s.t.Fatal(err)
	}
}

// mailOnlyOn files an inbound email under one company and nothing else, so
// only a member who sees that company can find it.
func (s *promotionSight) mailOnlyOn(company ids.UUID, subject string) ids.UUID {
	id := ids.NewV7()
	s.e.WsExec(s.t, `INSERT INTO activity (id, kind, subject, direction, occurred_at, source, captured_by)
	                 VALUES ($1, 'email', $2, 'inbound', now(), 'manual', 'human:x')`, id, subject)
	s.e.WsExec(s.t, `INSERT INTO activity_link (activity_id, entity_type, company_id) VALUES ($1, 'company', $2)`, id, company)
	return id
}

func (s *promotionSight) stage(lead ids.LeadID, payload string) ids.ApprovalID {
	s.t.Helper()
	change, hash, err := diffhash.Canonical([]byte(payload))
	if err != nil {
		s.t.Fatal(err)
	}
	id, _, err := s.svc.StageAgentCall(s.agent, approvals.StageInput{
		Kind: operationPromoteLead, TargetType: "lead", TargetID: lead.UUID, ProposedChange: change, DiffHash: hash,
	})
	if err != nil {
		s.t.Fatal(err)
	}
	return id
}

func (s *promotionSight) preview(id ids.ApprovalID) crmcontracts.ApprovalPreview {
	s.t.Helper()
	proposal, err := s.svc.GetWire(s.approver, id)
	if err != nil || proposal.Preview == nil {
		s.t.Fatalf("reading the promotion's preview: %+v, %v", proposal.Preview, err)
	}
	return *proposal.Preview
}

func (s *promotionSight) approveAndRelease(id ids.ApprovalID, lead ids.LeadID) ids.UUID {
	s.t.Helper()
	if _, err := s.svc.Decide(s.approver, id, true, nil); err != nil {
		s.t.Fatalf("approving the promotion: %v", err)
	}
	proposal, err := s.svc.GetWire(s.approver, id)
	if err != nil || proposal.DiffHash == nil {
		s.t.Fatalf("reading the released call: %v", err)
	}
	input, err := promotionApprovalInput(proposal)
	if err != nil {
		s.t.Fatal(err)
	}
	released, _, _, err := agents.RedeemAndMark(s.agent, approvalsAdapter{svc: s.svc}, id, operationPromoteLead, *proposal.DiffHash)
	if err != nil {
		s.t.Fatalf("redeeming the promotion: %v", err)
	}
	if _, err := s.e.Contacts.QualifyLead(released, lead, input); err != nil {
		s.t.Fatalf("running the released promotion: %v", err)
	}
	contact, err := ids.Parse(s.e.WsScalar(s.t, `SELECT id FROM contact WHERE converted_from_lead_id = $1`, lead.UUID))
	if err != nil {
		s.t.Fatal(err)
	}
	return contact
}

func previewDetailKind(preview crmcontracts.ApprovalPreview, field string) string {
	for _, detail := range *preview.Details {
		if detail.Field == field {
			return detail.Value.Kind
		}
	}
	return ""
}

func requireUnnamed(t *testing.T, preview crmcontracts.ApprovalPreview, names ...string) {
	t.Helper()
	for _, detail := range *preview.Details {
		for _, name := range names {
			if strings.Contains(detail.Value.Text, name) {
				t.Fatalf("the preview names %q to a member who cannot open it: %+v", name, detail)
			}
		}
	}
}

func approverIsAdmin(e *integration.Env) ids.UUID { return e.AdminUser }

func approverIsRep2(e *integration.Env) ids.UUID { return e.Rep2 }

// The approver sees the company holding the lead's domain and the agent does
// not. So the agent's promotion holds the typed name back instead of linking it.
func TestAPromotionPreviewShowsWhatTheAgentsPromotionWillDo(t *testing.T) {
	s := newPromotionSight(t, "rep", approverIsAdmin, "admin")
	domain := "held-" + ids.NewV7().String()[24:] + ".test"
	holder := s.company("Hidden Holder", s.e.AdminUser, domain)
	s.hide(holder)
	rep1 := ids.From[ids.UserKind](s.e.Rep1)
	lead := s.lead(contacts.CreateLeadInput{
		FullName: new("Pat Typed"), Email: new("pat@" + domain), CompanyName: new("Typed Works"), Source: "manual", OwnerID: &rep1,
	})
	id := s.stage(lead, fmt.Sprintf(`{"lead_id":%q,"trigger":"human_qualify"}`, lead.UUID))

	preview := s.preview(id)
	if preview.State != "ready" || preview.Effects == nil || !slices.Contains(*preview.Effects, "promote_propose_company") {
		t.Fatalf("preview = %s %v, want the agent's typed-name proposal", preview.State, preview.Effects)
	}
	if slices.Contains(*preview.Effects, contacts.ContactEmployerResolveEffect) || slices.Contains(*preview.Effects, "reuse_contact_employer") {
		t.Fatalf("the preview links the company only the approver sees: %v", *preview.Effects)
	}

	contact := s.approveAndRelease(id, lead)
	if n := s.e.WsCount(t, `SELECT count(*) FROM relationship WHERE contact_id = $1 AND company_id = $2`, contact, holder); n != 0 {
		t.Fatalf("the promotion linked the company the agent cannot see (%d links)", n)
	}
	if n := s.e.WsCount(t, `SELECT count(*) FROM contact_profile_field WHERE contact_id = $1 AND field = 'company_name' AND value = 'Typed Works'`, contact); n != 1 {
		t.Fatalf("the promotion did not propose the typed company the preview showed (%d facts)", n)
	}
}

// The agent links a company and cites a mail that only it can open.
func TestAPromotionPreviewNamesNoRecordTheApproverCannotOpen(t *testing.T) {
	s := newPromotionSight(t, "admin", approverIsRep2, "rep")
	secret := s.company("Secret Account", s.e.Rep1, "")
	rep2 := ids.From[ids.UserKind](s.e.Rep2)
	lead := s.lead(contacts.CreateLeadInput{
		FullName: new("Lee Linked"), Email: new("lee@gmail.com"), CompanyName: new("Secret Account"), Source: "manual", OwnerID: &rep2,
	})
	if _, err := s.e.Contacts.LinkLeadCompany(s.e.Admin(), lead, ids.From[ids.CompanyKind](secret)); err != nil {
		t.Fatal(err)
	}
	// Renamed, so the name the lead typed is no longer the company's name.
	if _, err := s.e.Contacts.UpdateCompany(s.e.Admin(), ids.From[ids.CompanyKind](secret), contacts.UpdateCompanyInput{DisplayName: new("Quiet Holdings")}); err != nil {
		t.Fatal(err)
	}
	s.hide(secret)
	mail := s.mailOnlyOn(secret, "Secret pricing thread")
	id := s.stage(lead, fmt.Sprintf(`{"operation":"PromoteLead","body":{"trigger":"human_qualify","evidence":{"activity_id":%q}}}`, mail))

	preview := s.preview(id)
	if preview.State != "ready" || preview.Details == nil {
		t.Fatalf("preview = %s, want ready", preview.State)
	}
	for _, field := range []string{refColCompany, "promotion_evidence"} {
		if kind := previewDetailKind(preview, field); kind != previewValueUnavailable {
			t.Fatalf("%s is shown as %q, want it withheld", field, kind)
		}
	}
	requireUnnamed(t, preview, "Quiet Holdings", "Secret pricing thread")

	contact := s.approveAndRelease(id, lead)
	if n := s.e.WsCount(t, `SELECT count(*) FROM relationship WHERE contact_id = $1 AND company_id = $2 AND kind = 'employment'`, contact, secret); n != 1 {
		t.Fatalf("the promotion did not link the company the preview withheld (%d links)", n)
	}
}

// The approver cannot find the mail the agent cites, and the pin stays put.
func TestAPromotionPinHoldsForAnApproverWhoCannotFindTheAgentsMail(t *testing.T) {
	s := newPromotionSight(t, "admin", approverIsRep2, "rep")
	mailbox := s.company("Mailbox Holder", s.e.Rep1, "")
	s.hide(mailbox)
	rep2 := ids.From[ids.UserKind](s.e.Rep2)
	lead := s.lead(contacts.CreateLeadInput{FullName: new("Mo Mail"), Email: new("mo@gmail.com"), Source: "manual", OwnerID: &rep2})
	mail := s.mailOnlyOn(mailbox, "Call notes")
	id := s.stage(lead, fmt.Sprintf(`{"operation":"PromoteLead","body":{"trigger":"human_qualify","evidence":{"activity_id":%q}}}`, mail))
	staged := s.e.WsScalar(t, `SELECT context_digest FROM approval WHERE id = $1`, id.UUID)

	if state := s.preview(id).State; state != "ready" {
		t.Fatalf("preview = %s, want ready", state)
	}
	s.approveAndRelease(id, lead)
	if decided := s.e.WsScalar(t, `SELECT context_digest FROM approval WHERE id = $1`, id.UUID); decided != staged {
		t.Fatalf("the digest moved from %s to %s across readers", staged, decided)
	}
}

// A member's own proposal runs when its decider approves it, so the decider's
// sight is still the one its preview shows.
func TestAHumanStagedPromotionIsStillPreviewedAsItsDecider(t *testing.T) {
	s := newPromotionSight(t, "rep", approverIsAdmin, "admin")
	domain := "decider-" + ids.NewV7().String()[24:] + ".test"
	s.hide(s.company("Decider Holder", s.e.AdminUser, domain))
	rep1 := ids.From[ids.UserKind](s.e.Rep1)
	lead := s.lead(contacts.CreateLeadInput{
		FullName: new("Dee Decider"), Email: new("dee@" + domain), CompanyName: new("Typed Works"), Source: "manual", OwnerID: &rep1,
	})
	change, hash, err := diffhash.Canonical(fmt.Appendf(nil, `{"lead_id":%q,"trigger":"human_qualify"}`, lead.UUID))
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.svc.Stage(storedMemberSeat(t, s.e, s.e.Rep1), approvals.StageInput{
		Kind: operationPromoteLead, TargetType: "lead", TargetID: lead.UUID, ProposedChange: change, DiffHash: hash,
	})
	if err != nil {
		t.Fatal(err)
	}
	preview := s.preview(id)
	if preview.State != "ready" || preview.Effects == nil || slices.Contains(*preview.Effects, "promote_propose_company") {
		t.Fatalf("preview = %s %v, want the decider's own resolution of the domain", preview.State, preview.Effects)
	}
}
