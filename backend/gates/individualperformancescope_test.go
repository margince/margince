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
// A test holds its keys to the sentence both ways.
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
	"getForecastAssurance":            "answers one installation-wide check run: deals checked, sources read and readiness, keyed to no seat",
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
	"getForecastStageChances":         "answers each stage's chance over the whole installation's outcomes, never grouped by seat",
	"getAccountCoverage":              "lists accounts and how they are covered, records the reader may already read, not a total per seat",
	"getAnalyticsContext":             "answers which populations the reader may measure, with no figure",
	"listSdrUpcomingMeetings":         "lists meetings as records the reader may already read, not a total per seat",
	"listForecastShares":              "lists the share links the reader issued, with no figure",
	"createForecastShare":             "answers the share link just issued, with no figure; opening it re-reads the forecast for whoever opens it",
	"revokeForecastShare":             "revokes a share link and answers nothing",
	"listInputChecks":                 "lists deals the reader may already read, with their amounts; GroupFindings sums per finding type and currency, never per seat",
})

// notYetHeldOnEveryPath holds operations the walk still flags and nobody has
// sorted into a fixed path or an exact walk. Each says what is left. An entry
// whose operation passes fails the test, so the list only shrinks.
var notYetHeldOnEveryPath = gatekit.Waive(map[string]string{
	"listSalesProgressDeals":      "salesprogress progressScope builds the reader's own scope inline instead of through ownScope",
	"getSalesGoalPlan":            "goalAccess proves the reader's own seat by comparing ids, which the walk cannot read as a check",
	"saveSalesGoalDraft":          "goalAccess proves the reader's own seat by comparing ids, which the walk cannot read as a check",
	"discardSalesGoalDraft":       "goalAccess proves the reader's own seat by comparing ids, which the walk cannot read as a check",
	"publishSalesGoalPlan":        "goalAccess proves the reader's own seat by comparing ids, which the walk cannot read as a check",
	"withdrawSalesGoalPlan":       "goalAccess proves the reader's own seat by comparing ids, which the walk cannot read as a check",
	"answerReportingQuestionCard": "the restricted answer carries no figure, but the walk reads its write as one",
	"assessSalesReview":           "a refusal returned through RequireHumanOrItsAgent reads to the walk as a success",
	"createReportingTarget":       "answers an owner-scoped target value, and the write path is not yet held to the switch on every scope",
	"recordForecastCall":          "answers a forecast call's amount for an owner scope, and the write path is not yet held to the switch on every scope",
	"evaluateReporting":           "the read_reporting tool shares one handler, and its editions mode gathers checked editions in a loop that may run no time",
	"evaluateReportingReport":     "the read_reporting tool shares one handler, and its editions mode gathers checked editions in a loop that may run no time",
	"getReportingMetrics":         "the read_reporting tool shares one handler, and its editions mode gathers checked editions in a loop that may run no time",
	"listReportingReports":        "the read_reporting tool shares one handler, and its editions mode gathers checked editions in a loop that may run no time",
	"getReportingReport":          "the read_reporting tool shares one handler, and its editions mode gathers checked editions in a loop that may run no time",
	"getReportingEditionEvidence": "the read_reporting tool shares one handler, and its editions mode gathers checked editions in a loop that may run no time",
	"getReportingEvidence":        "the read_reporting tool shares one handler, and its editions mode gathers checked editions in a loop that may run no time",
	"compareReportingEditions":    "the read_reporting tool shares one handler, and its editions mode gathers checked editions in a loop that may run no time",
	"getReportingEdition":         "the read_reporting tool shares one handler, and its editions mode gathers checked editions in a loop that may run no time",
	"listReportingEditions":       "the edition list gathers checked editions in a loop that may run no time, here and in the read_reporting tool",
	"renderAnalyticsReport":       "the route and the compose_analytics_report tool check inside a loop that may run no time",
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
		for _, prefix := range placed[family] {
			if !slices.ContainsFunc(slices.Collect(maps.Keys(paths)), func(path string) bool { return onPath(path, prefix) }) {
				return nil, fmt.Errorf("the family %q names the path %q, which the contract no longer has", family, prefix)
			}
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
	defer notYetHeldOnEveryPath.AssertAllMatched(t)
	doc := loadContract(t)
	paths, _ := doc["paths"].(map[string]any)
	ops, err := scopedOperations(paths, contractSwitchFamilies(t, doc), familyPaths)
	if err != nil {
		t.Fatal(err)
	}
	g := loadSwitchGraph(t, switchReachDirs...)
	if err := g.readers(switchReaders, answersNoColleagueFigure); err != nil {
		t.Fatal(err)
	}
	if err := g.dispatchers(toolDispatchers); err != nil {
		t.Fatal(err)
	}
	walk := newSwitchWalk(g)
	for _, op := range ops {
		census, paths := surfaceFindings(walk, op)
		for _, finding := range census {
			t.Error(finding)
		}
		for _, finding := range paths {
			if !carriesNoColleagueFigure.Waived(t, op.id) && !notYetHeldOnEveryPath.Waived(t, op.id) {
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
	subject := fmt.Sprintf("%s %s (%s, of %q)", strings.ToUpper(op.method), op.path, op.id, op.family)
	handlers := g.httpHandlers(op.id)
	if len(handlers) == 0 {
		census = append(census, subject+": no handler named "+handlerName(op.id)+" in "+strings.Join(switchReachDirs, ", "))
	}
	var tools []*types.Func
	if op.tool != "" && !op.route {
		if tools = g.toolHandlers(op.tool); len(tools) == 0 {
			census = append(census, subject+": no tool spec names "+op.tool+", the operation's x-mcp-tool")
		}
	}
	for _, root := range slices.Concat(handlers, tools) {
		verdict := walk.walk(run{fn: fnValue{decl: root}}, pathState{}, 0)
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
