// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestRecordPreviewIncludesEmptyAndStructuredFields(t *testing.T) {
	for _, tc := range []struct {
		entity, field, value, kind string
		owner                      bool
	}{
		{"contact", "title", `"Head of sales"`, "text", true},
		{"contact", "title", `"Head of sales"`, "text", false},
		{"company", "address", `{"city":"Berlin"}`, "structured", true},
		{"company", "address", `{"city":"Berlin"}`, "structured", false},
		{"deal", "description", `"Renew the support agreement"`, "text", true},
		{"deal", "description", `"Renew the support agreement"`, "text", false},
	} {
		t.Run(fmt.Sprintf("%s/owner=%t", tc.entity, tc.owner), func(t *testing.T) {
			e := integration.Setup(t)
			var target ids.UUID
			switch tc.entity {
			case "contact":
				target = e.SeedContact(t, "Dana Buyer", &e.AdminUser)
			case "company":
				target = e.SeedCompany(t, "Harbor", &e.AdminUser)
			case "deal":
				pipeline, open, _ := integration.DealFixture(t, e)
				target = e.SeedDeal(t, "Harbor renewal", pipeline, open, &e.AdminUser)
			}
			fields := fmt.Sprintf(`%q:%s`, tc.field, tc.value)
			wantChanges := 1
			if tc.owner {
				fields += `,"owner_id":null`
				wantChanges++
			}
			raw, hash, err := diffhash.Canonical([]byte(fmt.Sprintf(`{"record_type":%q,"id":%q,"fields":{%s}}`, tc.entity, target, fields)))
			if err != nil {
				t.Fatal(err)
			}
			svc := NewApprovalsService(e.DB())
			id, err := svc.Stage(e.Admin(), approvals.StageInput{Kind: "update_record", TargetType: tc.entity, TargetID: target, ProposedChange: raw, DiffHash: hash})
			if err != nil {
				t.Fatal(err)
			}
			proposal, err := svc.GetWire(e.Admin(), id)
			if err != nil || proposal.Preview == nil || proposal.Preview.State != "ready" || len(proposal.Preview.Changes) != wantChanges {
				t.Fatalf("record preview: %+v, %v", proposal.Preview, err)
			}
			found := false
			for _, change := range proposal.Preview.Changes {
				if change.Field == tc.field {
					found = true
					if change.Before.Kind != "empty" || change.After.Kind != tc.kind || change.After.Text == "" {
						t.Fatalf("companion field: %+v", change)
					}
				}
			}
			if !found {
				t.Fatalf("preview omitted %s", tc.field)
			}
		})
	}
}

func TestOwnerPreviewAllowsReplacingAnArchivedOwnerWithoutRevealingTheirName(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	seats := identity.NewServiceFor(e.DB())
	if _, err := seats.SaveMyDisplayName(e.As(e.Rep2, []ids.UUID{e.Team1}, integration.AdminPerms), "Ben"); err != nil {
		t.Fatal(err)
	}
	company := e.SeedCompany(t, "Harbor", &e.Rep1)
	archive := func(owner ids.UUID) {
		t.Helper()
		// No application writer archives seats; this exercises retained legacy ownership.
		if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE app_user SET archived_at = now() WHERE id = @id`, pgx.StrictNamedArgs{"id": owner})
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	archive(e.Rep1)
	raw, hash, err := diffhash.Canonical([]byte(fmt.Sprintf(`{"record_type":"company","id":%q,"fields":{"owner_id":%q}}`, company, e.Rep2)))
	if err != nil {
		t.Fatal(err)
	}
	svc := NewApprovalsService(e.DB())
	id, err := svc.Stage(ctx, approvals.StageInput{Kind: "update_record", TargetType: "company", TargetID: company, ProposedChange: raw, DiffHash: hash})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := svc.GetWire(ctx, id)
	if err != nil || proposal.Preview == nil || proposal.Preview.State != "ready" || len(proposal.Preview.Changes) != 1 {
		t.Fatalf("replacement preview: %+v, %v", proposal.Preview, err)
	}
	change := proposal.Preview.Changes[0]
	if change.Before.Kind != "unavailable_name" || change.Before.Text != "" || change.After.Text != "Ben" {
		t.Fatalf("owner names: %+v", change)
	}
	archive(e.Rep2)
	proposal, err = svc.GetWire(ctx, id)
	if err != nil || proposal.Preview == nil || proposal.Preview.State != "incomplete" || len(proposal.Preview.Changes) != 0 {
		t.Fatalf("archived successor preview: %+v, %v", proposal.Preview, err)
	}
	var unavailable *approvals.PreviewContextError
	if _, err := svc.Decide(ctx, id, true, nil); !errors.As(err, &unavailable) {
		t.Fatalf("archived successor decision: %v", err)
	}
}

func TestReferencePreviewUsesCurrentAuthorizedCompanyNames(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	contact := e.SeedContact(t, "Dana", &e.AdminUser)
	company := e.SeedCompany(t, "Harbor", &e.AdminUser)
	raw, hash, err := diffhash.Canonical([]byte(fmt.Sprintf(`{"record_type":"contact","id":%q,"fields":{"owner_id":null,"company_id":%q}}`, contact, company)))
	if err != nil {
		t.Fatal(err)
	}
	svc := NewApprovalsService(e.DB())
	id, err := svc.Stage(ctx, approvals.StageInput{Kind: "update_record", TargetType: "contact", TargetID: contact, ProposedChange: raw, DiffHash: hash})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Contacts.UpdateCompany(ctx, ids.From[ids.CompanyKind](company), contacts.UpdateCompanyInput{DisplayName: new("Harbor renamed")}); err != nil {
		t.Fatal(err)
	}
	proposal, err := svc.GetWire(ctx, id)
	if err != nil || proposal.Preview == nil || proposal.Preview.State != "ready" {
		t.Fatalf("reference preview: %+v, %v", proposal.Preview, err)
	}
	found := false
	for _, change := range proposal.Preview.Changes {
		if change.Field != "company_id" {
			continue
		}
		found = true
		if change.After.Kind != "reference" || change.After.Text != "Harbor renamed" || change.After.EntityId == nil || ids.UUID(*change.After.EntityId) != company {
			t.Fatalf("reference value: %+v", change.After)
		}
	}
	if !found {
		t.Fatal("company reference missing")
	}
	if _, err := e.Contacts.ArchiveCompany(ctx, ids.From[ids.CompanyKind](company), nil); err != nil {
		t.Fatal(err)
	}
	proposal, err = svc.GetWire(ctx, id)
	if err != nil || proposal.Preview == nil || proposal.Preview.State != "incomplete" || len(proposal.Preview.Changes) != 0 {
		t.Fatalf("archived reference: %+v, %v", proposal.Preview, err)
	}
}
