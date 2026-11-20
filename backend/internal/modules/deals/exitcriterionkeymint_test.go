// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"strings"
	"testing"
)

func TestCriterionKeyFromLabelAlwaysPassesTheKeyShape(t *testing.T) {
	cases := map[string]string{
		"Budget confirmed":        "budget_confirmed",
		"  Legal  sign-off!! ":    "legal_sign_off",
		"Prüfung abgeschlossen":   "prufung_abgeschlossen",
		"2026 contract signed":    "criterion_2026_contract_signed",
		"決裁者の確認":                  "criterion",
		"???":                     "criterion",
		"Café — décision":         "cafe_decision",
		strings.Repeat("ab ", 60): strings.TrimRight(strings.Repeat("ab_", 16)[:48], "_"),
	}
	for label, want := range cases {
		got := criterionKeyFromLabel(label)
		if got != want {
			t.Errorf("criterionKeyFromLabel(%q) = %q, want %q", label, got, want)
		}
		if err := validCriterionKey(got + "_999"); err != nil {
			t.Errorf("key %q from %q fails the shape once suffixed: %v", got, label, err)
		}
	}
}
