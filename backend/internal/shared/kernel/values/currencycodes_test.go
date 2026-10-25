// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import (
	"errors"
	"testing"
)

func TestOnlyAnAssignedCodeIsACurrency(t *testing.T) {
	for _, code := range []string{"EUR", "USD", "JPY", "KWD", "VND", "XAU"} {
		if !ValidCurrency(code) {
			t.Errorf("ValidCurrency(%q) = false, want true", code)
		}
	}
	for _, code := range []string{"ZZZ", "QQQ", "AAA", "EUT", "eur", "EU", "EURO", ""} {
		if ValidCurrency(code) {
			t.Errorf("ValidCurrency(%q) = true, want false", code)
		}
	}
}

func TestNewMoneyRefusesAnUnassignedCode(t *testing.T) {
	_, err := NewMoney(100, "ZZZ")
	var parse *ParseError
	if !errors.As(err, &parse) || parse.Field != "currency" {
		t.Fatalf("NewMoney(100, ZZZ) = %v, want a currency ParseError", err)
	}
}

func TestEveryCurrencyWithAMinorUnitRuleIsAssigned(t *testing.T) {
	for code := range MinorUnitExceptions() {
		if !IsAssignedCurrency(code) {
			t.Errorf("%s has a minor-unit rule but is not in the assigned list", code)
		}
	}
}
