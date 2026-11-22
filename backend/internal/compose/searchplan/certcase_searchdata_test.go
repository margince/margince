// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package searchplan

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
)

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

// A case may accept several plans that read the same rows; a plan that is none
// of them is still wrong.
func TestACaseAcceptsEachPlanItLists(t *testing.T) {
	plan := func(entity string) string {
		return `{"engine":"analytics","count":false,"arguments":"{\"entity\":\"` + entity + `\",\"measures\":[{\"fn\":\"count\"}],\"scope_kind\":\"workspace\"}"}`
	}
	prepared, err := Cases{}.Prepare([]byte(`{"question":"How many open deals?","vocabulary":"{\"context\":{\"base_currency\":\"EUR\"}}"}`),
		[]byte(`[`+plan("open-deals-per-company")+`,`+plan("deals-by-stage")+`]`))
	if err != nil {
		t.Fatal(err)
	}
	for entity, want := range map[string]string{
		"open-deals-per-company": aitasks.OutcomeAccepted,
		"deals-by-stage":         aitasks.OutcomeAccepted,
		"loss-events":            aitasks.OutcomeWrongAnswer,
	} {
		if got := prepared.Evaluate(aitasks.Trace{Output: plan(entity)}); got.Result != want {
			t.Errorf("%s: %v, want %v", entity, got, want)
		}
	}
}

// An empty list and an absent one ask the same: no grouping is no grouping.
func TestAnEmptyListComparesAsAnAbsentOne(t *testing.T) {
	plan := func(groupBy string) Plan {
		return Plan{Engine: EngineAnalytics, Arguments: `{"entity":"deals-by-stage","measures":[{"fn":"count"}]` + groupBy + `,"scope_kind":"workspace"}`}
	}
	if same, err := sameSearchPlanArguments(plan(""), plan(`,"group_by":[]`), ""); err != nil || !same {
		t.Errorf("group_by [] read as another plan than none (%v)", err)
	}
	if same, err := sameSearchPlanArguments(plan(""), plan(`,"group_by":["stage_id"]`), ""); err != nil || same {
		t.Errorf("a grouping read as no grouping (%v)", err)
	}
}

// Arguments the model fenced are the arguments it wrote: the comparison reads
// them through the same reduction the decoder does.
func TestFencedArgumentsCompareAsTheirContent(t *testing.T) {
	plain := `{"entity":"deals-by-stage","measures":[{"fn":"count"}],"scope_kind":"workspace"}`
	want := Plan{Engine: EngineAnalytics, Arguments: plain}
	got := Plan{Engine: EngineAnalytics, Arguments: "```json\n" + plain + "\n```"}
	if same, err := sameSearchPlanArguments(want, got, ""); err != nil || !same {
		t.Errorf("fenced arguments read as another plan (%v)", err)
	}
}
