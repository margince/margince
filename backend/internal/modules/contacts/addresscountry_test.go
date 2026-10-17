// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func countryOf(value string) *crmcontracts.Address {
	return &crmcontracts.Address{Country: &value}
}

// The contract declares an address country as an alpha-2 code, so the store
// holds the code and nothing that merely resembles one.
func TestACountryThatIsNotAnAssignedAlpha2CodeIsRefused(t *testing.T) {
	for _, bad := range []string{"Vietnam", "12", "zz", "DEU", "D", "", "  ", "D1", "XX"} {
		_, err := checkedAddress(countryOf(bad))
		if _, ok := errors.AsType[*InvalidCountryError](err); !ok {
			t.Errorf("country %q: err = %v, want an invalid-country refusal", bad, err)
		}
	}
}

func TestACountryCodeIsStoredUpperCase(t *testing.T) {
	for in, want := range map[string]string{"DE": "DE", "de": "DE", "vn": "VN", "Us": "US"} {
		got, err := checkedAddress(countryOf(in))
		if err != nil || got.Country == nil || *got.Country != want {
			t.Errorf("country %q: got %v / %v, want %q", in, got, err, want)
		}
	}
}

func TestAnAddressWithoutACountryPassesUntouched(t *testing.T) {
	city := "Leipzig"
	in := &crmcontracts.Address{City: &city}
	got, err := checkedAddress(in)
	if err != nil || got != in {
		t.Fatalf("an address with no country: got %v / %v, want it back unchanged", got, err)
	}
	if got, err := checkedAddress(nil); err != nil || got != nil {
		t.Fatalf("no address: got %v / %v, want nil", got, err)
	}
}

func TestCheckingAnAddressLeavesTheCallersCopyAlone(t *testing.T) {
	in := countryOf("de")
	if _, err := checkedAddress(in); err != nil {
		t.Fatal(err)
	}
	if *in.Country != "de" {
		t.Fatalf("the caller's address now reads %q; the check must work on a copy", *in.Country)
	}
}

func TestTheInvalidCountryFaultNamesTheNestedField(t *testing.T) {
	field, code, message := (&InvalidCountryError{}).FieldFault()
	if field != "address.country" || code != "invalid_country_code" || message == "" {
		t.Fatalf("fault = %q %q %q", field, code, message)
	}
}
