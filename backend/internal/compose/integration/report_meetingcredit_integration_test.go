// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"fmt"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A calendar meeting that Rep1 organized and Rep2 accepted, and that Rep3's
// calendar imported without Rep3 answering, is in Rep1's figures and in Rep2's.
// A report that does not break down by colleague counts it once. When Rep2's
// answer later turns tentative, the held meeting stays in Rep2's figures on
// every surface that reads it, and the surfaces agree.
func TestMeetingConversionListsAMeetingUnderEveryCreditedColleagueAndCountsItOnce(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	subject, start, booked := "Shared call", clocktest.Now(t).Add(-72*time.Hour), "booked"
	logged, _, err := e.Activities.LogActivity(admin, activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, OccurredAt: &start, MeetingStatus: &booked, Source: "manual",
	})
	if err != nil {
		t.Fatalf("logging the meeting: %v", err)
	}
	meeting := ids.UUID(logged.Id)
	e.WsExec(t, `UPDATE activity SET captured_by = 'connector:google_calendar', host_user_id = $2 WHERE id = $1`, meeting, e.Rep3)
	for _, guest := range []struct {
		seat           ids.UUID
		role, response string
	}{
		{e.Rep1, "organizer", "accepted"}, {e.Rep2, "attendee", "accepted"}, {e.Rep3, "attendee", "needs_action"},
	} {
		e.WsExec(t, `INSERT INTO activity_participant (activity_id, role, address, user_id, response) VALUES ($1, $2, $3, $4, $5)`,
			meeting, guest.role, guest.seat.String()+"@forecast.test", guest.seat, guest.response)
	}
	held := "held"
	if _, err := e.Activities.UpdateActivity(admin, ids.From[ids.ActivityKind](meeting), activities.UpdateActivityInput{MeetingStatus: &held}); err != nil {
		t.Fatalf("holding the meeting: %v", err)
	}
	e.WsExec(t, `UPDATE activity_participant SET response = 'tentative' WHERE activity_id = $1 AND user_id = $2`, meeting, e.Rep2)

	byColleague := map[string]string{}
	for _, row := range runPrebuiltReport(admin, t, e, "meeting-conversion", `{"group_by":["host_user_id"],"aggregates":[{"fn":"count","as":"meetings"}]}`) {
		byColleague[cell(row, "host_user_id")] = cell(row, "meetings")
	}
	if want := fmt.Sprint(map[string]string{e.Rep1.String(): "1", e.Rep2.String(): "1"}); fmt.Sprint(byColleague) != want {
		t.Errorf("meetings by colleague = %v, want one each for the organizer and the guest, none for the importer", byColleague)
	}
	if total := runPrebuiltReport(admin, t, e, "meeting-conversion", `{"aggregates":[{"fn":"count","as":"meetings"}]}`); len(total) != 1 || cell(total[0], "meetings") != "1" {
		t.Errorf("the total = %v, want the meeting once", total)
	}
	for seat, want := range map[ids.UUID]string{e.Rep1: "1", e.Rep2: "1", e.Rep3: ""} {
		for report, plan := range map[string]struct{ body, measure string }{
			"meeting-conversion": {`{"filters":{"host_user_id":"%s"},"aggregates":[{"fn":"count","as":"meetings"}]}`, "meetings"},
			"activities-by-kind": {`{"filters":{"kind":"meeting","host_user_id":"%s"},"group_by":["kind"],"aggregates":[{"fn":"count","as":"activities"}]}`, "activities"},
		} {
			rows := runPrebuiltReport(admin, t, e, report, fmt.Sprintf(plan.body, seat))
			got := ""
			if len(rows) == 1 {
				got = cell(rows[0], plan.measure)
			}
			if got != want {
				t.Errorf("%s filtered to %s = %q, want %q", report, seat, got, want)
			}
		}
	}
}
