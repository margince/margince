// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "testing"

func TestCommitmentMeaningKeepsNonActionsOutOfWork(t *testing.T) {
	for _, state := range commitmentMeaningStates {
		t.Run(state, func(t *testing.T) {
			meaning := &commitmentMeaning{State: state, DeadlinePrecision: "none"}
			if state == "conditional" {
				meaning.Condition = "if the buyer agrees"
			}
			if reason := validateCommitmentMeaning(meaning, "", "I will send it if the buyer agrees."); reason != "" {
				t.Fatal(reason)
			}
			outcome := meaning.withheldOutcome()
			if state == "open" {
				if outcome != "" {
					t.Fatalf("open promise withheld: %s", outcome)
				}
			} else if outcome == "" || !outcome.withheld() {
				t.Fatalf("%s became actionable: %s", state, outcome)
			}
		})
	}
}

func TestCommitmentMeaningPreservesDeadlinePrecision(t *testing.T) {
	for _, tc := range []struct {
		name, precision, quote, due, source string
		valid                               bool
	}{
		{"no deadline", "none", "", "", "I will send the scope.", true},
		{"explicit day", "day", "8 September 2026", "2026-09-08", "I will send it on 8 September 2026.", true},
		{"range stays undated", "range", "next week", "", "I will send it next week.", true},
		{"range cannot become a day", "range", "next week", "2026-09-08", "I will send it next week.", false},
		{"relabelled English range", "day", "next week", "2026-09-08", "I will send it next week.", false},
		{"relabelled German range", "day", "nächste Woche", "2026-09-08", "Ich sende es nächste Woche.", false},
		{"relabelled Vietnamese range", "day", "tuần tới", "2026-09-08", "Tôi sẽ gửi vào tuần tới.", false},
		{"missing day", "day", "Friday", "", "I will send it Friday.", false},
		{"invented quote", "day", "Friday", "2026-09-11", "I will send it.", false},
		{"no deadline cannot have a date", "none", "", "2026-09-08", "I will send it.", false},
		{"unknown precision", "sometime", "", "", "I will send it.", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			meaning := &commitmentMeaning{State: "open", DeadlinePrecision: tc.precision, DeadlineQuote: tc.quote}
			got := validateCommitmentMeaning(meaning, tc.due, tc.source)
			if (got == "") != tc.valid {
				t.Fatalf("valid=%v, reason=%q", tc.valid, got)
			}
		})
	}
}

func TestCommitmentMeaningRefusesMissingOrContradictoryState(t *testing.T) {
	for _, meaning := range []*commitmentMeaning{
		nil,
		{State: "invented", DeadlinePrecision: "none"},
		{State: "conditional", DeadlinePrecision: "none"},
		{State: "open", Condition: "if useful", DeadlinePrecision: "none"},
		{State: "conditional", Condition: "if useful", DeadlinePrecision: "none"},
	} {
		if reason := validateCommitmentMeaning(meaning, "", "The signed form is attached."); reason == "" {
			t.Fatalf("unusable meaning accepted: %+v", meaning)
		}
	}
	var absent *commitmentMeaning
	if got := absent.withheldOutcome(); got != "semantic_unassessed" {
		t.Fatalf("missing meaning = %s", got)
	}
}
