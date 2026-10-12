// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datasource

import (
	"testing"
	"time"
)

type rangeItem struct {
	At time.Time `json:"at"`
}

type rangeBody struct {
	DueAt *time.Time  `json:"due_at,omitempty"`
	Items []rangeItem `json:"items"`
	Plain time.Time
	Extra map[string]any `json:"-"`
}

func at(year int, month time.Month, day, hour int) time.Time {
	return time.Date(year, month, day, hour, 0, 0, 0, time.UTC)
}

func TestRejectOutOfRangeTimesNamesTheFieldAtEachEnd(t *testing.T) {
	cases := map[string]struct {
		body rangeBody
		want string
	}{
		"last accepted day":   {body: rangeBody{DueAt: new(at(9999, time.December, 30, 23))}},
		"first accepted day":  {body: rangeBody{DueAt: new(at(1, time.January, 2, 0))}},
		"omitted fields":      {body: rangeBody{}},
		"zero time sent":      {body: rangeBody{DueAt: new(time.Time{})}, want: "due_at"},
		"one day too late":    {body: rangeBody{DueAt: new(at(9999, time.December, 31, 0))}, want: "due_at"},
		"one hour too early":  {body: rangeBody{DueAt: new(at(1, time.January, 1, 23))}, want: "due_at"},
		"year zero":           {body: rangeBody{DueAt: new(at(0, time.June, 1, 0))}, want: "due_at"},
		"inside a list":       {body: rangeBody{Items: []rangeItem{{At: at(2026, 1, 1, 0)}, {At: at(9999, 12, 31, 12)}}}, want: "items[1].at"},
		"field without a tag": {body: rangeBody{Plain: at(9999, 12, 31, 12)}, want: "Plain"},
		"additional property": {body: rangeBody{Extra: map[string]any{"cf_when": at(9999, 12, 31, 12)}}, want: "cf_when"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := RejectOutOfRangeTimes(&tc.body)
			switch {
			case tc.want == "" && got != nil:
				t.Fatalf("refused %q, want accepted", got.Field)
			case tc.want != "" && (got == nil || got.Field != tc.want):
				t.Fatalf("refusal %+v, want one naming %q", got, tc.want)
			}
		})
	}
}

// The provider seam shares the bound, so an agent cannot store what the REST
// body decode refuses.
func TestStrictDecodeRefusesADateTimeNoZoneCanRender(t *testing.T) {
	var into rangeBody
	err := StrictDecode([]byte(`{"due_at":"9999-12-31T23:59:59Z"}`), &into)
	if _, ok := err.(*FieldDecodeError); !ok {
		t.Fatalf("StrictDecode → %v, want a FieldDecodeError", err)
	}
}
