// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"go/ast"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// Correcting a profile field and confirming one are the same write with a
// different provenance, and they are believed to be the same because they are
// literally the same call. Four effects ride on it — the provenance flip, the
// canonical-column write, the audit image and the event — and a verb that grows
// its own copy of any one of them makes the two disagree about what happened,
// while both still return a well-formed field.
//
// The forbidden set is DERIVED from writeProfileField's own callees rather than
// listed here. Whatever it reaches is what a verb must not reach around it, so
// a fifth effect added inside it is protected the day it is added and nobody
// has to remember this file. Listing the primitives by hand would mean the gate
// protects the four that existed when it was written.
//
// The file is derived too. Naming it meant a verb moved to another file was
// uncensused — and that a rename of the file left the gate parsing a path that
// no longer existed, which is a failure it would have reported as its own
// error rather than as a defect.
//
// What this does NOT hold: another surface writing organization_profile_field
// with its own SQL. Two do — the company form's bulk save and the cold-start
// seed — and they are different operations rather than second copies of this
// one: one writes a whole form at once, the other seeds a profile that has no
// values yet. Neither corrects or confirms a single field, which is what the
// one path is for. Said out loud because a reader who found this gate would
// otherwise reasonably assume it covered them.

const profileFieldOnePath = "writeProfileField"

// profileFieldTable is what a verb writing around the one path would have to
// name, and it is how such a verb is found.
const profileFieldTable = "organization_profile_field"

func TestBothProfileFieldVerbsWriteThroughTheOnePath(t *testing.T) {
	home, onePath := declarationOf(t, profileFieldOnePath)
	reserved := effectsOwnedBy(onePath, importsOf(home))
	if len(reserved) == 0 {
		t.Fatalf("%s reaches no effect of its own, so there is nothing for a verb to duplicate "+
			"— either the write moved out of it or the effect walk is broken", profileFieldOnePath)
	}

	// The verbs are the exported methods that WRITE a profile field: one that
	// reaches the one path, and one that writes the table with its own SQL —
	// which is the shape this holds against and must therefore be IN the
	// population rather than selected out of it.
	//
	// Writing is the claim, so proximity is not the test. Selecting every
	// exported method declared beside the one path put the module's profile
	// field READ verbs in the population and told each of them to go through a
	// write path, which is advice nobody can take.
	verbs := map[string]*ast.FuncDecl{}
	forEachModuleFunc(t, func(_ moduleFile, fn *ast.FuncDecl) {
		if fn.Recv == nil || !fn.Name.IsExported() {
			return
		}
		if callsFunc(fn, profileFieldOnePath) || writesTable(fn, profileFieldTable) {
			verbs[fn.Name.Name] = fn
		}
	})
	if len(verbs) < 2 {
		t.Errorf("%d exported verb(s) reach %s; the claim is that BOTH the correction and the "+
			"confirmation take one path, and one verb cannot disagree with itself",
			len(verbs), profileFieldOnePath)
	}
	for _, name := range sortedKeys(verbs) {
		fn := verbs[name]
		if !callsFunc(fn, profileFieldOnePath) {
			t.Errorf("%s does not go through %s.\n\nCorrecting and confirming are the same write "+
				"with a different provenance; a verb that writes on its own makes them disagree "+
				"about what happened while both still answer with a well-formed field.",
				name, profileFieldOnePath)
			continue
		}
		around := reachedAround(fn, reserved, importsOf(home))
		if len(around) > 0 {
			t.Errorf("%s goes through %s AND reaches %s directly.\n\nThose are effects the one "+
				"path owns. Reaching one around it is how the correction and the confirmation "+
				"come to record different things: move the work inside %s.",
				name, profileFieldOnePath, strings.Join(around, ", "), profileFieldOnePath)
		}
	}
}

// declarationOf finds the named function and the file that declares it.
func declarationOf(t *testing.T, name string) (moduleFile, *ast.FuncDecl) {
	t.Helper()
	var home moduleFile
	var found *ast.FuncDecl
	forEachModuleFunc(t, func(parsed moduleFile, fn *ast.FuncDecl) {
		if fn.Name.Name == name && found == nil {
			home, found = parsed, fn
		}
	})
	if found == nil {
		t.Fatalf("this module declares no %s, so this gate judged nothing and the one path it "+
			"holds has moved or been renamed", name)
	}
	return home, found
}

// importsOf maps a file's local package names to their import paths.
func importsOf(parsed moduleFile) map[string]string {
	paths := map[string]string{}
	for _, spec := range parsed.file.Imports {
		path := strings.Trim(spec.Path.Value, `"`)
		name := path
		if cut := strings.LastIndex(path, "/"); cut >= 0 {
			name = path[cut+1:]
		}
		if spec.Name != nil {
			name = spec.Name.Name
		}
		paths[name] = path
	}
	return paths
}

// effectsOwnedBy names what the ONE PATH reaches that is an effect on the
// record — the set a verb may not reach around it.
//
// Two exclusions, and each closes a way this gate would otherwise accuse
// correct code with advice nobody can follow. The set is DERIVED from the one
// path's callees, so it inherits whatever that function happens to call, and a
// name in it is an instruction to move work inside the one path.
//
//   - The standard library. The day writeProfileField gains an `fmt.Errorf`, a
//     bare-identifier walk reserves `Errorf` and every verb that formats an
//     error is reported as reaching around the path.
//   - Anything the one path does not hand the transaction. An effect on the
//     record is something done to the database, and it needs the tx to do it.
//     `canonicalOrgColumn` is a closed switch from a field name to a column
//     name; reserving it tells a verb to move a pure lookup inside a write.
//
// The tx test is the honest one available here: this walk has no type
// information, so "is it an effect" cannot be asked directly, and what the one
// path HANDS the call is the closest derivable stand-in.
//
// It belongs on this side only. Asking the same of a VERB's calls would let a
// verb reach a reserved effect and escape by handing it something this walk
// does not recognise as the transaction — which is the hole the filter was
// meant to leave alone, reopened from the other end. What a verb hands an
// effect is not the question; that it reaches one is.
func effectsOwnedBy(fn *ast.FuncDecl, imports map[string]string) map[string]bool {
	owned := map[string]bool{}
	if fn.Body == nil {
		return owned
	}
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall || !handedTheTransaction(call) {
			return true
		}
		if name, isEffect := effectNameOf(call.Fun, imports); isEffect {
			owned[name] = true
		}
		return true
	})
	return owned
}

// callsMade names every call fn makes, by the name a reader sees at the site.
func callsMade(fn *ast.FuncDecl, imports map[string]string) map[string]bool {
	callees := map[string]bool{}
	if fn.Body == nil {
		return callees
	}
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall {
			return true
		}
		if name, named := effectNameOf(call.Fun, imports); named {
			callees[name] = true
		}
		return true
	})
	return callees
}

// handedTheTransaction reports whether a call is given something named like the
// transaction — which is what separates a write from a lookup at a call site.
func handedTheTransaction(call *ast.CallExpr) bool {
	for _, arg := range call.Args {
		if name, isIdent := arg.(*ast.Ident); isIdent && (name.Name == "tx" || name.Name == "s") {
			return true
		}
		if closure, isClosure := arg.(*ast.FuncLit); isClosure && closureTakesTx(closure) {
			return true
		}
	}
	return false
}

// closureTakesTx reports a function literal that is handed a transaction — the
// shape `writeEvidence` uses to run each effect inside the one transaction.
func closureTakesTx(closure *ast.FuncLit) bool {
	if closure.Type.Params == nil {
		return false
	}
	for _, param := range closure.Type.Params.List {
		if strings.HasSuffix(typeText(param.Type), "Tx") {
			return true
		}
	}
	return false
}

// writesTable reports whether fn contains a statement writing the named table.
func writesTable(fn *ast.FuncDecl, table string) bool {
	if fn.Body == nil {
		return false
	}
	found := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		lit, isLit := node.(*ast.BasicLit)
		if !isLit {
			return true
		}
		text, isText := gatekit.LiteralText(lit)
		if isText && tableWriteStatement(table).MatchString(text) {
			found = true
		}
		return !found
	})
	return found
}

// tableWriteStatement matches a statement that changes the named table's rows.
func tableWriteStatement(table string) *regexp.Regexp {
	return regexp.MustCompile(`(?is)(INSERT\s+INTO|UPDATE|DELETE\s+FROM)\s+` + regexp.QuoteMeta(table) + `\b`)
}

// effectNameOf renders a call target, and reports whether it is one an effect
// could hide behind.
func effectNameOf(target ast.Expr, imports map[string]string) (string, bool) {
	switch fun := target.(type) {
	case *ast.Ident:
		return fun.Name, true
	case *ast.SelectorExpr:
		base, isIdent := fun.X.(*ast.Ident)
		if !isIdent {
			return fun.Sel.Name, true
		}
		path, isImport := imports[base.Name]
		if !isImport {
			// A method on the receiver or on a value it holds. The name alone
			// is how it reads at the call site and how a second copy would be
			// written.
			return fun.Sel.Name, true
		}
		if isStandardLibrary(path) {
			return "", false
		}
		return base.Name + "." + fun.Sel.Name, true
	}
	return "", false
}

// isStandardLibrary applies Go's own rule: an import path whose first segment
// carries no dot is in the standard library.
func isStandardLibrary(path string) bool {
	first := path
	if cut := strings.Index(path, "/"); cut >= 0 {
		first = path[:cut]
	}
	return !strings.Contains(first, ".")
}

// reachedAround reports which of the one path's own effects fn reaches
// directly, sorted so a failure reads the same on every run.
func reachedAround(fn *ast.FuncDecl, reserved map[string]bool, imports map[string]string) []string {
	reached := callsMade(fn, imports)
	around := map[string]bool{}
	for name := range reserved {
		if reached[name] {
			around[name] = true
		}
	}
	return sortedKeys(around)
}
