// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// The offering a company is judged against: which legal entity's profile a
// context describes, and the company write that chooses it.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func describedEntity(t *testing.T, e *promoteConsentEnv, offering LegalEntityOffering) LegalEntity {
	t.Helper()
	entity, err := e.store.CreateLegalEntity(actingAs(e, "admin", entityAdmin), LegalEntityInput{
		DisplayName: new("Wien"), LegalName: new("Wien GmbH"), Country: new("at"), Offering: offering,
	})
	if err != nil {
		t.Fatalf("create the entity: %v", err)
	}
	return entity
}

func offeringOf(t *testing.T, e *promoteConsentEnv, selector OfferingSelector) CompanyContext {
	t.Helper()
	got, err := e.store.GetCompanyContext(actingAs(e, "admin", entityAdmin), nil, selector)
	if err != nil {
		t.Fatalf("read the context of %v: %v", selector, err)
	}
	return got
}

func TestAnEntityOfferingIsReadAsItsOwnConfirmedContext(t *testing.T) {
	e := setupPromoteConsent(t)
	admin := actingAs(e, "admin", entityAdmin)
	if _, err := e.store.WithInstallationRenamer(ignoreInstallationName).SaveCompany(admin, SaveCompanyInput{
		DisplayName: "Heatwave AG",
		Fields:      map[string]*string{fieldOfferSummary: new("Heat pumps"), fieldICP: new("Plant operators")},
	}); err != nil {
		t.Fatalf("describe the company: %v", err)
	}
	wien := describedEntity(t, e, LegalEntityOffering{OfferSummary: new("Solar carports"), ICP: new("Car dealers")})

	main := offeringOf(t, e, OfferingSelector{})
	second := offeringOf(t, e, OfferingSelector{EntityID: wien.ID})

	if !main.Confirmed || !second.Confirmed {
		t.Errorf("confirmed = main %v, entity %v; both are described", main.Confirmed, second.Confirmed)
	}
	if main.Fingerprint == second.Fingerprint {
		t.Error("two different offerings share one fingerprint")
	}
	if second.OfferingID != wien.ID || second.OfferingName != "Wien" {
		t.Errorf("the entity's context names %v %q, want Wien", second.OfferingID, second.OfferingName)
	}
	if main.OfferingID != defaultEntity(t, e) || main.OfferingName != "Heatwave AG" {
		t.Errorf("the main context names %v %q, want the default entity and the company name", main.OfferingID, main.OfferingName)
	}
	if !second.CompanyID.IsZero() {
		t.Error("an entity's context carries a company id; only the main offering is a company")
	}
}

func TestAnEntityWithoutAnIdealCustomerIsNotConfirmed(t *testing.T) {
	e := setupPromoteConsent(t)
	wien := describedEntity(t, e, LegalEntityOffering{OfferSummary: new("Solar carports")})
	if offeringOf(t, e, OfferingSelector{EntityID: wien.ID}).Confirmed {
		t.Fatal("an offering with no ideal customer reads as confirmed")
	}
}

func TestADefaultArchivedOrUnknownEntityIsNotAnOtherOffering(t *testing.T) {
	e := setupPromoteConsent(t)
	admin := actingAs(e, "admin", entityAdmin)
	wien := describedEntity(t, e, LegalEntityOffering{})
	if _, err := e.store.ArchiveLegalEntity(admin, wien.ID, nil); err != nil {
		t.Fatalf("archive: %v", err)
	}
	for name, id := range map[string]ids.LegalEntityID{
		"default": defaultEntity(t, e), "archived": wien.ID, "unknown": ids.New[ids.LegalEntityKind](),
	} {
		if _, err := e.store.GetCompanyContext(admin, nil, OfferingSelector{EntityID: id}); !errors.Is(err, apperrors.ErrNotFound) {
			t.Errorf("%s entity answered %v, want not found", name, err)
		}
	}
}

func TestTheDefaultEntityRefusesOfferingTextBecauseTheCompanyFormOwnsIt(t *testing.T) {
	e := setupPromoteConsent(t)
	_, err := e.store.UpdateLegalEntity(actingAs(e, "admin", entityAdmin), defaultEntity(t, e),
		LegalEntityInput{Offering: LegalEntityOffering{OfferSummary: new("Heat pumps")}}, nil)
	if parseErr, ok := errors.AsType[*values.ParseError](err); !ok || parseErr.Code != codeFieldOnCompanyForm {
		t.Fatalf("answered %v, want the %s refusal", err, codeFieldOnCompanyForm)
	}
}

func TestChoosingAnOfferingIsAnAuditedEventedCompanyWrite(t *testing.T) {
	e := setupPromoteConsent(t)
	admin := actingAs(e, "admin", entityAdmin)
	wien := describedEntity(t, e, LegalEntityOffering{})
	company, err := e.store.CreateCompany(admin, CreateCompanyInput{DisplayName: "Autohaus", Source: "manual"})
	if err != nil {
		t.Fatalf("create the company: %v", err)
	}
	id := ids.From[ids.CompanyKind](ids.UUID(company.Id))

	chosen := wien.ID
	if _, err := e.store.UpdateCompany(admin, id, UpdateCompanyInput{AssessAgainstEntityID: &chosen}); err != nil {
		t.Fatalf("choose: %v", err)
	}
	var auditRows, events int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT (SELECT count(*) FROM audit_log WHERE entity_type = 'company' AND entity_id = $1 AND action = 'update'
		            AND after ? 'assess_against_entity_id'),
		        (SELECT count(*) FROM event_outbox WHERE envelope->'entity'->>'id' = $1::text AND envelope->>'type' = 'company.updated')`,
		id.UUID).Scan(&auditRows, &events); err != nil {
		t.Fatalf("count the write shape: %v", err)
	}
	if auditRows != 1 || events != 1 {
		t.Errorf("audit rows %d and events %d, want one of each", auditRows, events)
	}
	if _, err := e.store.ArchiveLegalEntity(admin, wien.ID, nil); err == nil {
		t.Error("archiving an entity that companies are still judged against was allowed")
	} else if refusal, ok := errors.AsType[*LegalEntityRefusal](err); !ok || refusal.code != "legal_entity_is_assessed_against" {
		t.Errorf("archive answered %v, want the legal_entity_is_assessed_against refusal", err)
	}
}

func TestAnUnknownOrArchivedEntityCannotBeChosen(t *testing.T) {
	e := setupPromoteConsent(t)
	admin := actingAs(e, "admin", entityAdmin)
	archived := describedEntity(t, e, LegalEntityOffering{})
	if _, err := e.store.ArchiveLegalEntity(admin, archived.ID, nil); err != nil {
		t.Fatalf("archive: %v", err)
	}
	company, err := e.store.CreateCompany(admin, CreateCompanyInput{DisplayName: "Autohaus", Source: "manual"})
	if err != nil {
		t.Fatalf("create the company: %v", err)
	}
	for name, entity := range map[string]ids.LegalEntityID{"archived": archived.ID, "unknown": ids.New[ids.LegalEntityKind]()} {
		_, err := e.store.UpdateCompany(admin, ids.From[ids.CompanyKind](ids.UUID(company.Id)),
			UpdateCompanyInput{AssessAgainstEntityID: &entity})
		if detailed, ok := errors.AsType[*httperr.DetailedError](err); !ok || detailed.Status != 422 {
			t.Errorf("%s entity answered %v, want 422", name, err)
		}
	}
}

func TestAnotherSeatWithoutCompanyWriteCannotChooseAnOffering(t *testing.T) {
	e := setupPromoteConsent(t)
	company, err := e.store.CreateCompany(actingAs(e, "admin", entityAdmin), CreateCompanyInput{DisplayName: "Autohaus", Source: "manual"})
	if err != nil {
		t.Fatalf("create the company: %v", err)
	}
	chosen := ids.New[ids.LegalEntityKind]()
	_, err = e.store.UpdateCompany(actingAs(e, "rep", entityRep), ids.From[ids.CompanyKind](ids.UUID(company.Id)),
		UpdateCompanyInput{AssessAgainstEntityID: &chosen})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("answered %v, want permission denied", err)
	}
}
