// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"fmt"
	"testing"
)

// A calendar meeting that Rep1 organized and Rep2 accepted, and that Rep3's
// calendar imported without Rep3 answering, is in Rep1's figures and in Rep2's.
// A report that does not break down by colleague counts it once.
func TestMeetingConversionListsAMeetingUnderEveryCreditedColleagueAndCountsItOnce(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	e.WsExec(t, `INSERT INTO lead (id, email, full_name, source, captured_by) VALUES ('00000000-0000-7000-8000-0000000000a1', 'shared@customer.test', 'Prospect', 'manual', 'human:x')`)
	e.WsExec(t, `INSERT INTO activity (id, kind, direction, meeting_status, subject, occurred_at, thread_key, source, captured_by, host_user_id)
		VALUES ('00000000-0000-7000-8000-0000000000a2', 'meeting', 'outbound', 'held', 'Shared call', now() - interval '3 days', 'shared-call', 'manual', 'connector:google_calendar', $1)`, e.Rep3)
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, lead_id) VALUES ('00000000-0000-7000-8000-0000000000a2', 'lead', '00000000-0000-7000-8000-0000000000a1')`)
	for _, guest := range []struct{ seat, role, response string }{
		{e.Rep1.String(), "organizer", "accepted"}, {e.Rep2.String(), "attendee", "accepted"}, {e.Rep3.String(), "attendee", "needs_action"},
	} {
		e.WsExec(t, `INSERT INTO activity_participant (activity_id, role, address, user_id, response)
			VALUES ('00000000-0000-7000-8000-0000000000a2', $1, $2, $3::uuid, $4)`, guest.role, guest.seat+"@forecast.test", guest.seat, guest.response)
	}

	byColleague := map[string]string{}
	for _, row := range runPrebuiltReport(admin, t, e, "meeting-conversion",
		`{"group_by":["host_user_id"],"aggregates":[{"fn":"count","as":"meetings"}]}`) {
		byColleague[cell(row, "host_user_id")] = cell(row, "meetings")
	}
	if want := fmt.Sprint(map[string]string{e.Rep1.String(): "1", e.Rep2.String(): "1"}); fmt.Sprint(byColleague) != want {
		t.Errorf("meetings by colleague = %v, want one each for the organizer and the guest, none for the importer", byColleague)
	}
	if total := runPrebuiltReport(admin, t, e, "meeting-conversion", `{"aggregates":[{"fn":"count","as":"meetings"}]}`); len(total) != 1 || cell(total[0], "meetings") != "1" {
		t.Errorf("the total = %v, want the meeting once", total)
	}
	for seat, want := range map[string]string{e.Rep1.String(): "1", e.Rep2.String(): "1", e.Rep3.String(): ""} {
		rows := runPrebuiltReport(admin, t, e, "meeting-conversion",
			`{"filters":{"host_user_id":"`+seat+`"},"aggregates":[{"fn":"count","as":"meetings"}]}`)
		got := ""
		if len(rows) == 1 {
			got = cell(rows[0], "meetings")
		}
		if got != want {
			t.Errorf("meetings filtered to %s = %q, want %q", seat, got, want)
		}
	}
}
