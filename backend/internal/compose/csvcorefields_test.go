// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"reflect"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
)

func TestAPhoneAndALinkedInURLAreCheckedAndCarriedInTheStoresSpelling(t *testing.T) {
	for _, tc := range []struct {
		object, target, raw string
		want                any
		problem             string
	}{
		{migration.ObjectContact, fieldPhone, "+49 30 1234567", "+49301234567", ""},
		{migration.ObjectLead, fieldPhone, "0049 30 1234567", "+49301234567", ""},
		{migration.ObjectContact, fieldPhone, "030 1234567", nil, `“030 1234567” needs its country code, like +49 30 1234567`},
		{migration.ObjectContact, fieldPhone, "+49 30 1", nil, "not an E.164 number"},
		{migration.ObjectContact, fieldLinkedIn, "http://www.linkedin.com/in/ada/?trk=x", "https://www.linkedin.com/in/ada", ""},
		{migration.ObjectContact, fieldLinkedIn, "https://example.com/in/ada", nil, "linkedin.com"},
		{migration.ObjectLead, fieldLinkedIn, "linkedin.com/in/ada", "https://www.linkedin.com/in/ada", ""},
		{migration.ObjectCompany, fieldLinkedIn, "https://www.linkedin.com/company/voltaq/about", "https://www.linkedin.com/company/voltaq", ""},
		{migration.ObjectCompany, fieldLinkedIn, "https://www.linkedin.com/in/ada", nil, "linkedin.com/company/"},
	} {
		fields := map[string]string{fieldDisplayName: "Acme", fieldFullName: "Ada", tc.target: tc.raw}
		typed, problem := checkRow(tc.object, fields, importCatalog{})
		if tc.problem == "" {
			if problem != nil || !reflect.DeepEqual(typed[tc.target], tc.want) {
				t.Errorf("%s %s %q → %v, %+v; want %v", tc.object, tc.target, tc.raw, typed[tc.target], problem, tc.want)
			}
			continue
		}
		if problem == nil || problem.Target != tc.target || problem.Value != tc.raw || !strings.Contains(problem.Message, tc.problem) {
			t.Errorf("%s %s %q → problem %+v, want one on the target saying %q", tc.object, tc.target, tc.raw, problem, tc.problem)
		}
	}
}

func TestAnAddressCountryCellIsHeldToTheStoresRule(t *testing.T) {
	for _, object := range []string{migration.ObjectContact, migration.ObjectCompany} {
		for _, bad := range []string{"Vietnam", "zz", "DEU", "12"} {
			fields := map[string]string{fieldDisplayName: "Acme", fieldFullName: "Ada", fieldAddressCountry: bad}
			_, problem := checkRow(object, fields, importCatalog{})
			if problem == nil || problem.Target != fieldAddressCountry || problem.Value != bad || !strings.Contains(problem.Message, "ISO 3166-1") {
				t.Errorf("%s country %q → %+v, want a problem on the country cell naming the rule", object, bad, problem)
			}
		}
		for _, ok := range []string{"DE", "de", "", "  "} {
			fields := map[string]string{fieldDisplayName: "Acme", fieldFullName: "Ada", fieldAddressCountry: ok}
			if _, problem := checkRow(object, fields, importCatalog{}); problem != nil {
				t.Errorf("%s country %q refused: %+v", object, ok, problem)
			}
		}
	}
}

func TestACheckedPhoneComparesUnchangedAgainstTheStoredPrimary(t *testing.T) {
	stored := []byte(`{"phones":[{"phone":"+84281234","phone_type":"mobile","is_primary":true},` +
		`{"phone":"+49301234567","phone_type":"work","is_primary":true}],` +
		`"social":{"linkedin":"https://www.linkedin.com/in/ada"}}`)
	fields := map[string]string{fieldPhone: "+49 30 1234567", fieldLinkedIn: "http://www.linkedin.com/in/ada/?trk=x"}
	typed, problem := checkRow(migration.ObjectContact, fields, importCatalog{})
	if problem != nil {
		t.Fatal(problem)
	}
	changed, err := changedFields(stored, fields, typed)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 0 {
		t.Fatalf("changed = %v, want none: the stored primary work number and profile are these", changed)
	}
}

func TestAFilePhoneBecomesThePrimaryWorkNumberAndKeepsTheOthers(t *testing.T) {
	stored := []byte(`{"phones":[{"phone":"+49301234567","phone_type":"work","is_primary":true},` +
		`{"phone":"+842812345678","phone_type":"mobile","is_primary":true}]}`)
	merged, given, err := phonesMergedOnto(stored, contactPhonesFrom(map[string]string{fieldPhone: "+49307654321"}, nil))
	if err != nil || !given {
		t.Fatalf("given %v err %v", given, err)
	}
	want := []contacts.ContactPhoneInput{
		{Phone: "+49307654321", PhoneType: contactPhoneTypeWork, IsPrimary: true, Position: 1},
		{Phone: "+49301234567", PhoneType: contactPhoneTypeWork, IsPrimary: false, Position: 2},
		{Phone: "+842812345678", PhoneType: "mobile", IsPrimary: true, Position: 3},
	}
	if !reflect.DeepEqual(merged, want) {
		t.Fatalf("merged = %+v\nwant %+v", merged, want)
	}
	if _, given, _ := phonesMergedOnto(stored, nil); given {
		t.Fatal("a file with no phone column rewrote the stored numbers")
	}
}

func TestAFileLinkedInURLReplacesOnlyThatSocialHandle(t *testing.T) {
	stored := []byte(`{"social":{"linkedin":"https://www.linkedin.com/in/old","x":"@ada"}}`)
	merged, given, err := socialMergedOnto(stored, map[string]any{contacts.SocialLinkedIn: "https://www.linkedin.com/in/ada"})
	if err != nil || !given {
		t.Fatalf("given %v err %v", given, err)
	}
	want := map[string]any{contacts.SocialLinkedIn: "https://www.linkedin.com/in/ada", "x": "@ada"}
	if !reflect.DeepEqual(merged, want) {
		t.Fatalf("merged = %v, want %v", merged, want)
	}
	if _, given, _ := socialMergedOnto(stored, nil); given {
		t.Fatal("a file with no LinkedIn column rewrote the stored handles")
	}
}

// An update row that names only a city carries the stored country forward,
// whatever it is. The store then sees it unchanged and does not hold a row
// written before the rule to it.
func TestACsvUpdateCarriesAStoredLegacyCountryForward(t *testing.T) {
	city := "Berlin"
	merged, changed, err := addressMergedOnto([]byte(`{"address":{"city":"Bonn","country":"Germany"}}`), &crmcontracts.Address{City: &city}, map[string]string{"address.city": city})
	if err != nil || !changed || merged.Country == nil || *merged.Country != "Germany" || *merged.City != "Berlin" {
		t.Fatalf("merged = %+v (changed %v, err %v), want Berlin with the stored country kept", merged, changed, err)
	}
}
