// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A census over the censuses: nobody writes a second reader for "what string
// does this Go expression hold".
//
// There were two, in two files of this package, and neither was a superset of
// the other — one followed `string("v1")` and the other did not, one refused a
// concatenation with an unresolvable half and the other folded the half it
// could read. A census's blast radius is exactly what its reader can see, and
// picking the narrower one does not fail, error, or look any different: it
// reports a clean tree over a shape the other reader would have caught. Which
// reader a census got was decided by which file its author had read most
// recently, and the author who wrote the THIRD one (caught in review) did not
// know either existed.
//
// So gatekit.StringExpr is the reader, with the strict/total question made a
// parameter, and this is what stops the fourth being written. It matches the
// shape rather than the name: a function that takes a syntax node, answers
// (string, bool), and calls itself is a string folder whatever it is called.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// stringFolderWaivers ratifies a recursive (string, bool) reader that is not a
// second answer to this question.
var stringFolderWaivers = gatekit.Waive(map[string]string{
	"gates/composerowscope_test.go:func concatenatedSQL": "records which nodes it consumed as it folds, so the caller does not also read them on their own — bookkeeping the shared reader has no business doing",
	"gates/sqlhelperwalk_test.go:func flattenSQL":        "marks the nodes it consumed AND resolves a helper call to that helper's name, neither of which is a question about what string an expression holds",
})

func TestNobodyWritesASecondStringReader(t *testing.T) {
	t.Parallel()
	var findings []string
	walked := 0
	eachGoFileInTheModule(t, func(path string, file *ast.File) {
		walked++
		if strings.HasPrefix(path, "internal/shared/gatekit/") {
			// The reader's own home. A shared reader that could not recurse
			// would not be one.
			return
		}
		for _, folder := range stringFoldersIn(file) {
			if stringFolderWaivers.Waived(t, path+":"+folder) {
				continue
			}
			findings = append(findings, path+": "+folder)
		}
	})
	if walked < goFileFloor {
		t.Fatalf("the walk reached only %d Go file(s), and this census is pinned at %d — a walk that "+
			"stopped reaching them reports a clean tree in the same words as a tree with nothing "+
			"left to fix", walked, goFileFloor)
	}
	stringFolderWaivers.AssertAllMatched(t)
	if len(findings) > 0 {
		t.Errorf("%d private string reader(s) beside gatekit.StringExpr:\n\t%s\n\n"+
			"Each answers \"what string does this expression hold\" its own way, and the census "+
			"that reaches for it inherits whatever that way cannot see. Call "+
			"gatekit.StringExpr(expr, consts, gatekit.FoldStrict) for \"is this definitely this "+
			"string\", or gatekit.FoldTotal for \"what can I see of this string\".",
			len(findings), strings.Join(findings, "\n\t"))
	}
}

// goFileFloor is what the walk reached when this census landed. A census that
// can fail SHORT has already failed.
const goFileFloor = 2000

// stringFoldersIn names every function in the file that reads a syntax node as
// a string and recurses to do it.
//
// Three properties together, because each on its own is common: a syntax node
// in, (string, bool) out, and a call to itself. A helper that reads ONE node
// shape without recursing is not a second reader: a concatenation and a
// conversion are where the two readers gave different answers, and a helper
// that never descends into either has no answer of its own to give.
func stringFoldersIn(file *ast.File) []string {
	var out []string
	for _, decl := range file.Decls {
		fn, isFunc := decl.(*ast.FuncDecl)
		if !isFunc || fn.Body == nil || fn.Recv != nil {
			continue
		}
		if !takesASyntaxNode(fn.Type) || !answersStringAndBool(fn.Type) || !callsItself(fn) {
			continue
		}
		out = append(out, "func "+fn.Name.Name)
	}
	return out
}

func takesASyntaxNode(sig *ast.FuncType) bool {
	if sig.Params == nil {
		return false
	}
	for _, param := range sig.Params.List {
		if isSyntaxNodeType(param.Type) {
			return true
		}
	}
	return false
}

// isSyntaxNodeType matches ast.Expr, ast.Node and *ast.BasicLit — the three
// spellings a reader of this shape takes.
func isSyntaxNodeType(expr ast.Expr) bool {
	if star, isStar := expr.(*ast.StarExpr); isStar {
		expr = star.X
	}
	selector, isSelector := expr.(*ast.SelectorExpr)
	if !isSelector {
		return false
	}
	pkg, isIdent := selector.X.(*ast.Ident)
	if !isIdent || pkg.Name != "ast" {
		return false
	}
	switch selector.Sel.Name {
	case "Expr", "Node", "BasicLit":
		return true
	}
	return false
}

func answersStringAndBool(sig *ast.FuncType) bool {
	if sig.Results == nil {
		return false
	}
	var kinds []string
	for _, result := range sig.Results.List {
		ident, isIdent := result.Type.(*ast.Ident)
		if !isIdent {
			return false
		}
		names := len(result.Names)
		if names == 0 {
			names = 1
		}
		for range names {
			kinds = append(kinds, ident.Name)
		}
	}
	return len(kinds) == 2 && kinds[0] == "string" && kinds[1] == "bool"
}

func callsItself(fn *ast.FuncDecl) bool {
	recurses := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall {
			return true
		}
		if ident, isIdent := call.Fun.(*ast.Ident); isIdent && ident.Name == fn.Name.Name {
			recurses = true
		}
		return !recurses
	})
	return recurses
}

// The detector, from the other end. Three properties have to coincide before a
// function is a second reader, and each on its own is common — so each is
// planted here alone, and must NOT fire.
func TestTheFolderCensusSeesEachShapeASecondReaderIsWrittenIn(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		source string
		want   int
	}{
		{
			name: "a node in, (string, bool) out, and it recurses",
			source: "package p\nimport \"go/ast\"\nfunc fold(e ast.Expr) (string, bool) {\n" +
				"\tif b, ok := e.(*ast.BinaryExpr); ok {\n\t\treturn fold(b.X)\n\t}\n\treturn \"\", false\n}\n",
			want: 1,
		},
		{
			name: "ast.Node and *ast.BasicLit are the same shape",
			source: "package p\nimport \"go/ast\"\nfunc fold(n ast.Node) (string, bool) {\n" +
				"\treturn fold(n)\n}\n",
			want: 1,
		},
		{
			// A helper that reads ONE node shape never descends into a
			// concatenation or a conversion, which is where the readers
			// this census exists for gave different answers.
			name: "the same signature without recursion is not a reader",
			source: "package p\nimport \"go/ast\"\nfunc text(e ast.Expr) (string, bool) {\n" +
				"\tlit, ok := e.(*ast.BasicLit)\n\treturn lit.Value, ok\n}\n",
			want: 0,
		},
		{
			name: "a recursive walk that answers something else is not a reader",
			source: "package p\nimport \"go/ast\"\nfunc depth(e ast.Expr) int {\n" +
				"\tif b, ok := e.(*ast.BinaryExpr); ok {\n\t\treturn depth(b.X) + 1\n\t}\n\treturn 0\n}\n",
			want: 0,
		},
		{
			name: "a recursive (string, bool) over something that is not syntax",
			source: "package p\nfunc fold(s []string) (string, bool) {\n" +
				"\tif len(s) == 0 {\n\t\treturn \"\", false\n\t}\n\treturn fold(s[1:])\n}\n",
			want: 0,
		},
		{
			name: "a method is not judged — it carries its receiver's own state",
			source: "package p\nimport \"go/ast\"\ntype w struct{}\nfunc (x w) fold(e ast.Expr) (string, bool) {\n" +
				"\treturn x.fold(e)\n}\n",
			want: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			parsed, err := parser.ParseFile(token.NewFileSet(), "probe.go", tc.source, 0)
			if err != nil {
				t.Fatalf("parsing the probe: %v", err)
			}
			if got := stringFoldersIn(parsed); len(got) != tc.want {
				t.Errorf("the detector found %d reader(s), want %d: %v", len(got), tc.want, got)
			}
		})
	}
}
