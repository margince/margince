// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package weekly

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A task with a date and no time belongs to the week of that date in the
// company zone. Picked from Los Angeles for the Sunday, its instant is already
// Monday in Berlin, so a count by instant hands it to the next review.
func TestADateOnlyTaskCountsInTheWeekOfItsPickedDate(t *testing.T) {
	e := setupWeekly(t)
	e.WsExec(t, `UPDATE setting SET value = to_jsonb($1::text) WHERE key = 'installation.timezone'`, "Europe/Berlin")
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	monday := time.Date(2026, 11, 9, 0, 0, 0, 0, berlin)
	nextMonday := monday.AddDate(0, 0, 7)
	assignee := ids.From[ids.UserKind](e.Rep1)
	log := func(subject, day, zone string) {
		task, _, err := e.Activities.LogActivity(e.repCtx, activities.LogActivityInput{
			Kind: "task", Subject: new(subject), AssigneeID: &assignee,
			DueIntent: &crmcontracts.IntentTime{WallClock: day + "T23:59:59", Timezone: zone},
		})
		if err != nil {
			t.Fatal(err)
		}
		e.WsExec(t, `UPDATE activity SET created_at = $2 WHERE id = $1`, task.Id, monday.AddDate(0, 0, -30))
	}
	log("Sunday from Los Angeles", "2026-11-15", "America/Los_Angeles")
	log("Monday from Los Angeles", "2026-11-16", "America/Los_Angeles")

	for name, week := range map[string]struct {
		start time.Time
		due   int
	}{"the week of the picked Sunday": {monday, 1}, "the next week": {nextMonday, 1}} {
		err := database.WithWorkspaceTx(e.repCtx, e.Pool, func(tx pgx.Tx) error {
			counts, err := countWeek(e.repCtx, tx, e.Rep1, week.start, week.start.AddDate(0, 0, 7))
			if err == nil && counts.TasksDue != week.due {
				t.Errorf("%s: %d tasks due, want %d", name, counts.TasksDue, week.due)
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
