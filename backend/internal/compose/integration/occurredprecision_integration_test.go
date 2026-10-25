// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// dayRead is the part of an activity a day entry is judged by.
type dayRead struct {
	ID                string    `json:"id"`
	OccurredAt        time.Time `json:"occurred_at"`
	OccurredPrecision string    `json:"occurred_precision"`
}

func readDay(t *testing.T, raw json.RawMessage) dayRead {
	t.Helper()
	var read dayRead
	if err := json.Unmarshal(raw, &read); err != nil {
		t.Fatal(err)
	}
	return read
}

// setRecordZone moves the installation's zone, the one day entries are dated in.
func setRecordZone(t *testing.T, e *apptest.AppEnv, zone string) *time.Location {
	t.Helper()
	if _, err := e.Owner.Exec(context.Background(),
		`UPDATE setting SET value = to_jsonb($1::text) WHERE key = 'installation.timezone'`, zone); err != nil {
		t.Fatal(err)
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

// isDayOn says a read is a day entry stored at noon of date in loc.
func isDayOn(read dayRead, date string, loc *time.Location) bool {
	local := read.OccurredAt.In(loc)
	return read.OccurredPrecision == "day" && local.Format(time.DateOnly) == date && local.Hour() == 12
}

// A date-only entry is a day on the web form's door and on the agent's. A later
// read says so, rather than offering the anchor as a time.
func TestADayEntryIsADayOverBothDoors(t *testing.T) {
	d := newTwoDoors(t, "day-entry-doors", "read", "write")
	for _, door := range []string{"http", "tool"} {
		t.Run(door, func(t *testing.T) {
			fields := map[string]any{
				"kind": "note", "subject": "Met at the fair",
				"occurred_at": "2026-03-09T12:00:00+07:00", "occurred_precision": "day",
			}
			var made json.RawMessage
			if door == "http" {
				fields["source"] = "manual"
				made = d.restStatus(t, "POST", "/v1/activities", fields, http.StatusCreated)
			} else {
				made = d.tool(t, "log_activity", fields)
			}
			read := readDay(t, d.restStatus(t, "GET", "/v1/activities/"+anIDIn(t, made), nil, http.StatusOK))
			if !isDayOn(read, "2026-03-09", time.UTC) {
				t.Fatalf("read back %v as %q, want 9 March at the record zone's noon as a day", read.OccurredAt, read.OccurredPrecision)
			}
		})
	}
}

// The web client sends a picked date at noon UTC. The server reads the date in
// that offset and stores it at noon in the record zone, east and west of UTC.
func TestAWebDayIsStoredOnItsDateInTheRecordZone(t *testing.T) {
	d := newTwoDoors(t, "day-entry-zones", "read", "write")
	for _, zone := range []string{"Pacific/Auckland", "Pacific/Pago_Pago"} {
		t.Run(zone, func(t *testing.T) {
			loc := setRecordZone(t, d.e, zone)
			made := d.restStatus(t, "POST", "/v1/activities", map[string]any{
				"kind": "note", "subject": "Met at the fair", "source": "manual",
				"occurred_at": "2025-03-09T12:00:00Z", "occurred_precision": "day",
			}, http.StatusCreated)
			read := readDay(t, d.restStatus(t, "GET", "/v1/activities/"+anIDIn(t, made), nil, http.StatusOK))
			if !isDayOn(read, "2025-03-09", loc) {
				t.Fatalf("read back %v as %q, want 2025-03-09 at noon in %s as a day", read.OccurredAt, read.OccurredPrecision, zone)
			}
		})
	}
}

// A day with no date to anchor it is refused on the wire, naming the field.
func TestADayWithoutADateIsRefusedOnTheWire(t *testing.T) {
	d := newTwoDoors(t, "day-entry-refused", "read", "write")
	body := d.restStatus(t, "POST", "/v1/activities", map[string]any{
		"kind": "note", "subject": "Met at the fair", "source": "manual", "occurred_precision": "day",
	}, http.StatusUnprocessableEntity)
	var refusal struct {
		Details struct {
			Errors []struct {
				Field string `json:"field"`
			} `json:"errors"`
		} `json:"details"`
	}
	if err := json.Unmarshal(body, &refusal); err != nil || len(refusal.Details.Errors) == 0 || refusal.Details.Errors[0].Field != "occurred_at" {
		t.Fatalf("the refusal = %s, want it to name occurred_at", body)
	}
}

// A day cell is a day's activity, and a cell with a time keeps its time.
func TestAnImportedDayCellIsADay(t *testing.T) {
	e := setupImportApp(t)
	loc := setRecordZone(t, e, "America/New_York")
	_, report := importActivities(t, e, map[string]string{
		"contacts.csv": "ref,email,name\nP-1,jo@day-cell.example,Jo Tran\n",
		"activities.csv": activityHeader + "\n" +
			"A-1,meeting,Day-only visit,,2025-03-09,contact:P-1,90,held,\n" +
			"A-2,call,Timed call,,2025-03-04T09:30:00+07:00,contact:P-1,25,,outbound\n",
	}, contactAndActivityParts())
	if report.Disposition.Created != 3 {
		t.Fatalf("disposition = %+v, issues %+v, want the contact and two activities", report.Disposition, report.Issues)
	}
	if read := storedDay(t, e, "Day-only visit"); !isDayOn(read, "2025-03-09", loc) {
		t.Errorf("the day cell landed at %v as %q, want 9 March at New York's noon as a day", read.OccurredAt, read.OccurredPrecision)
	}
	if read := storedDay(t, e, "Timed call"); read.OccurredPrecision != "instant" {
		t.Errorf("the timed cell landed as %q, want instant", read.OccurredPrecision)
	}
}

// storedDay reads one activity's time as the store holds it.
func storedDay(t *testing.T, e *apptest.AppEnv, subject string) dayRead {
	t.Helper()
	var read dayRead
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT id::text, occurred_at, occurred_precision FROM activity WHERE subject = $1 AND archived_at IS NULL`,
		subject).Scan(&read.ID, &read.OccurredAt, &read.OccurredPrecision); err != nil {
		t.Fatalf("reading %q: %v", subject, err)
	}
	return read
}

// A corrected file rewrites a day row by its date. The same date changes nothing,
// a new date moves it, and a timed cell makes it an instant. The same date
// again leaves a day the web made alone.
func TestAReImportedDayCellComparesByDate(t *testing.T) {
	e := setupImportApp(t)
	loc := setRecordZone(t, e, "America/New_York")
	parts := contactAndActivityParts()
	file := func(occurredAt string) map[string]string {
		return map[string]string{
			"contacts.csv":   "ref,email,name\nP-1,jo@day-reimport.example,Jo Tran\n",
			"activities.csv": activityHeader + "\nA-1,meeting,Fair visit,," + occurredAt + ",contact:P-1,90,held,\n",
		}
	}
	importActivities(t, e, file("2025-03-09"), parts)
	if _, same := importActivities(t, e, file("2025-03-09"), parts); same.Disposition.Updated != 0 || same.Disposition.Unchanged != 2 {
		t.Fatalf("the same date again = %+v, want everything unchanged", same.Disposition)
	}
	if _, moved := importActivities(t, e, file("2025-03-07"), parts); moved.Disposition.Updated != 1 {
		t.Fatalf("a new date = %+v, want the meeting rewritten", moved.Disposition)
	}
	if read := storedDay(t, e, "Fair visit"); !isDayOn(read, "2025-03-07", loc) {
		t.Errorf("the rewritten row reads %v as %q, want 7 March as a day", read.OccurredAt, read.OccurredPrecision)
	}
	importActivities(t, e, file("2025-03-07T09:30:00-05:00"), parts)
	read := storedDay(t, e, "Fair visit")
	if read.OccurredPrecision != "instant" || !read.OccurredAt.Equal(time.Date(2025, 3, 7, 14, 30, 0, 0, time.UTC)) {
		t.Errorf("a timed cell left %v as %q, want 14:30 UTC as an instant", read.OccurredAt, read.OccurredPrecision)
	}

	var patched json.RawMessage
	if status := e.Call(t, http.MethodPatch, "/v1/activities/"+read.ID, map[string]any{"occurred_precision": "day"}, nil, &patched); status != http.StatusOK {
		t.Fatalf("making it a day on the web = %d: %s", status, patched)
	}
	if _, again := importActivities(t, e, file("2025-03-07"), parts); again.Disposition.Updated != 0 {
		t.Fatalf("the web's day re-imported on its own date = %+v, want it untouched", again.Disposition)
	}
	if read := storedDay(t, e, "Fair visit"); !isDayOn(read, "2025-03-07", loc) {
		t.Errorf("the web's day reads %v as %q, want 7 March as a day", read.OccurredAt, read.OccurredPrecision)
	}
}
