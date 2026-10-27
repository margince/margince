// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every native writer of an activity row, and every reader of open tasks,
// found in the code of the whole backend tree. A writer is found by what it
// writes, whether or not it stamps a source_system. It states its automatic
// task family, so its tasks meet that family's currency rule in
// storekit.CurrentTaskClause. Otherwise it states that the task is a human's
// or an agent's own, or that it writes no task. A reader applies that clause
// or states why it reads another population. A new one fails until it says
// which.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/gatekit"
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
	// writesHumanTask makes the task a human or an agent asked for. It stores
	// no source_system, and CurrentTaskClause admits it as manual work, so the
	// function must not act as the system.
	writesHumanTask = "a human's or an agent's own task"
)

// declaredWithoutFamily are the declarations that name no storekit family.
var declaredWithoutFamily = []string{writesNoTask, writesCallerSource, writesForItsCaller, writesHumanTask}

// taskSourceWriters is each writer's declaration, keyed by file:function.
var taskSourceWriters = map[string][]string{
	"compose/assurancebundletask.go:mintBundleTask":                       {provenance.AssuranceTaskSource},
	"compose/leadsla.go:Plan":                                             {provenance.LeadSLATaskSource},
	"compose/capturewhichdeal.go:Ask":                                     {provenance.WhichDealSource},
	"compose/commitmenttask.go:writeCommitmentTask":                       {provenance.CommitmentTaskSource},
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
	"modules/contacts/handlers_company.go:ListCompanies":                  {writesNoTask},
	"modules/deals/handlers_deal.go:ListDeals":                            {writesNoTask},
	"compose/dealroomtimeline.go:write":                                   {writesNoTask},
	"compose/assurancebundletask.go:writeBundleTask":                      {provenance.AssuranceTaskSource},
	"compose/leadworktasks.go:repliedTaskInput":                           {provenance.LeadRepliedSource},
	"compose/leadworktasks.go:arrivalTaskInput":                           {provenance.LeadForContactSource},
	"compose/leadworktasks.go:logSystemTaskTx":                            {writesForItsCaller},
	"modules/activities/emailrequesttasks.go:captureEmailRequest":         {provenance.EmailRequestSource},
	"modules/activities/requesttask.go:takeEmailRequest":                  {provenance.EmailRequestSource},
	"compose/bulkrecordverbs.go:applyTask":                                {writesHumanTask},
	"modules/activities/handlers_activity.go:LogActivity":                 {writesHumanTask},
	"modules/activities/handlers_task.go:CreateTask":                      {writesHumanTask},
	"modules/activities/handlers_task.go:TaskInputFrom":                   {writesHumanTask},
	"modules/activities/requesttask.go:emailRequestTask":                  {provenance.EmailRequestSource},
	"compose/csvactivitywriter.go:createActivity":                         {writesCallerSource},
	"compose/extcore.go:Create":                                           {writesCallerSource},
	"modules/activities/provider.go:Create":                               {writesCallerSource},
	"modules/activities/activity.go:logActivityAndReadTranscript":         {writesForItsCaller},
	"modules/activities/channelsend.go:SendMessage":                       {writesNoTask},
	"modules/activities/sendcore.go:SendPreparedTx":                       {writesNoTask},
	"compose/controllermail.go:QueueConfirmationTx":                       {writesNoTask},
	"compose/extractionaccept.go:auditAcceptedFieldsTx":                   {writesNoTask},
	"modules/activities/booking.go:BookMeeting":                           {writesNoTask},
	"modules/activities/channelsend.go:activity":                          {writesNoTask},
	"modules/activities/extraction.go:RequestAttachmentAccess":            {writesNoTask},
	"modules/activities/reviewresponse.go:WriteOutcomeReviewTx":           {writesNoTask},
	"modules/activities/scheduling_invitation.go:insertInvitation":        {writesNoTask},
	"modules/activities/scheduling_proposal.go:CreateProposal":            {writesNoTask},
	"modules/activities/outboundmessage.go:activity":                      {writesNoTask},
}

// openTaskReadersElsewhere is each reader of open tasks that does not apply
// storekit.CurrentTaskClause itself, with why.
// gatekit:fixture the declared readers of another population, checked both ways below
var openTaskReadersElsewhere = map[string]string{
	"compose/automaticworksql.go:automaticItemsSQL":                              "lists what Margince did, done or not, for Done for you; it reports work, it assigns none",
	"modules/activities/bulkcomplete.go:LockTaskForBulkTx":                       "locks one task a human chose to complete",
	"compose/reportprojects.go:projectCommitmentsSpec":                           "a report over the project's recorded promises, not a personal queue",
	"modules/activities/companyscope.go:ownQueueClause":                          "narrows a list the caller already gates with the worklist clauses",
	"modules/activities/companyscope.go:unassignedQueueClause":                   "narrows a list the caller already gates with the worklist clauses",
	"modules/activities/handover.go:openTasksAssignedTo":                         "a handover moves every open task the departed member holds, current or not, so none stays on the seat",
	"modules/activities/homerepair.go:pristineRepairTaskSQL":                     "the repair reads exactly the automatic tasks that are no longer current",
	"modules/activities/homerepair.go:readHomeRepairTasksTx":                     "the repair reads exactly the automatic tasks that are no longer current",
	"modules/activities/lasttouch.go:openChildReminderHoldsAccount":              "an open reminder of the engine's own family holds its re-raise; it creates no work",
	"modules/activities/lasttouch.go:openReminderHoldsEntity":                    "an open reminder of the engine's own family holds its re-raise; it creates no work",
	"modules/activities/requestreminderreach.go:RecomputeRequestReminderReachTx": "re-derives the reach of one named reminder, so the waiting horizon would skip a stale one instead of correcting it",
	"modules/activities/requestrecovery.go:requestAvailableForAcceptance":        "checks a request has no accepted task before a human accepts it",
	"modules/activities/systemtaskopen.go:OpenSystemTaskOnTx":                    "keeps one open task of the system's own family on a record; it creates no work",
	"modules/activities/whichdealanswer.go:answerWhichDealTx":                    "closes the deal questions a colleague's filing has just answered; it creates no work",
	"modules/activities/taskwindow.go:openTaskWindowClauses":                     "narrows a list the caller already gates with the worklist clauses",
	"modules/activities/taskreminder.go:dueReminderClause":                       "narrows a list the caller already gates with the worklist clauses",
	"modules/contacts/claimrecord.go:SetClaimTaskTx":                             "links a promise to its own task row",
	"modules/contacts/lead_read.go:leadNextTaskDueSQL":                           "the lead's next due date as a record fact",
	"modules/deals/nextstep.go:OpenNextStepSQL":                                  "its caller passes storekit.CurrentTaskClause in as currentTask",
}

var (
	openTaskKind = regexp.MustCompile(`(?i)kind\s*(?:=\s*'task'|IN\s*\([^)]*'task'[^)]*\))`)
	openTaskDone = regexp.MustCompile(`is_done`)
)

// taskCensus is what one scan of a tree found.
type taskCensus struct {
	writers []string
	// builders maps a function that makes an activity row, stamped or not, to
	// what its body shows about that row. It is the denominator.
	builders map[string]activityShape
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
			strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") || isIntegrationTagged(path) {
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
	out := taskCensus{readers: map[string]bool{}, builders: map[string]activityShape{}}
	consts := packageConsts{t: t, byDir: map[string]map[string]string{}}
	for _, files := range byDir {
		censusPackage(root, consts, files, names, &out)
	}
	slices.Sort(out.writers)
	return out
}

// censusPackage finds one package's writers and readers. A package-level
// string constant counts for every function that names it. A function reaches
// the clause when it, or a same-package function it calls, calls it.
func censusPackage(root string, consts packageConsts, files []*ast.File, names map[*ast.File]string, out *taskCensus) {
	var packageValues []ast.Decl
	for _, file := range files {
		for _, decl := range file.Decls {
			if _, ok := decl.(*ast.GenDecl); ok {
				packageValues = append(packageValues, decl)
			}
		}
	}
	sqlConsts := stringConstants([]*ast.File{{Decls: packageValues}})
	type fnFacts struct {
		key           string
		writes, reads bool
		clause        bool
		calls         []string
		shape         activityShape
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
			values := taskValues(consts, gatekit.ParsedFile{Path: filepath.Join(root, names[file]), File: file}, fn)
			visibleConsts := maps.Clone(sqlConsts)
			for name := range declaredNames(fn) {
				delete(visibleConsts, name)
			}
			statements := gatekit.SQLStatementsWithConstants(fn.Body, visibleConsts)
			text := strings.Join(statements, "\n")
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				facts.shape.observe(n, values)
				switch x := n.(type) {
				case *ast.CompositeLit:
					facts.writes = facts.writes || namesSourceSystem(x)
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
			for _, statement := range statements {
				facts.shape.observeSQL(statement)
			}
			facts.shape.stamped = values.provesTaskStamp(fn.Body)
			facts.writes = facts.writes || facts.shape.stampsSQL
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
		if facts.shape.builds {
			out.builders[facts.key] = facts.shape
		}
		if facts.reads {
			out.readers[facts.key] = facts.clause
		}
	}
}

// namesSourceSystem finds a source assignment even when its value is absent.
func namesSourceSystem(lit *ast.CompositeLit) bool {
	if lastName(lit.Type) != "LogActivityInput" {
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
func taskCensusFaults(found taskCensus, declarations map[string][]string) []string {
	faults := builderFaults(found, declarations)
	families := storekit.TaskSourceFamilies()
	for key, declared := range declarations {
		for _, family := range declared {
			if !slices.Contains(families, family) && !slices.Contains(declaredWithoutFamily, family) {
				faults = append(faults, key+" declares "+family+", which storekit has no currency rule for")
			}
		}
	}
	claimed := map[string]bool{}
	for _, key := range found.writers {
		declared, ok := declarations[key]
		if !ok {
			faults = append(faults, key+" stamps a source_system and declares no task family")
			continue
		}
		for _, family := range declared {
			claimed[family] = true
		}
	}
	for _, family := range families {
		if !claimed[family] && !slices.Contains(provenance.RetiredTaskSources(), family) {
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

// builderFaults checks every function that makes an activity, stamped or not,
// against its declaration, and every declaration against what was found.
func builderFaults(found taskCensus, declarations map[string][]string) []string {
	var faults []string
	for key, shape := range found.builders {
		declared, ok := declarations[key]
		switch {
		case !ok || len(declared) == 0:
			faults = append(faults, key+" makes an activity and declares no task family, human or agent path, or non-task")
		case shape.task && slices.Contains(declared, writesNoTask):
			faults = append(faults, key+" declares it writes no task but may write a task")
		case shape.constructs && !shape.stamped && slices.ContainsFunc(declared, func(family string) bool { return slices.Contains(storekit.TaskSourceFamilies(), family) }):
			faults = append(faults, key+" builds an automatic task without its source_system stamp")
		case shape.automatic && slices.Contains(declared, writesHumanTask):
			faults = append(faults, key+" declares a human or agent task but acts as the system; declare its task family")
		}
	}
	for key := range declarations {
		_, builds := found.builders[key]
		if !builds && !slices.Contains(found.writers, key) {
			faults = append(faults, key+" is declared but makes no activity")
		}
	}
	return faults
}

func TestEveryAutomaticTaskWriterAndOpenTaskReaderIsDeclared(t *testing.T) {
	t.Parallel()
	if faults := taskCensusFaults(scanTaskCensus(t, "internal", nil), taskSourceWriters); len(faults) > 0 {
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
	planted := map[string]string{"writer": writer}
	for form, where := range map[string]string{
		"equality":         "a.kind = 'task' AND NOT a.is_done",
		"one-item IN":      "a.kind IN ('task') AND NOT a.is_done",
		"IN list":          "a.kind in ('note', 'task') AND a.is_done = false",
		"IS NOT TRUE done": "a.kind = 'task' AND a.is_done IS NOT TRUE",
	} {
		planted["reader, "+form] = "package planted\nconst botQueue = \"SELECT id FROM activity a WHERE " + where + "\"\nfunc readBotQueue() string { return botQueue }"
	}
	for name, src := range planted {
		found := scanTaskCensus(t, t.TempDir(), map[string]string{"modules/planted/planted.go": src})
		faults := taskCensusFaults(found, taskSourceWriters)
		if !slices.ContainsFunc(faults, func(fault string) bool { return strings.HasPrefix(fault, "modules/planted/planted.go:") }) {
			t.Errorf("a planted unregistered %s passed the task census", name)
		}
	}
}

// activityShape is what one function's body shows about the activity rows it
// makes. builds is found without the source stamp, so a writer that leaves
// source_system out is still in the denominator.
type activityShape struct {
	// builds: the function makes an activity row or a create-task action.
	builds     bool
	constructs bool
	stamped    bool
	// task: the row's kind is spelled as a task somewhere in the function.
	task bool
	// automatic: the function stamps a system origin or acts as the system.
	automatic bool
	// stampsSQL: its activity INSERT names source_system.
	stampsSQL bool
}

// sharedActivityWriters are the store methods that write a LogActivityInput.
// A caller of one makes an activity even when another function built the
// input. So a split builder that sets the kind later is still counted.
var sharedActivityWriters = []string{"LogActivity", "LogActivityTx", "logActivityInTx"}

// observe records one node of the function body.
func (s *activityShape) observe(n ast.Node, values taskValueReader) {
	switch x := n.(type) {
	case *ast.CompositeLit:
		input := lastName(x.Type) == "LogActivityInput"
		if input {
			s.builds = true
			s.constructs = s.constructs || len(x.Elts) > 0
		}
		for _, elt := range x.Elts {
			if kv, ok := elt.(*ast.KeyValueExpr); ok {
				if input || lastName(kv.Value) == "ActionCreateTask" {
					s.field(kv.Key, kv.Value, values)
				}
			}
		}
	case *ast.AssignStmt:
		for i, lhs := range x.Lhs {
			if i >= len(x.Rhs) {
				break
			}
			if lit, ok := x.Rhs[i].(*ast.CompositeLit); ok && lastName(lit.Type) == "LogActivityInput" {
				s.builds, s.constructs = true, true
			}
			if target, ok := lhs.(*ast.SelectorExpr); ok {
				s.field(target.Sel, x.Rhs[i], values)
			}
		}
	case *ast.ValueSpec:
		if lastName(x.Type) == "LogActivityInput" {
			s.builds, s.constructs = true, true
		}
	case *ast.CallExpr:
		if fun, ok := x.Fun.(*ast.Ident); ok && fun.Name == "new" && len(x.Args) == 1 {
			if lastName(x.Args[0]) == "LogActivityInput" {
				s.builds, s.constructs = true, true
			}
		}
		callee := lastName(x.Fun)
		s.builds = s.builds || slices.Contains(sharedActivityWriters, callee)
		s.automatic = s.automatic || callee == "SystemActing"
	case *ast.SelectorExpr:
		s.automatic = s.automatic || x.Sel.Name == "PrincipalSystem"
	}
}

// field reads one `Key: value` or `x.Key = value` of an activity or action.
func (s *activityShape) field(key ast.Node, value ast.Expr, values taskValueReader) {
	name, ok := key.(*ast.Ident)
	if !ok {
		return
	}
	if conversion, ok := value.(*ast.CallExpr); ok && len(conversion.Args) == 1 {
		if name, ok := conversion.Fun.(*ast.Ident); ok && name.Name == "string" {
			value = conversion.Args[0]
		}
	}
	spelled := lastName(value)
	if lit, ok := value.(*ast.BasicLit); ok && lit.Kind == token.STRING {
		spelled, _ = strconv.Unquote(lit.Value)
	}
	switch name.Name {
	case "Kind":
		if spelled == "ActionCreateTask" {
			s.builds, s.task = true, true
		}
		kind, known := values.read(value, map[string]bool{})
		s.task = s.task || !known || kind == "task" || spelled == "ActionCreateTask"
	case "Origin":
		s.automatic = s.automatic || strings.HasPrefix(spelled, "OriginSystem")
	}
}

// SQL inserts can carry a runtime kind, so a non-task declaration cannot hide one.
func (s *activityShape) observeSQL(text string) {
	for _, write := range sqlWrites(text) {
		if write.table == "activity" && write.verb == "insert" {
			s.builds, s.task = true, true
			s.stampsSQL = s.stampsSQL || slices.Contains(write.cols, "source_system")
		}
	}
}

// Isolated packages exercise the real scanner; the production census checks the tree.
func plantedAt(t *testing.T, src string, declarations map[string][]string) []string {
	t.Helper()
	const key = "modules/planted/planted.go:mint"
	var named []string
	for _, fault := range taskCensusFaults(scanTaskCensus(t, t.TempDir(), map[string]string{"modules/planted/planted.go": src}), declarations) {
		if strings.HasPrefix(fault, key+" ") {
			named = append(named, fault)
		}
	}
	return named
}

// An automatic task writer that leaves source_system out is still found, in
// every shape a writer can take.
func TestTheTaskCensusFindsAWriterThatOmitsTheSourceStamp(t *testing.T) {
	t.Parallel()
	for shape, body := range map[string]string{
		"input literal": `func mint() activities.LogActivityInput {
	line := "Review the account"
	return activities.LogActivityInput{Kind: "task", Subject: &line, Origin: activities.OriginSystemRemediation, Source: "system"}
}`,
		"zero value filled later": `func mint(ctx context.Context, store *activities.Store) error {
	var in activities.LogActivityInput
	in.Kind = "task"
	_, _, err := store.LogActivity(ctx, in)
	return err
}`,
		"split builder through the shared writer": `func mint(ctx context.Context, tx pgx.Tx, store *activities.Store) error {
	in := note()
	in.Kind = "task"
	_, _, err := store.LogActivityTx(ctx, tx, in)
	return err
}`,
		"activity SQL insert": "func mint() string {\n\treturn `INSERT INTO activity (kind, subject, assignee_id, source, captured_by) VALUES ('task', $1, $2, 'system', 'system')`\n}",
		"activity SQL upsert": "func mint() string {\n\treturn `insert into activity (id, kind, subject) VALUES ($1, 'task', $2) ON CONFLICT (id) DO UPDATE SET subject = excluded.subject`\n}",
		"create-task action": `func mint(ev workflow.Event) workflow.Action {
	return workflow.Action{Kind: workflow.ActionCreateTask, Target: ev.Entity}
}`,
	} {
		if faults := plantedAt(t, "package planted\n"+body, taskSourceWriters); len(faults) == 0 {
			t.Errorf("a planted %s with no source_system passed the task census", shape)
		}
	}
}

// A declaration is held to what the body shows, and the manual and agent
// writers it describes still pass.
func TestATaskWriterDeclarationMustMatchWhatItWrites(t *testing.T) {
	t.Parallel()
	declared := func(as string) map[string][]string {
		out := maps.Clone(taskSourceWriters)
		out["modules/planted/planted.go:mint"] = []string{as}
		return out
	}
	human := `package planted
func mint(ctx context.Context, store *activities.Store, line string) error {
	_, _, err := store.LogActivity(ctx, activities.LogActivityInput{Kind: "task", Subject: &line, Origin: activities.OriginHuman})
	return err
}`
	agent := strings.Replace(human, "activities.OriginHuman", "activities.OriginAgent", 1)
	for name, src := range map[string]string{"a human's task": human, "an agent's task": agent} {
		if faults := plantedAt(t, src, declared(writesHumanTask)); len(faults) > 0 {
			t.Errorf("%s declared as one failed the census: %v", name, faults)
		}
	}
	system := strings.Replace(human, "activities.OriginHuman", "activities.OriginSystemRemediation", 1)
	if faults := plantedAt(t, system, declared(writesHumanTask)); len(faults) == 0 {
		t.Error("a system-origin task declared as a human's own passed the census")
	}
	if faults := plantedAt(t, human, declared(writesNoTask)); len(faults) == 0 {
		t.Error("a task writer declared as writing no task passed the census")
	}
}

// The table-write census reads each SQL statement on its own. That makes it a
// second count of the activity inserts, and each must be in the task census.
// No UPDATE may turn a row into a task behind both counts.
func TestTheTaskCensusHoldsEveryActivityInsertTheTableCensusFinds(t *testing.T) {
	t.Parallel()
	builders := scanTaskCensus(t, "internal", nil).builders
	inserts := 0
	for owner, writes := range collectTableWrites(t) {
		for _, write := range writes {
			if write.table != "activity" {
				continue
			}
			file, fn, _ := strings.Cut(write.site, ":")
			key := strings.TrimPrefix(owner, "internal/") + "/" + file + ":" + fn[strings.LastIndex(fn, ".")+1:]
			if write.verb == "update" && slices.Contains(write.cols, "kind") {
				t.Errorf("%s changes an activity's kind; a task made that way escapes the task census", key)
			}
			if write.verb != "insert" {
				continue
			}
			inserts++
			if _, found := builders[key]; !found {
				t.Errorf("%s inserts an activity the task census does not see", key)
			}
		}
	}
	if inserts < 2 {
		t.Fatalf("the table census found %d activity inserts; the shared writer and the capture sink are two", inserts)
	}
}

// lastName is the name an identifier or a package-qualified identifier ends in.
func lastName(expr ast.Expr) string {
	switch x := expr.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return x.Sel.Name
	}
	return ""
}
