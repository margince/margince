// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"slices"
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
		if got := suggestImportMapping(p, targets, cat); got[header] != want {
			t.Errorf("suggestion for %q = %v, want %s", header, got, want)
		}
	}
}

// Two headers onto one field would be refused at staging, so the suggestion
// leaves both blank, as SuggestMapping does for two spellings of one core field.
func TestALabelAndAKeyHeaderForOneFieldSuggestNeither(t *testing.T) {
	cat := tierCatalog()
	targets, err := importTargets(migration.ObjectCompany, cat)
	if err != nil {
		t.Fatalf("importTargets: %v", err)
	}
	p := migration.Profile{Columns: []migration.Column{{Header: "Tier"}, {Header: "cf_tier"}}}
	if got := suggestImportMapping(p, targets, cat); len(got) != 0 {
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
	if got := suggestImportMapping(p, targets, cat); got["Email"] != fieldEmail {
		t.Fatalf("suggestion = %v, want Email onto the core email", got)
	}
}

func TestAnUnchangedCustomFieldIsNotRewritten(t *testing.T) {
	stored := []byte(`{"display_name":"Acme","cf_tier":"A"}`)
	changed, err := changedFields(stored, map[string]string{"cf_tier": "A"})
	if err != nil || len(changed) != 0 {
		t.Fatalf("changed = %v, err %v; want none for the stored value", changed, err)
	}
	changed, err = changedFields(stored, map[string]string{"cf_tier": "B"})
	if err != nil || changed["cf_tier"] != "B" {
		t.Fatalf("changed = %v, err %v; want cf_tier", changed, err)
	}
}

func TestOnlyCustomFieldTargetsReachCustomFields(t *testing.T) {
	got := customFieldsFrom(map[string]string{"cf_tier": "A", "display_name": "Acme", "cf_empty": " "})
	if len(got) != 1 || got["cf_tier"] != "A" {
		t.Fatalf("custom fields = %v, want only the filled cf_tier", got)
	}
	if customFieldsFrom(map[string]string{"display_name": "Acme"}) != nil {
		t.Fatal("a row with no custom field gave a non-nil map, which a caller-opened create reads as values")
	}
}
