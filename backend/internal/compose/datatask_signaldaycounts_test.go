// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Every shipped language's old wording is recognised, and the new wording is
// not, so the task rewrites each summary once.
func TestTheDayCountTaskKnowsEveryOldWordingAndNotTheNewOne(t *testing.T) {
	for _, lang := range textlang.Shipped {
		legacy, ok := legacySignalSummaryByLang[lang]
		if !ok {
			t.Errorf("the task does not know the old %s wording", lang)
			continue
		}
		said := signalSummaryCopyFor(lang)
		for _, c := range []struct {
			kind, old, current string
		}{
			{kindGhostedThread, fmt.Sprintf(legacy.ghostedThread, 15), fmt.Sprintf(said.ghostedThread, ghostedThresholdDays)},
			{kindProjectGoneQuiet, fmt.Sprintf(legacy.projectQuiet, "ERP rollout", 45), fmt.Sprintf(said.projectQuiet, "ERP rollout", 30)},
		} {
			if to, found := rewordedDaySummary(c.kind, c.old); !found || to != c.current {
				t.Errorf("%s %s: %q rewords to %q (found %v), want %q", lang, c.kind, c.old, to, found, c.current)
			}
			if _, found := rewordedDaySummary(c.kind, c.current); found {
				t.Errorf("%s %s: the new wording %q reads as old", lang, c.kind, c.current)
			}
		}
	}
}
