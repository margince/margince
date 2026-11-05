// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"fmt"
	"maps"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestContactMergePreviewMatchesRelationshipRetirementAndPrimaryChanges(t *testing.T) {
	for _, occupied := range []bool{false, true} {
		t.Run(fmt.Sprintf("primary=%t", occupied), func(t *testing.T) {
			e := integration.Setup(t)
			perms := integration.AdminPerms
			perms.Objects = maps.Clone(perms.Objects)
			perms.Objects["relationship"] = principal.ObjectGrant{Read: true, Create: true, Update: true, Delete: true}
			ctx := e.As(e.AdminUser, nil, perms)
			source := ids.From[ids.ContactKind](e.SeedContact(t, "Source contact", &e.AdminUser))
			target := ids.From[ids.ContactKind](e.SeedContact(t, "Surviving contact", &e.AdminUser))
			peer := ids.From[ids.ContactKind](e.SeedContact(t, "Shared colleague", &e.AdminUser))
			other := ids.From[ids.ContactKind](e.SeedContact(t, "Other colleague", &e.AdminUser))
			expected := map[ids.UUID]bool{}
			for i, pair := range [][2]ids.ContactID{{source, target}, {source, peer}, {peer, target}, {other, source}} {
				edge, err := e.Contacts.CreateRelationship(ctx, contacts.CreateRelationshipInput{Kind: "works_with", ContactID: &pair[0], CounterpartyContactID: &pair[1], Source: "manual"})
				if err != nil {
					t.Fatal(err)
				}
				if i != 2 {
					expected[edge.ID] = i < 2
				}
			}
			company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Source employer", &e.AdminUser))
			employment, err := e.Contacts.CreateRelationship(ctx, contacts.CreateRelationshipInput{Kind: "employment", ContactID: &source, CompanyID: &company, IsCurrentPrimary: new(true), Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			expected[employment.ID] = false
			var targetCompany ids.CompanyID
			if occupied {
				targetCompany = ids.From[ids.CompanyKind](e.SeedCompany(t, "Survivor employer", &e.Rep3))
				if _, err := e.Contacts.CreateRelationship(ctx, contacts.CreateRelationshipInput{Kind: "employment", ContactID: &target, CompanyID: &targetCompany, IsCurrentPrimary: new(true), Source: "manual"}); err != nil {
					t.Fatal(err)
				}
			}
			preview, err := e.Contacts.PreviewMergeSurvivorship(ctx, "contact", source.UUID, target.UUID, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(preview.Relationships) != len(expected) {
				t.Fatalf("relationships=%+v", preview.Relationships)
			}
			for _, edge := range preview.Relationships {
				id := ids.UUID(edge.Relationship.Id)
				retired, found := expected[id]
				if !found || edge.Retired != retired {
					t.Fatalf("disposition=%+v", edge)
				}
				if id == employment.ID && edge.WillBePrimary == occupied {
					t.Fatalf("primary disposition=%+v", edge)
				}
			}
			if occupied {
				colleague := e.As(e.Rep3, nil, perms)
				if _, err := e.Contacts.UpdateCompany(colleague, targetCompany, contacts.UpdateCompanyInput{Visibility: new("owner")}); err != nil {
					t.Fatal(err)
				}
				hidden, err := e.Contacts.PreviewMergeSurvivorship(ctx, "contact", source.UUID, target.UUID, nil, nil)
				if err == nil || hidden.Relationships != nil || hidden.Fields != nil {
					t.Fatalf("hidden primary employer disclosed: %+v err=%v", hidden, err)
				}
				if _, err := e.Contacts.UpdateCompany(colleague, targetCompany, contacts.UpdateCompanyInput{Visibility: new("workspace")}); err != nil {
					t.Fatal(err)
				}
			}
			denied := perms
			denied.Objects = maps.Clone(perms.Objects)
			denied.Objects["relationship"] = principal.ObjectGrant{}
			hidden, err := e.Contacts.PreviewMergeSurvivorship(e.As(e.AdminUser, nil, denied), "contact", source.UUID, target.UUID, nil, nil)
			if err == nil || hidden.Fields != nil || hidden.Relationships != nil {
				t.Fatalf("relationship scope disclosed: %+v err=%v", hidden, err)
			}
			if _, err := e.Contacts.MergeContact(ctx, source, target, &preview.SourceVersion); err != nil {
				t.Fatal(err)
			}
			if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
				for _, edge := range preview.Relationships {
					var retired, onTarget, primary bool
					if err := tx.QueryRow(ctx, `SELECT archived_at IS NOT NULL,COALESCE(contact_id=@target OR counterparty_contact_id=@target,false),is_current_primary FROM relationship WHERE id=@id`, pgx.StrictNamedArgs{"target": target, "id": edge.Relationship.Id}).Scan(&retired, &onTarget, &primary); err != nil {
						return err
					}
					if retired != edge.Retired || (!retired && (!onTarget || primary != edge.WillBePrimary)) {
						t.Errorf("actual retired=%t on target=%t primary=%t preview=%+v", retired, onTarget, primary, edge)
					}
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCompanyMergePreviewMatchesBothRelationshipEndpoints(t *testing.T) {
	e := integration.Setup(t)
	perms := integration.AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["relationship"] = principal.ObjectGrant{Read: true, Create: true, Update: true, Delete: true}
	ctx := e.As(e.AdminUser, nil, perms)
	source := ids.From[ids.CompanyKind](e.SeedCompany(t, "Source company", &e.AdminUser))
	target := ids.From[ids.CompanyKind](e.SeedCompany(t, "Surviving company", &e.AdminUser))
	peer := ids.From[ids.CompanyKind](e.SeedCompany(t, "Shared partner", &e.AdminUser))
	other := ids.From[ids.CompanyKind](e.SeedCompany(t, "Other partner", &e.Rep3))
	expected := map[ids.UUID]bool{}
	for i, pair := range [][2]ids.CompanyID{{source, target}, {source, peer}, {target, peer}, {other, source}} {
		edge, err := e.Contacts.CreateRelationship(ctx, contacts.CreateRelationshipInput{Kind: "partner_of", CompanyID: &pair[0], CounterpartyCompanyID: &pair[1], Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		if i != 2 {
			expected[edge.ID] = i < 2
		}
	}
	contact := ids.From[ids.ContactKind](e.SeedContact(t, "Company employee", &e.AdminUser))
	employment, err := e.Contacts.CreateRelationship(ctx, contacts.CreateRelationshipInput{Kind: "employment", ContactID: &contact, CompanyID: &source, IsCurrentPrimary: new(true), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	expected[employment.ID] = false
	preview, err := e.Contacts.PreviewMergeSurvivorship(ctx, "company", source.UUID, target.UUID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Relationships) != len(expected) {
		t.Fatalf("relationships=%+v", preview.Relationships)
	}
	for _, edge := range preview.Relationships {
		id := ids.UUID(edge.Relationship.Id)
		retired, found := expected[id]
		if !found || edge.Retired != retired || (id == employment.ID && !edge.WillBePrimary) {
			t.Fatalf("disposition=%+v", edge)
		}
	}
	colleague := e.As(e.Rep3, nil, perms)
	if _, err := e.Contacts.UpdateCompany(colleague, other, contacts.UpdateCompanyInput{Visibility: new("owner")}); err != nil {
		t.Fatal(err)
	}
	hidden, err := e.Contacts.PreviewMergeSurvivorship(ctx, "company", source.UUID, target.UUID, nil, nil)
	if err == nil || hidden.Fields != nil || hidden.Relationships != nil || hidden.TargetVersion != 0 {
		t.Fatalf("hidden endpoint leaked: %+v err=%v", hidden, err)
	}
	if _, err := e.Contacts.UpdateCompany(colleague, other, contacts.UpdateCompanyInput{Visibility: new("workspace")}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Contacts.MergeCompany(ctx, source, target, &preview.SourceVersion); err != nil {
		t.Fatal(err)
	}
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		for _, edge := range preview.Relationships {
			var retired, onTarget, primary bool
			if err := tx.QueryRow(ctx, `SELECT archived_at IS NOT NULL,COALESCE(company_id=@target OR counterparty_company_id=@target,false),is_current_primary FROM relationship WHERE id=@id`, pgx.StrictNamedArgs{"target": target, "id": edge.Relationship.Id}).Scan(&retired, &onTarget, &primary); err != nil {
				return err
			}
			if retired != edge.Retired || (!retired && (!onTarget || primary != edge.WillBePrimary)) {
				t.Errorf("actual retired=%t on target=%t primary=%t preview=%+v", retired, onTarget, primary, edge)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
