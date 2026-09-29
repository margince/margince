// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The two halves that need no database: reading `HH:MM` off the wire, and the
// refusals a form puts against the control the reader used.

import (
	"errors"
	"testing"
)

// The clock spellings the wire admits, and what each one means. The contract's
// `start_time`/`end_time` pattern is held against this same table, so the regex
// a generated client validates with and the parser the server runs cannot come
// to accept different days.
var clockTimesAccepted = map[string]int{
	"00:00": 0,
	"09:00": 9 * 60,
	"09:30": 9*60 + 30,
	"13:45": 13*60 + 45,
	"23:59": 23*60 + 59,
	// The end of the day, and the reason this is not time.Parse: a working day
	// that runs to midnight is one somebody keeps, and 23:59 is a different
	// answer.
	"24:00": 24 * 60,
}

// "9:00" is here on purpose: a single-digit hour is the shape a hand-written
// client sends, and admitting it would make the wire format two formats. "09"
// carries no colon, so no digit count makes it a time. The signed spellings
// are the same invariant from the other side: a two-character field whose
// first character is a sign is two characters long, so a length check alone
// lets the wire mean something the pattern never admits.
var clockTimesRefused = []string{
	"", "9:00", "0900", "24:01", "25:00", "09:60", "aa:bb", "09",
	"+9:00", "09:+5", "+9:+9",
}

func TestATimeOnTheWireIsReadAsTheMinuteItNames(t *testing.T) {
	for written, want := range clockTimesAccepted {
		t.Run(written, func(t *testing.T) {
			got, err := minutePastMidnight(written)
			if err != nil {
				t.Fatalf("reading %q: %v", written, err)
			}
			if got != want {
				t.Errorf("%q read as %d, want %d", written, got, want)
			}
		})
	}
}

func TestATimeThatIsNotOneIsRefused(t *testing.T) {
	for _, written := range clockTimesRefused {
		t.Run(written, func(t *testing.T) {
			if _, err := minutePastMidnight(written); err == nil {
				t.Errorf("%q was read as a time", written)
			}
		})
	}
}

func TestTheWorkingDayIsNormalizedAndItsRefusalsNameAField(t *testing.T) {
	valid := WorkingHours{
		StartMinute: 9 * 60, EndMinute: 17 * 60,
		Days: []int{5, 1, 1, 3}, Timezone: "Europe/Berlin",
	}
	normalized, err := validWorkingHours(valid)
	if err != nil {
		t.Fatalf("a valid week was refused: %v", err)
	}
	// Sorted and deduplicated: the days are a set, and a reader of the stored
	// row should not have to sort it to see the week.
	if len(normalized.Days) != 3 || normalized.Days[0] != 1 || normalized.Days[2] != 5 {
		t.Errorf("days normalized to %v, want 1,3,5", normalized.Days)
	}

	for _, refused := range []struct {
		name  string
		in    WorkingHours
		field string
	}{
		{"a start outside the day", WorkingHours{StartMinute: -1, EndMinute: 60, Days: []int{1}, Timezone: "UTC"}, fieldWorkStart},
		{"an end past midnight", WorkingHours{StartMinute: 0, EndMinute: 24*60 + 1, Days: []int{1}, Timezone: "UTC"}, fieldWorkEnd},
		{"an end before its start", WorkingHours{StartMinute: 600, EndMinute: 60, Days: []int{1}, Timezone: "UTC"}, fieldWorkEnd},
		{"a week with no days", WorkingHours{StartMinute: 0, EndMinute: 60, Timezone: "UTC"}, fieldWorkDays},
		{"a day that is not one", WorkingHours{StartMinute: 0, EndMinute: 60, Days: []int{8}, Timezone: "UTC"}, fieldWorkDays},
		{"a zone that is not one", WorkingHours{StartMinute: 0, EndMinute: 60, Days: []int{1}, Timezone: "Mars/Olympus"}, fieldWorkTimezone},
		{"no zone at all", WorkingHours{StartMinute: 0, EndMinute: 60, Days: []int{1}}, fieldWorkTimezone},
	} {
		t.Run(refused.name, func(t *testing.T) {
			_, err := validWorkingHours(refused.in)
			var fault *WorkingHoursError
			if !errors.As(err, &fault) {
				t.Fatalf("err = %v, want a refusal naming a field", err)
			}
			field, _, _ := fault.FieldFault()
			if field != refused.field {
				t.Errorf("the refusal names %q, want %q", field, refused.field)
			}
		})
	}
}

func TestTheFallbackIsTheStatedOneAndReadsItsOwnDays(t *testing.T) {
	hours := DefaultWorkingHours("Europe/Berlin")
	if hours.StartMinute != 9*60 || hours.EndMinute != 17*60 {
		t.Errorf("the fallback is %d–%d, want 09:00–17:00", hours.StartMinute, hours.EndMinute)
	}
	for _, weekday := range []int{1, 2, 3, 4, 5} {
		if !hours.Works(weekday) {
			t.Errorf("the fallback does not work on ISO day %d", weekday)
		}
	}
	for _, weekend := range []int{6, 7} {
		if hours.Works(weekend) {
			t.Errorf("the fallback works on ISO day %d", weekend)
		}
	}
	if _, err := hours.Location(); err != nil {
		t.Errorf("the fallback zone does not resolve: %v", err)
	}
	if _, err := (WorkingHours{Timezone: "Mars/Olympus"}).Location(); err == nil {
		t.Error("a zone this server does not carry resolved — which is how a scheduler " +
			"silently falls back to UTC, the defect this setting exists to remove")
	}
}
