// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestLeadCreationApprovalShowsDefaultsBeforeTheRecordExists(t *testing.T) {
	for _, rest := range []bool{false, true} {
		t.Run(fmt.Sprintf("rest=%t", rest), func(t *testing.T) {
			e := integration.Setup(t)
			e.GrantRole(t, e.AdminUser, "admin")
			perms := integration.AdminPerms
			perms.Objects = maps.Clone(perms.Objects)
			perms.Objects["custom_field"] = principal.ObjectGrant{Read: true}
			ctx := e.As(e.AdminUser, nil, perms)
			fields := map[string]any{"full_name": "New Buyer", "email": "BUYER@EXAMPLE.TEST", "title": "Director", "source": "manual"}
			payload := map[string]any{"record_type": "lead", "fields": fields}
			if rest {
				payload = map[string]any{"operation": "createLead", "body": fields}
			}
			encoded, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			raw, hash, err := diffhash.Canonical(encoded)
			if err != nil {
				t.Fatal(err)
			}
			svc := NewApprovalsService(e.DB())
			approvalID, err := svc.Stage(ctx, approvals.StageInput{Kind: "create_record", TargetType: "lead", ProposedChange: raw, DiffHash: hash})
			if err != nil {
				t.Fatal(err)
			}
			proposal, err := svc.GetWire(ctx, approvalID)
			if err != nil {
				t.Fatal(err)
			}
			preview := proposal.Preview
			if preview == nil || preview.Version != 2 || preview.State != "ready" || preview.Details == nil {
				t.Fatalf("creation preview: %+v", preview)
			}
			values := map[string]string{}
			kinds := map[string]string{}
			for _, detail := range *preview.Details {
				values[detail.Field] = detail.Value.Text
				kinds[detail.Field] = detail.Value.Kind
			}
			if values["full_name"] != "New Buyer" || values["email"] != "buyer@example.test" || values["lead_status"] != "new" || kinds["owner_id"] != "empty" {
				t.Fatalf("creation defaults: %+v, %+v", values, kinds)
			}
			if n := e.WsCount(t, `SELECT count(*) FROM lead WHERE email='buyer@example.test'`); n != 0 {
				t.Fatalf("preview created %d leads", n)
			}
			in, err := leadCreationRequest(proposal)
			if err != nil {
				t.Fatal(err)
			}
			created, inserted, err := e.Contacts.CreateLead(ctx, in)
			if err != nil || !inserted {
				t.Fatalf("create: %v,%v", inserted, err)
			}
			if created.Email == nil || string(*created.Email) != values["email"] || created.OwnerId != nil || ids.UUID(created.Id).IsZero() {
				t.Fatalf("writer differs: %+v", created)
			}
		})
	}
}

func TestCompanyCreationApprovalMatchesTheWriter(t *testing.T) {
	for _, rest := range []bool{false, true} {
		t.Run(fmt.Sprintf("rest=%t", rest), func(t *testing.T) {
			e := integration.Setup(t)
			e.GrantRole(t, e.AdminUser, "admin")
			perms := integration.AdminPerms
			perms.Objects = maps.Clone(perms.Objects)
			perms.Objects["custom_field"] = principal.ObjectGrant{Read: true}
			ctx := e.As(e.AdminUser, nil, perms)
			fields := map[string]any{"display_name": "Approval Buyer", "source": "manual", "domains": []any{map[string]any{"domain": "WWW.APPROVAL-BUYER.TEST"}}, "address": map[string]any{"city": "Berlin"}}
			payload := map[string]any{"record_type": "company", "fields": fields}
			if rest {
				payload = map[string]any{"operation": "createCompany", "body": fields}
			}
			encoded, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			raw, hash, err := diffhash.Canonical(encoded)
			if err != nil {
				t.Fatal(err)
			}
			svc := NewApprovalsService(e.DB())
			approvalID, err := svc.Stage(ctx, approvals.StageInput{Kind: "create_record", TargetType: "company", ProposedChange: raw, DiffHash: hash})
			if err != nil {
				t.Fatal(err)
			}
			proposal, err := svc.GetWire(ctx, approvalID)
			if err != nil {
				t.Fatal(err)
			}
			preview := proposal.Preview
			if preview == nil || preview.State != "ready" || preview.Details == nil || preview.Effects == nil {
				t.Fatalf("preview: %+v", preview)
			}
			for _, effect := range []string{"create_company_record", "create_workspace_visible", "geocode_if_enabled"} {
				if !slices.Contains(*preview.Effects, effect) {
					t.Errorf("missing creation effect %s", effect)
				}
			}
			values := map[string]string{}
			for _, detail := range *preview.Details {
				values[detail.Field] = detail.Value.Text
			}
			if values["display_name"] != "Approval Buyer" || values["creation_primary_domain"] != "approval-buyer.test" || values["address_city"] != "Berlin" || values["lifecycle"] != "unknown" {
				t.Fatalf("values: %+v", values)
			}
			if n := e.WsCount(t, `SELECT count(*) FROM company WHERE display_name='Approval Buyer'`); n != 0 {
				t.Fatalf("preview created %d companies", n)
			}
			in, err := companyCreationRequest(proposal)
			if err != nil {
				t.Fatal(err)
			}
			created, err := e.Contacts.CreateCompany(ctx, in)
			if err != nil {
				t.Fatal(err)
			}
			if created.DisplayName != values["display_name"] || created.Lifecycle == nil || string(*created.Lifecycle) != values["lifecycle"] || created.Domains == nil || len(*created.Domains) != 1 || (*created.Domains)[0].Domain != values["creation_primary_domain"] || !(*created.Domains)[0].IsPrimary {
				t.Fatalf("writer: %+v", created)
			}
		})
	}
}

func TestContactCreationApprovalOverALiveLeadReportsTheRefusal(t *testing.T) {
	for _, rest := range []bool{false, true} {
		t.Run(fmt.Sprintf("rest=%t", rest), func(t *testing.T) {
			e := integration.Setup(t)
			e.GrantRole(t, e.AdminUser, "admin")
			perms := integration.AdminPerms
			perms.Objects = maps.Clone(perms.Objects)
			perms.Objects["custom_field"] = principal.ObjectGrant{Read: true}
			ctx := e.As(e.AdminUser, nil, perms)
			lead, _, err := e.Contacts.CreateLead(ctx, contacts.CreateLeadInput{FullName: new("Earlier Buyer"), Email: new("created-contact@example.test"), Title: new("Director"), Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			fields := map[string]any{"full_name": "Confirmed Buyer", "source": "manual", "emails": []any{map[string]any{"email": "CREATED-CONTACT@EXAMPLE.TEST", "email_type": "work", "is_primary": true}}, "social": map[string]any{"linkedin": "buyer-profile"}}
			payload := map[string]any{"record_type": "contact", "fields": fields}
			if rest {
				payload = map[string]any{"operation": "createContact", "body": fields}
			}
			encoded, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			raw, hash, err := diffhash.Canonical(encoded)
			if err != nil {
				t.Fatal(err)
			}
			service := NewApprovalsService(e.DB())
			id, err := service.Stage(ctx, approvals.StageInput{Kind: "create_record", TargetType: "contact", ProposedChange: raw, DiffHash: hash})
			if err != nil {
				t.Fatal(err)
			}
			proposal, err := service.GetWire(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			preview := proposal.Preview
			if preview == nil || preview.State != "incomplete" || preview.Conflicts == nil || len(*preview.Conflicts) != 1 ||
				(*preview.Conflicts)[0].Kind != contacts.CodeLeadHoldsEmail || !(*preview.Conflicts)[0].BlocksApproval {
				t.Fatalf("preview of a create over a live lead: %+v", preview)
			}
			if n := e.WsCount(t, `SELECT count(*) FROM lead WHERE id=$1 AND status='new' AND archived_at IS NULL`, lead.Id); n != 1 {
				t.Fatal("preview changed the lead")
			}
			in, err := contactCreationRequest(proposal)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := e.Contacts.CreateContact(ctx, in); !errors.As(err, new(*contacts.LeadHoldsIdentityError)) {
				t.Fatalf("the writer the preview stands for: %v", err)
			}
		})
	}
}
