// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"errors"
	"reflect"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestLeadReferencesUseEmailWithoutChangingLegacyLabels(t *testing.T) {
	for _, name := range []string{"", "   ", "Named Lead"} {
		t.Run(name, func(t *testing.T) {
			e := setupCapturePrivacy(t)
			ctx := e.as(e.owner, principal.RowScopeAll)
			email := "reference@example.test"
			row, created, err := e.store.CreateLead(ctx, CreateLeadInput{FullName: &name, Email: &email, Source: "manual"})
			if err != nil || !created {
				t.Fatalf("create: %v %v", created, err)
			}
			id := ids.From[ids.LeadKind](ids.UUID(row.Id))
			detail, err := e.store.GetLead(ctx, id, storekit.IncludeArchived)
			if err != nil {
				t.Fatal(err)
			}
			got, err := e.store.LeadReferences(ctx, []ids.UUID{id.UUID})
			if err != nil || len(got) != 1 {
				t.Fatalf("references=%+v err=%v", got, err)
			}
			want := email
			if name == "Named Lead" {
				want = name
			}
			if got[0].Name == nil || *got[0].Name != want || *got[0].Name != leadIdentityName(detail) {
				t.Fatalf("reference=%+v, want %q", got, want)
			}
			legacy, err := e.store.LeadLabels(ctx, []ids.UUID{id.UUID})
			if err != nil {
				t.Fatal(err)
			}
			oldName := ""
			if detail.FullName != nil {
				oldName = *detail.FullName
			}
			if legacy[id.UUID] != oldName {
				t.Fatalf("legacy label=%q, primary name=%q", legacy[id.UUID], oldName)
			}
		})
	}
}

func TestNamelessLeadReferenceRetainsItsIdentity(t *testing.T) {
	e := setupCapturePrivacy(t)
	ctx := e.as(e.owner, principal.RowScopeAll)
	row, created, err := e.store.CreateLead(ctx, CreateLeadInput{CompanyName: new("Unidentified enquiry"), Source: "manual"})
	if err != nil || !created {
		t.Fatalf("create: %v %v", created, err)
	}
	got, err := e.store.LeadReferences(ctx, []ids.UUID{ids.UUID(row.Id)})
	if err != nil || len(got) != 1 || got[0].Id != row.Id || got[0].Name != nil || got[0].Archived {
		t.Fatalf("reference=%+v err=%v", got, err)
	}
	legacy, err := e.store.LeadLabels(ctx, []ids.UUID{ids.UUID(row.Id)})
	if err != nil || len(legacy) != 0 {
		t.Fatalf("legacy labels=%v err=%v", legacy, err)
	}
}

func TestCompanyReferencesPreserveAnchorAndLogoRevision(t *testing.T) {
	e := newAnchorEnv(t)
	actor, ok := principal.Actor(e.ctx)
	if !ok {
		t.Fatal("anchor fixture has no actor")
	}
	actor.Type = principal.PrincipalAgent
	actor.ID = "agent:test"
	actor.OnBehalfOf = actor.UserID
	actor.Scopes = principal.NewScopeSet(principal.ScopeRead, principal.ScopeWrite)
	logoContext := principal.WithActor(e.ctx, actor)
	for _, key := range []string{"", "first-reference-logo", "second-reference-logo"} {
		if key != "" {
			written, _, err := e.store.SetCompanyLogo(logoContext, e.anchorID, key, "https://example.test/mark.png")
			if err != nil || !written {
				t.Fatalf("logo write=%v err=%v", written, err)
			}
		}
		detail, err := e.store.GetCompany(e.ctx, e.anchorID, storekit.IncludeArchived)
		if err != nil {
			t.Fatal(err)
		}
		got, err := e.store.CompanyReferences(e.ctx, []ids.UUID{e.anchorID.UUID})
		if err != nil || len(got) != 1 {
			t.Fatalf("references=%+v err=%v", got, err)
		}
		if got[0].Name == nil || *got[0].Name != detail.DisplayName || !reflect.DeepEqual(got[0].Logo, detail.LogoUrl) {
			t.Fatalf("reference=%+v, company=%+v", got, detail)
		}
		if got[0].Logo != nil && *got[0].Logo == key {
			t.Fatal("reference exposed the storage key")
		}
	}
}

func TestCompanyReferencesRespectPrivateRowScope(t *testing.T) {
	e := setupCapturePrivacy(t)
	owner := e.as(e.owner, principal.RowScopeAll)
	company, err := e.store.CreateCompany(owner, CreateCompanyInput{DisplayName: "Private company", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.CompanyKind](ids.UUID(company.Id))
	if _, err := e.store.UpdateCompany(owner, id, UpdateCompanyInput{Visibility: new("owner")}); err != nil {
		t.Fatal(err)
	}
	other := e.as(e.admin, principal.RowScopeAll)
	if _, err := e.store.GetCompany(other, id, storekit.IncludeArchived); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("detail=%v", err)
	}
	got, err := e.store.CompanyReferences(other, []ids.UUID{id.UUID, ids.NewV7()})
	if err != nil || len(got) != 0 {
		t.Fatalf("hidden references=%+v err=%v", got, err)
	}
}

func TestCompanyReferencesAvoidFullListAttachmentReads(t *testing.T) {
	e := newAnchorEnv(t)
	id := e.newCompany(t)
	referenceCtx, referenceMeter := database.WithStatementMeter(e.ctx)
	references, err := e.store.CompanyReferences(referenceCtx, []ids.UUID{id.UUID})
	if err != nil || len(references) != 1 {
		t.Fatalf("references=%+v err=%v", references, err)
	}
	listCtx, listMeter := database.WithStatementMeter(e.ctx)
	companies, _, err := e.store.ListCompanies(listCtx, ListCompaniesInput{IDs: []ids.UUID{id.UUID}, IncludeArchived: true, IncludeAnchor: true})
	if err != nil || len(companies) != 1 {
		t.Fatalf("companies=%+v err=%v", companies, err)
	}
	if references[0].Name == nil || *references[0].Name != companies[0].DisplayName || !reflect.DeepEqual(references[0].Logo, companies[0].LogoUrl) {
		t.Fatalf("reference and list disagree: %+v %+v", references, companies)
	}
	compact, full := referenceMeter.Cost().Statements, listMeter.Cost().Statements
	if compact == 0 || compact >= full {
		t.Fatalf("reference statements=%d, list statements=%d", compact, full)
	}
}
