// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import (
	"errors"
	"testing"
)

func TestAnUnassignedCodeIsValidInShapeButNotAssignable(t *testing.T) {
	for _, code := range []string{"EUR", "USD", "JPY", "KWD", "VND", "XAU"} {
		if !ValidCurrency(code) || !AssignableCurrency(code) {
			t.Errorf("%q should be valid and assignable", code)
		}
	}
	for _, code := range []string{"ZZZ", "QQQ", "AAA", "EUT"} {
		if !ValidCurrency(code) || AssignableCurrency(code) {
			t.Errorf("%q should have a valid shape and not be assignable", code)
		}
	}
	for _, code := range []string{"eur", "EU", "EURO", ""} {
		if ValidCurrency(code) || AssignableCurrency(code) {
			t.Errorf("%q should fail both checks", code)
		}
	}
}

func TestRequireAssignableCurrencyNamesTheField(t *testing.T) {
	var parse *ParseError
	if err := RequireAssignableCurrency("EUR"); err != nil {
		t.Fatalf("EUR refused: %v", err)
	}
	if err := RequireAssignableCurrency("ZZZ"); !errors.As(err, &parse) || parse.Field != "currency" {
		t.Fatalf("ZZZ gave %v, want a currency ParseError", err)
	}
}

func TestEveryCurrencyWithAMinorUnitRuleIsAssigned(t *testing.T) {
	for code := range MinorUnitExceptions() {
		if !AssignableCurrency(code) {
			t.Errorf("%s has a minor-unit rule but is not in the assigned list", code)
		}
	}
}
