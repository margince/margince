// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealrole

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestEveryRecordedRoleHasWordsInEveryLanguage(t *testing.T) {
	langcopytest.Census(t, roleWords)
	if len(roleWords) != len(Shown)+1 {
		t.Errorf("roleWords has %d roles, Shown and the legacy role have %d", len(roleWords), len(Shown)+1)
	}
	for _, role := range append(append([]string{}, Shown...), LegacyDecisionMaker) {
		if _, ok := roleWords[role]; !ok {
			t.Errorf("role %s has no wording", role)
		}
		for _, language := range textlang.Shipped {
			if Label(role, string(language)) == "" {
				t.Errorf("%s/%s is empty", role, language)
			}
		}
	}
	for _, typed := range []string{"contact", "future_role", "Key contact", "economic buyer"} {
		if Recorded(typed) != "" || Label(typed, "de") != "" {
			t.Errorf("free-text role %q got a label", typed)
		}
	}
	for _, spelled := range []string{"Champion", " champion ", "CHAMPION"} {
		if Recorded(spelled) != Champion || Label(spelled, "de") != "Fürsprecher" {
			t.Errorf("stored %q is not read as the champion", spelled)
		}
	}
	if Recorded(Champion) != Champion || Label(Champion, "de") != "Fürsprecher" {
		t.Fatal("built-in role lost its German label")
	}
	if Label(EconomicBuyer, "unknown") != "economic buyer" {
		t.Fatal("unknown language lost English fallback")
	}
}
