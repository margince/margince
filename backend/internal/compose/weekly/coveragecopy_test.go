// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

import (
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
)

func TestCoverageCopyPreservesMeasurementFacts(t *testing.T) {
	langcopytest.Census(t, coverageWords)
	langcopytest.Census(t, coverageNouns)
	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 7)
	during, after := start.AddDate(0, 0, 2), end.AddDate(0, 0, 1)
	for noun, label := range coverageNouns {
		for _, since := range []*time.Time{nil, &during, &after, &start} {
			english := teamCopyFor("en").figureCoverage(since, start, end, noun)
			for _, language := range []string{"en", "de", "vi"} {
				words := teamCopyFor(language)
				got := words.figureCoverage(since, start, end, noun)
				if got.Status != english.Status || got.RecordedSince != since || (got.Reason == nil) != (english.Reason == nil) {
					t.Fatalf("%s %s changed coverage: %+v", language, noun, got)
				}
				if got.Reason != nil && (!strings.Contains(*got.Reason, words.Say(label)) || (language != "en" && *got.Reason == *english.Reason)) {
					t.Fatalf("%s %s reason: %s", language, noun, *got.Reason)
				}
			}
		}
	}
}

func TestUnknownCoverageFamilyRemainsNamed(t *testing.T) {
	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	for _, language := range []string{"en", "de", "vi"} {
		got := teamCopyFor(language).figureCoverage(nil, start, start.AddDate(0, 0, 7), "future_record")
		if got.Reason == nil || !strings.Contains(*got.Reason, "future_record") {
			t.Fatalf("%s lost unknown family: %+v", language, got)
		}
	}
}
