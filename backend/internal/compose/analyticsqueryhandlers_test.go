// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"reflect"
	"testing"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A saved run echoes the question it stored, so a client re-asking from the
// echo asks the same one. Every field of both shapes is filled here, so a field
// added to either fails until the two conversions carry it.
func TestASavedQuestionSurvivesTheWireBothWays(t *testing.T) {
	t.Parallel()
	asked := analyticsquery.Query{
		Entity:    "open-deals-per-company",
		GroupBy:   []string{"company_id"},
		Measures:  []analyticsquery.Measure{{Fn: analyticsquery.Sum, Field: "amount_minor", As: "total"}},
		Filters:   []analyticsquery.Filter{{Field: "owner_id", Op: analyticsquery.OpEq, Value: "someone"}},
		Limit:     50,
		ScopeKind: ScopeKindOwner,
		ScopeID:   ids.NewV7().String(),
	}
	assertEveryFieldSet(t, reflect.ValueOf(asked), "analyticsquery.Query")

	wire, err := wireFromQuery(asked)
	if err != nil {
		t.Fatal(err)
	}
	// Save alone stays behind: it is an instruction about one call, not part
	// of the question.
	save := true
	wire.Save = &save
	assertEveryFieldSet(t, reflect.ValueOf(wire), "crmcontracts.AnalyticsQuery")

	if back := queryFromWire(wire); !reflect.DeepEqual(back, asked) {
		t.Errorf("the question came back as %+v, want %+v", back, asked)
	}
}

// A question that named no limit is echoed with the one the engine applied.
func TestAnUnboundedQuestionEchoesTheLimitItRanUnder(t *testing.T) {
	t.Parallel()
	wire, err := wireFromQuery(analyticsquery.Query{Entity: "deals-by-stage"})
	if err != nil {
		t.Fatal(err)
	}
	if want := analyticsquery.AppliedLimit(0); wire.Limit == nil || *wire.Limit != want {
		t.Errorf("the echo's limit is %v, want the applied %d", wire.Limit, want)
	}
}
