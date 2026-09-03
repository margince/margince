// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// The catalog is published so an agent can call run_report without guessing, so
// what it publishes has to be what the engine accepts. Both halves are derived
// from prebuiltReports — but from DIFFERENT fields of it, and a name rendered
// under the wrong heading sends a caller to put a filter key in group_by and
// read a refusal for it.
//
// Derived from the catalog rather than listed here, so a report added to the
// engine inherits the check instead of quietly escaping it.
func TestReportToolCatalogPublishesOnlyWhatTheEngineAccepts(t *testing.T) {
	catalog := reportToolCatalog()
	if len(catalog) != len(prebuiltReports) {
		t.Fatalf("catalog has %d entries, the engine has %d prebuilt reports", len(catalog), len(prebuiltReports))
	}
	for _, entry := range catalog {
		spec, served := prebuiltReports[entry.Report]
		if !served {
			t.Errorf("catalog advertises %q, which the engine does not serve", entry.Report)
			continue
		}
		for _, name := range entry.GroupBy {
			if _, ok := spec.dimensions[name]; !ok {
				t.Errorf("%s: group_by advertises %q, not a dimension of that report", entry.Report, name)
			}
		}
		for _, name := range entry.Filters {
			_, equality := spec.filters[name]
			_, threshold := spec.thresholds[name]
			if !equality && !threshold {
				t.Errorf("%s: filters advertises %q, not a filter of that report", entry.Report, name)
			}
		}
		for _, name := range entry.Aggregates {
			if _, ok := spec.measures[name]; !ok {
				t.Errorf("%s: aggregate fields advertises %q, not a measure of that report", entry.Report, name)
			}
		}
	}
}

// The other direction: a vocabulary the engine accepts and the catalog omits is
// a capability no caller can reach, which is the same dead end in reverse.
func TestReportToolCatalogOmitsNothingTheEngineAccepts(t *testing.T) {
	byReport := make(map[string]ReportCatalogEntryView, len(prebuiltReports))
	for _, entry := range reportToolCatalog() {
		byReport[entry.Report] = ReportCatalogEntryView{entry.GroupBy, entry.Filters, entry.Aggregates}
	}
	for report, spec := range prebuiltReports {
		published, ok := byReport[report]
		if !ok {
			t.Errorf("the engine serves %q and the catalog never names it", report)
			continue
		}
		assertCovers(t, report, "group_by", spec.dimensions, published.GroupBy)
		assertCovers(t, report, "filters", spec.filters, published.Filters)
		for name := range spec.thresholds {
			if !slices.Contains(published.Filters, name) {
				t.Errorf("%s: the engine accepts threshold filter %q and the catalog does not publish it", report, name)
			}
		}
		assertCovers(t, report, "aggregate fields", spec.measures, published.Aggregates)
	}
}

// ReportCatalogEntryView is the three vocabularies of one entry, so the
// coverage walk above reads as three comparisons rather than nine lines.
type ReportCatalogEntryView struct{ GroupBy, Filters, Aggregates []string }

func assertCovers(t *testing.T, report, heading string, engine map[string]string, published []string) {
	t.Helper()
	for name := range engine {
		if !slices.Contains(published, name) {
			t.Errorf("%s: the engine accepts %s %q and the catalog does not publish it", report, heading, name)
		}
	}
}

// A report with default aggregates and no default grouping must not render a
// sentence that trails off. runAdHocPlan already builds that spec shape, so it
// is one prebuilt report away from being reachable.
func TestReportDefaultsRenderEachHalfOnlyWhenItExists(t *testing.T) {
	for _, tc := range []struct {
		name string
		spec reportSpec
		want string
	}{
		{
			name: "aggregates and grouping",
			spec: reportSpec{defaultBy: []string{"stage_id"}, defaultAggs: []reportAggregate{{Fn: "count", As: "deals"}}},
			want: "count as deals grouped by stage_id",
		},
		{
			name: "aggregates with no grouping",
			spec: reportSpec{defaultAggs: []reportAggregate{{Fn: "count", As: "deals"}}},
			want: "count as deals over the whole set",
		},
		{
			name: "grouping with no aggregates",
			spec: reportSpec{defaultBy: []string{"kind"}},
			want: "grouped by kind",
		},
		{
			name: "neither",
			spec: reportSpec{},
			want: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := describeReportDefaults(tc.spec); got != tc.want {
				t.Errorf("defaults = %q, want %q", got, tc.want)
			}
		})
	}
}

// An unknown report key is a CLIENT mistake about a static catalog, not a
// missing tenant record. It used to answer the row-scope not-found sentence,
// which told a caller their permissions might be at fault and named nothing
// available — a UAT agent read it and could not tell a typo from a denial.
func TestUnknownReportKeyNamesTheOnesThatExist(t *testing.T) {
	engine := &reportEngine{}
	_, err := engine.Run(t.Context(), "revenue-by-quarter", reportRequest{})
	if err == nil {
		t.Fatal("a report key the catalog does not serve was accepted")
	}
	// A 404, because crm.yaml says so: `report` is the path parameter naming the
	// resource, and the operation's 422 is scoped to the PLAN's fields. What the
	// refusal must also do is name the keys that exist, so a caller can tell a
	// typo from a denial.
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Error("an unknown report key no longer maps to the contract's 404")
	}
	var unknown *UnknownReportError
	if !errors.As(err, &unknown) {
		t.Fatalf("err = %v, want an UnknownReportError", err)
	}
	for report := range prebuiltReports {
		if !strings.Contains(unknown.Error(), report) {
			t.Errorf("the refusal never names the served report %q: %s", report, unknown.Error())
		}
	}
}

// A vocabulary refusal carries the vocabulary. It used to point at "the
// report's documented dimensions, filters and measures", which named something
// no tool on this surface yielded — so a caller guessed names until one worked.
func TestVocabularyRefusalCarriesTheVocabulary(t *testing.T) {
	spec := prebuiltReports["deals-by-stage"]
	refusal := &FieldNotAllowedError{Field: "owner_id", Allowed: allowedReportNames(spec.dimensions)}
	_, message := refusal.MessageFault()
	if !strings.Contains(message, "expected one of:") {
		t.Errorf("the refusal names no vocabulary: %s", message)
	}
	for dimension := range spec.dimensions {
		if !strings.Contains(message, dimension) {
			t.Errorf("the refusal omits the legal dimension %q: %s", dimension, message)
		}
	}
	// A site with no vocabulary in hand says so plainly rather than pointing at
	// documentation that does not exist.
	bare := &FieldNotAllowedError{Field: "fn=median"}
	if _, message := bare.MessageFault(); strings.Contains(message, "documented") {
		t.Errorf("a refusal still points at documentation this surface does not serve: %s", message)
	}
}

// A plan argument the engine does not serve is refused, not dropped. crm.yaml
// declares `as_of_date` on the runReport body and this engine has no field for
// it, so a lenient decode answered a request for a historical snapshot with
// current state and no warning.
func TestReportPlanRefusesAnArgumentTheEngineDoesNotServe(t *testing.T) {
	var plan reportRequest
	err := strictDecodeReportPlan(json.RawMessage(`{"as_of_date":"2026-01-31"}`), &plan)
	if err == nil {
		t.Fatal("as_of_date was accepted and silently dropped")
	}
	if !strings.Contains(err.Error(), "as_of_date") {
		t.Errorf("the refusal does not name the key it refused: %v", err)
	}
	// The arguments it DOES serve still decode.
	if err := strictDecodeReportPlan(json.RawMessage(`{"group_by":["stage_id"]}`), &plan); err != nil {
		t.Fatalf("a served plan argument was refused: %v", err)
	}
}

// A case-folded plan key is refused by NAME, not matched onto the field it
// case-folds towards. encoding/json matches struct fields case-insensitively,
// so `GROUP_BY` would decode into GroupBy and DisallowUnknownFields would never
// see it — the exact-key check has to run first, and this proves it does.
func TestReportPlanRefusesACaseFoldedArgument(t *testing.T) {
	for _, key := range []string{"GROUP_BY", "Group_By", "FILTERS", "Aggregates"} {
		unserved := unservedPlanArguments(json.RawMessage(`{"` + key + `":[]}`))
		if len(unserved) != 1 || !strings.Contains(unserved[0], key) {
			t.Errorf("%q was not refused by name: %v", key, unserved)
		}
	}
	// The byte-exact spellings are served.
	for _, key := range []string{slotGroupBy, slotFilters, slotAggregates} {
		if unserved := unservedPlanArguments(json.RawMessage(`{"` + key + `":[]}`)); len(unserved) != 0 {
			t.Errorf("the served argument %q was refused: %v", key, unserved)
		}
	}
}

// Exactly one JSON value. A second one after the plan object is a caller who
// believes they sent something this never read.
func TestReportPlanRefusesTrailingContent(t *testing.T) {
	var plan reportRequest
	if err := strictDecodeReportPlan(json.RawMessage(`{"group_by":["stage_id"]} {"group_by":["status"]}`), &plan); err == nil {
		t.Fatal("a second JSON value after the plan was ignored")
	}
}

// EVERY key in run_report's enum is described by its default answer.
//
// This is the invariant the certification lane bought at a price. The first pass
// deferred each report's default into the published document along with the
// three name lists, leaving an enum of nine bare keys — and on the goal "how
// much open pipeline do we have in each stage" every run reached for the
// vocabulary door instead of the report, because `deals-by-stage` answers that
// goal with no plan at all and a bare key does not say so. 2/3 to 0/3.
//
// So the default is the only per-report content left in the description, and a
// report that shipped without one would silently reopen exactly that failure:
// describeReportDefaults returns "" for a spec with neither default aggregates
// nor default grouping, which is a shape runAdHocPlan already builds. Nothing
// else on this surface tells a caller which report answers their question.
//
// Derived from the engine's own catalog, so a report added tomorrow inherits the
// obligation rather than escaping it.
func TestEveryReportKeyIsDescribedByItsDefaultAnswer(t *testing.T) {
	catalog := reportToolCatalog()
	if len(catalog) == 0 {
		t.Fatal("the prebuilt catalog is empty, so this test proved nothing")
	}
	described := agents.RenderedReportKeyGuidance(catalog)
	for _, entry := range catalog {
		if entry.Defaults == "" {
			t.Errorf("%s answers nothing by default, so run_report's description reduces it to a bare "+
				"enum key — which is what sent every certification run to the vocabulary door instead "+
				"of the report. Give the spec a defaultAggs or a defaultBy.", entry.Report)
			continue
		}
		if !strings.Contains(described, entry.Report) {
			t.Errorf("%s is in the enum and not in the description", entry.Report)
		}
		if !strings.Contains(described, entry.Defaults) {
			t.Errorf("%s: the description does not say what it answers by default: %q",
				entry.Report, entry.Defaults)
		}
	}
}

// A misspelled THRESHOLD key is refused with a list that contains the threshold
// names. `filters` is one object holding two families — the equality filters and
// the thresholds — and the catalog advertises them as one list
// (catalogFilterNames). A refusal built from the equality filters alone omits the
// family the caller was reaching for, so they read the list, do not find what
// they meant, and conclude the report cannot answer their question.
//
// Derived from every spec that declares a threshold rather than naming one, so a
// report that grows a threshold family is held by this the day it lands.
func TestAFilterRefusalNamesTheThresholdsToo(t *testing.T) {
	thresholded := 0
	for report, spec := range prebuiltReports {
		if len(spec.thresholds) == 0 {
			continue
		}
		thresholded++
		req := reportRequest{Filters: map[string]any{"no_such_filter": "x"}}
		_, err := buildReportWhere(t.Context(), spec, req, func(any) int { return 1 })
		var refusal *FieldNotAllowedError
		if !errors.As(err, &refusal) {
			t.Fatalf("%s: err = %v, want a FieldNotAllowedError", report, err)
		}
		_, message := refusal.MessageFault()
		for name := range spec.thresholds {
			if !strings.Contains(message, name) {
				t.Errorf("%s: the refusal omits the threshold key %q a caller may send in `filters`: %s",
					report, name, message)
			}
		}
		for name := range spec.filters {
			if !strings.Contains(message, name) {
				t.Errorf("%s: the refusal omits the equality filter %q: %s", report, name, message)
			}
		}
		// The refusal and the catalog are one vocabulary or they are two, and two
		// is what this test exists to prevent.
		if got, want := refusal.Allowed, catalogFilterNames(spec); !slices.Equal(got, want) {
			t.Errorf("%s: the refusal lists %v, the catalog advertises %v", report, got, want)
		}
	}
	// Under-recognition is the one way this must not fail: a prebuilt catalog with
	// no thresholds left would make every assertion above vacuous.
	if thresholded == 0 {
		t.Fatal("no prebuilt report declares a threshold, so this test proved nothing")
	}
}
