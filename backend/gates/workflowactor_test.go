// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// The id a workflow write is attributed to, and the id the selectors that
// recognise those writes look for, are ONE id.
//
// captured_by is written from the acting principal's id and never from a
// request body, which is what makes it the unforgeable half of "the system
// wrote this row" — the forgeable half, source, is a value any caller can
// spell. Two selectors rest on that half: the last-touch scan excludes the
// engine's own reminders from what counts as genuine engagement, and the
// follow-up resolver finds the open tasks the engine minted so it can close
// them. Both live in `activities`; the writer lives in `automation`; a module
// never imports a sibling, so the fact is spelled twice and nothing but this
// holds the two spellings equal.
//
// It has already come apart once. The time-scan acted as "system:time-scan"
// while both selectors looked for "system", so every reminder the clock minted
// counted as a touch on the record it was reminding about: one pass made the
// record look freshly worked, it was never reminded about again, and the task
// it left open was never closed. Nothing failed loudly — the reminder landed,
// the run row said applied, and the second one simply never came.
//
// Two halves, because either alone reads green over the defect:
//
//   - the CONSTANTS must agree, which is the drift a rename causes;
//   - every system principal the writing module binds must be that constant,
//     which is the drift a NEW entry point causes — the one that actually
//     happened, where the shared constant was right and a second caller
//     spelled its own.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The two files that spell the one id, and the constants in them. Named
// rather than searched: a gate that hunted for "whichever constant looks like
// an actor id" would go quiet the day one is renamed, which is the drift it
// exists to report.
const (
	workflowActorFile  = "internal/modules/automation/engine.go"
	workflowActorConst = "systemActor"

	workflowSelectorFile  = "internal/modules/activities/followupresolve.go"
	workflowSelectorConst = "systemCapturedBy"

	// workflowActorBinders are every file in the writing module that puts an
	// actor on a context. Stated so the census below can say it read them
	// rather than that it found nothing.
	workflowRunnerDir = "internal/modules/automation"
)

func TestTheEngineActsUnderTheIdItsOwnSelectorsLookFor(t *testing.T) {
	t.Parallel()
	actor := namedConst(t, workflowActorFile, workflowActorConst)
	selector := namedConst(t, workflowSelectorFile, workflowSelectorConst)
	if actor != selector {
		t.Fatalf("the workflow engine acts as %q (%s.%s) and its own selectors look for %q (%s.%s).\n\n"+
			"captured_by is written from the acting principal's id, so a row the engine writes under one "+
			"spelling is invisible to a selector reading the other: the last-touch scan counts the engine's "+
			"reminders as genuine engagement, and the follow-up resolver never finds the tasks it minted.",
			actor, workflowActorFile, workflowActorConst, selector, workflowSelectorFile, workflowSelectorConst)
	}

	bindings := systemActorBindings(t, workflowRunnerDir)
	// A census that can fail short has already failed: the module has two
	// entries into runOne and both bind one, so finding fewer means this gate
	// stopped recognising the shape rather than that the module stopped using
	// it.
	if len(bindings) < 2 {
		t.Fatalf("found %d system principal binding(s) under %s, and the module has at least two (the "+
			"matcher and the time-scan): this gate is reading a shape the module no longer uses",
			len(bindings), workflowRunnerDir)
	}
	for _, binding := range bindings {
		if binding.id != workflowActorConst {
			t.Errorf("%s binds the system principal as %s rather than %s — a second spelling of the actor "+
				"is a second answer to \"did the system write this row\", and the selectors only know one",
				binding.where, binding.id, workflowActorConst)
		}
	}
}

// actorBinding is one place the module names the principal it acts as.
type actorBinding struct {
	where string
	// id is the EXPRESSION the ID field was given, as source text. A literal
	// fails this gate the same way a wrong constant does: the point is that
	// there is one name for the id, not that two literals happen to match
	// today.
	id string
}

// systemActorBindings finds every principal.Principal composite literal in the
// module that names PrincipalSystem, and reports what it set ID to.
func systemActorBindings(t *testing.T, dir string) []actorBinding {
	t.Helper()
	// Read here rather than through parsePackageFiles, which the call-graph
	// censuses share: that one returns syntax without paths, and a finding
	// naming only the module is one a reader has to go looking for.
	sources, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("listing %s to find the actors it binds: %v", dir, err)
	}
	fset := token.NewFileSet()
	var found []actorBinding
	for _, where := range sources {
		if strings.HasSuffix(where, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(fset, where, nil, 0)
		if parseErr != nil {
			t.Fatalf("parsing %s: %v", where, parseErr)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			lit, isLit := node.(*ast.CompositeLit)
			if !isLit {
				return true
			}
			kind, id := principalFields(lit)
			if kind != "PrincipalSystem" {
				return true
			}
			found = append(found, actorBinding{where: where, id: id})
			return true
		})
	}
	return found
}

// principalFields reads the Type and ID a principal literal sets, as the
// selector or identifier name each was given. A field set to something this
// cannot read comes back as the empty string, which fails the comparison
// above rather than passing it.
func principalFields(lit *ast.CompositeLit) (kind, id string) {
	for _, element := range lit.Elts {
		kv, isKV := element.(*ast.KeyValueExpr)
		if !isKV {
			continue
		}
		key, isIdent := kv.Key.(*ast.Ident)
		if !isIdent {
			continue
		}
		switch key.Name {
		case "Type":
			kind = exprName(kv.Value)
		case "ID":
			id = exprName(kv.Value)
		}
	}
	return kind, id
}

// exprName is the name a value was written as: an identifier, the selected
// name of a qualified one, or the quoted text of a literal — so a hard-coded
// id is reported as the string it is rather than as nothing.
func exprName(expr ast.Expr) string {
	switch value := expr.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		return value.Sel.Name
	case *ast.BasicLit:
		if text, err := strconv.Unquote(value.Value); err == nil {
			return strconv.Quote(text)
		}
		return value.Value
	}
	return ""
}

// namedConst reads one string constant out of one file, and fatals when it is
// not there. Not-there is the interesting case: a renamed constant means this
// gate is comparing something it no longer understands, and answering "they
// differ" would send a reader looking for a drift that is really a rename.
func namedConst(t *testing.T, file, name string) string {
	t.Helper()
	value, declared := constValuesIn(t, file)[name]
	if !declared {
		t.Fatalf("%s declares no string constant %s — this gate holds two spellings of one id equal and "+
			"can no longer find one of them, so it is judging nothing", file, name)
	}
	return value
}
