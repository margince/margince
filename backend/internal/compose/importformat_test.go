// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/importguide"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

func TestEveryImportTargetHasAGuide(t *testing.T) {
	for object := range csvTargets {
		targets, err := importTargets(object, importCatalog{})
		if err != nil {
			t.Fatal(err)
		}
		for _, target := range targets {
			if _, ok := guideFor(object, target); !ok {
				t.Errorf("%s.%s has no guide; the screen would show an empty popover", object, target)
			}
		}
	}
}

func TestEveryGuideNamesATarget(t *testing.T) {
	// A guide for a column nothing imports is a promise the import does not keep.
	for key := range importguide.Columns {
		object, target, scoped := strings.Cut(key, ".")
		if !scoped || strings.HasPrefix(key, "address.") {
			target, object = key, ""
		}
		if !targetExistsSomewhere(t, object, target) {
			t.Errorf("guide %q names no import target", key)
		}
	}
}

// targetExistsSomewhere answers whether object imports target, or any object
// does when object is empty.
func targetExistsSomewhere(t *testing.T, object, target string) bool {
	t.Helper()
	for candidate := range csvTargets {
		if object != "" && candidate != object {
			continue
		}
		targets, err := importTargets(candidate, importCatalog{})
		if err != nil {
			t.Fatal(err)
		}
		if slices.Contains(targets, target) {
			return true
		}
	}
	return false
}

func TestTheSizeBandExampleNamesOnlyBandsTheImportTakes(t *testing.T) {
	for _, lang := range textlang.Shipped {
		for band := range strings.SplitSeq(importguide.Columns[fieldSizeBand].Example.In(lang), " · ") {
			if !importableSizeBands[band] {
				t.Errorf("%s example names %q, which the import refuses", lang, band)
			}
		}
	}
}

func TestTheFormatNamesEachFileSoAnUploadDetectsIt(t *testing.T) {
	f, err := formatFrom(map[string]importCatalog{}, langcopy.For("en"), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Order) != len(migration.ImportOrder) || len(f.Files) != len(migration.ImportOrder) {
		t.Fatalf("order %v and %d files, want %v", f.Order, len(f.Files), migration.ImportOrder)
	}
	for i, file := range f.Files {
		if string(f.Order[i]) != migration.ImportOrder[i] || file.Object != f.Order[i] {
			t.Errorf("file %d is %s under order %s, want %s", i, file.Object, f.Order[i], migration.ImportOrder[i])
		}
		if got := migration.DetectObject(file.File); got != string(file.Object) {
			t.Errorf("%s is detected as %q, want %s", file.File, got, file.Object)
		}
	}
	if f.SizeLimitMb != 10 || len(f.KeyRules) != len(importguide.KeyRules) {
		t.Errorf("size %d, %d key rules; want 10 and %d", f.SizeLimitMb, len(f.KeyRules), len(importguide.KeyRules))
	}
}

func TestACoreColumnIsItsTargetWithNoHeaderLabelAndAFieldLabelInTheCallersLanguage(t *testing.T) {
	f, err := formatFrom(map[string]importCatalog{}, langcopy.For("de"), 10)
	if err != nil {
		t.Fatal(err)
	}
	name := columnOf(t, f, crmcontracts.ImportObjectCompany, fieldDisplayName)
	if name.Label != nil || name.FieldLabel == nil || *name.FieldLabel != "Firmenname" || name.Custom || !name.RequiredOnNewRow ||
		name.Description != importguide.Columns["company.display_name"].Description.De {
		t.Fatalf("display_name = %+v, want the German company guide and field label, no header label, needed on a new row", name)
	}
	if email := columnOf(t, f, crmcontracts.ImportObjectContact, fieldEmail); email.RequiredOnNewRow {
		t.Errorf("a contact's email is marked needed on a new row: %+v", email)
	}
}

func TestACustomColumnIsNamedByItsKeyAndShowsItsLabel(t *testing.T) {
	tier := fieldcatalog.Column{Name: "cf_tier", Type: fieldcatalog.TypePicklist, Options: []string{"Gold", "Silver"}}
	cat := importCatalog{byName: map[string]fieldcatalog.Column{tier.Name: tier}, labels: map[string]string{tier.Name: "Tier"}, optionsReadable: true}
	f, err := formatFrom(map[string]importCatalog{migration.ObjectCompany: cat}, langcopy.For("vi"), 10)
	if err != nil {
		t.Fatal(err)
	}
	col := columnOf(t, f, crmcontracts.ImportObjectCompany, "cf_tier")
	if col.Label == nil || *col.Label != "Tier" || !col.Custom || col.Type == nil || *col.Type != fieldcatalog.TypePicklist {
		t.Fatalf("cf_tier = %+v, want label Tier, custom, picklist", col)
	}
	if col.Options == nil || !slices.Equal(*col.Options, tier.Options) || col.Example != "Gold" {
		t.Fatalf("cf_tier options %v example %q, want the field's options and the first as example", col.Options, col.Example)
	}
	if col.Description != importguide.Custom[fieldcatalog.TypePicklist].Description.Vi {
		t.Errorf("cf_tier description %q, want the Vietnamese picklist guide", col.Description)
	}
}

func TestTheFormatOmitsCustomColumnsFromACallerWithoutCustomFieldRead(t *testing.T) {
	tier := fieldcatalog.Column{Name: "cf_tier", Type: fieldcatalog.TypePicklist, Options: []string{"Gold", "Silver"}}
	cat := importCatalog{byName: map[string]fieldcatalog.Column{tier.Name: tier}}
	f, err := formatFrom(map[string]importCatalog{migration.ObjectCompany: cat}, langcopy.For("en"), 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range f.Files {
		for _, col := range file.Columns {
			if col.Custom || col.Name == tier.Name {
				t.Fatalf("%s lists %+v; a caller who may not read custom fields learns none of their keys or types", file.File, col)
			}
		}
	}
}

func columnOf(t *testing.T, f crmcontracts.ImportFormat, object crmcontracts.ImportObject, name string) crmcontracts.ImportFormatColumn {
	t.Helper()
	for _, file := range f.Files {
		if file.Object != object {
			continue
		}
		for _, col := range file.Columns {
			if col.Name == name {
				return col
			}
		}
	}
	t.Fatalf("%s has no column %s", object, name)
	return crmcontracts.ImportFormatColumn{}
}

func TestNoCoreColumnShowsItsInternalNameToTheCaller(t *testing.T) {
	f, err := formatFrom(map[string]importCatalog{}, langcopy.For("en"), 10)
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, file := range f.Files {
		for _, col := range file.Columns {
			seen++
			if col.FieldLabel == nil || *col.FieldLabel == "" || *col.FieldLabel == col.Name || strings.Contains(*col.FieldLabel, "_") {
				t.Errorf("%s.%s is labelled %v, which somebody choosing a field would have to translate", file.Object, col.Name, col.FieldLabel)
			}
		}
	}
	if seen == 0 {
		t.Fatal("the format listed no columns, so this check compared nothing")
	}
}
