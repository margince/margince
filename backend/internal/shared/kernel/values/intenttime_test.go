// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestIntentResolvesWithCurrentTimezoneRules(t *testing.T) {
	body, err := os.ReadFile("testdata/intenttime.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct{ Wall, Zone, Instant string }
	if err := json.Unmarshal(body, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 8 {
		t.Fatal("local-time cases are missing")
	}
	for _, c := range cases {
		t.Run(c.Zone+"/"+c.Wall, func(t *testing.T) {
			zone, err := ParseTimezone(c.Zone)
			if err != nil {
				t.Fatal(err)
			}
			intent, err := ParseIntentTime(c.Wall, zone)
			if err != nil {
				t.Fatal(err)
			}
			got, err := intent.Resolve()
			if err != nil {
				t.Fatal(err)
			}
			want, err := time.Parse(time.RFC3339Nano, c.Instant)
			if err != nil {
				t.Fatal(err)
			}
			if !got.Equal(want) {
				t.Fatalf("got %s, want %s", got, want)
			}
		})
	}
}

func TestIntentRefusesOffsetsAndInvalidDates(t *testing.T) {
	zone, err := ParseTimezone("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	for _, wall := range []string{"", "2026-02-30T09:00", "2026-01-01T09:00Z", "2026-01-01T09:00+01:00", "2026-01-01T09:00:00.1234567", "0000-01-01T00:00"} {
		if _, err := ParseIntentTime(wall, zone); err == nil {
			t.Errorf("accepted %q", wall)
		}
	}
	if _, err := ParseIntentTime("2026-01-01T09:00", Timezone{}); err == nil {
		t.Fatal("accepted missing timezone")
	}
	if _, err := (IntentTime{}).Resolve(); err == nil {
		t.Fatal("resolved missing intent")
	}
}

func TestIntentResolutionUsesTheGivenRules(t *testing.T) {
	wall := time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)
	first, err := resolveIntent(wall, time.FixedZone("rules", 3600))
	if err != nil {
		t.Fatal(err)
	}
	changed, err := resolveIntent(wall, time.FixedZone("rules", 7200))
	if err != nil {
		t.Fatal(err)
	}
	if first.Sub(changed) != time.Hour {
		t.Fatal("changed timezone rules did not change the instant")
	}
}
