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
	for _, typed := range []string{"contact", "future_role", "Champion", " champion"} {
		if Recorded(typed) != "" || Label(typed, "de") != "" {
			t.Errorf("free-text role %q got a label", typed)
		}
	}
	if Recorded(Champion) != Champion || Label(Champion, "de") != "Fürsprecher" {
		t.Fatal("built-in role lost its German label")
	}
	if Label(EconomicBuyer, "unknown") != "economic buyer" {
		t.Fatal("unknown language lost English fallback")
	}
}
