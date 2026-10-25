// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/migration"
)

var activityNow = cellClock{now: time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC), zone: time.UTC}

func activityProblem(t *testing.T, fields map[string]string) *cellProblem {
	t.Helper()
	_, problem := checkActivityCells(fields, nil, activityNow)
	return problem
}

func TestAnActivityTypeMapsToItsKindAndMessageIsRefused(t *testing.T) {
	for _, name := range []string{"note", "Call", " MEETING ", "email"} {
		typed, problem := checkActivityCells(map[string]string{"type": name}, nil, activityNow)
		if problem != nil {
			t.Fatalf("type %q refused: %+v", name, problem)
		}
		if got := typed["type"]; got == "" || got == nil {
			t.Fatalf("type %q left no kind in typed", name)
		}
	}
	for name, want := range map[string]migration.ReasonCode{
		"message": migration.ReasonMessageNotImportable,
		"task":    migration.ReasonTaskNotImportable,
		"postit":  migration.ReasonActivityTypeUnknown,
		"":        migration.ReasonActivityTypeUnnamed,
	} {
		problem := activityProblem(t, map[string]string{"type": name})
		if problem == nil || problem.Why.Code != want || problem.Target != "type" {
			t.Errorf("type %q = %+v, want %s on the type column", name, problem, want)
		}
	}
}

func TestATimestampWithoutAZoneIsAProblem(t *testing.T) {
	for _, text := range []string{"2026-03-02 10:00", "2026-03-02T10:00:00", "2026-03-02 10:00:00"} {
		problem := activityProblem(t, map[string]string{"occurred_at": text})
		if problem == nil || problem.Why.Code != migration.ReasonTimestampNeedsZone {
			t.Errorf("%q = %+v, want %s", text, problem, migration.ReasonTimestampNeedsZone)
		}
	}
}

func TestATimestampIsReadAsUTCAndNeverAsTheFuture(t *testing.T) {
	cases := map[string]time.Time{
		"2026-03-02":                time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC),
		"2026-03-02T10:00:00+07:00": time.Date(2026, 3, 2, 3, 0, 0, 0, time.UTC),
		"2026-03-02T10:00:00Z":      time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC),
	}
	for text, want := range cases {
		typed, problem := checkActivityCells(map[string]string{"occurred_at": text}, nil, activityNow)
		if problem != nil {
			t.Fatalf("%q refused: %+v", text, problem)
		}
		if got, _ := typed["occurred_at"].(time.Time); !got.Equal(want) || got.Location() != time.UTC {
			t.Errorf("%q = %v, want %v in UTC", text, got, want)
		}
	}
	for text, want := range map[string]migration.ReasonCode{
		"2026-10-16":                migration.ReasonOccurredAtFuture,
		"2026-10-15T12:00:01+00:00": migration.ReasonOccurredAtFuture,
		"yesterday":                 migration.ReasonNotADate,
		"":                          migration.ReasonOccurredAtUnnamed,
	} {
		problem := activityProblem(t, map[string]string{"occurred_at": text})
		if problem == nil || problem.Why.Code != want {
			t.Errorf("%q = %+v, want %s", text, problem, want)
		}
	}
}

func TestADurationOnANoteIsAProblemFromTheStoresOwnRule(t *testing.T) {
	problem := activityProblem(t, map[string]string{"type": "note", "duration_minutes": "30"})
	if problem == nil || problem.Why.Code != migration.ReasonDurationNotValidForKind || problem.Target != "duration_minutes" {
		t.Fatalf("a duration on a note = %+v, want %s on the duration column", problem, migration.ReasonDurationNotValidForKind)
	}
	typed, problem := checkActivityCells(map[string]string{"type": "call", "duration_minutes": "30"}, nil, activityNow)
	if problem != nil || typed["duration_minutes"] != 1800 {
		t.Fatalf("a 30 minute call = %v, %+v; want 1800 seconds", typed["duration_minutes"], problem)
	}
	for text, want := range map[string]migration.ReasonCode{
		"-5":    migration.ReasonDurationOutOfRange,
		"99999": migration.ReasonDurationOutOfRange,
		"1.5":   migration.ReasonNotANumber,
	} {
		problem := activityProblem(t, map[string]string{"type": "call", "duration_minutes": text})
		if problem == nil || problem.Why.Code != want {
			t.Errorf("duration %q = %+v, want %s", text, problem, want)
		}
	}
}

func TestAMeetingStatusIsForAMeetingAndInTheStoresVocabulary(t *testing.T) {
	if problem := activityProblem(t, map[string]string{"type": "meeting", "meeting_status": "Held"}); problem != nil {
		t.Fatalf("a held meeting refused: %+v", problem)
	}
	problem := activityProblem(t, map[string]string{"type": "call", "meeting_status": "held"})
	if problem == nil || problem.Why.Code != migration.ReasonMeetingStatusNotForKind {
		t.Fatalf("a held call = %+v, want %s", problem, migration.ReasonMeetingStatusNotForKind)
	}
	problem = activityProblem(t, map[string]string{"type": "meeting", "meeting_status": "postponed"})
	if problem == nil || problem.Why.Code != migration.ReasonMeetingStatusUnknown {
		t.Fatalf("a postponed meeting = %+v, want %s", problem, migration.ReasonMeetingStatusUnknown)
	}
}

func TestADirectionIsInboundOrOutbound(t *testing.T) {
	if problem := activityProblem(t, map[string]string{"type": "email", "direction": "OUTBOUND"}); problem != nil {
		t.Fatalf("an outbound email refused: %+v", problem)
	}
	problem := activityProblem(t, map[string]string{"type": "email", "direction": "sideways"})
	if problem == nil || problem.Why.Code != migration.ReasonDirectionUnknown {
		t.Fatalf("a sideways email = %+v, want %s", problem, migration.ReasonDirectionUnknown)
	}
}

// A day cell is held against today's date in the record zone. Today passes all
// day, and tomorrow is refused even once UTC has reached it.
func TestADayCellIsHeldAgainstTodayInTheRecordZone(t *testing.T) {
	york, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	morning := cellClock{now: time.Date(2026, 3, 9, 8, 0, 0, 0, york).UTC(), zone: york}
	if _, problem := checkActivityCells(map[string]string{"occurred_at": "2026-03-09"}, nil, morning); problem != nil {
		t.Errorf("today's date refused before noon: %+v", problem)
	}
	evening := cellClock{now: time.Date(2026, 3, 9, 22, 0, 0, 0, york).UTC(), zone: york}
	_, problem := checkActivityCells(map[string]string{"occurred_at": "2026-03-10"}, nil, evening)
	if problem == nil || problem.Why.Code != migration.ReasonOccurredAtFuture {
		t.Errorf("tomorrow's date = %+v, want %s", problem, migration.ReasonOccurredAtFuture)
	}
}
