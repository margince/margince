// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/margince/margince/backend/internal/shared/apperrors"
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
	s := &promotionSight{t: t, e: e, svc: NewApprovalsService(e.DB()), approver: storedMemberSeat(t, e, approver)}
	s.agentOn(e.SeedPassport(t, integration.OwnerConn(t), "promotion sight"))
	return s
}

// agentOn makes the agent the one that authenticates with passport.
func (s *promotionSight) agentOn(passport ids.UUID) {
	s.t.Helper()
	live, err := identity.NewService(s.e.Pool).AuthenticateAgentByID(sightContext(s.e), ids.From[ids.PassportKind](passport))
	if err != nil {
		s.t.Fatal(err)
	}
	s.agent = principal.WithActor(sightContext(s.e), live.Principal())
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

func promotionDetailValue(t *testing.T, preview crmcontracts.ApprovalPreview, field string) crmcontracts.ApprovalPreviewValue {
	t.Helper()
	if preview.Details != nil {
		for _, detail := range *preview.Details {
			if detail.Field == field {
				return detail.Value
			}
		}
	}
	t.Fatalf("the preview has no %s detail: %+v", field, preview.Details)
	return crmcontracts.ApprovalPreviewValue{}
}

// requireShowsNothing holds that a refused preview carries no part of the plan.
func requireShowsNothing(t *testing.T, preview crmcontracts.ApprovalPreview, secrets ...string) {
	t.Helper()
	if preview.State != approvals.PreviewNeedsWiderSight {
		t.Fatalf("preview = %s, want %s", preview.State, approvals.PreviewNeedsWiderSight)
	}
	if preview.Details != nil || preview.Effects != nil || len(preview.Changes) != 0 {
		t.Fatalf("a refused preview still shows the plan: %+v", preview)
	}
	raw, err := json.Marshal(preview)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range secrets {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("a refused preview carries %q: %s", secret, raw)
		}
	}
}

func requireRefusedForSight(t *testing.T, err error) {
	t.Helper()
	refusal, ok := errors.AsType[*approvals.PreviewContextError](err)
	if !ok || !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("deciding = %v, want a permission refusal for want of sight", err)
	}
	if code, _ := refusal.MessageFault(); code != "approval_needs_wider_sight" {
		t.Fatalf("refusal code = %s", code)
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
	if email := promotionDetailValue(t, preview, "promotion_contact_email"); email.Text != "pat@"+domain {
		t.Fatalf("an approver who sees everything gets the address as %+v", email)
	}

	contact := s.approveAndRelease(id, lead)
	if n := s.e.WsCount(t, `SELECT count(*) FROM relationship WHERE contact_id = $1 AND company_id = $2`, contact, holder); n != 0 {
		t.Fatalf("the promotion linked the company the agent cannot see (%d links)", n)
	}
	if n := s.e.WsCount(t, `SELECT count(*) FROM contact_profile_field WHERE contact_id = $1 AND field = 'company_name' AND value = 'Typed Works'`, contact); n != 1 {
		t.Fatalf("the promotion did not propose the typed company the preview showed (%d facts)", n)
	}
}

// The agent's promotion merges into a contact private to its human.
func TestAnApproverWhoCannotOpenTheMergeTargetIsShownNothing(t *testing.T) {
	s := newPromotionSight(t, "admin", approverIsRep2, "rep")
	address := "kit@private-" + ids.NewV7().String()[24:] + ".test"
	rep2 := ids.From[ids.UserKind](s.e.Rep2)
	lead := s.lead(contacts.CreateLeadInput{FullName: new("Kit Merge"), Email: new(address), Source: "manual", OwnerID: &rep2})
	// The writers refuse a contact a live lead's address, so it arrives the
	// way an older capture left it.
	target := s.e.SeedContact(t, "Kit Hidden", &s.e.Rep1)
	s.e.WsExec(t, `INSERT INTO contact_email (contact_id, email, is_primary, position, source, captured_by)
	               VALUES ($1, $2, true, 1, 'manual', 'human:x')`, target, address)
	s.e.WsExec(t, `UPDATE contact SET visibility = 'owner' WHERE id = $1`, target)
	id := s.stage(lead, fmt.Sprintf(`{"lead_id":%q,"trigger":"human_qualify"}`, lead.UUID))

	requireShowsNothing(t, s.preview(id), target.String(), "Kit Hidden", "consent", "promote_")
	_, err := s.svc.Decide(s.approver, id, true, nil)
	requireRefusedForSight(t, err)
	if s.e.WsCount(t, `SELECT count(*) FROM approval WHERE id = $1 AND status = 'pending'`, id.UUID) != 1 {
		t.Fatal("a refused decision moved the approval")
	}
}

// The approver cannot find the mail the agent cites. The card says so instead
// of going stale, and an approver who can see the mail still approves it.
func TestAPromotionCitingMailTheApproverCannotFindWaitsForOneWhoCan(t *testing.T) {
	s := newPromotionSight(t, "admin", approverIsRep2, "rep")
	mailbox := s.company("Mailbox Holder", s.e.Rep1, "")
	s.hide(mailbox)
	rep2 := ids.From[ids.UserKind](s.e.Rep2)
	lead := s.lead(contacts.CreateLeadInput{FullName: new("Mo Mail"), Email: new("mo@gmail.com"), Source: "manual", OwnerID: &rep2})
	mail := s.mailOnlyOn(mailbox, "Secret pricing thread")
	id := s.stage(lead, fmt.Sprintf(`{"operation":"PromoteLead","body":{"trigger":"human_qualify","evidence":{"activity_id":%q}}}`, mail))
	staged := s.e.WsScalar(t, `SELECT context_digest FROM approval WHERE id = $1`, id.UUID)

	requireShowsNothing(t, s.preview(id), mail.String(), "Secret pricing thread")
	_, err := s.svc.Decide(s.approver, id, true, nil)
	requireRefusedForSight(t, err)

	s.e.GrantRole(t, s.e.Rep1, "admin")
	s.approver = storedMemberSeat(t, s.e, s.e.Rep1)
	if state := s.preview(id).State; state != "ready" {
		t.Fatalf("preview for an approver who sees the mail = %s, want ready", state)
	}
	s.approveAndRelease(id, lead)
	if decided := s.e.WsScalar(t, `SELECT context_digest FROM approval WHERE id = $1`, id.UUID); decided != staged {
		t.Fatalf("the digest moved from %s to %s across readers", staged, decided)
	}
}

func TestAPromotionPreviewKeepsTheFieldsTheApproversRoleMasks(t *testing.T) {
	s := newPromotionSight(t, "admin", approverIsRep2, "rep")
	viewer, _ := principal.Actor(s.approver)
	viewer.Permissions.FieldMasks = []principal.FieldMask{
		{Object: "lead", Field: "email", Condition: principal.MaskAlways},
		{Object: "lead", Field: "phone", Condition: principal.MaskAlways},
	}
	s.approver = principal.WithActor(s.approver, viewer)
	rep2 := ids.From[ids.UserKind](s.e.Rep2)
	lead := s.lead(contacts.CreateLeadInput{
		FullName: new("Mia Masked"), Email: new("mia@gmail.com"), Phone: new("+49 30 1234567"), Source: "manual", OwnerID: &rep2,
	})
	id := s.stage(lead, fmt.Sprintf(`{"lead_id":%q,"trigger":"human_qualify"}`, lead.UUID))

	preview := s.preview(id)
	if preview.State != "ready" {
		t.Fatalf("preview = %s, want ready", preview.State)
	}
	for _, field := range []string{"promotion_contact_email", "promotion_contact_phone"} {
		if value := promotionDetailValue(t, preview, field); value.Kind != previewValueUnavailable || value.Text != "" {
			t.Fatalf("%s reaches a role that masks it: %+v", field, value)
		}
	}
	if name := promotionDetailValue(t, preview, "promotion_contact_name"); name.Text != "Mia Masked" {
		t.Fatalf("an unmasked field is withheld too: %+v", name)
	}
}

func TestAPromotionStagedOnARevokedPassportIsNotReady(t *testing.T) {
	s := newPromotionSight(t, "admin", approverIsAdmin, "admin")
	rep1 := ids.From[ids.UserKind](s.e.Rep1)
	lead := s.lead(contacts.CreateLeadInput{FullName: new("Rae Revoked"), Email: new("rae@gmail.com"), Source: "manual", OwnerID: &rep1})
	id := s.stage(lead, fmt.Sprintf(`{"lead_id":%q,"trigger":"human_qualify"}`, lead.UUID))
	agent, _ := principal.Actor(s.agent)
	s.e.WsExec(t, `UPDATE passport SET revoked_at = now() WHERE id = $1`, agent.PassportID)
	if state := s.preview(id).State; state == "ready" {
		t.Fatal("a promotion whose agent can no longer call reads as ready")
	}
}

// A refresh retires the passport and mints its successor on the same
// connection. The successor spends the call, so the preview still holds.
func TestAPromotionOutlivesARefreshOfItsAgentsPassport(t *testing.T) {
	s := newPromotionSight(t, "admin", approverIsAdmin, "admin")
	passport, grant := seedConnectionPassport(t, s.e)
	s.agentOn(passport)
	rep1 := ids.From[ids.UserKind](s.e.Rep1)
	lead := s.lead(contacts.CreateLeadInput{FullName: new("Ren Refresh"), Email: new("ren@gmail.com"), Source: "manual", OwnerID: &rep1})
	id := s.stage(lead, fmt.Sprintf(`{"lead_id":%q,"trigger":"human_qualify"}`, lead.UUID))
	successor := ids.NewV7()
	s.e.WsExec(t, `INSERT INTO passport (id, on_behalf_of, granted_by, label, scopes, token_hash, expires_at, oauth_grant_id)
	               VALUES ($1, $2, $2, 'oauth:claude', ARRAY['read']::text[], $3, now() + interval '1 day', $4)`,
		successor, s.e.Rep1, "hash-"+successor.String(), grant)
	s.e.WsExec(t, `UPDATE passport SET revoked_at = now() WHERE id = $1`, passport)
	if state := s.preview(id).State; state != "ready" {
		t.Fatalf("preview after a refresh = %s, want ready", state)
	}
	if _, err := s.svc.Decide(s.approver, id, true, nil); err != nil {
		t.Fatalf("approving after a refresh: %v", err)
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
