// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
)

func TestTeamSummaryCopyCensus(t *testing.T) {
	langcopytest.Census(t, teamWords)
	langcopytest.Census(t, focusWords)
	for _, kind := range focusPriority {
		if _, ok := focusWords[kind]; !ok {
			t.Errorf("focus %s lacks words", kind)
		}
	}
}

func TestWithheldWeeklySummaryKeepsItsFrozenLanguage(t *testing.T) {
	for _, tc := range []struct{ language, prefix string }{
		{"en", "1 of 2 members counted; 1 deal won and 0 lost."},
		{"de", "Erfasste Teammitglieder: 1 von 2; ein Deal gewonnen und 0 verloren."},
		{"vi", "Đã tính 1 trên 2 thành viên; thắng một giao dịch và thua 0."},
	} {
		review := TeamReview{Language: tc.language, Counts: TeamCounts{RepsCounted: 1, DealsWon: 1}, RepsUnread: 1, Coverage: []Coverage{{PeriodKind: "month", Basis: "manager_call", CoverageBp: 6400, ReplanRecommended: true}}}
		review.Summary = summarise(review)
		if !strings.HasPrefix(review.Summary, tc.prefix) || !strings.Contains(review.Summary, "64%") {
			t.Fatalf("%s summary: %s", tc.language, review.Summary)
		}
		withheldFromReader(context.Background(), &review)
		if review.Language != tc.language || review.Summary != tc.prefix || review.Coverage[0].Absent != CoverageNotAuthorized {
			t.Fatalf("withheld %s: %+v", tc.language, review)
		}
	}
}

func TestSummaryLanguageOnlyBelongsToAMeasuredSnapshot(t *testing.T) {
	if got := (TeamReview{Language: "vi"}).summaryLanguage(); got != nil {
		t.Fatal("empty snapshot retained language for an older replacement writer")
	}
	for _, language := range []string{"", "future"} {
		got := (TeamReview{Language: language, Counts: TeamCounts{RepsCounted: 1}}).summaryLanguage()
		if got == nil || *got != "en" {
			t.Fatalf("legacy language fallback: %v", got)
		}
	}
}
