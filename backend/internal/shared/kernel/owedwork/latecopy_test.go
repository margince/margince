// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package owedwork

import (
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
)

func TestLateCopyCensus(t *testing.T) {
	langcopytest.Census(t, lateWords)
	langcopytest.NoCount(t, map[string]langcopy.Phrase{"day": lateWords.Day})
}

func TestLateWordsKeepDeadlineMeaningInEveryLanguage(t *testing.T) {
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct{ lang, days, today string }{
		{"en", "Due 4 days ago and still open.", "Due earlier today and still open."},
		{"de", "Seit 4 Tagen fällig und noch offen.", "Seit heute fällig und noch offen."},
		{"vi", "Quá hạn 4 ngày và vẫn đang mở.", "Quá hạn từ sớm hôm nay và vẫn đang mở."},
	} {
		for _, span := range []time.Duration{-96 * time.Hour, -time.Hour, 0, time.Hour} {
			due := now.Add(span)
			got, late := LateWords(&due, now, time.UTC, tc.lang)
			want := ""
			if span == -96*time.Hour {
				want = tc.days
			}
			if span == -time.Hour {
				want = tc.today
			}
			if got != want || late != (span < 0) {
				t.Errorf("%s %s: %q/%t, want %q", tc.lang, span, got, late, want)
			}
		}
	}
	due := now.Add(-time.Hour)
	got, _ := LateWords(&due, now, time.UTC, "unknown")
	if !strings.HasPrefix(got, "Due ") {
		t.Fatalf("unknown language lost fallback: %s", got)
	}
}

func TestOneDayLateUsesSingularCopy(t *testing.T) {
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct{ lang, want string }{
		{"en", "Due one day ago and still open."}, {"de", "Seit einem Tag fällig und noch offen."}, {"vi", "Quá hạn một ngày và vẫn đang mở."},
	} {
		for _, clock := range []int{0, 12, 23} {
			due := time.Date(2026, 9, 1, clock, 30, 0, 0, time.UTC)
			got, late := LateWords(&due, now, time.UTC, tc.lang)
			if !late || got != tc.want {
				t.Errorf("%s/%02d:30: %q %t", tc.lang, clock, got, late)
			}
		}
	}
}

// Due at 23:00, read at 01:00 in Bangkok: two hours late, and the page's badge
// beside the sentence says "1 day late" because the date has turned.
func TestLateWordsCountTheDateInTheReadersZone(t *testing.T) {
	bangkok := time.FixedZone("UTC+7", 7*60*60)
	due := time.Date(2026, 9, 1, 23, 0, 0, 0, bangkok)
	now := time.Date(2026, 9, 2, 1, 0, 0, 0, bangkok)
	if got, _ := LateWords(&due, now, bangkok, "en"); got != "Due one day ago and still open." {
		t.Errorf("in Bangkok: %q", got)
	}
	if got, _ := LateWords(&due, now, time.UTC, "en"); got != "Due earlier today and still open." {
		t.Errorf("both instants are 1 Sep in UTC: %q", got)
	}
}
