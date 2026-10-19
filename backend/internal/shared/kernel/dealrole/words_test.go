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
			if Word(role, string(language)) == "" {
				t.Errorf("%s/%s is empty", role, language)
			}
		}
	}
	if Word("future_role", "de") != "future_role" {
		t.Fatal("unknown role changed")
	}
	if Word(EconomicBuyer, "unknown") != "economic buyer" {
		t.Fatal("unknown language lost English fallback")
	}
}
