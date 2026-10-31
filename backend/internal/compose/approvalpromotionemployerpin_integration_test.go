// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// pinnedPromotion is one lead an agent asks to promote, staged through one door.
type pinnedPromotion struct {
	t                         *testing.T
	e                         *integration.Env
	svc                       *approvals.Service
	agent, decider, colleague context.Context
	lead                      ids.LeadID
	domain                    string
	change                    json.RawMessage
	hash                      string
}

func newPinnedPromotion(t *testing.T, rest bool, trigger string) *pinnedPromotion {
	t.Helper()
	e := integration.Setup(t)
	perms := integration.AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["custom_field"] = principal.ObjectGrant{Read: true}
	passport := e.SeedPassport(t, integration.OwnerConn(t), "promotion pin")
	p := &pinnedPromotion{
		t: t, e: e, svc: NewApprovalsService(e.DB()),
		agent:     e.AgentOnPassport(t, e.Rep1, nil, perms, ids.From[ids.PassportKind](passport)),
		decider:   e.As(e.Rep1, nil, perms),
		colleague: e.As(e.AdminUser, nil, perms),
		domain:    "pinned-" + ids.NewV7().String()[24:] + ".test",
	}
	lead, _, err := e.Contacts.CreateLead(p.decider, contacts.CreateLeadInput{
		FullName: new("Pat Pinned"), Email: new("pat@" + p.domain), CompanyName: new("Pinned Works"), Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	p.lead = ids.From[ids.LeadKind](ids.UUID(lead.Id))
	payload := fmt.Sprintf(`{"lead_id":%q,"trigger":%q}`, lead.Id, trigger)
	if rest {
		payload = fmt.Sprintf(`{"operation":"PromoteLead","body":{"trigger":%q}}`, trigger)
	}
	if p.change, p.hash, err = diffhash.Canonical([]byte(payload)); err != nil {
		t.Fatal(err)
	}
	return p
}

// stage is the agent's refused call, and its identical retry.
func (p *pinnedPromotion) stage() ids.ApprovalID {
	p.t.Helper()
	id, _, err := p.svc.StageAgentCall(p.agent, approvals.StageInput{
		Kind: operationPromoteLead, TargetType: "lead", TargetID: p.lead.UUID, ProposedChange: p.change, DiffHash: p.hash,
	})
	if err != nil {
		p.t.Fatal(err)
	}
	return id
}

func (p *pinnedPromotion) preview(ctx context.Context, id ids.ApprovalID) crmcontracts.ApprovalPreview {
	p.t.Helper()
	proposal, err := p.svc.GetWire(ctx, id)
	if err != nil || proposal.Preview == nil {
		p.t.Fatalf("reading the promotion's preview: %+v, %v", proposal.Preview, err)
	}
	return *proposal.Preview
}

func (p *pinnedPromotion) requireReady(ctx context.Context, id ids.ApprovalID, effect string) {
	p.t.Helper()
	preview := p.preview(ctx, id)
	if preview.State != "ready" || preview.Effects == nil || !slices.Contains(*preview.Effects, effect) {
		p.t.Fatalf("preview = %s %v, want ready with %s", preview.State, preview.Effects, effect)
	}
}

func (p *pinnedPromotion) admit(admission, domain string) {
	p.t.Helper()
	if _, err := p.e.Contacts.SetDomainAdmission(p.decider, domain, admission, "a decision about this domain"); err != nil {
		p.t.Fatal(err)
	}
}

func (p *pinnedPromotion) approve(id ids.ApprovalID) error {
	_, err := p.svc.Decide(p.decider, id, true, nil)
	return err
}

// release redeems the decision as the agent and runs the promotion it releases.
func (p *pinnedPromotion) release(id ids.ApprovalID) error {
	p.t.Helper()
	proposal, err := p.svc.GetWire(p.decider, id)
	if err != nil {
		p.t.Fatal(err)
	}
	input, err := promotionApprovalInput(proposal)
	if err != nil {
		p.t.Fatal(err)
	}
	released, _, _, err := agents.RedeemAndMark(p.agent, approvalsAdapter{svc: p.svc}, id, operationPromoteLead, p.hash)
	if err != nil {
		return err
	}
	_, err = p.e.Contacts.QualifyLead(released, p.lead, input)
	return err
}

func (p *pinnedPromotion) requireUnpromoted(id ids.ApprovalID) {
	p.t.Helper()
	if n := p.e.WsCount(p.t, `SELECT count(*) FROM contact WHERE converted_from_lead_id = $1`, p.lead.UUID) +
		p.e.WsCount(p.t, `SELECT count(*) FROM company WHERE display_name = 'Pinned Works'`); n != 0 {
		p.t.Fatalf("a refused promotion wrote %d contacts and companies", n)
	}
	if p.e.WsCount(p.t, `SELECT count(*) FROM lead WHERE id = $1 AND status <> 'promoted'`, p.lead.UUID) != 1 {
		p.t.Fatal("a refused promotion closed the lead")
	}
	if p.e.WsCount(p.t, `SELECT count(*) FROM approval WHERE id = $1 AND consumed_at IS NULL`, id.UUID) != 1 {
		p.t.Fatal("a refused promotion spent its approval")
	}
}

func eachPromotionDoor(t *testing.T, run func(t *testing.T, rest bool)) {
	for _, rest := range []bool{false, true} {
		t.Run(fmt.Sprintf("rest=%t", rest), func(t *testing.T) { run(t, rest) })
	}
}

func requireSkew(t *testing.T, what string, err error) {
	t.Helper()
	if !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("%s = %v, want version skew", what, err)
	}
}

func TestAPromotionStagedOnABlockedDomainGoesStaleWhenTheBlockLifts(t *testing.T) {
	eachPromotionDoor(t, func(t *testing.T, rest bool) {
		p := newPinnedPromotion(t, rest, "human_qualify")
		p.admit(contacts.DomainSuppressed, p.domain)
		stale := p.stage()
		p.requireReady(p.decider, stale, "promote_propose_company")
		p.admit(contacts.DomainAdmitted, p.domain)
		if state := p.preview(p.decider, stale).State; state != "stale" {
			t.Fatalf("preview after the block lifted = %s, want stale", state)
		}
		requireSkew(t, "approving the stale promotion", p.approve(stale))
		p.requireUnpromoted(stale)

		fresh := p.stage()
		if fresh == stale {
			t.Fatal("the agent's retry rejoined the stale approval")
		}
		p.requireReady(p.decider, fresh, contacts.ContactEmployerCreateEffect)
	})
}

func TestAPromotionApprovedOnABlockedDomainIsNotRedeemedOnceTheBlockLifts(t *testing.T) {
	eachPromotionDoor(t, func(t *testing.T, rest bool) {
		p := newPinnedPromotion(t, rest, "human_qualify")
		p.admit(contacts.DomainSuppressed, p.domain)
		id := p.stage()
		if err := p.approve(id); err != nil {
			t.Fatal(err)
		}
		p.admit(contacts.DomainAdmitted, p.domain)
		requireSkew(t, "redeeming after the block lifted", p.release(id))
		p.requireUnpromoted(id)
	})
}

func TestAPromotionIsNotRedeemedOnceACompanyTakesItsDomain(t *testing.T) {
	eachPromotionDoor(t, func(t *testing.T, rest bool) {
		p := newPinnedPromotion(t, rest, "human_qualify")
		id := p.stage()
		if err := p.approve(id); err != nil {
			t.Fatal(err)
		}
		if _, err := p.e.Contacts.CreateCompany(p.decider, contacts.CreateCompanyInput{
			DisplayName: "Domain Holder", Source: "manual", Domains: []contacts.CompanyDomainInput{{Domain: p.domain, IsPrimary: true}},
		}); err != nil {
			t.Fatal(err)
		}
		requireSkew(t, "redeeming after a company took the domain", p.release(id))
		p.requireUnpromoted(id)
	})
}

// The digest leaves out who reads it and every fact the decision does not read.
func TestAPromotionStaysReadyAcrossReadersAndUnrelatedChanges(t *testing.T) {
	eachPromotionDoor(t, func(t *testing.T, rest bool) {
		p := newPinnedPromotion(t, rest, "human_qualify")
		id := p.stage()
		p.requireReady(p.colleague, id, contacts.ContactEmployerCreateEffect)
		p.admit(contacts.DomainSuppressed, "unrelated-"+p.domain)
		bystander := p.e.SeedCompany(t, "Bystander Ltd", nil)
		if _, err := p.e.Contacts.UpdateCompany(p.decider, ids.From[ids.CompanyKind](bystander), contacts.UpdateCompanyInput{DisplayName: new("Bystander Group")}); err != nil {
			t.Fatal(err)
		}
		p.requireReady(p.decider, id, contacts.ContactEmployerCreateEffect)
		if err := p.approve(id); err != nil {
			t.Fatal(err)
		}
		if err := p.release(id); err != nil {
			t.Fatalf("releasing a promotion nothing it reads has changed: %v", err)
		}
		if p.e.WsCount(t, `SELECT count(*) FROM lead WHERE id = $1 AND status = 'promoted'`, p.lead.UUID) != 1 {
			t.Fatal("the released promotion did not promote the lead")
		}
	})
}

func TestAPromotionStagedBeforeThePinReadsStaleAndTheRetryRestagesIt(t *testing.T) {
	eachPromotionDoor(t, func(t *testing.T, rest bool) {
		p := newPinnedPromotion(t, rest, "human_qualify")
		old := p.stage()
		p.e.WsExec(t, `UPDATE approval SET context_digest = NULL WHERE id = $1`, old.UUID)
		if state := p.preview(p.decider, old).State; state != "stale" {
			t.Fatalf("preview of a row with no digest = %s, want stale", state)
		}
		fresh := p.stage()
		if fresh == old {
			t.Fatal("the retry rejoined the row with no digest")
		}
		p.requireReady(p.decider, fresh, contacts.ContactEmployerCreateEffect)
	})
}

// The evidence the card showed is the evidence the promotion must record.
func TestAPromotionIsNotRedeemedOnceStrongerEvidenceArrives(t *testing.T) {
	eachPromotionDoor(t, func(t *testing.T, rest bool) {
		p := newPinnedPromotion(t, rest, "meeting_booked")
		meeting := func(status string) {
			id := ids.NewV7()
			p.e.WsExec(t, `INSERT INTO activity (id, kind, subject, meeting_status, occurred_at, source, captured_by, counterparty_email)
			               VALUES ($1, 'meeting', 'Intro call', $2, now(), 'manual', 'human:x', $3)`, id, status, "pat@"+p.domain)
			p.e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, lead_id) VALUES ($1, 'lead', $2)`, id, p.lead.UUID)
		}
		meeting("booked")
		id := p.stage()
		p.requireReady(p.decider, id, contacts.ContactEmployerCreateEffect)
		if err := p.approve(id); err != nil {
			t.Fatal(err)
		}
		meeting("held")
		requireSkew(t, "redeeming after stronger evidence arrived", p.release(id))
		p.requireUnpromoted(id)
	})
}
