// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import "testing"

func TestTheMeetingBriefAndPlanKeepADealsOwnAmountAndDropAnInventedSum(t *testing.T) {
	in := fullInput()
	floor := floorFor(in)
	cite := `"evidence":[{"entity_type":"deal","entity_id":"` + dealID + `"}]`
	cases := map[string]int{
		"Ask how the 95,000 EUR proposal sits with finance.":  1,
		"Ask how the 950,000 EUR proposal sits with finance.": 0,
	}
	for text, want := range cases {
		sections, err := parseCitedSections(`{"sections":[{"kind":"goal","sentences":[{"text":"`+text+`","nature":"recommendation",`+cite+`}]}]}`, in)
		if err != nil || len(sections) != want {
			t.Errorf("the brief on %q kept %d sections (%v), want %d", text, len(sections), err, want)
		}
		plan, err := parseCitedPlan(`{"opening":{"text":"`+text+`","nature":"recommendation",`+cite+`}}`, in, floor)
		if kept := err == nil && plan.Opening != nil && plan.Opening.Text == text; kept != (want == 1) {
			t.Errorf("the plan on %q kept = %v (%v), want %v", text, kept, err, want == 1)
		}
	}
}
