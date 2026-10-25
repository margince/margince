// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The currency picker must never offer a code the server refuses.
// frontend/src/format/assignedcurrencies.ts mirrors the list in
// values.AssignedCurrencies, and this gate fails when either side has a code
// the other lacks.

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

const frontendAssignedCurrencies = "../frontend/src/format/assignedcurrencies.ts"

var (
	tsCurrencyCode    = regexp.MustCompile(`"([A-Z]{3})"`)
	tsCurrencyComment = regexp.MustCompile(`(?s)//[^\n]*|/\*.*?\*/`)
)

func TestTheFrontendCurrencyListMatchesTheGoOne(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(frontendAssignedCurrencies)
	if err != nil {
		t.Fatalf("reading the frontend currency list: %v", err)
	}
	const marker = "ASSIGNED_CURRENCIES: readonly string[] = ["
	text := tsCurrencyComment.ReplaceAllString(string(source), " ")
	start := strings.Index(text, marker)
	if start < 0 {
		t.Fatalf("%s no longer declares the list as an array literal, so this gate reads a shape that is gone", frontendAssignedCurrencies)
	}
	body := text[start+len(marker):]
	end := strings.Index(body, "];")
	if end < 0 {
		t.Fatalf("%s: the currency array is unterminated", frontendAssignedCurrencies)
	}
	inTS := map[string]bool{}
	for _, m := range tsCurrencyCode.FindAllStringSubmatch(body[:end], -1) {
		inTS[m[1]] = true
	}
	if len(inTS) == 0 {
		t.Fatal("no codes parsed out of the frontend list, and a gate that reads nothing agrees with everything")
	}

	inGo := map[string]bool{}
	for _, code := range values.AssignedCurrencies() {
		inGo[code] = true
		if !inTS[code] {
			t.Errorf("%s is assignable on the server and missing from the frontend list", code)
		}
	}
	for code := range inTS {
		if !inGo[code] {
			t.Errorf("%s is in the frontend list and refused by the server, so the picker would offer a code that fails", code)
		}
	}
}
