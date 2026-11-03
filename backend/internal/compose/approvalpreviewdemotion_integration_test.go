// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func TestDemotionApprovalDescribesTheContactItWillArchive(t *testing.T) {
	for _, rest := range []bool{false, true} {
		t.Run(fmt.Sprintf("rest=%t", rest), func(t *testing.T) {
			e := integration.Setup(t)
			perms := integration.AdminPerms
			perms.Objects = maps.Clone(perms.Objects)
			perms.Objects["custom_field"] = principal.ObjectGrant{Read: true}
			contactGrant := perms.Objects["contact"]
			contactGrant.Delete = false
			perms.Objects["contact"] = contactGrant
			ctx := e.As(e.AdminUser, nil, perms)
			lead, _, err := e.Contacts.CreateLead(ctx, contacts.CreateLeadInput{FullName: new("Dana Buyer"), Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			leadID := ids.From[ids.LeadKind](ids.UUID(lead.Id))
			contact, _, err := e.Contacts.PromoteLead(ctx, leadID, contacts.PromoteLeadInput{Trigger: "human_qualify"})
			if err != nil {
				t.Fatal(err)
			}
			assertDemotionSubjects(ctx, t, e, leadID.UUID)
			payload := fmt.Sprintf(`{"lead_id":%q,"reason":"Wrong prospect"}`, lead.Id)
			if rest {
				payload = `{"operation":"DemoteLead","body":{"reason":"Wrong prospect"}}`
			}
			raw, hash, err := diffhash.Canonical([]byte(payload))
			if err != nil {
				t.Fatal(err)
			}
			svc := NewApprovalsService(e.DB())
			id, err := svc.Stage(ctx, approvals.StageInput{Kind: "demote_lead", TargetType: "lead", TargetID: leadID.UUID, ProposedChange: raw, DiffHash: hash})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := svc.Stage(ctx, approvals.StageInput{Kind: "update_record", TargetType: "lead", TargetID: leadID.UUID, ProposedChange: []byte(`{"fields":{"full_name":"Changed"}}`), DiffHash: "hidden-archived-update"}); err != nil {
				t.Fatal(err)
			}
			assertDemotionListed(ctx, t, e, svc, leadID.UUID, id)
			assertDemotionWithheld(t, e, svc, leadID.UUID, id, perms)
			proposal, err := svc.GetWire(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			preview := proposal.Preview
			if preview == nil || preview.State != "ready" || preview.Details == nil || preview.Effects == nil || !slices.Contains(*preview.Effects, "restore_lead_archive_contact") {
				t.Fatalf("preview: %+v", preview)
			}
			details := *preview.Details
			if len(details) != 2 || details[0].Value.Text != "Wrong prospect" || details[1].Value.EntityId == nil || *details[1].Value.EntityId != contact.Id || details[1].Value.Text != "Dana Buyer" {
				t.Fatalf("details: %+v", details)
			}
			out, err := e.Contacts.DemoteLead(ctx, leadID, "Wrong prospect", contacts.OnlyAtVersion(new(int64(*proposal.TargetVersion))))
			if err != nil || out.Unwind != "reversed" {
				t.Fatalf("demotion: %+v, %v", out, err)
			}
		})
	}
}

func assertDemotionListed(ctx context.Context, t *testing.T, e *integration.Env, svc *approvals.Service, lead ids.UUID, id ids.ApprovalID) {
	t.Helper()
	for _, filter := range []approvals.ListInput{{}, {TargetType: new("lead"), TargetID: &lead}} {
		rows, _, err := svc.ListWire(ctx, filter)
		if err != nil || len(rows) != 1 || ids.UUID(rows[0].Id) != id.UUID {
			t.Fatalf("demotion inbox: %+v, %v", rows, err)
		}
	}
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		pending, err := svc.PendingForTarget(ctx, tx, "lead", lead)
		if err != nil {
			return err
		}
		rows := pending.Approvals
		if len(rows) != 1 || ids.UUID(rows[0].Id) != id.UUID {
			t.Fatalf("demotion panel: %+v", rows)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func assertDemotionSubjects(ctx context.Context, t *testing.T, e *integration.Env, lead ids.UUID) {
	t.Helper()
	req, body := doorRequest(http.MethodPost, "/v1/leads/"+lead.String()+"/demote", lead, `{"reason":"Wrong prospect"}`)
	call, err := demoteLeadCommand(agentPolicy{}, restCommandDeps{records: NewProviderFor(e.DB())}, req, body)
	if err != nil {
		t.Fatalf("decode REST reversal: %v", err)
	}
	if _, err := NewProviderFor(e.DB()).Read(ctx, datasource.EntityRef{Type: datasource.EntityLead, ID: lead}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("ordinary read exposed an archived lead: %v", err)
	}
	rest, err := agents.StageSubject(ctx, call)
	if err != nil {
		t.Fatalf("REST reversal subject: %v", err)
	}
	tool, err := stageToolSubject(ctx, NewRegistryFor(e.DB(), SendPath{}), "demote_lead", []byte(fmt.Sprintf(`{"lead_id":%q,"reason":"Wrong prospect"}`, lead)))
	if err != nil {
		t.Fatalf("tool reversal subject: %v", err)
	}
	if rest.TargetID != lead || tool.TargetID != lead || rest.Summary != tool.Summary || rest.TargetVersion == nil || tool.TargetVersion == nil || *rest.TargetVersion != *tool.TargetVersion {
		t.Fatalf("different reversal subjects: REST %+v, tool %+v", rest, tool)
	}
}

func assertDemotionWithheld(t *testing.T, e *integration.Env, svc *approvals.Service, lead ids.UUID, id ids.ApprovalID, perms principal.Permissions) {
	t.Helper()
	perms.Objects = maps.Clone(perms.Objects)
	grant := perms.Objects["contact"]
	grant.Update, grant.Delete = false, true
	perms.Objects["contact"] = grant
	ctx := e.As(e.AdminUser, nil, perms)
	if _, err := svc.GetWire(ctx, id); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("delete without update exposed reversal: %v", err)
	}
	for _, filter := range []approvals.ListInput{{}, {TargetType: new("lead"), TargetID: &lead}} {
		rows, _, err := svc.ListWire(ctx, filter)
		if err != nil || len(rows) != 0 {
			t.Fatalf("unauthorized reversal list: %+v, %v", rows, err)
		}
	}
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		pending, err := svc.PendingForTarget(ctx, tx, "lead", lead)
		if err != nil {
			return err
		}
		rows := pending.Approvals
		if len(rows) != 0 {
			t.Fatalf("unauthorized reversal panel: %+v", rows)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
