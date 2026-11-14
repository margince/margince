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
// A test holds its keys to the sentence both ways. It only names a family.
// The census walks every operation under the first segment of a path here,
// and every tool the families' routers run, placed here or not.
var familyPaths = map[string][]string{
	"reports": {
		"/analytics/reports", "/reports", "/analytics/evaluate", "/analytics/evaluate.csv", "/analytics/metrics",
		"/analytics/schedules", "/analytics/executions", "/analytics/sdr-summary", "/analytics/sdr-team",
		"/analytics/assessment", "/analytics/account-coverage", "/analytics/context",
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
	"describeReportBlocks":            "describes the block kinds a report document holds, and no block carries a figure here",
	"describeReportVocabulary":        "describes the words a report is written in, and no word carries a figure",
	"describeAnalyticsVocabulary":     "describes the populations and fields a question is written in, with no figure",
	"getAnalyticsSchema":              "lists the fields a question may group or measure by; a query naming an owner is refused when it runs",
	"listReportRuns":                  "lists the reader's own saved questions by their shape alone, with no answer and no figure",
	"startForecastAssuranceRun":       "queues an installation-wide check run and answers its id, with no figure",
	"resolveInputCheck":               "records a reader's answer to one deal's check and returns no figure",
	"getForecastAssuranceEnforcement": "retired: the route answers 501 and serves no data at all",
	"setForecastAssuranceEnforcement": "retired: the route answers 501 and changes nothing",
	"waitInputCheck":                  "retired: the route answers 501 and changes nothing",
	"closeLostInputCheck":             "retired: the route answers 501 and changes nothing",
	"createReportingReport":           "answers the report definition just saved: its selection, not its figures",
	"updateReportingReport":           "answers the report definition just changed: its selection, not its figures",
	"archiveReportingReport":          "answers the archived report definition, with no figure",
	"saveReportingReportDraft":        "answers the draft definition just saved, with no figure",
	"discardReportingReportDraft":     "answers the definition left after the draft is dropped, with no figure",
	"publishReportingReportDraft":     "answers the definition the draft became, with no figure",
	"listReportingReportRevisions":    "lists a report's past definitions, with no figure",
	"restoreReportingReportRevision":  "answers the definition a past revision restored, with no figure",
	"listReportingSchedules":          "lists when a report is sent and to whom, with no figure",
	"createReportingSchedule":         "answers the schedule just made, with no figure",
	"updateReportingSchedule":         "answers the schedule just changed, with no figure",
	"getReportingExecution":           "answers a run's status and attempt, with no figure",
	"listReportingExecutions":         "lists a report's runs by status, with no figure",
	"retryReportingExecution":         "answers the status of the run queued again, with no figure",
	"freezeReportingEdition":          "answers the status of the capture just queued; its figures are read through the edition, which meets the floor",
	"getAccountCoverage":              "lists accounts and how they are covered, records the reader may already read, not a total per seat",
	"getAnalyticsContext":             "answers which populations the reader may measure, with no figure",
	"listSdrUpcomingMeetings":         "lists meetings as records the reader may already read, not a total per seat",
	"listForecastShares":              "lists the share links the reader issued, with no figure",
	"createForecastShare":             "answers the share link just issued, with no figure; opening it re-reads the forecast for whoever opens it",
	"revokeForecastShare":             "revokes a share link and answers nothing",
	"createReportingTarget":           "answers the target just written: the scope, period and value the caller sent, with no measured figure; a colleague's own scope is refused where it resolves",
	"recordForecastCall":              "answers the call just recorded: the amount and note the caller sent; the baseline it freezes is read later through the floored snapshot reads",
	"getDataCoverage":                 "answers each source's state and the time it was checked through, keyed to no seat",
	"listSalesDecisions":              "lists decisions kept as tasks on deals, read through the activity list's own scope; each carries the words someone wrote, no measured figure",
	"recordSalesDecision":             "answers the decision task just written: the deal, the words and the review date the caller sent",
	"recordSalesDecisionOutcome":      "answers a decision task with the result the caller typed, no measured figure",
	"getReportingFramework":           "answers the stage framework: qualifying stages, capture scopes and thresholds, with no figure",
	"publishReportingFramework":       "answers the stage framework just published, with no figure",
	"listSalesInitiatives":            "lists initiative definitions: name, rules, owner, target and deadline; their results are read through the floored reporting evaluation",
	"createSalesInitiative":           "answers the initiative definition just saved, with no result",
	"getSalesInitiative":              "answers one initiative definition, with no result",
	"updateSalesInitiative":           "answers the initiative definition just changed, with no result",
	"launchSalesInitiative":           "answers the initiative definition just launched, with no result",
	"listSalesInitiativeMembers":      "lists the leads and deals an initiative holds, each under the reader's row scope and masks, and how many the list holds; records, not a total per seat",
	"replaceSalesInitiativeMembers":   "answers the initiative's member records under the reader's row scope and masks, and how many the list holds",
	"proposeReportSetup":              "answers a model's proposed selection, read from the metric catalog, products and industries, with no figure",
})

// scopedOperation is one contract operation inside a family.
type scopedOperation struct {
	family, method, path, id string
	// tool is the x-mcp-tool verb, and route says the tool runs through
	// this operation's own HTTP handler, as read_view runs each view's route.
	// toolOnly is a tool a family's router runs that no operation serves.
	tool     string
	route    bool
	toolOnly bool
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

// unplacedFamily names an operation of the census that no path in
// familyPaths places in a family.
const unplacedFamily = "no family placed"

// scopedOperations places every contract operation in the families whose
// paths it lies on, and fails a family that holds none. Every other operation
// under the first segment of a placed path is in the census too. One added
// beside a family's paths is walked before anyone places it.
func scopedOperations(paths map[string]any, families []string, placed map[string][]string) ([]scopedOperation, error) {
	var ops []scopedOperation
	inFamily := map[string]bool{}
	for _, family := range families {
		count := 0
		for _, path := range slices.Sorted(maps.Keys(paths)) {
			if !slices.ContainsFunc(placed[family], func(prefix string) bool { return onPath(path, prefix) }) {
				continue
			}
			found := operationsAt(paths, path, family)
			ops, count, inFamily[path] = append(ops, found...), count+len(found), true
		}
		if count == 0 {
			return nil, fmt.Errorf("the family %q lies on no contract operation, so nothing of it is checked", family)
		}
		for _, prefix := range placed[family] {
			if !slices.ContainsFunc(slices.Collect(maps.Keys(paths)), func(path string) bool { return onPath(path, prefix) }) {
				return nil, fmt.Errorf("the family %q names the path %q, which the contract no longer has", family, prefix)
			}
		}
	}
	segments := familySegments(placed)
	for _, path := range slices.Sorted(maps.Keys(paths)) {
		if !inFamily[path] && slices.ContainsFunc(segments, func(segment string) bool { return onPath(path, segment) }) {
			ops = append(ops, operationsAt(paths, path, unplacedFamily)...)
		}
	}
	return ops, nil
}

// familySegments are the first segments of the paths the families are
// placed on, as /analytics is of /analytics/reports.
func familySegments(placed map[string][]string) []string {
	var segments []string
	for _, prefixes := range placed {
		for _, prefix := range prefixes {
			first, _, _ := strings.Cut(strings.TrimPrefix(prefix, "/"), "/")
			if segment := "/" + first; !slices.Contains(segments, segment) {
				segments = append(segments, segment)
			}
		}
	}
	slices.Sort(segments)
	return segments
}

// operationsAt are the operations of one contract path, in family.
func operationsAt(paths map[string]any, path, family string) []scopedOperation {
	var ops []scopedOperation
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
	}
	return ops
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
	g := loadSwitchGraph(t, switchWalkConfig, switchReachDirs...)
	defer routersBesideTheFamilies.AssertAllMatched(t)
	ops = append(ops, routedOperations(t, g, paths, ops)...)
	walk := newSwitchWalk(g)
	for _, op := range ops {
		census, paths := surfaceFindings(walk, op)
		for _, finding := range census {
			t.Error(finding)
		}
		for _, finding := range paths {
			if !carriesNoColleagueFigure.Waived(t, op.id) {
				t.Error(finding)
			}
		}
	}
}

// surfaceFindings says how op fails the switch. census holds a handler or
// tool the census cannot find, which no waiver covers. paths holds a figure
// handed out on a path that never read the switch, or none the walk can see.
func surfaceFindings(walk *switchWalk, op scopedOperation) (census, paths []string) {
	g := walk.g
	subject := fmt.Sprintf("%s (%s, of %q)", placedOn(op), op.id, op.family)
	handlers := g.httpHandlers(op.id)
	if len(handlers) == 0 && !op.toolOnly {
		census = append(census, subject+": no handler named "+handlerName(op.id)+" in "+strings.Join(switchReachDirs, ", "))
	}
	var tools []*types.Func
	if op.tool != "" && !op.route {
		if tools = g.toolHandlers(op.tool); len(tools) == 0 {
			census = append(census, subject+": no tool spec names "+op.tool+", the operation's x-mcp-tool")
		}
	}
	for _, root := range slices.Concat(handlers, tools) {
		verdict := walk.walk(run{fn: fnValue{decl: root}}, pathState{covered: rootCovered(root)}, 0)
		figures := verdict.writes
		if slices.Contains(tools, root) {
			figures.merge(verdict.returns)
		}
		if figures.count == 0 {
			paths = append(paths, fmt.Sprintf("%s: the walk finds no figure that %s (%s) writes or returns, "+
				"so it cannot tell whether the switch covers one. Write the answer through httperr.WriteJSON or return it, "+
				"or waive the operation in carriesNoColleagueFigure if it answers no figure at all", subject, root.FullName(), g.where(root)))
		}
		for _, at := range figures.unchecked {
			paths = append(paths, fmt.Sprintf("%s: %s (%s) hands out a figure at %s on a path that never reads the "+
				"individual-performance switch. Ask it on every path the way compose/individualperformance.go does, "+
				"or waive the operation in carriesNoColleagueFigure if it answers no figure at all",
				subject, root.FullName(), g.where(root), g.fset.Position(at)))
		}
	}
	return census, paths
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
