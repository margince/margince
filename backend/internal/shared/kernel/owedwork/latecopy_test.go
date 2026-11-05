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
	for _, tc := range []struct{ lang, days, hours string }{
		{"en", "Due 4 days ago and still open.", "Due less than a day ago and still open."},
		{"de", "Seit 4 Tagen fällig und noch offen.", "Seit weniger als einem Tag fällig und noch offen."},
		{"vi", "Quá hạn 4 ngày và vẫn đang mở.", "Quá hạn chưa đến một ngày và vẫn đang mở."},
	} {
		for _, span := range []time.Duration{-96 * time.Hour, -time.Hour, 0, time.Hour} {
			due := now.Add(span)
			got, late := LateWords(&due, now, tc.lang)
			want := ""
			if span == -96*time.Hour {
				want = tc.days
			}
			if span == -time.Hour {
				want = tc.hours
			}
			if got != want || late != (span < 0) {
				t.Errorf("%s %s: %q/%t, want %q", tc.lang, span, got, late, want)
			}
		}
	}
	due := now.Add(-time.Hour)
	got, _ := LateWords(&due, now, "unknown")
	if !strings.HasPrefix(got, "Due ") {
		t.Fatalf("unknown language lost fallback: %s", got)
	}
}

func TestOneDayLateUsesSingularCopy(t *testing.T) {
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct{ lang, want string }{
		{"en", "Due one day ago and still open."}, {"de", "Seit einem Tag fällig und noch offen."}, {"vi", "Quá hạn một ngày và vẫn đang mở."},
	} {
		for _, hours := range []int{24, 47} {
			due := now.Add(-time.Duration(hours) * time.Hour)
			got, late := LateWords(&due, now, tc.lang)
			if !late || got != tc.want {
				t.Errorf("%s/%d: %q %t", tc.lang, hours, got, late)
			}
		}
	}
}
