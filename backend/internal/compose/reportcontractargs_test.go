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
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// contractAggregateFns reads the fn enum out of RunReportRequest.
//
// It matches the enum in EITHER YAML shape — inline on the `fn:` line, or as a
// block under it. The first version matched only the inline form, and adding a
// description to that property reformatted it into a block: the census then
// found nothing and said so, which is the one thing it must do rather than
// report a vacuous pass.
var contractAggregateFns = regexp.MustCompile(`(?s)fn:.{0,200}?enum: \[([^\]]+)\]`)

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
		// The constants themselves, not a second list beside them. Written out
		// here, this line is one more place the vocabulary is spelled — and it
		// is the place that goes stale silently, because a function added to
		// the engine and to the contract would leave this test passing on the
		// pair it still remembers.
		served := servedAggregateFns()
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
		declared := declaredProperties(schema)
		requireNonEmptyCensus(t, "RunReportRequest property", declared)
		for _, prop := range declared {
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
//
// The name pattern is deliberately wider than an identifier: `\w+` would skip a
// property whose name carries punctuation (`as-of-date`), and a census that
// skips its subject reports PASS with nothing to notice. Anything YAML admits
// as a plain key is matched, so a name this gate cannot see does not exist.
var propertyLine = regexp.MustCompile(`(?m)^        ([A-Za-z0-9_.-]+):`)

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

// requireNonEmptyCensus fails a census that recognized nothing. An empty result
// is the one outcome that looks identical to agreement: every comparison below
// holds vacuously, the gate reports PASS, and the drift it exists to catch
// walks straight through.
func requireNonEmptyCensus(t *testing.T, what string, found []string) {
	t.Helper()
	if len(found) == 0 {
		t.Fatalf("the %s census recognized nothing — the schema was renamed or "+
			"reindented under this test's pattern, and every assertion below "+
			"would pass without comparing anything", what)
	}
}

// servedAggregateFns is the aggregate vocabulary the engine actually switches
// on, derived from aggregateSelect's own cases.
//
// Read from the source rather than restated: a function added to the switch and
// forgotten here is exactly what this gate exists to notice, and a hand-kept
// list would go stale in the direction that reads as success.
func servedAggregateFns() []string {
	const path = "report.go"
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		panic("compose: reading the aggregate vocabulary: " + err.Error())
	}
	var out []string
	ast.Inspect(file, func(node ast.Node) bool {
		spec, ok := node.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for i, name := range spec.Names {
			if !strings.HasPrefix(name.Name, "aggFn") || i >= len(spec.Values) {
				continue
			}
			lit, ok := spec.Values[i].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			out = append(out, strings.Trim(lit.Value, `"`))
		}
		return true
	})
	return out
}
