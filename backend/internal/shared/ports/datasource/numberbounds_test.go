// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datasource

import (
	"math"
	"testing"
)

type pageSize int

type boundedItem struct {
	Weight float64 `json:"weight" minimum:"0.5"`
}

type boundedBody struct {
	Limit   *pageSize     `json:"limit,omitempty" maximum:"25" minimum:"1"`
	Days    int           `json:"days" minimum:"0"`
	Ceiling uint          `json:"ceiling" maximum:"10"`
	Huge    *int64        `json:"huge,omitempty" maximum:"9007199254740992"`
	Items   []boundedItem `json:"items"`
}

func TestRejectOutOfBoundsNumbersNamesTheNumberOutsideItsRange(t *testing.T) {
	cases := map[string]struct {
		body boundedBody
		want string
	}{
		"omitted optional number":   {body: boundedBody{}},
		"on the lower bound":        {body: boundedBody{Limit: new(pageSize(1))}},
		"on the upper bound":        {body: boundedBody{Limit: new(pageSize(25))}},
		"below the minimum":         {body: boundedBody{Limit: new(pageSize(0))}, want: "limit"},
		"above the maximum":         {body: boundedBody{Limit: new(pageSize(26))}, want: "limit"},
		"a negative required count": {body: boundedBody{Days: -1}, want: "days"},
		"an omitted required count": {body: boundedBody{Items: []boundedItem{{}}}},
		"an unsigned past its cap":  {body: boundedBody{Ceiling: 11}, want: "ceiling"},
		"one past a bound 2^53":     {body: boundedBody{Huge: new(int64(9007199254740993))}, want: "huge"},
		"a decimal bound in a list": {body: boundedBody{Items: []boundedItem{{Weight: 1}, {Weight: 0.25}}}, want: "items[1].weight"},
		"the largest int":           {body: boundedBody{Huge: new(int64(math.MaxInt64))}, want: "huge"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := RejectOutOfBoundsNumbers(&tc.body)
			switch {
			case tc.want == "" && got != nil:
				t.Fatalf("refused %q, want accepted", got.Field)
			case tc.want != "" && (got == nil || got.Field != tc.want):
				t.Fatalf("refusal %+v, want one naming %q", got, tc.want)
			}
		})
	}
}

func TestAnOutOfBoundsRefusalNamesTheRangeAndTheValue(t *testing.T) {
	for name, tc := range map[string]struct {
		body boundedBody
		want string
	}{
		"both bounds":  {boundedBody{Limit: new(pageSize(0))}, "`limit` must be a number from 1 to 25, not 0"},
		"a floor only": {boundedBody{Days: -2}, "`days` must be a number of at least 0, not -2"},
		"a cap only":   {boundedBody{Ceiling: 12}, "`ceiling` must be a number of at most 10, not 12"},
	} {
		got := RejectOutOfBoundsNumbers(&tc.body)
		if got == nil || got.Error() != tc.want {
			t.Errorf("%s: refusal %v, want %q", name, got, tc.want)
		}
	}
}
