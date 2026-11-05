// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package elapsed

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestCalendarAgesSharedWithTheBoard(t *testing.T) {
	raw, err := os.ReadFile("testdata/ages.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name, At, Now, Zone string
		Days                int
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 6 {
		t.Fatal("calendar fixture census is incomplete")
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			at, err := time.Parse(time.RFC3339, c.At)
			if err != nil {
				t.Fatal(err)
			}
			now, err := time.Parse(time.RFC3339, c.Now)
			if err != nil {
				t.Fatal(err)
			}
			zone, err := time.LoadLocation(c.Zone)
			if err != nil {
				t.Fatal(err)
			}
			if got := Days(at, now, zone); got != c.Days {
				t.Fatalf("Days = %d, want %d", got, c.Days)
			}
		})
	}
}
