// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Every rule a night can raise says, in words, what it found. A rule without a
// phrase would leave its task a bare count.
func TestEveryForecastRuleExplainsItsTask(t *testing.T) {
	t.Parallel()
	rules := Rules()
	if len(rules) == 0 {
		t.Fatal("the census read no rules")
	}
	for _, rule := range rules {
		why := TaskReason([]Exception{{Type: rule.Type}})
		if why == nil {
			t.Errorf("rule %q has no finding phrase", rule.Type)
			continue
		}
		for _, lang := range textlang.Shipped {
			if _, err := why.Render(lang); err != nil {
				t.Errorf("rule %q in %s: %v", rule.Type, lang, err)
			}
		}
	}
}

func TestTheForecastTaskLeadsWithItsMostMaterialFinding(t *testing.T) {
	t.Parallel()
	why := TaskReason([]Exception{{Type: TypeClosePast}, {Type: TypeNoNextStep}})
	got, err := why.Render(textlang.English)
	if err != nil {
		t.Fatal(err)
	}
	if want := "Margince's nightly forecast check found that the close date has already passed, plus 1 more on this deal."; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if TaskReason([]Exception{{Type: TypeBuyerSilent}}) != nil {
		t.Fatal("a retired finding got a reason it has no phrase for")
	}
}
