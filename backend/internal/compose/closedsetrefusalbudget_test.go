// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A refusal that names a closed set must be able to DELIVER that set.
//
// The tool surface bounds what a refusal may say (agents.MaxFaultDetail),
// because the text lands in a transcript whose later prompts the same model
// reads. Every vocabulary this file measures is refused against by naming the
// whole set, and each grew independently of that bound — so the two agree only
// while somebody checks, and this is the check.
//
// The defect it exists for: the bound was the figure sized for a bad-args echo,
// and the sets were measured against it and lost. A report's block grammar never
// reached a caller at all, and the analytics populations arrived cut mid-name —
// which is worse than absent, because a truncated list reads as a complete one
// and a caller stops looking for the entry that was removed.
//
// The corpus is DERIVED from the producers rather than listed here. A listed one
// would pass while a fifteenth block kind or a thirteenth population went
// unmeasured, which is the census that fails short.

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/compose/reportdoc"
	"github.com/margince/margince/backend/internal/modules/agents"
)

// floodedToken is what a caller sends when the refusal's budget is the thing
// under attack: far past any bound, so a producer that does not bound its echo
// leaves nothing for the set.
var floodedToken = strings.Repeat("k", 4000)

// setBearingRefusal is one vocabulary, the refusal that names it, and the way
// to provoke that refusal with a token of the caller's choosing.
type setBearingRefusal struct {
	what    string
	members []string
	refuse  func(token string) error
}

// closedSetRefusals derives every set-bearing refusal the tool surface can
// answer with, by asking the producers themselves rather than restating them.
func closedSetRefusals(t *testing.T) []setBearingRefusal {
	t.Helper()

	// The widest seat, because a refusal's length is worst when the caller may
	// see everything — and it is the seat an admin passport actually gets.
	schema := AnalyticsSchemaFor(seatWith(
		"deal", "activity", "person", "organization", "project", "partner"))
	if len(schema.Entities) == 0 {
		t.Fatal("the derived analytics schema is empty, so this census measures nothing")
	}

	pipeline, ok := schema.Entities["pipeline-current"]
	if !ok {
		t.Fatal("pipeline-current is absent from the widest schema; this census needs a real entity")
	}

	return []setBearingRefusal{
		{
			what:    "report block kinds",
			members: reportdoc.KindNames(),
			refuse: func(token string) error {
				_, err := reportdoc.Validate(reportdoc.Document{
					Blocks: []reportdoc.Block{{Kind: reportdoc.Kind(token)}},
				})
				return err
			},
		},
		{
			what:    "analytics populations",
			members: schema.EntityNames(),
			refuse: func(token string) error {
				return analyticsquery.Query{
					Entity:   token,
					Measures: []analyticsquery.Measure{{Fn: analyticsquery.Sum, Field: "amount_base_minor"}},
				}.Validate(schema)
			},
		},
		{
			what:    "the dimensions of one population",
			members: pipeline.FieldNames(analyticsquery.KindDimension),
			refuse: func(token string) error {
				return analyticsquery.Query{
					Entity:   "pipeline-current",
					GroupBy:  []string{token},
					Measures: []analyticsquery.Measure{{Fn: analyticsquery.Sum, Field: "amount_base_minor"}},
				}.Validate(schema)
			},
		},
		{
			// The catalog read here, the refusal built by the engine — so this
			// asserts the engine names every key rather than replaying a set the
			// test handed it. An unknown key is refused before the engine touches
			// anything, which is why a zero-value one answers.
			what:    "prebuilt report keys",
			members: slices.Sorted(maps.Keys(prebuiltReports)),
			refuse: func(token string) error {
				_, err := (&reportEngine{}).Run(seatWith("deal"), token, reportRequest{})
				return err
			},
		},
	}
}

// TestEveryClosedSetSurvivesTheToolSurface is the census: each vocabulary,
// refused against, must arrive whole inside the surface's budget.
func TestEveryClosedSetSurvivesTheToolSurface(t *testing.T) {
	t.Parallel()
	for _, subject := range closedSetRefusals(t) {
		t.Run(subject.what, func(t *testing.T) {
			t.Parallel()
			if len(subject.members) == 0 {
				t.Fatalf("%s came back empty, so this case measures nothing", subject.what)
			}

			err := subject.refuse("a-name-nothing-serves")
			if err == nil {
				t.Fatalf("%s admitted a name it does not serve, so no refusal names the set", subject.what)
			}
			detail := err.Error()

			if len(detail) > agents.MaxFaultDetail {
				t.Errorf("the %s refusal is %d bytes and the tool surface admits %d, so the set is cut "+
					"before a caller reads it:\n%s", subject.what, len(detail), agents.MaxFaultDetail, detail)
			}
			for _, member := range subject.members {
				if !strings.Contains(detail, member) {
					t.Errorf("the %s refusal never names %q, so a caller cannot pick it:\n%s",
						subject.what, member, detail)
				}
			}
		})
	}
}

// TestACallersTokenCannotCrowdOutTheSet holds the other half, and it is what
// makes the surface's larger budget safe: the echoed token is bounded where it
// enters, so the refusal's length is a property of the vocabulary rather than of
// what a caller chose to send.
func TestACallersTokenCannotCrowdOutTheSet(t *testing.T) {
	t.Parallel()
	for _, subject := range closedSetRefusals(t) {
		t.Run(subject.what, func(t *testing.T) {
			t.Parallel()
			err := subject.refuse(floodedToken)
			if err == nil {
				t.Fatalf("%s admitted a 4000-byte name", subject.what)
			}
			detail := err.Error()

			if len(detail) > agents.MaxFaultDetail {
				t.Errorf("a caller's oversized name pushed the %s refusal to %d bytes, past the %d "+
					"the surface admits — so the token is not bounded where it enters",
					subject.what, len(detail), agents.MaxFaultDetail)
			}
			for _, member := range subject.members {
				if !strings.Contains(detail, member) {
					t.Errorf("a caller's oversized name pushed %q out of the %s refusal:\n%s",
						member, subject.what, detail)
				}
			}
		})
	}
}
