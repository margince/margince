// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

func seatsCatalog() importCatalog {
	return importCatalog{byName: map[string]fieldcatalog.Column{
		"cf_seats": {Name: "cf_seats", Type: fieldcatalog.TypeNumber},
		"cf_tier":  {Name: "cf_tier", Type: fieldcatalog.TypePicklist, Options: []string{"Gold"}},
	}}
}

// firstOf is a check's typed cells and the first problem it found, for a test
// about one cell.
func firstOf(typed map[string]any, problems []*cellProblem) (map[string]any, *cellProblem) {
	if len(problems) == 0 {
		return typed, nil
	}
	return typed, problems[0]
}

// A row is checked to its last cell, so one upload names every cell to correct.
func TestEveryProblemOfARowIsListedCoreCellsFirst(t *testing.T) {
	fields := map[string]string{fieldDisplayName: "Acme", fieldSizeBand: "240", fieldDomain: "not a domain", "cf_seats": "many"}
	typed, problems := checkRow(migration.ObjectCompany, fields, seatsCatalog())
	var targets []string
	for _, problem := range problems {
		targets = append(targets, problem.Target)
	}
	if typed != nil || !reflect.DeepEqual(targets, []string{fieldDomain, fieldSizeBand, "cf_seats"}) {
		t.Fatalf("typed = %v, problems on %v; want none typed and the domain, size_band and cf_seats cells", typed, targets)
	}
}

func TestACheckedRowCarriesOnlyItsFilledCustomFieldsTyped(t *testing.T) {
	fields := map[string]string{fieldDisplayName: "Acme", "cf_tier": "gold", "cf_seats": ""}
	typed, problem := firstOf(checkRow(migration.ObjectCompany, fields, seatsCatalog()))
	if problem != nil || !reflect.DeepEqual(typed, map[string]any{"cf_tier": "Gold"}) {
		t.Fatalf("typed = %v, problem %+v; want only cf_tier in the catalog's spelling", typed, problem)
	}
	if typed, _ := firstOf(checkRow(migration.ObjectCompany, map[string]string{fieldDisplayName: "Acme"}, seatsCatalog())); typed != nil {
		t.Fatalf("typed = %v, want nil for a row with no custom field", typed)
	}
}

// A field retired after staging is no longer in the commit's catalog. Its
// value is refused rather than reaching a store that has no column for it.
func TestAValueForAFieldNoLongerActiveIsRefused(t *testing.T) {
	fields := map[string]string{fieldDisplayName: "Acme", "cf_gone": "x"}
	if _, problem := firstOf(checkRow(migration.ObjectCompany, fields, seatsCatalog())); problem == nil || problem.Target != "cf_gone" {
		t.Fatalf("problem = %+v, want one on cf_gone", problem)
	}
}

func TestAProblemNamesTheFilesOwnHeader(t *testing.T) {
	w := &csvWriters{columns: map[string]string{"Tier": "cf_tier", "Name": fieldDisplayName}}
	if got := w.headerFor("cf_tier"); got != "Tier" {
		t.Fatalf("headerFor(cf_tier) = %q, want Tier", got)
	}
}

// The column counts characters, so a description at the cap in
// multi-byte letters lands and one more character does not.
func TestADescriptionOverTheColumnsLengthIsAProblemOfItsCell(t *testing.T) {
	atCap := strings.Repeat("ü", contacts.CompanyDescriptionMax)
	if _, problem := firstOf(checkRow(migration.ObjectCompany, map[string]string{fieldDisplayName: "Acme", descriptionField: atCap}, importCatalog{})); problem != nil {
		t.Fatalf("problem = %+v, want none for a description at the cap", problem)
	}
	over := atCap + "x"
	_, problem := firstOf(checkRow(migration.ObjectCompany, map[string]string{fieldDisplayName: "Acme", descriptionField: over}, importCatalog{}))
	if problem == nil || problem.Target != descriptionField || problem.Why.Code != migration.ReasonDescriptionTooLong ||
		!strings.Contains(problem.Message, strconv.Itoa(contacts.CompanyDescriptionMax+1)) {
		t.Fatalf("problem = %+v, want the description refused with its length", problem)
	}
}

// The contacts store refuses a sending system's address, so the dry run skips
// the row with the same reason the commit gives. A lead may still hold one.
func TestAContactRowWithASendingSystemsAddressIsSkipped(t *testing.T) {
	fields := map[string]string{fieldFullName: "Signing-Noreply", fieldEmail: "signing-noreply@example.com"}
	_, problem := firstOf(checkRow(migration.ObjectContact, fields, importCatalog{}))
	if problem == nil || problem.Target != fieldEmail || problem.Why.Code != migration.ReasonEmailNamesNoContact {
		t.Fatalf("problem = %+v, want the email refused as %s", problem, migration.ReasonEmailNamesNoContact)
	}
	if _, problem := firstOf(checkRow(migration.ObjectLead, fields, importCatalog{})); problem != nil {
		t.Fatalf("lead problem = %+v, want none: only the contacts store refuses the address", problem)
	}
	fields[fieldEmail] = "anna.notify.weber@example.com"
	if _, problem := firstOf(checkRow(migration.ObjectContact, fields, importCatalog{})); problem != nil {
		t.Fatalf("problem = %+v, want none for a human whose name holds a marker word", problem)
	}
}

// A CRM's BCC logging address is refused the same way; a human at the
// provider's own domain is not.
func TestImportRefusesHubSpotBCC(t *testing.T) {
	fields := map[string]string{fieldFullName: "Logger", fieldEmail: "4711@bcc.eu1.hubspot.com"}
	if _, problem := firstOf(checkRow(migration.ObjectContact, fields, importCatalog{})); problem == nil ||
		problem.Why.Code != migration.ReasonEmailNamesNoContact {
		t.Fatalf("problem = %+v, want the BCC address refused", problem)
	}
	fields[fieldEmail] = "anna@hubspot.com"
	if _, problem := firstOf(checkRow(migration.ObjectContact, fields, importCatalog{})); problem != nil {
		t.Fatalf("problem = %+v, want none for a human at the provider", problem)
	}
}

// A created row that states both name parts takes its full name from them. An
// update never sends a full name that disagrees with the parts beside it. It
// sends new parts alone, and the store decides whether the display name follows.
func TestAnImportRowsNamePartsSetItsFullName(t *testing.T) {
	fields := map[string]string{fieldFullName: "Signing-Noreply", "first_name": "Contact", "last_name": "A"}
	if got := contactCreateFrom(fields, nil, "csv", "src").FullName; got != "Contact A" {
		t.Errorf("created full name = %q, want %q", got, "Contact A")
	}
	// The update compares the row as its parts name it, so a full name that
	// disagrees with unchanged parts is not sent on its own.
	current := []byte(`{"full_name":"Contact A","first_name":"Contact","last_name":"A"}`)
	if changed, err := changedFields(current, fields, nil); err != nil || len(changed) != 0 {
		t.Errorf("changed = %v (err %v), want nothing: the parts still name Contact A", changed, err)
	}
	renamed := map[string]string{"first_name": "Ada", "last_name": "Lovelace"}
	changed, err := changedFields(current, renamed, nil)
	if _, sent := changed[fieldFullName]; err != nil || sent || changed["first_name"] != "Ada" {
		t.Errorf("changed = %v (err %v), want the new parts and no full name", changed, err)
	}
	renamed[fieldFullName] = "Signing-Noreply"
	if changed, err := changedFields(current, renamed, nil); err != nil || changed[fieldFullName] != "" {
		t.Errorf("changed = %v (err %v), want the disagreeing full name left out", changed, err)
	}
	delete(fields, "last_name")
	if got := contactCreateFrom(fields, nil, "csv", "src").FullName; got != "Signing-Noreply" {
		t.Errorf("full name with one part = %q, want the row's own full name", got)
	}
}
