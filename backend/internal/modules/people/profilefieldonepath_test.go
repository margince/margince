// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"go/ast"
	"sort"
	"strings"
	"testing"
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

func TestBothProfileFieldVerbsWriteThroughTheOnePath(t *testing.T) {
	home, onePath := declarationOf(t, profileFieldOnePath)
	reserved := effectCallees(onePath, importsOf(home))
	if len(reserved) == 0 {
		t.Fatalf("%s reaches no effect of its own, so there is nothing for a verb to duplicate "+
			"— either the write moved out of it or the callee walk is broken", profileFieldOnePath)
	}

	// The verbs are those declared beside the one path, plus any that reach it
	// from elsewhere: a verb moved to another file is still one of the two this
	// claim is about.
	verbs := map[string]*ast.FuncDecl{}
	forEachModuleFunc(t, func(parsed moduleFile, fn *ast.FuncDecl) {
		if fn.Recv == nil || !fn.Name.IsExported() {
			return
		}
		if parsed.name == home.name || callsFunc(fn, profileFieldOnePath) {
			verbs[fn.Name.Name] = fn
		}
	})
	if len(verbs) < 2 {
		t.Errorf("%d exported verb(s) reach %s; the claim is that BOTH the correction and the "+
			"confirmation take one path, and one verb cannot disagree with itself",
			len(verbs), profileFieldOnePath)
	}
	for _, name := range sortedNames(verbs) {
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

// effectCallees names what fn reaches that could BE an effect: its own methods,
// this package's functions, and calls into other modules — but not the standard
// library.
//
// The standard library is excluded because the reserved set is derived, and a
// derived set inherits whatever the owner happens to call. The day
// writeProfileField gains an fmt.Errorf, a bare-identifier walk puts `Errorf`
// in the reserved set and every verb that formats an error is reported as
// reaching around the one path. The finding would be entirely an artifact of
// how the gate looks, and it would arrive on somebody else's diff.
func effectCallees(fn *ast.FuncDecl, imports map[string]string) map[string]bool {
	callees := map[string]bool{}
	if fn.Body == nil {
		return callees
	}
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall {
			return true
		}
		if name, isEffect := effectNameOf(call.Fun, imports); isEffect {
			callees[name] = true
		}
		return true
	})
	return callees
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
	reached := effectCallees(fn, imports)
	var around []string
	for name := range reserved {
		if reached[name] {
			around = append(around, name)
		}
	}
	sort.Strings(around)
	return around
}

func sortedNames(verbs map[string]*ast.FuncDecl) []string {
	names := make([]string, 0, len(verbs))
	for name := range verbs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
