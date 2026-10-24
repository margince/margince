// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// rowProducers is each function in this package that builds a Worklist row,
// with the sources its rows carry. The census derives the producers from the
// code and compares both ways. A new producer fails here until it states its
// sources, even when it reuses a source that already has a policy.
var rowProducers = map[string][]crmcontracts.WorklistItemSource{
	"classifyAwaiting":          {crmcontracts.WorklistItemSourceAwaitingReply},
	"classifyMeetingFollowUp":   {crmcontracts.WorklistItemSourceMeetingFollowUp},
	"batchRow":                  {crmcontracts.WorklistItemSourceBatch},
	"classifyCommitment":        {crmcontracts.WorklistItemSourceConversationClaim},
	"classifyFailedApproval":    {crmcontracts.WorklistItemSourceFailedApproval},
	"classifyIntroduction":      {crmcontracts.WorklistItemSourceIntroductionRequest},
	"classifyTask":              {crmcontracts.WorklistItemSourceTask},
	"classifyBriefItem":         {crmcontracts.WorklistItemSourceBriefItem},
	"classifyDecay":             {crmcontracts.WorklistItemSourceRelationshipDecay},
	"classifyDecision":          {crmcontracts.WorklistItemSourceApproval, crmcontracts.WorklistItemSourceDedupeCandidate, crmcontracts.WorklistItemSourceDealSuggestion, crmcontracts.WorklistItemSourceTagSuggestion},
	"classifyBounce":            {crmcontracts.WorklistItemSourceBounce},
	"classifySystem":            {crmcontracts.WorklistItemSourceCaptureHealth, crmcontracts.WorklistItemSourceAiWorkHealth, crmcontracts.WorklistItemSourceAutomationRun, crmcontracts.WorklistItemSourceNotice},
	"classifyUndelivered":       {crmcontracts.WorklistItemSourceUndelivered},
	"classifyDomainQuestion":    {crmcontracts.WorklistItemSourceDomainQuestion},
	"classifyLegalDeadline":     {crmcontracts.WorklistItemSourceDsr, crmcontracts.WorklistItemSourceNoticeCase},
	"classifyRisk":              {crmcontracts.WorklistItemSourceDealAtRisk},
	"classifyWaiting":           {crmcontracts.WorklistItemSourceCustomerWaiting},
	"classifyMeeting":           {crmcontracts.WorklistItemSourceMeeting},
	"classifyUnansweredMeeting": {crmcontracts.WorklistItemSourceMeetingOutcome},
	"planRowsOf":                {crmcontracts.WorklistItemSourceWeeklyCommitment},
}

// packageFiles parses this package's production files, plus any planted ones.
func packageFiles(t *testing.T, planted ...string) []*ast.File {
	t.Helper()
	fset := token.NewFileSet()
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var files []*ast.File
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, file)
	}
	for i, src := range planted {
		file, err := parser.ParseFile(fset, "planted"+string(rune('a'+i))+".go", src, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, file)
	}
	return files
}

// admissionCensus answers what the code does that the registry and the
// admission rule do not account for.
func admissionCensus(files []*ast.File) []string {
	var faults []string
	found := map[string]bool{}
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if buildsARow(fn.Body) {
				found[fn.Name.Name] = true
				if _, registered := rowProducers[fn.Name.Name]; !registered {
					faults = append(faults, fn.Name.Name+" builds Worklist rows and states no sources in rowProducers")
				}
			}
			faults = append(faults, unadmittedDayReads(fn)...)
		}
	}
	for name, sources := range rowProducers {
		if !found[name] {
			faults = append(faults, name+" is registered but builds no row")
		}
		for _, source := range sources {
			if _, ok := sourceOwnerAnswers[source]; !ok {
				faults = append(faults, name+" emits "+string(source)+", which has no admission policy")
			}
		}
	}
	slices.Sort(faults)
	return faults
}

// buildsARow is a non-empty `ranked{...}` literal: a row with facts in it.
func buildsARow(body *ast.BlockStmt) bool {
	built := false
	ast.Inspect(body, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if ok && len(lit.Elts) > 0 {
			if ident, isIdent := lit.Type.(*ast.Ident); isIdent && ident.Name == "ranked" {
				built = true
			}
		}
		return true
	})
	return built
}

// unadmittedDayReads lists the classifyDay calls in fn that sit outside an
// admitBySource argument: a reader of the day that skips admission.
func unadmittedDayReads(fn *ast.FuncDecl) []string {
	if fn.Name.Name == "classifyDay" {
		return nil
	}
	admitted := map[ast.Expr]bool{}
	var calls []*ast.CallExpr
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if ident, isIdent := call.Fun.(*ast.Ident); isIdent {
			switch ident.Name {
			case "admitBySource":
				for _, arg := range call.Args {
					ast.Inspect(arg, func(inner ast.Node) bool {
						if expr, isExpr := inner.(ast.Expr); isExpr {
							admitted[expr] = true
						}
						return true
					})
				}
			case "classifyDay":
				calls = append(calls, call)
			}
		}
		return true
	})
	var faults []string
	for _, call := range calls {
		if !admitted[call] {
			faults = append(faults, fn.Name.Name+" reads classifyDay without admitBySource")
		}
	}
	return faults
}

func TestEveryRowProducerAndDayReaderIsAccountedFor(t *testing.T) {
	t.Parallel()
	if faults := admissionCensus(packageFiles(t)); len(faults) > 0 {
		t.Fatalf("the admission census found:\n%s", strings.Join(faults, "\n"))
	}
}

// The census must fail on a producer it was not told about, even one reusing
// a source with a policy. It must also fail on a reader that skips admission.
func TestTheCensusCatchesAnUnregisteredProducerAndConsumer(t *testing.T) {
	t.Parallel()
	producer := `package attention
func classifySneakyTask(item crmcontracts.AttentionItem) ranked {
	return ranked{item: base(item, levelAgreed, "tasks", "task_slips"), ownerRef: unassigned()}
}`
	consumer := `package attention
func sneakyQueue(day crmcontracts.Attention) []ranked {
	return classifyDay(day, day.AsOf, dayMoney{})
}`
	for name, src := range map[string]string{"producer": producer, "consumer": consumer} {
		if faults := admissionCensus(packageFiles(t, src)); len(faults) == 0 {
			t.Errorf("a planted unregistered %s passed the census", name)
		}
	}
}
