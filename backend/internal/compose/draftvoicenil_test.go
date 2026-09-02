// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// No drafting entry point can only ever draft in nobody's voice.
//
// The census next door (TestEveryDraftingSurfaceLoadsTheSenderVoice) asks
// whether a surface's PACKAGE can reach draftvoice.Load. That question has one
// answer for a package and several entry points inside it, which is how the
// first-message drafter shipped unvoiced: it lives beside the reply drafter, its
// package loads the voice for the reply, and its own function passed a literal
// nil straight to the model. The package census read green the whole time.
//
// So this reads the entry points. An unvoiced call is not itself a defect — a
// rep with no profile is drafted for unvoiced, and a voiced draft that trips the
// anti-AI floor is re-drafted unvoiced on purpose. Both of those live in a
// function that ALSO has a voiced path. What is a defect is a function whose
// every model call is unvoiced: no profile can reach it, so the rep who built
// one is written by a generic writer here and by their own voice next door.

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"testing"
)

// voiceBlockParameterNames are the parameter names a drafting call uses for the
// voice block it renders into the prompt.
//
// Named rather than positional because the two shapes in this tree differ: the
// reply lane passes a voiceBlockFor func and the composers pass a
// draftvoice.Context. What they share is the name, which is what makes an
// argument at that position readable without type information.
var voiceBlockParameterNames = []string{"voiceBlock", "voice"}

// Every drafting entry point can draft in the sender's own voice.
func TestNoDraftingEntryPointIsAlwaysUnvoiced(t *testing.T) {
	surfaces := filesComposingTheSharedRules(t)
	if len(surfaces) == 0 {
		t.Fatal("no file under internal/compose composes draftrules.Shared, so this gate is reading an " +
			"empty tree rather than a governed one")
	}
	// The package, not the one file: a surface splits its model call and its
	// voice load across files (the reply drafter does), so a caller in a
	// sibling file is still this surface's.
	checked := map[string]bool{}
	for _, where := range surfaces {
		dir := filepath.Dir(where)
		if checked[dir] {
			continue
		}
		checked[dir] = true
		for _, finding := range alwaysUnvoicedCallers(t, dir) {
			t.Errorf("%s reaches the model only ever with a literal nil where the sender's voice goes, so "+
				"a rep who has built a voice profile is written by a generic writer here and by their own "+
				"voice on every other drafting surface. Load the profile with draftvoice.Load and pass it; "+
				"an unvoiced call is fine only as the fallback beside a voiced one", finding)
		}
	}
}

// alwaysUnvoicedCallers reports every production function in the package at dir
// that reaches a voice-taking call and passes a literal nil at EVERY such call.
//
// Every, not any. A function holding both shapes is the healthy one: it drafts
// under the profile when there is a profile and falls back to the plain draft
// when the sender has none or the voiced attempt broke the anti-AI floor. A
// function holding only the nil shape is a path no profile can ever reach.
func alwaysUnvoicedCallers(t *testing.T, dir string) []string {
	t.Helper()
	fset := token.NewFileSet()
	files := parsePackageFiles(t, fset, dir)
	positions := voiceParameterPositions(files)
	if len(positions) == 0 {
		return nil
	}
	var out []string
	for _, parsed := range files {
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			// A function that itself takes the voice block is plumbing rather
			// than an entry point: it passes on what its own caller resolved,
			// and the caller is where the question belongs.
			if _, plumbing := positions[fn.Name.Name]; plumbing {
				continue
			}
			voiced, unvoiced := voiceArgumentShapes(fn, positions)
			if unvoiced > 0 && voiced == 0 {
				out = append(out, fset.Position(fn.Pos()).String()+" ("+fn.Name.Name+")")
			}
		}
	}
	return out
}

// voiceArgumentShapes counts the calls in fn that pass a voice block, split by
// whether the argument is a literal nil or anything else.
func voiceArgumentShapes(fn *ast.FuncDecl, positions map[string]int) (voiced, unvoiced int) {
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		name, ok := calledFunctionName(call)
		if !ok {
			return true
		}
		at, governed := positions[name]
		if !governed || at >= len(call.Args) {
			return true
		}
		if isUntypedNil(call.Args[at]) {
			unvoiced++
			return true
		}
		voiced++
		return true
	})
	return voiced, unvoiced
}

// voiceParameterPositions maps each function in the package that takes a voice
// block to the argument index that block occupies.
//
// One parameter declaration can name several parameters (`a, b Type`), so the
// index is counted over NAMES rather than over fields — counting fields would
// point at the wrong argument for every call to such a function.
func voiceParameterPositions(files []*ast.File) map[string]int {
	out := map[string]int{}
	for _, parsed := range files {
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Type.Params == nil {
				continue
			}
			at := 0
			for _, field := range fn.Type.Params.List {
				for _, name := range field.Names {
					if isVoiceBlockParameter(name.Name) {
						out[fn.Name.Name] = at
					}
					at++
				}
				if len(field.Names) == 0 {
					at++
				}
			}
		}
	}
	return out
}

// isVoiceBlockParameter reports whether a parameter name is the voice block's.
func isVoiceBlockParameter(name string) bool {
	for _, want := range voiceBlockParameterNames {
		if name == want {
			return true
		}
	}
	return false
}

// calledFunctionName reads the name of the function a call targets, for both a
// plain call and a method call on a receiver.
func calledFunctionName(call *ast.CallExpr) (string, bool) {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name, true
	case *ast.SelectorExpr:
		return fn.Sel.Name, true
	default:
		return "", false
	}
}

// isUntypedNil reports whether an argument is the bare identifier nil.
func isUntypedNil(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "nil"
}
