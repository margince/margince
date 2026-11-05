// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package weekly

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// A day meeting held on a Sunday counts in the week its date names. East of
// UTC+12 its stored noon is already the next Monday, so a week bounded on that
// noon would hand it to the following review. Instants keep their own time.
func TestASundayDayMeetingCountsInItsOwnWeek(t *testing.T) {
	for _, zone := range []string{"Pacific/Auckland", "Pacific/Kiritimati"} {
		t.Run(zone, func(t *testing.T) {
			e := setupWeekly(t)
			e.WsExec(t, `UPDATE setting SET value = to_jsonb($1::text) WHERE key = 'installation.timezone'`, zone)
			loc, err := time.LoadLocation(zone)
			if err != nil {
				t.Fatal(err)
			}
			monday := time.Date(2026, 9, 14, 0, 0, 0, 0, loc)
			nextMonday := monday.AddDate(0, 0, 7)
			sunday := occurredday.Anchor(time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC))
			e.logHeldMeeting(t, sunday, activities.PrecisionDay)
			e.logHeldMeeting(t, nextMonday.Add(-time.Hour), activities.PrecisionInstant)
			e.logHeldMeeting(t, nextMonday.Add(time.Hour), activities.PrecisionInstant)

			for name, week := range map[string]struct {
				start time.Time
				want  int
			}{"its own week": {monday, 2}, "the next week": {nextMonday, 1}} {
				err := database.WithWorkspaceTx(e.repCtx, e.Pool, func(tx pgx.Tx) error {
					held, _, err := countWeekMeetings(e.repCtx, tx, e.Rep1, week.start, week.start.AddDate(0, 0, 7))
					if err == nil && held != week.want {
						t.Errorf("%s: %d meetings held, want %d", name, held, week.want)
					}
					return err
				})
				if err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

// logHeldMeeting logs a held meeting the rep hosted at at, of precision.
func (e *weekEnv) logHeldMeeting(t *testing.T, at time.Time, precision string) {
	t.Helper()
	held, host := "held", ids.From[ids.UserKind](e.Rep1)
	if _, _, err := e.Activities.LogActivity(e.repCtx, activities.LogActivityInput{
		Kind: "meeting", Source: "manual", MeetingStatus: &held, HostUserID: &host,
		OccurredAt: &at, OccurredPrecision: precision,
	}); err != nil {
		t.Fatalf("logging the meeting at %v: %v", at, err)
	}
}
