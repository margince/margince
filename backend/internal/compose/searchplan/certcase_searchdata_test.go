// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package searchplan

import "testing"

const datedVocabulary = `{"record_vocabulary":{"targets":[{"target":"deal","fields":[
{"name":"expected_close_date","kind":"date"},{"name":"created_at","kind":"timestamp"}]}]}}`

// A closed bound on a date selects the days the half-open one does, so the two
// are one plan. On an instant, or past the day, they are not.
func TestAClosedDateBoundIsTheHalfOpenPlanItEquals(t *testing.T) {
	plan := func(where string) Plan {
		return Plan{Engine: EngineRecords, Arguments: `{"plan":{"version":"v1","target":"deal","where":[` + where + `]}}`}
	}
	want := plan(`{"field":"expected_close_date","op":"gte","value":"2026-11-01"},{"field":"expected_close_date","op":"lt","value":"2026-12-01"}`)
	for name, test := range map[string]struct {
		got  Plan
		same bool
	}{
		"the last day, closed":  {plan(`{"field":"expected_close_date","op":"lte","value":"2026-11-30"},{"field":"expected_close_date","op":"gte","value":"2026-11-01"}`), true},
		"the day before, open":  {plan(`{"field":"expected_close_date","op":"gt","value":"2026-10-31"},{"field":"expected_close_date","op":"lt","value":"2026-12-01"}`), true},
		"one day short":         {plan(`{"field":"expected_close_date","op":"gte","value":"2026-11-01"},{"field":"expected_close_date","op":"lt","value":"2026-11-30"}`), false},
		"the first day, closed": {plan(`{"field":"expected_close_date","op":"gte","value":"2026-11-01"},{"field":"expected_close_date","op":"lte","value":"2026-12-01"}`), false},
	} {
		same, err := sameSearchPlanArguments(want, test.got, datedVocabulary)
		if err != nil || same != test.same {
			t.Errorf("%s: same=%v (%v), want %v", name, same, err, test.same)
		}
	}
	instant := func(op, value string) Plan {
		return plan(`{"field":"created_at","op":"` + op + `","value":"` + value + `"}`)
	}
	if same, err := sameSearchPlanArguments(instant("lt", "2026-12-01"), instant("lte", "2026-11-30"), datedVocabulary); err != nil || same {
		t.Errorf("a closed bound on an instant read as the half-open one: %v", err)
	}
}
