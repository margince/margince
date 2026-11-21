// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The mark orders as the walk does: by instant, then by id within one instant.
func TestAClockRefoldMarkOrdersByInstantThenByID(t *testing.T) {
	instant := time.Date(2026, 5, 4, 9, 30, 0, 0, time.UTC)
	low := ids.MustParse("00000000-0000-7000-8000-000000000001")
	high := ids.MustParse("00000000-0000-7000-8000-000000000002")
	cases := []struct {
		name        string
		mark, other ClockRefoldMark
		before      bool
	}{
		{"an earlier instant", ClockRefoldMark{At: instant, ID: high}, ClockRefoldMark{At: instant.Add(time.Second), ID: low}, true},
		{"a later instant", ClockRefoldMark{At: instant.Add(time.Second), ID: low}, ClockRefoldMark{At: instant, ID: high}, false},
		{"a lower id at one instant", ClockRefoldMark{At: instant, ID: low}, ClockRefoldMark{At: instant, ID: high}, true},
		{"a higher id at one instant", ClockRefoldMark{At: instant, ID: high}, ClockRefoldMark{At: instant, ID: low}, false},
		{"the same mark", ClockRefoldMark{At: instant, ID: low}, ClockRefoldMark{At: instant, ID: low}, false},
		{"one instant in another zone", ClockRefoldMark{At: instant.In(time.FixedZone("east", 7200)), ID: low}, ClockRefoldMark{At: instant, ID: low}, false},
	}
	for _, c := range cases {
		if got := c.mark.before(c.other); got != c.before {
			t.Errorf("%s: before answered %t, want %t", c.name, got, c.before)
		}
	}
}

// The stored names are the ones the migration wrote, so a mark moved from the
// old table reads back.
func TestAClockRefoldMarkReadsTheShapeTheMigrationStored(t *testing.T) {
	stored := `{"passed_up_to": "2026-05-04T09:30:00.123456+00:00", "passed_id": "00000000-0000-7000-8000-000000000001"}`
	var mark ClockRefoldMark
	if err := json.Unmarshal([]byte(stored), &mark); err != nil {
		t.Fatalf("reading a migrated mark: %v", err)
	}
	want := ClockRefoldMark{
		At: time.Date(2026, 5, 4, 9, 30, 0, 123456000, time.UTC),
		ID: ids.MustParse("00000000-0000-7000-8000-000000000001"),
	}
	if !mark.At.Equal(want.At) || mark.ID != want.ID {
		t.Fatalf("a migrated mark read as %+v, want %+v", mark, want)
	}
}
