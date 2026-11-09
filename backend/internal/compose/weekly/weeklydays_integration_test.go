// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package weekly

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// The week under review is Monday 1 June to Sunday 7 June 2026 (weekClock).
var (
	weekTuesday  = time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	weekThursday = time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
)

func TestTheWeekChartsOnlyTheRepsOwnWorkPerDay(t *testing.T) {
	e := setupWeekly(t)
	owner := integration.OwnerConn(t)
	seed := func(kind, direction string, at time.Time, stamp string) {
		t.Helper()
		integration.SeedIDRow(t, owner, `
			INSERT INTO activity (id, kind, subject, occurred_at, direction, source, captured_by)
			VALUES ($1, $2, 'Day fixture', $3, nullif($4, ''), 'manual', $5)`,
			kind, at, direction, stamp)
	}
	mine, colleague := "human:"+e.Rep1.String(), "human:"+e.Rep2.String()
	seed("email", "outbound", weekTuesday, mine)
	seed("email", "outbound", weekTuesday, "connector:gmail:"+e.Rep1.String())
	seed("email", "inbound", weekTuesday, "connector:gmail:"+e.Rep1.String())
	seed("email", "outbound", weekTuesday, colleague)
	seed("email", "outbound", weekTuesday.AddDate(0, 0, -7), mine)
	seed("call", "", weekThursday, mine)
	seed("call", "", weekThursday, colleague)
	seed("note", "", weekThursday, mine)
	integration.SeedIDRow(t, owner, `
		INSERT INTO activity (id, kind, subject, occurred_at, meeting_status, host_user_id, source, captured_by)
		VALUES ($1, 'meeting', 'Hosted', $2, 'held', $3, 'manual', 'human:' || $4::text)`,
		weekThursday, e.Rep1, e.Rep3)

	review, _, err := e.engine.AssembleFor(e.repCtx, weekClock)
	if err != nil {
		t.Fatal(err)
	}
	read, err := e.engine.LatestReview(e.repCtx, &review.LocalWeekStart)
	if err != nil {
		t.Fatal(err)
	}
	for _, days := range [][]crmcontracts.WeeklyDayActivity{review.DailyActivity, read.DailyActivity} {
		if len(days) != daysInWeek {
			t.Fatalf("charted %d days, want %d", len(days), daysInWeek)
		}
		if got := days[0].Day.Format(time.DateOnly); got != "2026-06-01" {
			t.Errorf("the chart starts on %s, want Monday 2026-06-01", got)
		}
		want := map[int]crmcontracts.WeeklyDayActivity{
			1: {EmailsSent: 2},
			3: {CallsLogged: 1, MeetingsHeld: 1},
		}
		for i, day := range days {
			w := want[i]
			if day.EmailsSent != w.EmailsSent || day.CallsLogged != w.CallsLogged || day.MeetingsHeld != w.MeetingsHeld {
				t.Errorf("day %d counted %d emails, %d calls, %d meetings; want %d, %d, %d",
					i, day.EmailsSent, day.CallsLogged, day.MeetingsHeld, w.EmailsSent, w.CallsLogged, w.MeetingsHeld)
			}
		}
	}
}
