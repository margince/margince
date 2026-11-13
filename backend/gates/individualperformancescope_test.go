// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind reachability H2

//go:build !integration

package gates

// Every operation and tool of a family the "Show individual performance"
// switch names reaches a read of that switch.
//
// The families are read from the contract's own sentence on
// InstallationSettings.individual_performance, never copied here. A new
// operation in one of them that never asks the switch fails. So does a tool
// that answers for one of them through its own code.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"maps"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// switchScopeSentence finds the families in the switch's description: the
// list between "Off," and the clause that says what they no longer carry.
var switchScopeSentence = regexp.MustCompile(`Off, (.+?) carry no figure keyed to a colleague`)

// familyPaths places each family the sentence names on the contract's paths.
// The contract marks no operation with a family, so this map is typed by hand.
// A test holds its keys to the sentence both ways.
var familyPaths = map[string][]string{
	"reports": {
		"/analytics/reports", "/reports", "/analytics/evaluate", "/analytics/evaluate.csv", "/analytics/metrics",
		"/analytics/schedules", "/analytics/executions", "/analytics/sdr-summary", "/analytics/sdr-team",
	},
	"analytics questions":           {"/analytics/schema", "/analytics/query", "/analytics/runs", "/analytics/explain", "/analytics/vocabulary"},
	"forecasts and forecast shares": {"/forecast"},
	"targets":                       {"/analytics/targets", "/analytics/sales-goals", "/analytics/progress"},
	"evidence":                      {"/analytics/evidence", "/analytics/editions/{id}/evidence", "/analytics/runs/{run_id}/evidence-search"},
	"editions and their exports":    {"/analytics/editions", "/analytics/reports/{id}/editions"},
	"team weeks":                    {"/weekly-reviews/team"},
}

// carriesNoColleagueFigure ratifies operations of a family that answer no
// figure at all, so there is nothing for the switch to withhold.
var carriesNoColleagueFigure = gatekit.Waive(map[string]string{
	"getReportingMetrics":             "lists the metric catalog by name and definition; a metric's figures come from evaluating it, which asks the switch",
	"listReportingReports":            "lists saved report definitions by name; their figures come from evaluating one, which asks the switch",
	"describeReportBlocks":            "describes the block kinds a report document holds, and no block carries a figure here",
	"describeReportVocabulary":        "describes the words a report is written in, and no word carries a figure",
	"describeAnalyticsVocabulary":     "describes the populations and fields a question is written in, with no figure",
	"getAnalyticsSchema":              "lists the fields a question may group or measure by; a query naming an owner is refused when it runs",
	"listReportRuns":                  "lists the reader's own saved questions by their shape alone, with no answer and no figure",
	"getForecastAssurance":            "answers one installation-wide check run: deals checked, sources read and readiness, keyed to no seat",
	"startForecastAssuranceRun":       "queues an installation-wide check run and answers its id, with no figure",
	"resolveInputCheck":               "records a reader's answer to one deal's check and returns no figure",
	"getForecastAssuranceEnforcement": "retired: the route answers 501 and serves no data at all",
	"setForecastAssuranceEnforcement": "retired: the route answers 501 and changes nothing",
	"waitInputCheck":                  "retired: the route answers 501 and changes nothing",
	"closeLostInputCheck":             "retired: the route answers 501 and changes nothing",
})

// scopedOperation is one contract operation inside a family.
type scopedOperation struct {
	family, method, path, id string
	// tool is the x-mcp-tool verb, and route says the tool runs through
	// this operation's own HTTP handler, as read_view runs each view's route.
	tool  string
	route bool
}

// switchFamilies reads the families from the switch's description.
func switchFamilies(description string) ([]string, error) {
	match := switchScopeSentence.FindStringSubmatch(strings.Join(strings.Fields(description), " "))
	if match == nil {
		return nil, fmt.Errorf("the individual_performance description no longer says which surfaces carry no colleague's figure")
	}
	var families []string
	for _, item := range strings.Split(match[1], ", ") {
		families = append(families, strings.TrimPrefix(item, "and "))
	}
	return families, nil
}

// onPath reports whether path is prefix itself or lies under it.
func onPath(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

// scopedOperations places every contract operation in the families whose
// paths it lies on, and fails a family that holds none.
func scopedOperations(paths map[string]any, families []string, placed map[string][]string) ([]scopedOperation, error) {
	var ops []scopedOperation
	for _, family := range families {
		count := 0
		for _, path := range slices.Sorted(maps.Keys(paths)) {
			if !slices.ContainsFunc(placed[family], func(prefix string) bool { return onPath(path, prefix) }) {
				continue
			}
			item, _ := paths[path].(map[string]any)
			for _, method := range slices.Sorted(maps.Keys(item)) {
				op, ok := item[method].(map[string]any)
				if !ok || !httpMethods[method] {
					continue
				}
				id, _ := op["operationId"].(string)
				scoped := scopedOperation{family: family, method: method, path: path, id: id}
				if tool, ok := op["x-mcp-tool"].(map[string]any); ok {
					scoped.tool, _ = tool["verb"].(string)
					servedBy, _ := tool["served_by"].(string)
					scoped.route = servedBy == "route" || scoped.tool == agents.ReadViewTool
				}
				ops = append(ops, scoped)
				count++
			}
		}
		if count == 0 {
			return nil, fmt.Errorf("the family %q lies on no contract operation, so nothing of it is checked", family)
		}
	}
	return ops, nil
}

func contractSwitchFamilies(t *testing.T, doc map[string]any) []string {
	t.Helper()
	node := descendContract(t, doc, "components", "schemas", "InstallationSettings", "properties", "individual_performance")
	description, _ := node["description"].(string)
	families, err := switchFamilies(description)
	if err != nil {
		t.Fatal(err)
	}
	return families
}

func TestEveryFamilyTheSwitchNamesIsPlacedOnTheContract(t *testing.T) {
	t.Parallel()
	families := contractSwitchFamilies(t, loadContract(t))
	if got, want := slices.Sorted(maps.Keys(familyPaths)), slices.Sorted(slices.Values(families)); !slices.Equal(got, want) {
		t.Errorf("familyPaths places %q, but the individual_performance description names %q.\n\t"+
			"Place each family the sentence names on its paths, and drop one it no longer names", got, want)
	}
}

func TestEverySurfaceTheSwitchCoversReadsIt(t *testing.T) {
	t.Parallel()
	defer carriesNoColleagueFigure.AssertAllMatched(t)
	doc := loadContract(t)
	paths, _ := doc["paths"].(map[string]any)
	ops, err := scopedOperations(paths, contractSwitchFamilies(t, doc), familyPaths)
	if err != nil {
		t.Fatal(err)
	}
	g := loadSwitchGraph(t, switchReachDirs...)
	readers, err := g.readers(switchReaders)
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range ops {
		for _, finding := range surfaceFindings(g, op, readers) {
			if !carriesNoColleagueFigure.Waived(t, op.id) {
				t.Error(finding)
			}
		}
	}
}

// surfaceFindings says how op fails to reach the switch: a handler or tool
// that never reads it, or one the census cannot find.
func surfaceFindings(g *switchGraph, op scopedOperation, readers map[types.Object]bool) []string {
	subject := fmt.Sprintf("%s %s (%s, of %q)", strings.ToUpper(op.method), op.path, op.id, op.family)
	var roots []*types.Func
	var findings []string
	handlers := g.httpHandlers(op.id)
	if len(handlers) == 0 {
		findings = append(findings, subject+": no handler named "+handlerName(op.id)+" in "+strings.Join(switchReachDirs, ", "))
	}
	roots = append(roots, handlers...)
	if op.tool != "" && !op.route {
		tools := g.toolHandlers(op.tool)
		if len(tools) == 0 {
			findings = append(findings, subject+": no tool spec names "+op.tool+", the operation's x-mcp-tool")
		}
		roots = append(roots, tools...)
	}
	for _, root := range roots {
		if !g.reaches(root, readers) {
			findings = append(findings, fmt.Sprintf("%s: %s (%s) never reads the individual-performance switch. "+
				"Ask it the way compose/individualperformance.go does, or waive the operation in carriesNoColleagueFigure "+
				"if it answers no figure at all", subject, root.FullName(), g.where(root)))
		}
	}
	return findings
}

// A handler reaches the switch through a func-typed field or an interface, and
// one that reaches it through neither is reported.
func TestTheScopeCheckFindsAHandlerThatNeverReadsTheSwitch(t *testing.T) {
	t.Parallel()
	const reader = `package auth
import "context"
func IndividualPerformanceOf(ctx context.Context) (bool, bool) { return false, false }
`
	const surface = `package surface
import (
	"context"
	"net/http"
	"example.test/auth"
)
type lens interface{ Allowed(ctx context.Context) bool }
type bound struct{}
func (bound) Allowed(ctx context.Context) bool { ok, _ := auth.IndividualPerformanceOf(ctx); return ok }
func allowed(ctx context.Context) bool { ok, _ := auth.IndividualPerformanceOf(ctx); return ok }
type Handlers struct{ read func(context.Context) bool }
func New() Handlers { return Handlers{read: allowed} }
func (h Handlers) GetThroughAField(w http.ResponseWriter, r *http.Request) { h.read(r.Context()) }
func (h Handlers) GetThroughAnInterface(w http.ResponseWriter, r *http.Request) { var l lens = bound{}; l.Allowed(r.Context()) }
func (h Handlers) ListUnchecked(w http.ResponseWriter, r *http.Request) {}
`
	fset := token.NewFileSet()
	byPackage := map[string][]*ast.File{}
	for path, src := range map[string]string{"example.test/auth": reader, "example.test/surface": surface} {
		file, err := parser.ParseFile(fset, path+".go", src, 0)
		if err != nil {
			t.Fatal(err)
		}
		byPackage[path] = []*ast.File{file}
	}
	g := newSwitchGraph(fset, byPackage)
	readers, err := g.readers([]switchReader{{"auth", "IndividualPerformanceOf"}})
	if err != nil {
		t.Fatal(err)
	}
	var flagged []string
	for _, id := range []string{"getThroughAField", "getThroughAnInterface", "listUnchecked"} {
		if len(surfaceFindings(g, scopedOperation{family: "fixture", method: "get", path: "/" + id, id: id}, readers)) > 0 {
			flagged = append(flagged, id)
		}
	}
	if !slices.Equal(flagged, []string{"listUnchecked"}) {
		t.Errorf("flagged %v, want only listUnchecked", flagged)
	}
}

// A family the sentence names but no path holds fails, rather than checking
// nothing.
func TestAFamilyOnNoOperationFailsTheCensus(t *testing.T) {
	t.Parallel()
	families, err := switchFamilies("Show it. Off, reports and team weeks carry no figure keyed to a colleague of the reader.")
	if err != nil || !slices.Equal(families, []string{"reports and team weeks"}) {
		t.Fatalf("read %q (%v), want the one item between Off and carry", families, err)
	}
	paths := map[string]any{"/reports": map[string]any{"get": map[string]any{"operationId": "listReports"}}}
	_, err = scopedOperations(paths, []string{"reports", "team weeks"}, map[string][]string{"reports": {"/reports"}, "team weeks": {"/weekly"}})
	if err == nil || !strings.Contains(err.Error(), `"team weeks"`) {
		t.Errorf("a family on no path gave %v, want an error naming it", err)
	}
}
