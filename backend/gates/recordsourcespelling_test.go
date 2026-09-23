// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A record's `source` names where the row came from, and whoever uses this
// product is one origin with one spelling.
//
// Three words meant it once — `manual` from the web app, `ui` from the project
// surface, `mcp` from the tool surface. Collapsing them left a comment saying
// so, and a comment is not a thing that fails, so the words come back. This is
// what fails.
//
// The corpus is provenance.RetiredRecordSourceSpellings(), so adding a fourth
// retired word arms both halves of this gate at once rather than one.
//
// Fixtures are IN SCOPE. A gate that judges only what ships would leave a test
// free to seed the retired word forever, teaching it beside the assertion that
// reads it back — which is exactly how the word returned before. So this walk
// does not carve "_test.go" out the way a product-only sweep does; a seeded
// fixture is as much a second spelling as a production default.
//
// WHAT THIS CANNOT SEE, stated rather than discovered. A source assembled at
// runtime — from config, from a concatenation, from a variable — reads as no
// literal at all, and a third-party client posting `ui` to the public edge is
// outside this tree entirely. `source` is free-form on the wire by contract, so
// neither is refused; both would land a retired word in a row. A retired word
// spelled inside a raw JSON body a test hands to a decoder is the same blind
// spot from the other side: the bytes carry it, but no `KeyValueExpr` does, so
// this walks past it exactly as it walks past a variable. What this holds is
// the tree read as Go syntax, which is where every regression so far came from.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// probeFile plants the shapes below as deliberate evidence. Judging it would
// report this gate's own proof as a finding. Named rather than exempted as "a
// test file": a test that seeds a retired spelling teaches it, and is a finding.
const probeFile = "gates/recordsourcespelling_test.go"

// recordSourceGoRoots are the subtrees a record's `source` is written or
// asserted in Go. A site the sweep below finds outside both means the
// obligation has moved somewhere this gate does not yet look.
var recordSourceGoRoots = []string{"internal", "gates"}

// retiredGoSpellings returns one report line per record-source literal spelling
// a retired word — a struct field or wire key set in a composite literal, and
// the same site set by a later ASSIGNMENT (`spec.Source = "ui"`,
// `body["source"] = "ui"`). A fixture builds most of its record by literal and
// then overwrites one field for the one case under test, which is exactly
// where the second shape came from.
//
// Reported by FILE PATH, not by line: this runs over sources swept from disk
// by their own path rather than through a shared *token.FileSet, so a position
// resolved against the wrong FileSet would print a line that is confidently
// wrong rather than absent. A wrong line is worse than none — the reader who
// trusts it opens the wrong place.
//
// Read directly by the probe suite below. A census asserting a shape is ABSENT
// passes identically over a clean tree and over a detector that has stopped
// detecting, so the detector is tested against planted sources of its own.
func retiredGoSpellings(path string, file *ast.File, retired []string) []string {
	var found []string
	report := func(word string) {
		for _, r := range retired {
			if word == r {
				found = append(found, fmt.Sprintf("%s: source is %q, and the one spelling is %q",
					path, word, provenance.RecordSourceManual))
			}
		}
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.KeyValueExpr:
			if word, ok := recordSourceValue(node.Key, node.Value); ok {
				report(word)
			}
		case *ast.AssignStmt:
			for i, lhs := range node.Lhs {
				if i >= len(node.Rhs) {
					continue
				}
				if word, ok := recordSourceValue(lhs, node.Rhs[i]); ok {
					report(word)
				}
			}
		}
		return true
	})
	return found
}

// recordSourceValue reports the literal a record-source site is set to, from
// either shape the tree writes it in: a composite literal's key, or an
// assignment's target. False for every other key/target, and for a value
// that is not itself an untyped string literal — a constant reference or a
// variable is what the doc comment above says this census cannot judge.
func recordSourceValue(target, value ast.Expr) (string, bool) {
	if !keyIsRecordSource(target) {
		return "", false
	}
	return retiredSourceLiteral(value)
}

// keyIsRecordSource reports whether a key or an assignment target names the
// record's provenance column, in every spelling the tree writes it: the Go
// field `Source:` (a composite literal) or `.Source` (an assignment), and the
// wire key `"source":` or `["source"]`.
//
// EXACT, never a suffix match. `source_system`, `served_identity_source` and
// `current_source` are different questions with vocabularies of their own, and
// a suffix match would report every one of them.
func keyIsRecordSource(key ast.Expr) bool {
	switch k := key.(type) {
	case *ast.Ident:
		return k.Name == "Source"
	case *ast.SelectorExpr:
		return k.Sel.Name == "Source"
	case *ast.BasicLit:
		name, ok := retiredSourceLiteral(k)
		return ok && name == "source"
	case *ast.IndexExpr:
		name, ok := retiredSourceLiteral(k.Index)
		return ok && name == "source"
	}
	return false
}

// hasRecordSourceKey reports whether a file holds ANY record-source key,
// regardless of what it is spelled — the census's SUBJECT, as against
// retiredGoSpellings' narrower FINDING. Keeping the two separate is what lets
// a root stay provably non-vacuous on a clean tree: once every site reads
// "manual", a Subject keyed to the violation itself would find nothing left
// to judge and read the clean result as a stale root instead of a fixed one.
func hasRecordSourceKey(file *ast.File) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		if found {
			return false
		}
		switch node := n.(type) {
		case *ast.KeyValueExpr:
			found = keyIsRecordSource(node.Key)
		case *ast.AssignStmt:
			for _, lhs := range node.Lhs {
				if keyIsRecordSource(lhs) {
					found = true
					break
				}
			}
		}
		return !found
	})
	return found
}

// retiredSourceLiteral unquotes an untyped string literal, reporting false for every
// other expression — a constant reference, a variable, a concatenation. Those
// are the shapes the doc comment above says this census cannot judge.
func retiredSourceLiteral(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return value, true
}

// sweepGoRecordSourcesUnder walks one root, parsing every Go source beneath
// it — hand-written and test alike, since a seeded fixture teaches the
// retired word as surely as a production default. Generated files and
// testdata fixtures are excluded because nothing there is a site anyone
// wrote by hand. Returns every file that holds a record-source key, whether
// or not its value is retired — the census's SUBJECT set, over which
// retiredGoSpellings runs its narrower FINDING check.
func sweepGoRecordSourcesUnder(t *testing.T, root string) []gatekit.ParsedFile {
	t.Helper()
	var swept []gatekit.ParsedFile
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		switch {
		case walkErr != nil:
			return walkErr
		case entry.IsDir():
			if entry.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		case !strings.HasSuffix(path, ".go"), strings.HasSuffix(path, "_gen.go"):
			return nil
		}
		rel := filepath.ToSlash(path)
		if rel == probeFile {
			return nil
		}
		fset := token.NewFileSet()
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if parseErr != nil {
			return fmt.Errorf("could not read %s, and a source the sweep cannot read may hold a subject "+
				"this census is never proven against: %w", rel, parseErr)
		}
		if hasRecordSourceKey(file) {
			swept = append(swept, gatekit.ParsedFile{Path: rel, File: file})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("sweeping %s for record-source sites: %v", root, err)
	}
	return swept
}

func TestNoGoSourceLiteralSpellsARetiredWord(t *testing.T) {
	t.Parallel()
	retired := provenance.RetiredRecordSourceSpellings()
	for _, root := range recordSourceGoRoots {
		swept := sweepGoRecordSourcesUnder(t, root)
		if len(swept) == 0 {
			t.Errorf("%s holds no record-source site this gate judges — a root that finds nothing "+
				"certifies nothing, so either the root is stale or the obligation has moved; correct it "+
				"rather than leaving the gate reading green over an empty tree", root)
		}
		for _, file := range swept {
			for _, finding := range retiredGoSpellings(file.Path, file.File, retired) {
				t.Error(finding)
			}
		}
	}
}

func TestTheGoDetectorAnswersEveryPlantedShape(t *testing.T) {
	t.Parallel()
	for _, probe := range []struct {
		name string
		body string
		want bool
	}{
		{"a struct field", `package p
var _ = struct{ Source string }{Source: "ui"}`, true},
		{"a wire key", `package p
var _ = map[string]any{"source": "mcp"}`, true},
		{"the one spelling", `package p
var _ = struct{ Source string }{Source: "manual"}`, false},
		{"a different provenance field", `package p
var _ = struct{ SourceSystem string }{SourceSystem: "ui"}`, false},
		{"a different vocabulary sharing a word", `package p
var _ = struct{ Mode string }{Mode: "manual"}`, false},
		{"a constant whose VALUE is a retired word", `package p
const metaUIKey = "ui"`, false},
		{"a source built at runtime", `package p
var word = "ui"
var _ = struct{ Source string }{Source: word}`, false},
		{"a struct field set by assignment", `package p
type T struct{ Source string }
func f() { var t T; t.Source = "mcp" }`, true},
		{"a wire key set by assignment", `package p
func f() { m := map[string]any{}; m["source"] = "ui" }`, true},
		{"an assignment to a different field", `package p
type T struct{ SourceSystem string }
func f() { var t T; t.SourceSystem = "ui" }`, false},
	} {
		t.Run(probe.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "probe.go", probe.body, parser.ParseComments)
			if err != nil {
				t.Fatalf("parsing the planted source: %v", err)
			}
			found := retiredGoSpellings("probe.go", file, provenance.RetiredRecordSourceSpellings())
			if got := len(found) > 0; got != probe.want {
				t.Errorf("the detector reported %v, want %v, for:\n%s\nfindings: %v", got, probe.want, probe.body, found)
			}
		})
	}
}
