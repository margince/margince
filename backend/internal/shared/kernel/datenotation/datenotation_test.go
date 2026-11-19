// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datenotation

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestADayIsWrittenInTheInstallationsNotation(t *testing.T) {
	t.Parallel()
	day := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		lang textlang.Lang
		date string
		want string
	}{
		{textlang.English, "locale", "06/05/2026"},
		{textlang.German, "locale", "06.05.2026"},
		{textlang.Vietnamese, "locale", "06/05/2026"},
		{textlang.English, "dmy", "06.05.2026"},
		{textlang.Vietnamese, "mdy", "05/06/2026"},
		{textlang.German, "ymd", "2026-05-06"},
		{textlang.Unknown, "", "06/05/2026"},
	} {
		if got := (Notation{Lang: tc.lang, Date: tc.date}).CalendarDay(day); got != tc.want {
			t.Errorf("%s with %q wrote %q, want %q", tc.lang, tc.date, got, tc.want)
		}
	}
}

func TestAnInstantIsWrittenOnTheInstallationsClock(t *testing.T) {
	t.Parallel()
	saigon, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		t.Fatal(err)
	}
	late := time.Date(2026, 11, 16, 17, 34, 0, 0, time.UTC)
	n := Notation{Lang: textlang.English, Zone: saigon}
	if got := n.Day(late); got != "17/11/2026" {
		t.Errorf("day = %q, want the next day on the installation's clock", got)
	}
	if got := n.Clock(late); got != "00:34" {
		t.Errorf("clock = %q, want 00:34", got)
	}
	n.Time = "12h"
	if got := n.Clock(late); got != "12:34 AM" {
		t.Errorf("12h clock = %q, want 12:34 AM", got)
	}
}
