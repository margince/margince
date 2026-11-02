// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

// Both meeting-brief prompts state the deal's amount in major units. A model
// reads the minor-unit integer as an amount a hundred times too large.

import (
	"strings"
	"testing"
)

func TestThePromptsStateADealAmountInMajorUnits(t *testing.T) {
	t.Parallel()
	cases := []struct {
		currency  string
		minor     int64
		forbidden string
		want      string
	}{
		{currency: "EUR", minor: 1_850_000, forbidden: "1850000", want: `"amount":"18500.00"`},
		// JPY has no minor unit, so the integer is the amount and stays unscaled.
		{currency: "JPY", minor: 1_850_000, forbidden: "18500.00", want: `"amount":"1850000"`},
	}
	for _, tc := range cases {
		in := Input{
			ActivityID: "00000000-0000-0000-0000-000000000001",
			Subject:    "Meeting",
			Deal: &DealIn{
				ID: "00000000-0000-0000-0000-000000000002", Name: "Deal B",
				Stage: "Discovery", AmountMinor: new(tc.minor), Currency: tc.currency,
			},
		}
		brief := BriefRequest(in, "en").Messages[0].Content
		plan := encodePlanPrompt(planPromptOf(in, Plan{}))
		for name, body := range map[string]string{"sections": brief, "plan": plan} {
			if strings.Contains(body, "AmountMinor") || strings.Contains(body, tc.forbidden) {
				t.Errorf("%s %s prompt carries the amount in the wrong scale: %s", tc.currency, name, body)
			}
			if !strings.Contains(body, tc.want) {
				t.Errorf("%s %s prompt does not state %s: %s", tc.currency, name, tc.want, body)
			}
		}
	}
}

// A deal with no amount says nothing about one. A zero amount is still an
// amount, and the prompt states it.
func TestAZeroAmountIsStatedAndANullOneIsNot(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		minor  *int64
		want   string
		absent bool
	}{
		{name: "null", minor: nil, absent: true},
		{name: "zero", minor: new(int64(0)), want: `"amount":"0.00"`},
	}
	for _, tc := range cases {
		in := Input{
			ActivityID: "00000000-0000-0000-0000-000000000001",
			Subject:    "Meeting",
			Deal: &DealIn{
				ID: "00000000-0000-0000-0000-000000000002", Name: "Deal B",
				AmountMinor: tc.minor, Currency: "EUR",
			},
		}
		body := BriefRequest(in, "en").Messages[0].Content
		if tc.absent && strings.Contains(body, `"amount"`) {
			t.Errorf("%s: a deal with no amount put one in the prompt: %s", tc.name, body)
		}
		if !tc.absent && !strings.Contains(body, tc.want) {
			t.Errorf("%s: the prompt does not state %s: %s", tc.name, tc.want, body)
		}
	}
}
