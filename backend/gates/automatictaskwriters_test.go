// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every native writer that stamps a source_system on an activity, and every
// reader of open tasks, found in the code of the whole backend tree. A writer
// states its automatic task family, so its tasks meet that family's currency
// rule in storekit.CurrentTaskClause. A reader applies that clause or states
// why it reads another population. A new one fails until it says which.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// Families a writer may declare besides an automatic task source.
const (
	// writesNoTask stamps source_system on some other kind of record.
	writesNoTask = "not a task"
	// writesCallerSource stores a source a caller, an import or a connector
	// supplied. It is never one of our automatic families: the API refuses
	// those (provenance.ReservedSourceSystem).
	writesCallerSource = "caller-supplied"
	// writesForItsCaller is the shared insert every LogActivityInput reaches;
	// the source is its caller's, and the caller is declared here.
	writesForItsCaller = "the caller's"
)

// taskSourceWriters is each writer's declaration, keyed by file:function.
var taskSourceWriters = map[string][]string{
	"compose/assurancebundletask.go:mintBundleTask":                       {provenance.AssuranceTaskSource},
	"compose/commitmenttask.go:writeCommitmentTask":                       {provenance.CommitmentTaskSource},
	"compose/leadqualify.go:remindUnqualifiedLeads":                       {provenance.LeadQualifyTaskSource},
	"compose/leadsla.go:Apply":                                            {provenance.LeadSLATaskSource},
	"compose/reconcile.go:followUpConfirmEffect":                          {provenance.OvernightReconcileSource},
	"modules/activities/requesttask.go:emailRequestTaskInput":             {provenance.EmailRequestSource},
	"modules/automation/taskeffect.go:taskCreateEffectFor":                provenance.EngineReminderSources(),
	"compose/csvactivityfields.go:activityCreateFrom":                     {writesCallerSource},
	"modules/activities/mapping.go:logActivityInput":                      {writesCallerSource},
	"modules/capture/sinkactivity.go:capturedActivityInsert":              {writesCallerSource},
	"modules/activities/activity.go:logActivityInTx":                      {writesForItsCaller},
	"modules/activities/activityinitialaudience.go:recordInitialActivity": {writesForItsCaller},
	"compose/csvcontactfields.go:contactCreateFrom":                       {writesNoTask},
	"compose/dealroomtimeline.go:write":                                   {writesNoTask},
	"modules/activities/meetinghistory.go:recordMeetingTransition":        {writesNoTask},
	"modules/activities/outboundmessage.go:activity":                      {writesNoTask},
}

// openTaskReadersElsewhere is each reader of open tasks that does not apply
// storekit.CurrentTaskClause itself, with why.
// gatekit:fixture the declared readers of another population, checked both ways below
var openTaskReadersElsewhere = map[string]string{
	"compose/automaticworksql.go:automaticItemsSQL":                       "lists what Margince did, done or not, for Done for you; it reports work, it assigns none",
	"modules/activities/bulkcomplete.go:LockTaskForBulkTx":                "locks one task a human chose to complete",
	"compose/reportprojects.go:projectCommitmentsSpec":                    "a report over the project's recorded promises, not a personal queue",
	"modules/activities/companyscope.go:ownQueueClause":                   "narrows a list the caller already gates with the worklist clauses",
	"modules/activities/companyscope.go:unassignedQueueClause":            "narrows a list the caller already gates with the worklist clauses",
	"modules/activities/homerepair.go:pristineRepairTaskSQL":              "the repair reads exactly the automatic tasks that are no longer current",
	"modules/activities/homerepair.go:readHomeRepairTasksTx":              "the repair reads exactly the automatic tasks that are no longer current",
	"modules/activities/lasttouch.go:openChildReminderHoldsAccount":       "an open reminder of the engine's own family holds its re-raise; it creates no work",
	"modules/activities/lasttouch.go:openReminderHoldsEntity":             "an open reminder of the engine's own family holds its re-raise; it creates no work",
	"modules/activities/requestrecovery.go:requestAvailableForAcceptance": "checks a request has no accepted task before a human accepts it",
	"modules/activities/taskwindow.go:openTaskWindowClauses":              "narrows a list the caller already gates with the worklist clauses",
	"modules/contacts/claimrecord.go:SetClaimTaskTx":                      "links a promise to its own task row",
	"modules/contacts/lead_read.go:leadNextTaskDueSQL":                    "the lead's next due date as a record fact",
	"modules/deals/nextstep.go:OpenNextStepSQL":                           "its caller passes storekit.CurrentTaskClause in as currentTask",
}

var (
	openTaskKind = regexp.MustCompile(`kind\s*=\s*'task'`)
	openTaskDone = regexp.MustCompile(`is_done`)
)

// taskCensus is what one scan of a tree found.
type taskCensus struct {
	writers []string
	// readers maps each open-task reader to whether it reaches the clause.
	readers map[string]bool
}

// scanTaskCensus reads every hand-written Go file under root, plus any
// planted sources keyed by their path under root.
func scanTaskCensus(t *testing.T, root string, planted map[string]string) taskCensus {
	t.Helper()
	fset := token.NewFileSet()
	byDir := map[string][]*ast.File{}
	names := map[*ast.File]string{}
	add := func(rel string, src any) {
		file, err := parser.ParseFile(fset, rel, src, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", rel, err)
		}
		dir := filepath.Dir(rel)
		byDir[dir] = append(byDir[dir], file)
		names[file] = filepath.ToSlash(rel)
	}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || strings.HasPrefix(rel, "contracts") {
			return err
		}
		body, err := os.ReadFile(path) // #nosec G304 -- a *.go file from walking the trusted backend tree
		if err != nil {
			return err
		}
		add(rel, body)
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	for rel, src := range planted {
		add(rel, src)
	}
	out := taskCensus{readers: map[string]bool{}}
	for _, files := range byDir {
		censusPackage(files, names, &out)
	}
	slices.Sort(out.writers)
	return out
}

// censusPackage finds one package's writers and readers. A package-level
// string constant counts for every function that names it. A function reaches
// the clause when it, or a same-package function it calls, calls it.
func censusPackage(files []*ast.File, names map[*ast.File]string, out *taskCensus) {
	sqlConsts := map[string]string{}
	for _, file := range files {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gen.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range value.Names {
					if i < len(value.Values) {
						sqlConsts[name.Name] = literalText(value.Values[i])
					}
				}
			}
		}
	}
	type fnFacts struct {
		key           string
		writes, reads bool
		clause        bool
		calls         []string
	}
	var all []fnFacts
	byName := map[string]*fnFacts{}
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			facts := fnFacts{key: names[file] + ":" + fn.Name.Name}
			text := literalText(fn.Body)
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.Ident:
					text += sqlConsts[x.Name]
				case *ast.CompositeLit:
					facts.writes = facts.writes || stampsSourceSystem(x)
				case *ast.AssignStmt:
					facts.writes = facts.writes || assignsSourceSystem(x)
				case *ast.CallExpr:
					switch callee := x.Fun.(type) {
					case *ast.Ident:
						facts.calls = append(facts.calls, callee.Name)
						facts.clause = facts.clause || callee.Name == "CurrentTaskClause"
					case *ast.SelectorExpr:
						facts.calls = append(facts.calls, callee.Sel.Name)
						facts.clause = facts.clause || callee.Sel.Name == "CurrentTaskClause"
					}
				}
				return true
			})
			facts.writes = facts.writes || (strings.Contains(text, "INSERT INTO activity") && strings.Contains(text, "source_system"))
			facts.reads = openTaskKind.MatchString(text) && openTaskDone.MatchString(text)
			all = append(all, facts)
		}
	}
	for i := range all {
		byName[all[i].key[strings.LastIndex(all[i].key, ":")+1:]] = &all[i]
	}
	for changed := true; changed; {
		changed = false
		for i := range all {
			if all[i].clause {
				continue
			}
			for _, callee := range all[i].calls {
				if reached, ok := byName[callee]; ok && reached.clause {
					all[i].clause, changed = true, true
					break
				}
			}
		}
	}
	for _, facts := range all {
		if facts.writes {
			out.writers = append(out.writers, facts.key)
		}
		if facts.reads {
			out.readers[facts.key] = facts.clause
		}
	}
}

// literalText joins the string literals under node.
func literalText(node ast.Node) string {
	var text strings.Builder
	ast.Inspect(node, func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			if value, err := strconv.Unquote(lit.Value); err == nil {
				text.WriteString(value)
			}
		}
		return true
	})
	return text.String()
}

// stampsSourceSystem is a LogActivityInput literal that sets SourceSystem.
func stampsSourceSystem(lit *ast.CompositeLit) bool {
	var name string
	switch typ := lit.Type.(type) {
	case *ast.Ident:
		name = typ.Name
	case *ast.SelectorExpr:
		name = typ.Sel.Name
	}
	if name != "LogActivityInput" {
		return false
	}
	for _, elt := range lit.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "SourceSystem" {
				return true
			}
		}
	}
	return false
}

// assignsSourceSystem is `x.SourceSystem = …` or `fields["source_system"] = …`.
func assignsSourceSystem(stmt *ast.AssignStmt) bool {
	for _, lhs := range stmt.Lhs {
		switch target := lhs.(type) {
		case *ast.SelectorExpr:
			if target.Sel.Name == "SourceSystem" {
				return true
			}
		case *ast.IndexExpr:
			if key, ok := target.Index.(*ast.BasicLit); ok && key.Kind == token.STRING {
				if name, err := strconv.Unquote(key.Value); err == nil && name == "source_system" {
					return true
				}
			}
		}
	}
	return false
}

// taskCensusFaults compares a scan with the declarations, both ways.
func taskCensusFaults(found taskCensus) []string {
	var faults []string
	families := storekit.TaskSourceFamilies()
	claimed := map[string]bool{}
	known := map[string]bool{}
	for _, key := range found.writers {
		known[key] = true
		declared, ok := taskSourceWriters[key]
		if !ok {
			faults = append(faults, key+" stamps a source_system and declares no task family")
			continue
		}
		for _, family := range declared {
			claimed[family] = true
			if !slices.Contains(families, family) &&
				family != writesNoTask && family != writesCallerSource && family != writesForItsCaller {
				faults = append(faults, key+" declares "+family+", which storekit has no currency rule for")
			}
		}
	}
	for key := range taskSourceWriters {
		if !known[key] {
			faults = append(faults, key+" is declared but stamps no source_system")
		}
	}
	for _, family := range families {
		if !claimed[family] {
			faults = append(faults, "storekit family "+family+" has no writer")
		}
	}
	for key, reaches := range found.readers {
		_, waived := openTaskReadersElsewhere[key]
		if !reaches && !waived {
			faults = append(faults, key+" reads open tasks without storekit.CurrentTaskClause")
		}
	}
	for key := range openTaskReadersElsewhere {
		if reaches, found := found.readers[key]; !found || reaches {
			faults = append(faults, key+" is excused but no longer reads open tasks without the clause")
		}
	}
	slices.Sort(faults)
	return faults
}

func TestEveryAutomaticTaskWriterAndOpenTaskReaderIsDeclared(t *testing.T) {
	t.Parallel()
	if faults := taskCensusFaults(scanTaskCensus(t, "internal", nil)); len(faults) > 0 {
		t.Fatalf("the task census found:\n%s", strings.Join(faults, "\n"))
	}
}

// A writer and a reader planted outside every package the declarations name
// must fail, even when the writer reuses the public kind `task`.
func TestTheTaskCensusCatchesAnUnregisteredWriterAndReader(t *testing.T) {
	t.Parallel()
	writer := `package planted
func mintBotTask() activities.LogActivityInput {
	source := "bot"
	return activities.LogActivityInput{Kind: "task", SourceSystem: &source}
}`
	reader := `package planted
const botQueue = "SELECT id FROM activity a WHERE a.kind = 'task' AND NOT a.is_done"
func readBotQueue() string { return botQueue }`
	for name, src := range map[string]string{"writer": writer, "reader": reader} {
		found := scanTaskCensus(t, "internal", map[string]string{"modules/planted/" + name + ".go": src})
		if faults := taskCensusFaults(found); len(faults) == 0 {
			t.Errorf("a planted unregistered %s passed the task census", name)
		}
	}
}
