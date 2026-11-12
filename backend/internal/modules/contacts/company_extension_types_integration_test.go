// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A company's extension types share a table with core's own but reach the wire in
// their own field. The closed core list never carries a value an extension made up.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/companytypes"
	"github.com/margince/margince/backend/pkg/extension"
)

const (
	resellerType = "ext:acme-pack:reseller"
	otherType    = "ext:acme-pack:distributor"
)

func declareAcmeTypes(t *testing.T) {
	t.Helper()
	companytypes.Reset()
	t.Cleanup(companytypes.Reset)
	companytypes.Register("acme-pack", []extension.CompanyType{
		{Type: "reseller", Label: "Reseller"}, {Type: "distributor", Label: "Distributor"},
	})
}

func newTypedCompany(ctx context.Context, t *testing.T, e *dedupeEnv, name string) ids.CompanyID {
	t.Helper()
	company, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: name, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	return ids.From[ids.CompanyKind](ids.UUID(company.Id))
}

func wantFieldError(t *testing.T, err error, field, code string) {
	t.Helper()
	var detailed *httperr.DetailedError
	if !errors.As(err, &detailed) || len(detailed.Fields) != 1 || detailed.Fields[0].Field != field || detailed.Fields[0].Code != code {
		t.Fatalf("err = %v, want a 422 on %s with code %s", err, field, code)
	}
}

func TestADeclaredExtensionTypeLandsInItsOwnFieldAndNotCoreList(t *testing.T) {
	declareAcmeTypes(t)
	e := setupDedupe(t)
	ctx := e.as()
	id := newTypedCompany(ctx, t, e, "Reseller GmbH")

	core, ext := []string{"customer"}, []string{resellerType}
	updated, err := e.store.UpdateCompany(ctx, id, UpdateCompanyInput{RelationshipTypes: &core, ExtensionRelationshipTypes: &ext})
	if err != nil {
		t.Fatalf("UpdateCompany: %v", err)
	}
	if updated.ExtensionRelationshipTypes == nil || len(*updated.ExtensionRelationshipTypes) != 1 || (*updated.ExtensionRelationshipTypes)[0] != resellerType {
		t.Errorf("extension_relationship_types = %v, want [%s]", updated.ExtensionRelationshipTypes, resellerType)
	}
	if updated.RelationshipTypes == nil || len(*updated.RelationshipTypes) != 1 || string((*updated.RelationshipTypes)[0]) != "customer" {
		t.Errorf("relationship_types = %v, want [customer] only", updated.RelationshipTypes)
	}

	var before, after []byte
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT before, after FROM audit_log
			WHERE entity_type = 'company' AND entity_id = $1 AND action = 'update'
			ORDER BY occurred_at DESC, id DESC LIMIT 1`, id.UUID).Scan(&before, &after)
	}); err != nil {
		t.Fatal(err)
	}
	var afterImage map[string]any
	if err := json.Unmarshal(after, &afterImage); err != nil {
		t.Fatal(err)
	}
	if got, _ := afterImage["extension_relationship_types"].([]any); len(got) != 1 || got[0] != resellerType {
		t.Errorf("audit after image extension_relationship_types = %v", afterImage["extension_relationship_types"])
	}
}

func TestAnExtensionTypeNoUnitDeclaredIsRefused(t *testing.T) {
	declareAcmeTypes(t)
	e := setupDedupe(t)
	ctx := e.as()
	id := newTypedCompany(ctx, t, e, "Undeclared GmbH")

	// A core word is refused here even when the company carries it, so the
	// extension field cannot be used to set `partner` around its partner row.
	core := []string{"customer"}
	if _, err := e.store.UpdateCompany(ctx, id, UpdateCompanyInput{RelationshipTypes: &core}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"ext:acme-pack:wholesaler", "ext:other-pack:reseller", "reseller", "ext:BAD", "customer", "partner"} {
		ext := []string{resellerType, bad}
		_, err := e.store.UpdateCompany(ctx, id, UpdateCompanyInput{ExtensionRelationshipTypes: &ext})
		wantFieldError(t, err, "extension_relationship_types", "invalid_enum")
	}
	if live := liveTypesOf(ctx, t, e, id); len(live) != 1 || !live["customer"] {
		t.Fatalf("a refused set changed the types to %+v", live)
	}
}

func TestACoreListStillRefusesAnExtensionValue(t *testing.T) {
	declareAcmeTypes(t)
	e := setupDedupe(t)
	ctx := e.as()
	id := newTypedCompany(ctx, t, e, "Closed List GmbH")

	core := []string{resellerType}
	_, err := e.store.UpdateCompany(ctx, id, UpdateCompanyInput{RelationshipTypes: &core})
	wantFieldError(t, err, "relationship_types", "invalid_enum")
}

func TestEachSetIsReplacedWithoutTouchingTheOther(t *testing.T) {
	declareAcmeTypes(t)
	e := setupDedupe(t)
	ctx := e.as()
	id := newTypedCompany(ctx, t, e, "Two Sets GmbH")

	core, ext := []string{"customer", "supplier"}, []string{resellerType}
	if _, err := e.store.UpdateCompany(ctx, id, UpdateCompanyInput{RelationshipTypes: &core, ExtensionRelationshipTypes: &ext}); err != nil {
		t.Fatal(err)
	}
	ext = []string{otherType}
	if _, err := e.store.UpdateCompany(ctx, id, UpdateCompanyInput{ExtensionRelationshipTypes: &ext}); err != nil {
		t.Fatal(err)
	}
	live := liveTypesOf(ctx, t, e, id)
	if !live["customer"] || !live["supplier"] || !live[otherType] || live[resellerType] || len(live) != 3 {
		t.Fatalf("after replacing only the extension set: %+v", live)
	}
	core = []string{"customer"}
	if _, err := e.store.UpdateCompany(ctx, id, UpdateCompanyInput{RelationshipTypes: &core}); err != nil {
		t.Fatal(err)
	}
	live = liveTypesOf(ctx, t, e, id)
	if !live["customer"] || live["supplier"] || !live[otherType] || len(live) != 2 {
		t.Fatalf("after replacing only the core set: %+v", live)
	}
}

func TestAnUnchangedSaveKeepsAnExtensionTypeItsUnitNoLongerDeclares(t *testing.T) {
	declareAcmeTypes(t)
	e := setupDedupe(t)
	ctx := e.as()
	id := newTypedCompany(ctx, t, e, "Orphaned GmbH")
	ext := []string{resellerType}
	if _, err := e.store.UpdateCompany(ctx, id, UpdateCompanyInput{ExtensionRelationshipTypes: &ext}); err != nil {
		t.Fatal(err)
	}

	companytypes.Reset()
	if _, err := e.store.UpdateCompany(ctx, id, UpdateCompanyInput{ExtensionRelationshipTypes: &ext}); err != nil {
		t.Fatalf("re-saving the type the company already carries: %v", err)
	}
	cleared := []string{}
	if _, err := e.store.UpdateCompany(ctx, id, UpdateCompanyInput{ExtensionRelationshipTypes: &cleared}); err != nil {
		t.Fatalf("clearing an undeclared type: %v", err)
	}
	if live := liveTypesOf(ctx, t, e, id); len(live) != 0 {
		t.Fatalf("types after clear = %+v", live)
	}
}

func TestPartnerStillNeedsItsRowWhenAnExtensionTypeIsSet(t *testing.T) {
	declareAcmeTypes(t)
	e := setupDedupe(t)
	ctx := asAdmin(e)
	id := newTypedCompany(ctx, t, e, "Partner Reseller GmbH")
	if _, err := e.store.UpsertPartner(ctx, UpsertPartnerInput{CompanyID: id, PartnerRole: "hosting"}); err != nil {
		t.Fatal(err)
	}

	ext := []string{resellerType}
	if _, err := e.store.UpdateCompany(ctx, id, UpdateCompanyInput{ExtensionRelationshipTypes: &ext}); err != nil {
		t.Fatalf("setting only an extension type on a partner: %v", err)
	}
	if live := liveTypesOf(ctx, t, e, id); !live["partner"] || !live[resellerType] {
		t.Fatalf("types = %+v, want partner kept beside the extension type", live)
	}
	without := []string{"customer"}
	_, err := e.store.UpdateCompany(ctx, id, UpdateCompanyInput{RelationshipTypes: &without, ExtensionRelationshipTypes: &ext})
	wantFieldError(t, err, "relationship_types", "partner_row_exists")
}

func TestTheExtensionRelationshipTypeFilterTakesAnExtensionValue(t *testing.T) {
	declareAcmeTypes(t)
	e := setupDedupe(t)
	ctx := e.as()
	typed := newTypedCompany(ctx, t, e, "Filtered In GmbH")
	other := newTypedCompany(ctx, t, e, "Filtered Out GmbH")
	ext, core := []string{resellerType}, []string{"customer"}
	if _, err := e.store.UpdateCompany(ctx, typed, UpdateCompanyInput{ExtensionRelationshipTypes: &ext}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.UpdateCompany(ctx, other, UpdateCompanyInput{RelationshipTypes: &core}); err != nil {
		t.Fatal(err)
	}

	want := resellerType
	listed, _, err := e.store.ListCompanies(ctx, ListCompaniesInput{ExtensionRelationshipType: &want})
	if err != nil {
		t.Fatalf("ListCompanies: %v", err)
	}
	if len(listed) != 1 || ids.UUID(listed[0].Id) != typed.UUID {
		t.Fatalf("filter %s listed %d companies, want the one that carries it", want, len(listed))
	}
	if listed[0].ExtensionRelationshipTypes == nil || len(*listed[0].ExtensionRelationshipTypes) != 1 {
		t.Errorf("listed company lost its extension types: %v", listed[0].ExtensionRelationshipTypes)
	}

	undeclared := "ext:acme-pack:wholesaler"
	_, _, err = e.store.ListCompanies(ctx, ListCompaniesInput{ExtensionRelationshipType: &undeclared})
	wantFieldError(t, err, "extension_relationship_type", "not_a_known_value")

	// The closed core filter stays closed.
	_, _, err = e.store.ListCompanies(ctx, ListCompaniesInput{RelationshipType: &want})
	wantFieldError(t, err, "relationship_type", "not_a_known_value")
}
