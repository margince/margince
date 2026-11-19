// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

func tierCatalog() importCatalog {
	return importCatalog{
		byName: map[string]fieldcatalog.Column{"cf_tier": {Name: "cf_tier", Type: fieldcatalog.TypePicklist, Options: []string{"A", "B"}}},
		labels: map[string]string{"cf_tier": "Tier"},
	}
}

func TestACustomFieldIsSuggestedByItsLabelOrItsKey(t *testing.T) {
	cat := tierCatalog()
	targets, err := importTargets(migration.ObjectCompany, cat)
	if err != nil || !slices.Contains(targets, "cf_tier") {
		t.Fatalf("targets = %v, err %v", targets, err)
	}
	for header, want := range map[string]string{"TIER": "cf_tier", " tier ": "cf_tier", "cf_tier ": "cf_tier"} {
		p := migration.Profile{Columns: []migration.Column{{Header: header}, {Header: "Name"}}}
		if got := suggestImportMapping(p, migration.ObjectCompany, targets, cat); got[header] != want {
			t.Errorf("suggestion for %q = %v, want %s", header, got, want)
		}
	}
}

// Two headers onto one field would be refused at staging, so the suggestion
// leaves both blank. SuggestMapping does the same for one core field.
func TestALabelAndAKeyHeaderForOneFieldSuggestNeither(t *testing.T) {
	cat := tierCatalog()
	targets, err := importTargets(migration.ObjectCompany, cat)
	if err != nil {
		t.Fatalf("importTargets: %v", err)
	}
	p := migration.Profile{Columns: []migration.Column{{Header: "Tier"}, {Header: "cf_tier"}}}
	if got := suggestImportMapping(p, migration.ObjectCompany, targets, cat); len(got) != 0 {
		t.Fatalf("suggestion = %v, want none for an ambiguous field", got)
	}
}

// A label that spells a core field loses to it: the core field is what the
// header names first.
func TestACoreFieldKeepsItsHeaderOverACustomLabel(t *testing.T) {
	cat := importCatalog{
		byName: map[string]fieldcatalog.Column{"cf_email": {Name: "cf_email", Type: fieldcatalog.TypeText}},
		labels: map[string]string{"cf_email": "Email"},
	}
	targets, err := importTargets(migration.ObjectLead, cat)
	if err != nil {
		t.Fatalf("importTargets: %v", err)
	}
	p := migration.Profile{Columns: []migration.Column{{Header: "Email"}}}
	if got := suggestImportMapping(p, migration.ObjectLead, targets, cat); got["Email"] != fieldEmail {
		t.Fatalf("suggestion = %v, want Email onto the core email", got)
	}
}

func TestAnUnchangedCustomFieldIsNotRewritten(t *testing.T) {
	stored := []byte(`{"display_name":"Acme","cf_tier":"A"}`)
	changed, err := changedFields(stored, map[string]string{"cf_tier": "a"}, map[string]any{"cf_tier": "A"})
	if err != nil || len(changed) != 0 {
		t.Fatalf("changed = %v, err %v; want none for the stored value", changed, err)
	}
	changed, err = changedFields(stored, map[string]string{"cf_tier": "B"}, map[string]any{"cf_tier": "B"})
	if err != nil || changed["cf_tier"] != "B" {
		t.Fatalf("changed = %v, err %v; want cf_tier", changed, err)
	}
}

// A typed value is compared in the shape the store holds, so "Yes" against a
// stored true, or 12000.50 against stored minor units, is no change.
func TestATypedCustomFieldComparesInTheStoredShape(t *testing.T) {
	stored := []byte(`{"cf_active":true,"cf_value":1200050,"cf_comp":["HubSpot","Other"],"cf_seats":24}`)
	mapped := map[string]string{"cf_active": "Yes", "cf_value": "12000.50", "cf_comp": "hubspot;other", "cf_seats": "24"}
	typed := map[string]any{"cf_active": true, "cf_value": float64(1200050), "cf_comp": []string{"HubSpot", "Other"}, "cf_seats": "24"}
	changed, err := changedFields(stored, mapped, typed)
	if err != nil || len(changed) != 0 {
		t.Fatalf("changed = %v, err %v; want none", changed, err)
	}
}

func TestOnlyCheckedCustomFieldsReachCustomFields(t *testing.T) {
	typed := map[string]any{"cf_tier": "A", "cf_active": true}
	got := customFieldsFrom(map[string]string{"cf_tier": "a", "display_name": "Acme"}, typed)
	if len(got) != 1 || got["cf_tier"] != "A" {
		t.Fatalf("custom fields = %v, want only cf_tier, typed", got)
	}
	if customFieldsFrom(map[string]string{"display_name": "Acme"}, typed) != nil {
		t.Fatal("a row with no custom field gave a non-nil map, which a caller-opened create reads as values")
	}
}

// A file carrying both the common header and the field's own name gives the
// field two columns, so neither is suggested.
func TestANameHeaderBesideTheFieldsOwnNameSuggestsNeither(t *testing.T) {
	targets, err := importTargets(migration.ObjectCompany, importCatalog{})
	if err != nil {
		t.Fatalf("importTargets: %v", err)
	}
	p := migration.Profile{Columns: []migration.Column{{Header: "name"}, {Header: "display_name"}}}
	if got := suggestImportMapping(p, migration.ObjectCompany, targets, importCatalog{}); len(got) != 0 {
		t.Fatalf("suggestion = %v, want none for a field two columns claim", got)
	}
}

// Another system's export, as a companies file: its fields by label, language or
// common name, and its own "ID", which is no Margince ID.
func TestAnotherSystemsHeadersAreSuggestedByLabelOrCommonName(t *testing.T) {
	targets, err := importTargets(migration.ObjectCompany, importCatalog{})
	for _, c := range []struct{ header, samples, want string }{
		{"Company Name", "Acme", fieldDisplayName},
		{"Website", "acme.com", fieldDomain},
		{"Telefon", "+49", fieldPhone},
		{"Adresse", "Kai 1", "address.line1"},
		{"E-Mail", "a@acme.com", fieldEmail},
		{"Owner", "anna", ""},
		// A guarded field is suggested only for values it takes: version-7 IDs, and bands.
		{"ID", "H-1 H-2", ""},
		{"ID", "0190a3c2-6b1e-7c3d-8e4f-5a6b7c8d9e0f", csvTargetID},
		{"ID", "3f2504e0-4f89-41d3-9a0c-0305e82c3301", ""},
		{"ID", "", ""},
		{"Employees", "11-50 51-200", fieldSizeBand},
		{"Number of employees", "40 120", ""},
	} {
		p := migration.Profile{Columns: []migration.Column{{Header: c.header, Samples: strings.Fields(c.samples)}}}
		if got := suggestImportMapping(p, migration.ObjectCompany, targets, importCatalog{}); err != nil || got[c.header] != c.want {
			t.Errorf("%s of %q → %q (err %v), want %q", c.header, c.samples, got[c.header], err, c.want)
		}
	}
}
