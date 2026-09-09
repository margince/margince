// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A refusal that names a closed set must be able to DELIVER that set, and a
// caller must not be able to push it out.
//
// The tool surface bounds what a refusal may say, because the text lands in a
// transcript whose later prompts the same model reads. Every vocabulary below is
// refused against by naming the whole set, and each grew independently of that
// bound — so the two agree only while somebody checks.
//
// A truncated set is worse than an absent one: it reads as complete, so a caller
// stops looking for the entry that was removed.
//
// WHAT IS DERIVED AND WHAT IS NOT, said plainly, because the first pass of this
// file claimed the whole of it and was wrong. Every set's MEMBERS come from the
// producer that owns them, so a member added or removed moves this test. The
// SUBJECT LIST is hand-built and cannot be otherwise: provoking a refusal means
// composing a call that earns it, and no reflection writes that. What guards the
// list instead is that every subject asserts its members are non-empty, so a
// vocabulary that silently empties fails here rather than passing vacuously.
//
// AND IT ONLY SEES compose's OWN vocabularies. It cannot reach the ones in
// modules/agents, modules/search or shared/ports/datasource — compose is
// downstream of all three, so importing them here is legal but provoking their
// refusals from this package is not the same test. Those carry the same
// obligation and are held where they live; this file does not speak for them.
//
// IT ASSERTS ON THE CLASSIFIED FAULT, not on err.Error(). The renderer is not
// the only ceiling: httperr bounds a module-declared fault at MaxFaultText
// BEFORE the renderer is reached, so a MessageFault-bearing vocabulary is cut at
// the smaller figure and a test measuring the raw error would never see it.
// Asking httperr.Classify is asking the surface rather than a model of it.

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/compose/reportdoc"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// floodedCallerToken is what a caller sends when the refusal's budget is the
// thing under attack: far past any bound, so a producer that does not bound its
// echo leaves nothing for the set.
const floodedCallerTokenLen = 4000

func floodedCallerToken() string { return strings.Repeat("k", floodedCallerTokenLen) }

// setBearingRefusal is one vocabulary, the refusal that names it, and the way to
// provoke that refusal with a token of the caller's choosing.
type setBearingRefusal struct {
	what    string
	members []string
	refuse  func(token string) error
}

// closedSetRefusals is compose's set-bearing refusals, each vocabulary taken
// from the producer that owns it.
//
// It fatals on an empty or unresolvable subject rather than leaving that to the
// callers: two of the three tests below loop over members and would be green
// over an empty list, which is the vacuous pass this file exists to prevent.
func closedSetRefusals(t *testing.T) []setBearingRefusal {
	t.Helper()

	// The widest seat, because a refusal's length is worst when the caller may
	// see everything — and it is the seat an admin passport actually gets.
	schema := AnalyticsSchemaFor(seatWith(
		"deal", "activity", "person", "company", "project", "partner"))
	if len(schema.Entities) == 0 {
		t.Fatal("the derived analytics schema is empty, so this census measures nothing")
	}
	const entity = "pipeline-current"
	pipeline, ok := schema.Entities[entity]
	if !ok {
		t.Fatalf("%s is absent from the widest schema; this census needs a real entity", entity)
	}
	measure := analyticsquery.Measure{Fn: analyticsquery.Sum, Field: "amount_base_minor"}

	// A named report, resolved rather than indexed: a renamed key would
	// otherwise hand two of the tests below an empty vocabulary and pass.
	const namedReport = "deals-by-stage"
	reportSpec, ok := prebuiltReports[namedReport]
	if !ok {
		t.Fatalf("%s is absent from the report catalog; this census needs a real report", namedReport)
	}

	subjects := []setBearingRefusal{
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
			what:    "report block severities",
			members: reportdoc.Severities(),
			refuse: func(token string) error {
				_, err := reportdoc.Validate(reportdoc.Document{
					Blocks: []reportdoc.Block{{
						Kind:     reportdoc.KindCallout,
						Text:     "a callout states its severity",
						Severity: reportdoc.Severity(token),
					}},
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
					Measures: []analyticsquery.Measure{measure},
				}.Validate(schema)
			},
		},
		{
			what:    "the dimensions of one population",
			members: pipeline.FieldNames(analyticsquery.KindDimension),
			refuse: func(token string) error {
				return analyticsquery.Query{
					Entity:   entity,
					GroupBy:  []string{token},
					Measures: []analyticsquery.Measure{measure},
				}.Validate(schema)
			},
		},
		{
			what:    "the measures of one population",
			members: pipeline.FieldNames(analyticsquery.KindMeasure),
			refuse: func(token string) error {
				return analyticsquery.Query{
					Entity:   entity,
					Measures: []analyticsquery.Measure{{Fn: analyticsquery.Sum, Field: token}},
				}.Validate(schema)
			},
		},
		{
			what:    "analytics aggregates",
			members: analyticsquery.AggregateNames(),
			refuse: func(token string) error {
				return analyticsquery.Query{
					Entity: entity,
					Measures: []analyticsquery.Measure{
						{Fn: analyticsquery.AggFn(token), Field: "amount_base_minor"},
					},
				}.Validate(schema)
			},
		},
		{
			what:    "analytics comparison operators",
			members: analyticsquery.FilterOpNames(),
			refuse: func(token string) error {
				return analyticsquery.Query{
					Entity:   entity,
					Measures: []analyticsquery.Measure{measure},
					Filters: []analyticsquery.Filter{
						{Field: "currency", Op: analyticsquery.FilterOp(token), Value: "EUR"},
					},
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
		{
			// A MessageFault, so httperr caps it at MaxFaultText before the
			// renderer is reached. This is the subject the first census could not
			// see, and the one whose set was being destroyed outright.
			// The catalog read here, the refusal built by the ENGINE — so this
			// asserts the engine hands over the right vocabulary rather than
			// replaying one the test supplied. Hand-building the error would
			// have tested the renderer and nothing else.
			what:    "the dimensions of one prebuilt report",
			members: allowedReportNames(reportSpec.dimensions),
			refuse: func(token string) error {
				_, err := (&reportEngine{}).Derive(seatWith("deal"), namedReport,
					derivationQuery{GroupBy: []string{token}})
				return err
			},
		},
	}

	for _, subject := range subjects {
		if len(subject.members) == 0 {
			t.Fatalf("%s came back empty, so every case over it would pass vacuously", subject.what)
		}
	}
	return subjects
}

// TestEveryClosedSetSurvivesTheToolSurface is the census: each vocabulary,
// refused against, must arrive whole inside the surface's budget.
func TestEveryClosedSetSurvivesTheToolSurface(t *testing.T) {
	t.Parallel()
	for _, subject := range closedSetRefusals(t) {
		t.Run(subject.what, func(t *testing.T) {
			t.Parallel()
			detail := classifiedDetail(t, subject, "a-name-nothing-serves")

			if len(detail) > agents.MaxFaultDetail {
				t.Errorf("the %s refusal is %d bytes and the tool surface admits %d, so the set is "+
					"cut before a caller reads it:\n%s",
					subject.what, len(detail), agents.MaxFaultDetail, detail)
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
			detail := classifiedDetail(t, subject, floodedCallerToken())

			if len(detail) > agents.MaxFaultDetail {
				t.Errorf("a caller's oversized name pushed the %s refusal to %d bytes, past the %d "+
					"the surface admits — so the token is not bounded where it enters",
					subject.what, len(detail), agents.MaxFaultDetail)
			}
			if strings.Contains(detail, strings.Repeat("k", httperr.MaxCallerToken+1)) {
				t.Errorf("the %s refusal echoed more of the caller's token than MaxCallerToken (%d) "+
					"admits:\n%s", subject.what, httperr.MaxCallerToken, detail)
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

// TestEveryClosedSetMemberFitsTheCallerTokenBound holds the sentence that
// justifies MaxCallerToken's figure — "every name in every closed vocabulary is
// comfortably inside it" — which was a claim with nothing behind it. A member
// longer than the bound would be truncated when a caller named it back, so the
// refusal would quote a name that matches nothing in the set beside it.
func TestEveryClosedSetMemberFitsTheCallerTokenBound(t *testing.T) {
	t.Parallel()
	for _, subject := range closedSetRefusals(t) {
		for _, member := range subject.members {
			if rendered := httperr.QuoteCaller(member); strings.Contains(rendered, "…") {
				t.Errorf("%s: %q is longer than MaxCallerToken (%d), so a caller naming it back "+
					"is quoted a name that is in no set: %s",
					subject.what, member, httperr.MaxCallerToken, rendered)
			}
		}
	}
}

// classifiedDetail asks the ONE taxonomy what a caller is told, rather than
// reading Error() and hoping the two agree. httperr bounds a module-declared
// fault before any renderer sees it, so this is where a ceiling smaller than the
// renderer's becomes visible.
func classifiedDetail(t *testing.T, subject setBearingRefusal, token string) string {
	t.Helper()
	err := subject.refuse(token)
	if err == nil {
		t.Fatalf("%s admitted a name it does not serve, so no refusal names the set", subject.what)
	}
	fault, ok := httperr.Classify(err)
	if !ok {
		t.Fatalf("%s refused with an error the taxonomy does not classify, so an agent is told "+
			"nothing actionable: %v", subject.what, err)
	}
	return fault.Detail
}
