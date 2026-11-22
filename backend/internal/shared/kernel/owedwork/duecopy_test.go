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
	langcopytest.NoCount(t, map[string]langcopy.Phrase{"tomorrow": dueWords.Tomorrow})
}

// The reader is seven hours ahead of UTC, so the calendar the words count on
// is not the one the instants are written in.
func TestDeadlineWordsCountCalendarDaysInTheReadersZone(t *testing.T) {
	bangkok := time.FixedZone("UTC+7", 7*60*60)
	now := time.Date(2026, 9, 2, 23, 0, 0, 0, bangkok)
	for _, tc := range []struct{ lang, today, tomorrow, many string }{
		{"en", "Due today.", "Due tomorrow.", "Due in 2 days."},
		{"de", "Heute fällig.", "Morgen fällig.", "Fällig in 2 Tagen."},
		{"vi", "Đến hạn hôm nay.", "Đến hạn ngày mai.", "Đến hạn sau 2 ngày."},
	} {
		for _, span := range []struct {
			due  time.Time
			want string
		}{
			{now, tc.today},
			{time.Date(2026, 9, 2, 23, 59, 0, 0, bangkok), tc.today},
			{time.Date(2026, 9, 3, 1, 0, 0, 0, bangkok), tc.tomorrow},
			{time.Date(2026, 9, 3, 22, 0, 0, 0, bangkok), tc.tomorrow},
			{time.Date(2026, 9, 4, 1, 0, 0, 0, bangkok), tc.many},
		} {
			if got := DeadlineWords(span.due, now, bangkok, tc.lang); got != span.want {
				t.Errorf("%s/%s: %q, want %q", tc.lang, span.due, got, span.want)
			}
		}
	}
}

// 23:00 and 01:00 the next morning in Bangkok fall on one UTC date, so a count
// read in UTC would call the task due today.
func TestATaskDueAfterMidnightIsDueTomorrowWhateverTheUTCDate(t *testing.T) {
	bangkok := time.FixedZone("UTC+7", 7*60*60)
	now := time.Date(2026, 9, 2, 23, 0, 0, 0, bangkok)
	due := time.Date(2026, 9, 3, 1, 0, 0, 0, bangkok)
	if got := UpcomingWords(due, now, bangkok, "en"); got != "Due tomorrow." {
		t.Errorf("due 01:00 tomorrow, read at 23:00: %q", got)
	}
	if got := UpcomingWords(due, now, time.UTC, "en"); got != "Due today." {
		t.Fatalf("the two instants share a UTC date, so UTC must read today: %q", got)
	}
}
