// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The deal list's country filter offers the codes the server stores, and no other.
// frontend/src/features/deals/assignedcountries.ts mirrors contacts.AssignedCountryCodes,
// and this gate fails when either side has a code the other lacks.

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
)

const frontendAssignedCountries = "../frontend/src/features/deals/assignedcountries.ts"

var tsCountryCode = regexp.MustCompile(`"([A-Z]{2})"`)

func TestTheFrontendCountryListMatchesTheGoOne(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(frontendAssignedCountries)
	if err != nil {
		t.Fatalf("reading the frontend country list: %v", err)
	}
	const marker = "ASSIGNED_COUNTRIES: readonly string[] = ["
	text := tsCurrencyComment.ReplaceAllString(string(source), " ")
	_, body, found := strings.Cut(text, marker)
	if !found {
		t.Fatalf("%s no longer declares the list as an array literal, so this gate reads a shape that is gone", frontendAssignedCountries)
	}
	list, _, terminated := strings.Cut(body, "];")
	if !terminated {
		t.Fatalf("%s: the country array is unterminated", frontendAssignedCountries)
	}
	inTS := map[string]bool{}
	for _, m := range tsCountryCode.FindAllStringSubmatch(list, -1) {
		inTS[m[1]] = true
	}
	if len(inTS) == 0 {
		t.Fatal("no codes parsed out of the frontend list, and a gate that reads nothing agrees with everything")
	}

	inGo := map[string]bool{}
	for _, code := range contacts.AssignedCountryCodes() {
		inGo[code] = true
		if !inTS[code] {
			t.Errorf("%s is assignable on the server and missing from the frontend list", code)
		}
	}
	for code := range inTS {
		if !inGo[code] {
			t.Errorf("%s is in the frontend list and refused by the server, so the filter would offer a country no address can hold", code)
		}
	}
}
