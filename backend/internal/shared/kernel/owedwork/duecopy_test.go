// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package owedwork

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
)

func TestDueCopyCensus(t *testing.T) {
	langcopytest.Census(t, dueWords)
	langcopytest.NoCount(t, map[string]langcopy.Phrase{"one day": dueWords.One})
}

func TestDeadlineWordsUseRemainingWholeDays(t *testing.T) {
	now := time.Date(2026, 9, 2, 23, 0, 0, 0, time.UTC)
	for _, tc := range []struct{ lang, today, one, many string }{
		{"en", "Due today.", "Due in one day.", "Due in 2 days."},
		{"de", "Heute fällig.", "In einem Tag fällig.", "Fällig in 2 Tagen."},
		{"vi", "Đến hạn hôm nay.", "Đến hạn sau một ngày.", "Đến hạn sau 2 ngày."},
	} {
		for _, span := range []struct {
			hours int
			want  string
		}{{0, tc.today}, {2, tc.today}, {24, tc.one}, {47, tc.one}, {48, tc.many}} {
			due := now.Add(time.Duration(span.hours) * time.Hour)
			if got := DeadlineWords(due, now, tc.lang); got != span.want {
				t.Errorf("%s/%dh: %q, want %q", tc.lang, span.hours, got, span.want)
			}
		}
	}
}
