// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The published contract and the engine agree about what run_report accepts.
//
// These two drifted apart and stayed apart: crm.yaml offered callers an
// `as_of_date` for "snapshot / historical reporting" and a `count_distinct`
// aggregate, and the engine served neither. A caller reading the contract wrote
// a plan the runtime refused, and the refusal was the honest half — the
// document was the half that lied.
//
// A drift in the other direction is worse and this test catches it too: an
// engine that grows a capability the contract never mentions is a surface
// nobody reviewed.

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// contractAggregateFns reads the fn enum out of RunReportRequest.
var contractAggregateFns = regexp.MustCompile(`fn: \{ type: string, enum: \[([^\]]+)\] \}`)

// contractPlanSlots reads the property names RunReportRequest declares.
func TestTheContractOffersExactlyWhatTheEngineServes(t *testing.T) {
	raw, err := os.ReadFile("../../api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	schema := runReportRequestSchema(t, string(raw))

	t.Run("aggregate functions", func(t *testing.T) {
		m := contractAggregateFns.FindStringSubmatch(schema)
		if m == nil {
			t.Fatal("no fn enum found in RunReportRequest — the regex has lost its " +
				"subject, which is a census reading a smaller tree and reporting PASS")
		}
		var published []string
		for _, fn := range strings.Split(m[1], ",") {
			published = append(published, strings.TrimSpace(fn))
		}
		served := []string{aggFnCount, aggFnSum, aggFnAvg, aggFnMin, aggFnMax}
		sort.Strings(published)
		sort.Strings(served)
		if strings.Join(published, ",") != strings.Join(served, ",") {
			t.Errorf("the contract publishes %v and the engine serves %v — a function "+
				"in one and not the other is a promise the runtime refuses or a "+
				"capability nobody reviewed", published, served)
		}
	})

	t.Run("plan slots", func(t *testing.T) {
		for _, slot := range []string{slotFilters, slotGroupBy, slotAggregates} {
			if !strings.Contains(schema, "\n        "+slot+":") {
				t.Errorf("the engine serves the %q plan slot and RunReportRequest does "+
					"not declare it", slot)
			}
		}
		// The other direction: every property the schema declares must be one
		// the engine actually reads.
		for _, prop := range declaredProperties(schema) {
			if !servedPlanArguments[prop] {
				t.Errorf("RunReportRequest offers %q and the engine does not serve it — "+
					"a caller following the contract writes a plan the runtime refuses",
					prop)
			}
		}
	})
}

// runReportRequestSchema returns the RunReportRequest block's text.
func runReportRequestSchema(t *testing.T, doc string) string {
	t.Helper()
	const head = "\n    RunReportRequest:\n"
	start := strings.Index(doc, head)
	if start < 0 {
		t.Fatal("RunReportRequest not found in crm.yaml — this test is guarding a " +
			"schema that has been renamed")
	}
	rest := doc[start+len(head):]
	// The next schema at the same indentation ends this one.
	if end := regexp.MustCompile(`\n    [A-Z]\w*:\n`).FindStringIndex(rest); end != nil {
		return rest[:end[0]]
	}
	return rest
}

// declaredProperties lists the top-level property names of the schema block.
var propertyLine = regexp.MustCompile(`(?m)^        (\w+):`)

func declaredProperties(schema string) []string {
	body := schema
	if i := strings.Index(schema, "\n      properties:\n"); i >= 0 {
		body = schema[i:]
	}
	var out []string
	for _, m := range propertyLine.FindAllStringSubmatch(body, -1) {
		out = append(out, m[1])
	}
	return out
}
