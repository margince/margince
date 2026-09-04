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
	"os"
	"path/filepath"
	"strings"
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
	// Plus the packages a surface calls the MODEL through.
	//
	// The corpus above is "who composes the prompt", which was the same set as
	// "who calls the model" right up until the two grounded surfaces moved
	// their writer into draftcore. They still compose their own prompts — that
	// part is per-surface by design — so they stayed in the corpus while the
	// nine voice-carrying calls they now share dropped out of the sweep
	// entirely. The census would have gone on reading a smaller tree and
	// reporting PASS, which is the one way it must not break.
	surfaces = append(surfaces, sharedWriterFiles(t)...)
	// The package, not the one file: a surface splits its model call and its
	// voice load across files (the reply drafter does), so a caller in a
	// sibling file is still this surface's.
	checked := map[string]bool{}
	governed := 0
	for _, where := range surfaces {
		dir := filepath.Dir(where)
		if checked[dir] {
			continue
		}
		checked[dir] = true
		found, reached := alwaysUnvoicedCallers(t, dir)
		governed += reached
		for _, finding := range found {
			t.Errorf("%s reaches the model only ever without the sender's voice, so a rep who has built a "+
				"voice profile is written by a generic writer here and by their own voice on every other "+
				"drafting surface. Load the profile with draftvoice.Load and pass it; an unvoiced call is "+
				"fine only as the fallback beside a voiced one", finding)
		}
	}
	// The census must not be able to fail SHORT. This gate recognises a voice
	// argument by its parameter NAME, so renaming that parameter would drop its
	// callers from the sweep — the gate would then read a smaller tree, find
	// nothing, and report PASS with no failing assertion to notice.
	//
	// Pinning the count is what makes that visible. A drop means the sweep
	// stopped seeing calls it used to see: either look at why, or lower this
	// number deliberately. A rise is ordinary — a new drafting call — and
	// raising it is the whole of the response.
	// Was 36 while the two grounded surfaces each carried their own copy of the
	// writer: 22 in the reply lane, 7 in accountdraft, 7 in persondraft. The
	// copies became one, so those fourteen are now 2 + 2 in the surfaces (their
	// own prompt assembly) plus 5 in draftcore, which every surface runs.
	const governedCalls = 31
	if governed != governedCalls {
		t.Errorf("the sweep reached %d voice-carrying calls and this gate pins %d. Fewer means it stopped "+
			"recognising calls it used to read — most likely a voice parameter was renamed out of "+
			"voiceBlockParameterNames, which silently shrinks what this gate governs. Confirm every "+
			"drafting call is still swept, then move the number", governed, governedCalls)
	}
}

// alwaysUnvoicedCallers reports every production function in the package at dir
// that reaches a voice-taking call and carries no voice at EVERY such call, plus
// how many voice-carrying calls the sweep read in total.
//
// Every, not any. A function holding both shapes is the healthy one: it drafts
// under the profile when there is a profile and falls back to the plain draft
// when the sender has none or the voiced attempt broke the anti-AI floor. A
// function holding only the empty shape is a path no profile can ever reach.
//
// The count comes back so the caller can pin it. This sweep recognises its
// subject by parameter name, and a census that can quietly read a smaller tree
// than it did yesterday reports PASS for the wrong reason.
func alwaysUnvoicedCallers(t *testing.T, dir string) (findings []string, reached int) {
	t.Helper()
	fset := token.NewFileSet()
	files := parsePackageFiles(t, fset, dir)
	positions := voiceParameterPositions(files)
	if len(positions) == 0 {
		return nil, 0
	}
	for _, parsed := range files {
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			voiced, unvoiced := voiceArgumentShapes(fn, positions)
			reached += voiced + unvoiced
			// A function that itself takes the voice block is plumbing rather
			// than an entry point: it passes on what its own caller resolved,
			// and the caller is where the question belongs. Its calls still
			// COUNT — the census must see them — but it is not judged.
			if _, plumbing := positions[fn.Name.Name]; plumbing {
				continue
			}
			// Nor is a certification case an entry point. It drives the
			// production lane to MEASURE one register, and the register it
			// measures is the scenario's to state — a case pinned to the plain
			// variant is certifying the plain variant, not drafting a mail
			// somebody sends. Its calls still count toward the census.
			if isCertificationCase(fset.Position(fn.Pos()).Filename) {
				continue
			}
			if unvoiced > 0 && voiced == 0 {
				findings = append(findings, fset.Position(fn.Pos()).String()+" ("+fn.Name.Name+")")
			}
		}
	}
	return findings, reached
}

// voiceArgumentShapes counts the calls in fn that pass a voice block, split by
// whether the argument can carry a voice at all.
//
// Two spellings say "no voice", and both must count as unvoiced: a literal nil
// for the func-shaped block, and an empty draftvoice.Context{} for the struct
// one. Counting only nil let a site that always constructs the empty struct read
// as voiced while taking the unvoiced branch on every call — which is what the
// first-message certification case does deliberately, and what a production site
// could do by accident.
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
		if carriesNoVoice(call.Args[at]) {
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

// isCertificationCase reports whether a path holds an ai-tasks certification
// case rather than a drafting surface.
//
// Read off the file NAME, which this tree spells one way (certcase_*.go).
// Reading it off the content instead — "does this file mention aitasks" — would
// let a drafting surface that happened to import the package exempt itself, and
// a selector that can eat its own subject is worse than no selector.
func isCertificationCase(path string) bool {
	return strings.HasPrefix(filepath.Base(path), "certcase")
}

// carriesNoVoice reports whether an argument is one of the two ways this tree
// spells "no profile": the bare identifier nil, or an empty composite literal
// (draftvoice.Context{}), whose OK field is false and which every drafting
// method branches to the unvoiced path on.
//
// A composite literal with fields set is NOT this: draftvoice.Context{OK: true,
// …} is a real voice, and the empty one is the only shape that cannot be.
func carriesNoVoice(expr ast.Expr) bool {
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name == "nil"
	}
	composite, ok := expr.(*ast.CompositeLit)
	return ok && len(composite.Elts) == 0
}

// sharedWriterFiles are the files of the package the grounded surfaces call the
// model through.
//
// Derived from the surfaces themselves rather than named here: a package that
// composes the shared rules AND imports draftcore has handed its model call
// over, so draftcore's own calls are that surface's calls and belong in the
// same census.
func sharedWriterFiles(t *testing.T) []string {
	t.Helper()
	const writer = "draftcore"
	entries, err := os.ReadDir(filepath.Join(".", writer))
	if err != nil {
		t.Fatalf("reading the shared writer package: %v", err)
	}
	var out []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		out = append(out, filepath.Join(writer, name))
	}
	if len(out) == 0 {
		t.Fatal("the shared writer package has no files, so the calls both grounded surfaces make " +
			"through it are outside this census")
	}
	return out
}
