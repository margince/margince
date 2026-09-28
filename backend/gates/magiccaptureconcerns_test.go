// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// Every condition capture can raise is a condition the watching lane draws.
//
// The vocabulary belongs to capture: health.go declares one exported constant
// per standing condition a connection can be in. The magic lane keys
// watchedConcerns by those same words, spelled again as bare string literals,
// because compose/magic declares its own seam row rather than importing a
// sibling module's — so the two halves of one vocabulary sit in different
// packages with nothing between them.
//
// concernLine DROPS a kind the map does not carry, deliberately: a condition
// with no sentence is not a call to action. That makes the failure silent in
// the one direction that matters. A condition capture adds or renames vanishes
// from the lane, the totals agree with the short page, every test passes, and
// an administrator is told their mail capture is healthy on the day it stopped
// collecting.
//
// THE STALE DIRECTION IS THE OTHER HALF. A kind retired from capture leaves a
// map entry behind that reads exactly like coverage, and the next author greps,
// finds it, and stops looking. Both directions fail here.
//
// THE CORPUS IS TWO NAMED FILES rather than a gatekit.Scope sweep, because
// each side is one named declaration and not a pattern a module may spell
// anywhere: a sweep would prove a root nothing can drift out of. The one drift
// a named file does admit is a constant moving to another file of the capture
// package, so the rest of that package is read as well — a Concern constant
// found outside captureConcernSource still counts, and says the name is stale.
//
// WHICH SENTENCE EACH CONDITION DRAWS IS NOT THIS GATE'S BUSINESS. That the
// keys in watchedConcerns resolve to words a reader can read is held by
// TestEverySentenceTheReceiptEmitsHasAWord. This gate holds the other axis:
// that the set of conditions is one set.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const (
	captureConcernSource = "internal/modules/capture/health.go"
	captureConcernPkg    = "internal/modules/capture"
	watchedConcernSource = "internal/compose/magic/watching.go"
	// captureConcernPrefix names the constants that are the vocabulary. The
	// file declares other string constants — the connection statuses the
	// conditions are derived FROM — and those are capture's own inputs, which
	// the lane has no business naming.
	captureConcernPrefix = "Concern"
	// watchedConcernsVar is the map this lane can draw, read by name because
	// it is unexported and no import can reach it.
	watchedConcernsVar = "watchedConcerns"
	// captureConcernFloor guards against a vacuous pass. Four conditions ship
	// today; the floor sits low enough that retiring two does not drag it
	// along, and high enough that a side which resolved nothing is reported
	// rather than read as two empty sets that happen to agree.
	captureConcernFloor = 2
)

// TestEveryConditionCaptureRaisesTheReceiptCanDraw holds the two sides of the
// capture-concern vocabulary together.
func TestEveryConditionCaptureRaisesTheReceiptCanDraw(t *testing.T) {
	t.Parallel()

	declared := captureConcernKinds(t)
	drawn := watchedConcernKinds(t)
	for side, kinds := range map[string]map[string]bool{
		captureConcernSource: declared,
		watchedConcernSource: drawn,
	} {
		if len(kinds) < captureConcernFloor {
			t.Fatalf("resolved only %d concern kind(s) from %s, expected at least %d — a side this short would agree with any other",
				len(kinds), side, captureConcernFloor)
		}
	}

	for _, kind := range absentFrom(declared, drawn) {
		t.Errorf("capture declares concern %q and %s does not draw it: the receipt drops the condition silently — an administrator is told their capture is healthy while it is not",
			kind, watchedConcernSource)
	}
	for _, kind := range absentFrom(drawn, declared) {
		t.Errorf("%s draws concern %q and capture declares no such condition: nothing raises it, so the entry is a retired kind that still reads as coverage",
			watchedConcernSource, kind)
	}
}

// captureConcernKinds reads the vocabulary out of the package that owns it.
func captureConcernKinds(t *testing.T) map[string]bool {
	t.Helper()
	sources, err := filepath.Glob(filepath.Join(captureConcernPkg, "*.go"))
	if err != nil {
		t.Fatalf("listing %s for its concern vocabulary: %v", captureConcernPkg, err)
	}
	kinds := map[string]bool{}
	for _, source := range sources {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}
		for _, kind := range captureConcernKindsIn(t, source) {
			kinds[kind] = true
		}
	}
	return kinds
}

// captureConcernKindsIn collects the concern constants one file declares.
//
// An exported Concern constant this reader cannot resolve to a string is a
// FAILURE rather than a skip: an iota block or a grouped spec would otherwise
// shorten this side of the census, and a short side reads as agreement.
func captureConcernKindsIn(t *testing.T, source string) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), source, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing %s for its concern vocabulary: %v", source, err)
	}
	var kinds []string
	for _, spec := range concernConstSpecs(file) {
		if len(spec.Names) != 1 || len(spec.Values) != 1 {
			t.Fatalf("%s declares a %s constant in a spec this gate cannot read as one name and one string, so the vocabulary reads short",
				source, captureConcernPrefix)
		}
		value, ok := concernStringLiteral(spec.Values[0])
		if !ok {
			t.Fatalf("%s declares %s with an expression rather than a string literal, so the vocabulary reads short",
				source, spec.Names[0].Name)
		}
		kinds = append(kinds, value)
	}
	if source != captureConcernSource && len(kinds) > 0 {
		t.Errorf("%s declares %d concern constant(s) and this gate's prose names %s as where the vocabulary lives. They are counted either way — the corpus, not the census, is what has gone stale",
			source, len(kinds), captureConcernSource)
	}
	return kinds
}

// concernConstSpecs names the exported constants that are the vocabulary.
func concernConstSpecs(file *ast.File) []*ast.ValueSpec {
	var specs []*ast.ValueSpec
	for _, decl := range file.Decls {
		general, isGeneral := decl.(*ast.GenDecl)
		if !isGeneral || general.Tok != token.CONST {
			continue
		}
		for _, spec := range general.Specs {
			value, isValue := spec.(*ast.ValueSpec)
			if isValue && concernNamed(value) {
				specs = append(specs, value)
			}
		}
	}
	return specs
}

// concernNamed reports whether a spec names the vocabulary. It asks of EVERY
// name in the spec, so a grouped declaration carrying one reaches the reader
// that refuses it rather than slipping past unseen.
func concernNamed(spec *ast.ValueSpec) bool {
	for _, name := range spec.Names {
		if strings.HasPrefix(name.Name, captureConcernPrefix) && name.IsExported() {
			return true
		}
	}
	return false
}

// watchedConcernKinds reads the keys of the map the lane draws from.
//
// A missing map is fatal: the identifier is unexported and renaming it would
// otherwise leave this gate comparing capture's vocabulary against nothing,
// which is the shape where both directions pass and neither was checked.
func watchedConcernKinds(t *testing.T) map[string]bool {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), watchedConcernSource, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing %s for the conditions it draws: %v", watchedConcernSource, err)
	}
	literal, ok := watchedConcernsLiteral(file)
	if !ok {
		t.Fatalf("%s declares no `var %s = map[string]...{...}`: this gate reads that map by name, so a rename leaves it comparing capture's vocabulary against nothing",
			watchedConcernSource, watchedConcernsVar)
	}
	kinds := map[string]bool{}
	for _, element := range literal.Elts {
		pair, isPair := element.(*ast.KeyValueExpr)
		if !isPair {
			t.Fatalf("%s has an entry in %s that is not a keyed one, so the set of drawn conditions reads short",
				watchedConcernSource, watchedConcernsVar)
		}
		key, isString := concernStringLiteral(pair.Key)
		if !isString {
			t.Fatalf("%s keys %s by an expression rather than a string literal, so the set of drawn conditions reads short",
				watchedConcernSource, watchedConcernsVar)
		}
		kinds[key] = true
	}
	return kinds
}

// watchedConcernsLiteral finds the map declaration by name.
func watchedConcernsLiteral(file *ast.File) (*ast.CompositeLit, bool) {
	for _, decl := range file.Decls {
		general, isGeneral := decl.(*ast.GenDecl)
		if !isGeneral || general.Tok != token.VAR {
			continue
		}
		for _, spec := range general.Specs {
			value, isValue := spec.(*ast.ValueSpec)
			if !isValue || len(value.Names) != 1 || value.Names[0].Name != watchedConcernsVar || len(value.Values) != 1 {
				continue
			}
			if literal, isLiteral := value.Values[0].(*ast.CompositeLit); isLiteral {
				return literal, true
			}
		}
	}
	return nil, false
}

// concernStringLiteral reads an expression that must be a quoted string.
func concernStringLiteral(expr ast.Expr) (string, bool) {
	lit, isLit := expr.(*ast.BasicLit)
	if !isLit || lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return value, true
}
