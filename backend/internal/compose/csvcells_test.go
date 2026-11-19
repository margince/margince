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

func TestEveryCellIsCheckedAgainstItsFieldsType(t *testing.T) {
	picklist := fieldcatalog.Column{Name: "cf_tier", Type: fieldcatalog.TypePicklist, Options: []string{"Gold", "Silver"}}
	multi := fieldcatalog.Column{Name: "cf_comp", Type: fieldcatalog.TypeMultiselect, Options: []string{"HubSpot", "Other"}}
	money := fieldcatalog.Column{Name: "cf_value", Type: fieldcatalog.TypeCurrency, Currency: "EUR"}
	vnd := fieldcatalog.Column{Name: "cf_vnd", Type: fieldcatalog.TypeCurrency, Currency: "VND"}
	noCurrency := fieldcatalog.Column{Name: "cf_amount", Type: fieldcatalog.TypeCurrency}
	date := fieldcatalog.Column{Name: "cf_renewal", Type: fieldcatalog.TypeDate}
	number := fieldcatalog.Column{Name: "cf_seats", Type: fieldcatalog.TypeNumber}
	flag := fieldcatalog.Column{Name: "cf_active", Type: fieldcatalog.TypeBoolean}
	for _, tc := range []struct {
		col     fieldcatalog.Column
		raw     string
		want    any
		problem string
	}{
		{picklist, "gold", "Gold", ""},
		{picklist, "  Silver ", "Silver", ""},
		{picklist, "Bronze", nil, "not one of the allowed values: Gold, Silver"},
		{multi, "hubspot; Other", []string{"HubSpot", "Other"}, ""},
		{multi, "HubSpot, Other", nil, "separate several values with semicolons"},
		{multi, "HubSpot; Bronze", nil, "not one of the allowed values: HubSpot, Other"},
		{money, "12000.50", float64(1200050), ""},
		{money, "12.000,50", nil, "not an amount in EUR"},
		{money, "€ 12,000", nil, "not an amount in EUR"},
		{money, "12k", nil, "not an amount in EUR"},
		{vnd, "1500000", float64(1500000), ""},
		{vnd, "1500000.5", nil, "not an amount in VND"},
		{noCurrency, "12000.50", nil, "has no currency set"},
		{date, "2026-03-02", "2026-03-02", ""},
		{date, "03/02/2026", nil, "Write it as YYYY-MM-DD"},
		{date, "2.3.2026", nil, "Write it as YYYY-MM-DD"},
		{date, "2026-03-02 10:00", nil, "Write it as YYYY-MM-DD"},
		{date, "45351", nil, "Write it as YYYY-MM-DD"},
		{number, "24", "24", ""},
		{number, "24 seats", nil, "is not a number"},
		{flag, "Yes", true, ""},
		{flag, "0", false, ""},
		{flag, "maybe", nil, "is not yes or no"},
	} {
		got, problem := checkCell(tc.col.Name, tc.col, tc.raw, true)
		if tc.problem == "" {
			if problem != nil || !reflect.DeepEqual(got, tc.want) {
				t.Errorf("%s %q → %v, %+v; want %v", tc.col.Type, tc.raw, got, problem, tc.want)
			}
			continue
		}
		if problem == nil || !strings.Contains(problem.Message, tc.problem) || problem.Value != tc.raw {
			t.Errorf("%s %q → problem %+v, want one saying %q and quoting the value", tc.col.Type, tc.raw, problem, tc.problem)
		}
		if problem != nil && problem.Target != tc.col.Name {
			t.Errorf("%s %q → problem on %q, want the target %q", tc.col.Type, tc.raw, problem.Target, tc.col.Name)
		}
	}
}

func TestAPicklistProblemNamesNoOptionToACallerWithoutCustomFieldRead(t *testing.T) {
	picklist := fieldcatalog.Column{Name: "cf_tier", Type: fieldcatalog.TypePicklist, Options: []string{"Gold", "Silver"}}
	multi := fieldcatalog.Column{Name: "cf_comp", Type: fieldcatalog.TypeMultiselect, Options: []string{"HubSpot", "Other"}}
	for _, tc := range []struct {
		col fieldcatalog.Column
		raw string
	}{{picklist, "Bronze"}, {multi, "HubSpot; Bronze"}} {
		_, problem := checkCell(tc.col.Name, tc.col, tc.raw, false)
		if problem == nil || !strings.Contains(problem.Message, "not one of this field’s allowed values") {
			t.Fatalf("%s %q → problem %+v, want the refusal without the list", tc.col.Type, tc.raw, problem)
		}
		for _, option := range tc.col.Options {
			if strings.Contains(problem.Message, option) && !strings.Contains(tc.raw, option) {
				t.Errorf("%s %q → %q names the option %q", tc.col.Type, tc.raw, problem.Message, option)
			}
		}
	}
}

func seatsCatalog() importCatalog {
	return importCatalog{byName: map[string]fieldcatalog.Column{
		"cf_seats": {Name: "cf_seats", Type: fieldcatalog.TypeNumber},
		"cf_tier":  {Name: "cf_tier", Type: fieldcatalog.TypePicklist, Options: []string{"Gold"}},
	}}
}

func TestACoreProblemWinsOverACustomFieldProblem(t *testing.T) {
	fields := map[string]string{fieldDisplayName: "Acme", fieldSizeBand: "240", "cf_seats": "many"}
	if _, problem := checkRow(migration.ObjectCompany, fields, seatsCatalog()); problem == nil || problem.Target != fieldSizeBand {
		t.Fatalf("problem = %+v, want the size_band one", problem)
	}
	delete(fields, fieldSizeBand)
	if _, problem := checkRow(migration.ObjectCompany, fields, seatsCatalog()); problem == nil || problem.Target != "cf_seats" {
		t.Fatalf("problem = %+v, want the cf_seats one", problem)
	}
}

func TestACheckedRowCarriesOnlyItsFilledCustomFieldsTyped(t *testing.T) {
	fields := map[string]string{fieldDisplayName: "Acme", "cf_tier": "gold", "cf_seats": ""}
	typed, problem := checkRow(migration.ObjectCompany, fields, seatsCatalog())
	if problem != nil || !reflect.DeepEqual(typed, map[string]any{"cf_tier": "Gold"}) {
		t.Fatalf("typed = %v, problem %+v; want only cf_tier in the catalog's spelling", typed, problem)
	}
	if typed, _ := checkRow(migration.ObjectCompany, map[string]string{fieldDisplayName: "Acme"}, seatsCatalog()); typed != nil {
		t.Fatalf("typed = %v, want nil for a row with no custom field", typed)
	}
}

// A field retired after staging is no longer in the commit's catalog. Its
// value is refused rather than reaching a store that has no column for it.
func TestAValueForAFieldNoLongerActiveIsRefused(t *testing.T) {
	fields := map[string]string{fieldDisplayName: "Acme", "cf_gone": "x"}
	if _, problem := checkRow(migration.ObjectCompany, fields, seatsCatalog()); problem == nil || problem.Target != "cf_gone" {
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
	if _, problem := checkRow(migration.ObjectCompany, map[string]string{fieldDisplayName: "Acme", descriptionField: atCap}, importCatalog{}); problem != nil {
		t.Fatalf("problem = %+v, want none for a description at the cap", problem)
	}
	over := atCap + "x"
	_, problem := checkRow(migration.ObjectCompany, map[string]string{fieldDisplayName: "Acme", descriptionField: over}, importCatalog{})
	if problem == nil || problem.Target != descriptionField || problem.Why.Code != migration.ReasonDescriptionTooLong ||
		!strings.Contains(problem.Message, strconv.Itoa(contacts.CompanyDescriptionMax+1)) {
		t.Fatalf("problem = %+v, want the description refused with its length", problem)
	}
}

// A contact name part keeps the capture bound, counted in characters. One at
// the bound lands; one past it is refused on its own cell with its length.
func TestAContactNameCellPastTheImportBoundIsAProblemOfItsCell(t *testing.T) {
	atCap := strings.Repeat("ü", contacts.ContactNameLimit)
	for _, target := range []string{fieldFullName, "first_name", "last_name"} {
		fields := map[string]string{fieldFullName: "Anna Weber", target: atCap}
		if _, problem := checkRow(migration.ObjectContact, fields, importCatalog{}); problem != nil {
			t.Fatalf("%s problem = %+v, want none for a name at the bound", target, problem)
		}
		fields[target] = atCap + "x"
		_, problem := checkRow(migration.ObjectContact, fields, importCatalog{})
		want := "This value is " + strconv.Itoa(contacts.ContactNameLimit+1) + " characters. The field holds at most " +
			strconv.Itoa(contacts.ContactNameLimit) + "."
		if problem == nil || problem.Target != target || problem.Why.Code != migration.ReasonValueTooLong || problem.Message != want {
			t.Fatalf("%s problem = %+v, want the cell refused as %q", target, problem, want)
		}
	}
}

// Two name parts that fit alone but join past the limit are refused on the
// file's last-name column, with the joined name as the value. A join at the limit lands.
func TestContactNamePartsJoiningPastTheLimitAreRefusedOnTheLastNameColumn(t *testing.T) {
	limit := contacts.ContactNameLimit
	w := &csvWriters{object: migration.ObjectContact, columns: map[string]string{"Vorname": "first_name", "Nachname": "last_name"}}
	fits := map[string]any{"first_name": strings.Repeat("a", limit/2), "last_name": strings.Repeat("b", limit-limit/2-1)}
	if _, skipped := w.checkedCells(migration.Row{ExternalID: "1", Line: 2, Fields: fits}); skipped != nil {
		t.Fatalf("skipped = %+v, want none for parts joining to %d characters", skipped, limit)
	}
	part := limit * 2 / 3
	first, last := strings.Repeat("a", part), strings.Repeat("b", part)
	_, skipped := w.checkedCells(migration.Row{ExternalID: "1", Line: 2, Fields: map[string]any{"first_name": first, "last_name": last}})
	want := "This value is " + strconv.Itoa(2*part+1) + " characters. The field holds at most " + strconv.Itoa(limit) + "."
	if skipped == nil || skipped.Column != "Nachname" || skipped.Value != first+" "+last ||
		skipped.Code != migration.ReasonValueTooLong || skipped.Reason != want {
		t.Fatalf("skipped = %+v, want the Nachname cell refused with the joined name as %q", skipped, want)
	}
}

// The contacts store refuses a sending system's address, so the dry run skips
// the row with the same reason the commit gives. A lead may still hold one.
func TestAContactRowWithASendingSystemsAddressIsSkipped(t *testing.T) {
	fields := map[string]string{fieldFullName: "Signing-Noreply", fieldEmail: "signing-noreply@example.com"}
	_, problem := checkRow(migration.ObjectContact, fields, importCatalog{})
	if problem == nil || problem.Target != fieldEmail || problem.Why.Code != migration.ReasonEmailNamesNoContact {
		t.Fatalf("problem = %+v, want the email refused as %s", problem, migration.ReasonEmailNamesNoContact)
	}
	if _, problem := checkRow(migration.ObjectLead, fields, importCatalog{}); problem != nil {
		t.Fatalf("lead problem = %+v, want none: only the contacts store refuses the address", problem)
	}
	fields[fieldEmail] = "anna.notify.weber@example.com"
	if _, problem := checkRow(migration.ObjectContact, fields, importCatalog{}); problem != nil {
		t.Fatalf("problem = %+v, want none for a human whose name holds a marker word", problem)
	}
}

// A CRM's BCC logging address is refused the same way; a human at the
// provider's own domain is not.
func TestImportRefusesHubSpotBCC(t *testing.T) {
	fields := map[string]string{fieldFullName: "Logger", fieldEmail: "4711@bcc.eu1.hubspot.com"}
	if _, problem := checkRow(migration.ObjectContact, fields, importCatalog{}); problem == nil ||
		problem.Why.Code != migration.ReasonEmailNamesNoContact {
		t.Fatalf("problem = %+v, want the BCC address refused", problem)
	}
	fields[fieldEmail] = "anna@hubspot.com"
	if _, problem := checkRow(migration.ObjectContact, fields, importCatalog{}); problem != nil {
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
