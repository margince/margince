// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gatekit_test

// The reader, from both ends. Every row here is a case one of the two readers
// this replaced could see and the other could not — which is the defect: the
// blast radius of a census was decided by which file its author had read most
// recently, and picking the narrower reader reports a clean tree rather than
// failing.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"runtime"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

func TestTheStringReaderAnswersBothQuestionsItIsAsked(t *testing.T) {
	t.Parallel()
	consts := map[string]string{"known": "value"}
	cases := []struct {
		name       string
		expr       string
		strictText string
		strictIs   bool
		totalText  string
		totalIs    bool
	}{
		{
			name: "a plain literal", expr: `"gmail.com"`,
			strictText: "gmail.com", strictIs: true,
			totalText: "gmail.com", totalIs: true,
		},
		{
			// The escapes are the point: the reader hands back what the program
			// holds, not what the author typed.
			name: "an interpreted literal", expr: `"a\nb"`,
			strictText: "a\nb", strictIs: true,
			totalText: "a\nb", totalIs: true,
		},
		{
			name: "a resolved identifier", expr: `known`,
			strictText: "value", strictIs: true,
			totalText: "value", totalIs: true,
		},
		{
			// Strict says "not a string" so its census never judges invented
			// text; total says "a hole" so its census keeps descending.
			name: "an unresolvable identifier", expr: `whatever`,
			strictText: "", strictIs: false,
			totalText: gatekit.ComputedFragment, totalIs: false,
		},
		{
			name: "a parenthesised literal", expr: `("x")`,
			strictText: "x", strictIs: true,
			totalText: "x", totalIs: true,
		},
		{
			// The row the total reader used to miss entirely: it had no call
			// arm, so a constant conversion fell through as unreadable.
			name: "a string conversion", expr: `string("v1")`,
			strictText: "v1", strictIs: true,
			totalText: "v1", totalIs: true,
		},
		{
			name: "a call that is not a conversion", expr: `strings.ToLower("x")`,
			strictText: "", strictIs: false,
			totalText: gatekit.ComputedFragment, totalIs: false,
		},
		{
			name: "a concatenation of literals", expr: `"a" + "b"`,
			strictText: "ab", strictIs: true,
			totalText: "ab", totalIs: true,
		},
		{
			// The row the strict reader misses: half a statement is still the
			// half a census's words are in.
			name: "a concatenation with an unreadable half", expr: `"SELECT " + column`,
			strictText: "", strictIs: false,
			totalText: "SELECT " + gatekit.ComputedFragment, totalIs: true,
		},
		{
			// Both halves unreadable is two HOLES, not nothing. Returning
			// nothing let a surrounding concatenation close over the gap —
			// `"a" + (x + y) + "b"` folding to `ab` — and a census then reads a
			// word the source does not contain.
			name: "a concatenation with neither half readable", expr: `x + y`,
			strictText: "", strictIs: false,
			totalText: gatekit.ComputedFragment + gatekit.ComputedFragment, totalIs: false,
		},
		{
			name: "a hole inside a chain does not fuse its neighbours", expr: `"a" + (x + y) + "b"`,
			strictText: "", strictIs: false,
			totalText: "a" + gatekit.ComputedFragment + gatekit.ComputedFragment + "b", totalIs: true,
		},
		{
			name: "a non-string literal", expr: `42`,
			strictText: "", strictIs: false,
			totalText: "", totalIs: false,
		},
		{
			name: "an expression that is not a string at all", expr: `x - y`,
			strictText: "", strictIs: false,
			totalText: "", totalIs: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			expr, err := parser.ParseExpr(tc.expr)
			if err != nil {
				t.Fatalf("parsing %s: %v", tc.expr, err)
			}
			text, is := gatekit.StringExpr(expr, consts, gatekit.FoldStrict)
			if text != tc.strictText || is != tc.strictIs {
				t.Errorf("strict read %q as (%q, %t), want (%q, %t)",
					tc.expr, text, is, tc.strictText, tc.strictIs)
			}
			text, is = gatekit.StringExpr(expr, consts, gatekit.FoldTotal)
			if text != tc.totalText || is != tc.totalIs {
				t.Errorf("total read %q as (%q, %t), want (%q, %t)",
					tc.expr, text, is, tc.totalText, tc.totalIs)
			}
		})
	}
}

// Each row is an input on which one of the package-constant readers this one
// replaced read less than it does.
func TestThePackageConstantReaderReadsEveryShapeGoDeclaresAStringIn(t *testing.T) {
	t.Parallel()
	got := gatekit.StringConstantValues(t, parsedSources(t,
		`package p
const forward = later
const joined = "a" + "b"
const converted = string("c")
const wrapped = ("d")
const first, second = "e", "f"
const (
	kept = "g"
	repeated
)
const dup = "h"
const fromDup = dup
const notString = 4
`,
		`package p
const later = "i"
const dup = "j"
`))
	want := map[string][]string{
		"forward": {"i"}, "later": {"i"}, "joined": {"ab"}, "converted": {"c"}, "wrapped": {"d"},
		"first": {"e"}, "second": {"f"}, "kept": {"g"}, "repeated": {"g"}, "dup": {"h", "j"},
	}
	for name, values := range want {
		if !slices.Equal(got[name], values) {
			t.Errorf("const %s read as %q, want %q", name, got[name], values)
		}
	}
	// A name that holds two values resolves nothing that names it.
	for _, name := range []string{"fromDup", "notString"} {
		if values, read := got[name]; read {
			t.Errorf("const %s read as %q, want it unread", name, values)
		}
	}
}

func TestTheConstantReaderFailsOnAConstantItCannotTellTheValueOf(t *testing.T) {
	t.Parallel()
	// The parser refuses a literal that does not unquote, so that row is built by hand.
	undecodable := &ast.File{Decls: []ast.Decl{&ast.GenDecl{Tok: token.CONST, Specs: []ast.Spec{&ast.ValueSpec{
		Names:  []*ast.Ident{ast.NewIdent("bad")},
		Values: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: `"\q"`}},
	}}}}}
	for name, files := range map[string][]*ast.File{
		"two builds declare it apart":  parsedSources(t, "package p\nconst dsn = \"a\"\n", "package p\nconst dsn = \"b\"\n"),
		"its literal does not unquote": {undecodable},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rec := &fatalRecorder{TB: t}
			finished := make(chan struct{})
			go func() {
				defer close(finished)
				gatekit.StringConstantsOf(rec, files)
			}()
			<-finished
			if rec.fatal == "" {
				t.Error("the reader answered instead of failing")
			}
		})
	}
}

func parsedSources(t *testing.T, sources ...string) []*ast.File {
	t.Helper()
	fset := token.NewFileSet()
	var files []*ast.File
	for i, source := range sources {
		file, err := parser.ParseFile(fset, fmt.Sprintf("f%d.go", i), source, 0)
		if err != nil {
			t.Fatalf("parsing source %d: %v", i, err)
		}
		files = append(files, file)
	}
	return files
}

// fatalRecorder keeps the message of a Fatalf and ends the goroutine, as the
// real Fatalf does.
type fatalRecorder struct {
	testing.TB
	fatal string
}

func (r *fatalRecorder) Helper() {}

func (r *fatalRecorder) Fatalf(format string, args ...any) {
	r.fatal = fmt.Sprintf(format, args...)
	runtime.Goexit()
}
