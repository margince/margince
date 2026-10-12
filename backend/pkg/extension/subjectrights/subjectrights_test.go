// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package subjectrights

import (
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/pkg/extension/jurisdiction"
	"github.com/margince/margince/backend/pkg/extension/messaging"
)

func cited() []messaging.Instrument { return []messaging.Instrument{{Name: "Test Act 1/2026"}} }

func TestARuleSetIsRefusedWhenItCannotBeApplied(t *testing.T) {
	for _, c := range []struct {
		name  string
		rules Rules
		want  string
	}{
		{"bad code", Rules{Jurisdiction: "DE", Version: 1}, "alpha-2"},
		{"no version", Rules{Jurisdiction: "zz"}, "version 0"},
		{
			"uncited deadline",
			Rules{Jurisdiction: "zz", Version: 1, Deadlines: map[RequestKind]Deadline{Access: {Hours: 72}}},
			"cite no instrument",
		},
		{
			"unknown kind",
			Rules{Jurisdiction: "zz", Version: 1, Instruments: cited(), Deadlines: map[RequestKind]Deadline{"portability": {Hours: 72}}},
			"not one the request queue carries",
		},
		{
			"both units",
			Rules{Jurisdiction: "zz", Version: 1, Instruments: cited(), Deadlines: map[RequestKind]Deadline{Access: {Hours: 72, Period: jurisdiction.Period{Months: 1}}}},
			"both",
		},
		{
			"no unit",
			Rules{Jurisdiction: "zz", Version: 1, Instruments: cited(), Deadlines: map[RequestKind]Deadline{Access: {}}},
			"no period and no hours",
		},
		{
			"hours past a year",
			Rules{Jurisdiction: "zz", Version: 1, Instruments: cited(), Deadlines: map[RequestKind]Deadline{Access: {Hours: 24*366 + 1}}},
			"longer than a year",
		},
		{
			"negative hours",
			Rules{Jurisdiction: "zz", Version: 1, Instruments: cited(), Deadlines: map[RequestKind]Deadline{Access: {Hours: -1}}},
			"reaches back",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := c.rules.Validate()
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("Validate() = %v, want an error containing %q", err, c.want)
			}
		})
	}
	ok := Rules{
		Jurisdiction: "zz", Version: 1, Instruments: cited(),
		Deadlines: map[RequestKind]Deadline{Access: {Hours: 72}, Erasure: {Period: jurisdiction.Period{Months: 1}}},
	}
	if err := ok.Validate(); err != nil {
		t.Fatalf("a cited set with one deadline per unit is refused: %v", err)
	}
	if err := (Rules{Jurisdiction: "zz", Version: 1}).Validate(); err != nil {
		t.Fatalf("a set that declares no deadline needs no citation, got %v", err)
	}
}

// Seventy-two hours near a month end is three days, while a month from the
// last day of January is the last day of February.
func TestADeadlineCountsHoursExactlyAndMonthsToTheMonthEnd(t *testing.T) {
	received := time.Date(2026, time.January, 31, 15, 4, 0, 0, time.UTC)
	if got, want := (Deadline{Hours: 72}).Due(received), time.Date(2026, time.February, 3, 15, 4, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("72 hours after %s = %s, want %s", received, got, want)
	}
	if got, want := (Deadline{Period: jurisdiction.Period{Months: 1}}).Due(received), time.Date(2026, time.February, 28, 15, 4, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("one month after %s = %s, want %s", received, got, want)
	}
	december := time.Date(2026, time.December, 15, 9, 0, 0, 0, time.UTC)
	if got, want := (Deadline{Period: jurisdiction.Period{Months: 1, Days: 2}}).Due(december), time.Date(2027, time.January, 17, 9, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("a month and two days after %s = %s, want %s", december, got, want)
	}
}
