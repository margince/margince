// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package network

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestEveryShippedLanguageWritesEveryRisk(t *testing.T) {
	t.Parallel()
	langcopytest.Census(t, riskWords)
}

// The going-cold sentence counts days and never has to count one, because the
// rule does not fire below its window. Held here rather than asserted in a
// comment: a window lowered to a day would give every language "1 days".
func TestTheGoingColdWindowNeverCountsOneDay(t *testing.T) {
	t.Parallel()
	if goingColdDays < 2 {
		t.Fatalf("the going-cold window is %d days, so its sentence would count one and needs a "+
			"singular of its own in every language", goingColdDays)
	}
}

// The fold writes in the language it is handed. That is its purpose, and what
// a table alone does not prove.
func TestTheFoldWritesInTheLanguageItIsGiven(t *testing.T) {
	t.Parallel()
	coverage := singleThreadedCoverage()
	for _, lang := range textlang.Shipped {
		risks := foldRisks(coverage, langcopy.For(string(lang)))
		if len(risks) == 0 {
			t.Fatalf("%s: a single-threaded deal produced no risk to read", lang)
		}
		want := riskWords.SingleThreaded.In(lang)
		if !strings.Contains(risks[0].Summary, want) {
			t.Errorf("the %s fold summarised a single-threaded deal as %q, want %q",
				lang, risks[0].Summary, want)
		}
	}
}

// singleThreadedCoverage is a deal with one engaged contact: enough to fire
// RiskSingleThreadedTheirs and nothing else, so the fold's first risk is
// always the one this test is reading.
func singleThreadedCoverage() DealCoverage {
	return DealCoverage{
		DealID:      ids.NewV7(),
		EverTouched: true,
		Stakeholders: []deals.DealStakeholder{
			seat(true, roleChampion), seat(false, "user"), seat(false, "legal"),
		},
	}
}
