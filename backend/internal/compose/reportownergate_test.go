// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The fitness function for the owner gate: a spec that measures every
// readable row over a table whose row scope renders empty is narrowed by
// nothing, so every owner it lets a caller name has to reach the gate. The
// corpus is every spec the engine runs, derived rather than listed, so the next
// spec that adds the combination is judged the day it lands.

import (
	"context"
	"errors"
	"maps"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ownerShaped is deliberately wider than namesOwner: any expression reading an
// owner_id column, and any name saying owner. What it catches that the gate
// does not recognise is the gap.
var ownerShaped = regexp.MustCompile(`\bowner_id\b`)

// everyReportSpec gathers the specs the report engine runs from: the prebuilt
// catalog and the ad-hoc vocabulary of each schema entity.
func everyReportSpec(t *testing.T) map[string]reportSpec {
	t.Helper()
	specs := maps.Clone(prebuiltReports)
	for _, obj := range schemaObjects {
		spec, ok := adHocSpec(obj.Type)
		if !ok {
			t.Fatalf("schema entity %s has no ad-hoc spec", obj.Type)
		}
		specs["adhoc:"+string(obj.Type)] = spec
	}
	return specs
}

// repReading is an own-scope rep holding the read grant on every table, the
// seat for whom an empty row scope and a missing owner gate cost the most.
func repReading() context.Context {
	grants := map[string]principal.ObjectGrant{}
	for _, obj := range schemaObjects {
		grants[string(obj.Type)] = principal.ObjectGrant{Read: true}
	}
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:rep", UserID: ids.NewV7(),
		Permissions: principal.Permissions{Objects: grants, RowScope: principal.RowScopeOwn},
	})
}

// rowScopeIsEmpty answers whether the rep's row scope narrows the spec's own
// rows by nothing, which is what leaves the owner gate as the only narrowing.
func rowScopeIsEmpty(ctx context.Context, spec reportSpec) (bool, error) {
	arg := func(any) int { return 1 }
	var clause string
	var err error
	if spec.activityWalk {
		clause, err = auth.ActivityContentClause(ctx, "t", arg)
	} else {
		clause, err = auth.ScopeClauseFor(ctx, string(spec.entity), "t", arg)
	}
	return clause == "", err
}

// ownerGateGaps judges a corpus and answers how many specs it judged and what
// each uncovered owner name is.
func ownerGateGaps(ctx context.Context, specs map[string]reportSpec) (int, []string, error) {
	judged := 0
	var gaps []string
	for _, report := range slices.Sorted(maps.Keys(specs)) {
		spec := specs[report]
		if spec.population != measureEveryReadableRow {
			continue
		}
		empty, err := rowScopeIsEmpty(ctx, spec)
		if err != nil {
			return 0, nil, err
		}
		if !empty {
			continue
		}
		judged++
		names := slices.Sorted(maps.Keys(spec.dimensions))
		names = append(names, slices.Sorted(maps.Keys(spec.filters))...)
		for _, name := range slices.Compact(slices.Sorted(slices.Values(names))) {
			if gap := ownerGateGap(ctx, spec, name); gap != "" {
				gaps = append(gaps, report+": "+gap)
			}
		}
	}
	return judged, gaps, nil
}

// ownerGateGap asks the gate itself, not a copy of its rules: a filter naming
// a stranger must be refused, and a grouping on the name must be narrowed.
func ownerGateGap(ctx context.Context, spec reportSpec, name string) string {
	shaped := strings.Contains(name, "owner") ||
		ownerShaped.MatchString(spec.dimensions[name]) || ownerShaped.MatchString(spec.filters[name])
	if !shaped {
		return ""
	}
	stranger := ids.NewV7().String()
	if _, isFilter := spec.filters[name]; isFilter {
		// nil tx: an own-scope rep's lens refuses a stranger without a read.
		err := requireMeasurableOwners(ctx, nil, spec, map[string]any{name: stranger})
		if !errors.Is(err, apperrors.ErrPermissionDenied) {
			return "the filter " + name + " naming a stranger is not refused"
		}
	}
	if _, isDimension := spec.dimensions[name]; !isDimension {
		return ""
	}
	if !breaksDownByOwner(spec, []string{name}) {
		return "grouping by " + name + " is not narrowed to the owners the caller may measure"
	}
	// The typed grammar names the owner as a dimension, in a filter under any
	// comparison, and beside a row id that is not part of the grouping.
	typed := analyticsquery.Filter{Field: name, Op: analyticsquery.OpNe, Value: stranger}
	err := requireMeasurableTypedOwners(ctx, nil, spec, []analyticsquery.Filter{typed})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		return "a typed filter on " + name + " naming a stranger is not refused"
	}
	for _, q := range rowNamedBesideTheGrouping(spec, name) {
		if !typedOwnerBreakdown(spec, q) {
			return "a typed breakdown by " + name + " is not narrowed when a row id is named outside the grouping"
		}
	}
	return ""
}

// rowNamedBesideTheGrouping is a breakdown by owner that also names the row's
// own id, once as a measure and once as a filter, for every spec whose
// vocabulary has a row id.
func rowNamedBesideTheGrouping(spec reportSpec, owner string) []analyticsquery.Query {
	var out []analyticsquery.Query
	for name, expr := range spec.dimensions {
		if expr != colRowID {
			continue
		}
		byOwner := []string{owner}
		out = append(out,
			analyticsquery.Query{GroupBy: byOwner, Measures: []analyticsquery.Measure{
				{Fn: analyticsquery.CountDistinct, Field: name},
			}},
			analyticsquery.Query{GroupBy: byOwner, Filters: []analyticsquery.Filter{
				{Field: name, Op: analyticsquery.OpIsNotNull},
			}})
	}
	return out
}

func TestEveryInstallWideOwnerNameGoesThroughTheOwnerGate(t *testing.T) {
	judged, gaps, err := ownerGateGaps(repReading(), everyReportSpec(t))
	if err != nil {
		t.Fatalf("judging the corpus: %v", err)
	}
	// A census that reads nothing reports a clean pass over nothing.
	if judged == 0 {
		t.Fatal("no install-wide spec over an unscoped table was judged — the corpus has " +
			"stopped reaching the specs this test exists for")
	}
	for _, gap := range gaps {
		t.Errorf(`%s.

The spec measures every readable row over a table whose row scope is empty, so
nothing but the owner gate (reportownergate.go) stands between a rep and a named
colleague's figures. Read the owner through colOwnerID so the gate recognises
it, or keep the spec on the caller's own population.`, gap)
	}
}

// The census has to be able to fail. Each planted spec hides the owner where
// the gate does not look; the last is on the caller's own population, where
// the population clause narrows it and nothing is owed.
func TestTheOwnerGateCensusCatchesAnOwnerTheGateDoesNotRecognise(t *testing.T) {
	planted := map[string]reportSpec{
		"coalesced-dimension": {
			entity: prebuiltReports["win-loss"].entity, population: measureEveryReadableRow,
			dimensions: map[string]string{"rep": "COALESCE(t.owner_id, t.created_by)"},
		},
		"joined-filter": {
			entity: prebuiltReports["projects-by-phase"].entity, population: measureEveryReadableRow,
			filters: map[string]string{"account_owner": "c.owner_id"},
		},
		"own-population": {
			entity:     prebuiltReports["win-loss"].entity,
			dimensions: map[string]string{"rep": "COALESCE(t.owner_id, t.created_by)"},
		},
	}
	judged, gaps, err := ownerGateGaps(repReading(), planted)
	if err != nil {
		t.Fatalf("judging the planted corpus: %v", err)
	}
	if judged != 2 || len(gaps) != 2 {
		t.Fatalf("judged %d specs and found %v, want the two install-wide plants flagged and "+
			"the own-population one left alone", judged, gaps)
	}
}
