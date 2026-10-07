// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// A result column's name is its only handle on the row, so a second column
// answering to the same name replaces the first rather than sitting beside it.
func TestAPlanNamingTwoColumnsTheSameIsRefused(t *testing.T) {
	spec := prebuiltReports["deals-by-stage"]
	cases := map[string]struct {
		groupBy    []string
		aggregates []reportAggregate
	}{
		"an aggregate named like a group-by column": {
			groupBy:    []string{"stage_id"},
			aggregates: []reportAggregate{{Fn: aggFnCount, As: "stage_id"}},
		},
		"two aggregates sharing an alias": {
			aggregates: []reportAggregate{{Fn: aggFnCount, As: "n"}, {Fn: aggFnCount, As: "n"}},
		},
		"a bare aggregate repeated": {
			aggregates: []reportAggregate{{Fn: aggFnCount}, {Fn: aggFnCount}},
		},
		"a dimension grouped twice": {
			groupBy: []string{"stage_id", "stage_id"},
		},
	}
	for name, plan := range cases {
		t.Run(name, func(t *testing.T) {
			_, _, err := buildSelectList(spec, plan.groupBy, plan.aggregates)
			assertReportPlanRefused(t, err)
		})
	}
}

// A period value no bucket can ever equal answers "no deals closed" for a
// mistyped request, which reads as a real answer.
func TestAMalformedPeriodFilterIsRefusedNotAnsweredEmpty(t *testing.T) {
	for _, bad := range []struct{ filter, value string }{
		{fieldPeriodYear, "abc"},
		{fieldPeriodYear, "26"},
		{fieldPeriodQuarter, "2026-Q5"},
		{fieldPeriodQuarter, "2026Q1"},
		{fieldPeriodMonth, "2026-13"},
		{fieldPeriodMonth, "2026-1"},
	} {
		t.Run(bad.filter+"="+bad.value, func(t *testing.T) {
			_, err := reportFilterValue(bad.filter, bad.value)
			assertReportPlanRefused(t, err)
		})
	}
	// Both spellings an installation's fiscal start can render are real buckets.
	for _, good := range []struct{ filter, value string }{
		{fieldPeriodYear, "2026"},
		{fieldPeriodYear, "FY2025/26"},
		{fieldPeriodQuarter, "2026-Q4"},
		{fieldPeriodQuarter, "FY2099/00-Q1"},
		{fieldPeriodMonth, "2026-12"},
		{fieldSource, "anything goes"},
	} {
		if _, err := reportFilterValue(good.filter, good.value); err != nil {
			t.Errorf("%s=%q refused: %v", good.filter, good.value, err)
		}
	}
}

func assertReportPlanRefused(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("the plan was accepted, want a refusal")
	}
	var fault apperrors.MessageFault
	if !errors.As(err, &fault) {
		t.Fatalf("refusal %T carries no message fault, so it would reach a caller as a server error", err)
	}
	if code, _ := fault.MessageFault(); code != reportFieldNotAllowedCode {
		t.Errorf("refusal code = %q, want %q", code, reportFieldNotAllowedCode)
	}
}
