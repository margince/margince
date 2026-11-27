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
	var findings, collectors []string
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
		for _, collector := range statementCollectorsIn(file) {
			if statementCollectorWaivers.Waived(t, path+":"+collector) {
				continue
			}
			collectors = append(collectors, path+": "+collector)
		}
	})
	if walked < goFileFloor {
		t.Fatalf("the walk reached only %d Go file(s), and this census is pinned at %d — a walk that "+
			"stopped reaching them reports a clean tree in the same words as a tree with nothing "+
			"left to fix", walked, goFileFloor)
	}
	stringFolderWaivers.AssertAllMatched(t)
	statementCollectorWaivers.AssertAllMatched(t)
	if len(collectors) > 0 {
		t.Errorf("%d private statement collector(s) beside gatekit.SQLStatementsOf:\n\t%s\n\n"+
			"Each joins `+` chains its own way, and the census built on it cannot see what that way "+
			"drops. Read the statements with gatekit.SQLStatementsOf, or "+
			"gatekit.SQLStatementsWithConstants when the chain names package constants.",
			len(collectors), strings.Join(collectors, "\n\t"))
	}
	if len(findings) > 0 {
		t.Errorf("%d private string reader(s) beside gatekit.StringExpr:\n\t%s\n\n"+
			"Each answers \"what string does this expression hold\" its own way, and the census "+
			"that reaches for it inherits whatever that way cannot see. Call "+
			"gatekit.StringExpr(expr, consts, gatekit.FoldStrict) for \"is this definitely this "+
			"string\", or gatekit.FoldTotal for \"what can I see of this string\".",
			len(findings), strings.Join(findings, "\n\t"))
	}
}

// statementCollectorWaivers ratifies a walk that joins `+` chains itself because
// it needs something gatekit.SQLStatementsOf does not return.
//
// That walk is the other way of writing a second reader. Each private copy
// chose for itself whether to descend into a chain or read an operand it cannot
// fold, and each chose wrong once.
var statementCollectorWaivers = gatekit.Waive(map[string]string{
	"gates/answerwalk_test.go:func answerWalkStatements":                                                "joins each chain's statements, read by gatekit.SQLStatementsOf, with the SQL of the helpers the chain calls, so a statement a helper finishes reads whole",
	"gates/callgraph_test.go:func packageLevelStatements":                                               "keeps every reading of a package value on purpose (the whole fold, each inner chain, each literal), so a shape matched in any one of them is seen",
	"gates/captureseatlessrows_test.go:func TestEveryReaderOfACaptureSeatSaysWhatASharedMailboxRowDoes": "reads each chain through gatekit.StringExpr and reports it at its own line, which a list of statement texts cannot give",
	"gates/composerowscope_test.go:func indexFuncBody":                                                  "folds through concatenatedSQL, which marks the nodes it consumed so a reference site is not counted twice",
	"gates/fxrateversion_test.go:func fxVersionStatementsIn":                                            "records for each chain whether the chain itself calls the version rule, which needs the chain node a statement text does not carry",
	"gates/oneidlebase_test.go:func spellsTheIdleBase":                                                  "joins every literal of a chain with no hole, those inside its calls too, so a COALESCE spelled across a call's arguments still reads as one",
	"gates/piisqlreader_test.go:func assembledSweepTargets":                                             "reads only the literal that sits beside a computed operand, the fragment a sweep assembles its target from",
	"internal/modules/contacts/opendealcount_test.go:func moduleSQLLiterals":                            "needs each statement's position, to find the reason comment beside it, and a statement text carries none",
})

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
	// The file's OWN name for go/ast, resolved rather than assumed. Matched as
	// the literal `ast` it missed a file that aliases the import; matched as
	// ANY identifier it read `yaml.Node` as a syntax node, and a walk over a
	// YAML document is not a second answer to what string a Go expression
	// holds. gatekit.ImportedAs is the one reader of an import there is.
	syntax, dotImported := gatekit.ImportedAs(file, "go/ast")
	if syntax == "" && !dotImported {
		// A file that does not import go/ast reads no Go syntax.
		return nil
	}
	if dotImported {
		syntax = "."
	}
	var out []string
	for _, decl := range file.Decls {
		fn, isFunc := decl.(*ast.FuncDecl)
		if !isFunc || fn.Body == nil || fn.Recv != nil {
			continue
		}
		if !takesASyntaxNode(fn.Type, syntax) || !answersStringAndBool(fn.Type) || !callsItself(fn) {
			continue
		}
		out = append(out, "func "+fn.Name.Name)
	}
	return out
}

// statementCollectorsIn names every function in the file whose ast.Inspect walk
// matches a `+` chain, and which reads string text.
func statementCollectorsIn(file *ast.File) []string {
	syntax, dotSyntax := gatekit.ImportedAs(file, "go/ast")
	if syntax == "" && !dotSyntax {
		return nil
	}
	if dotSyntax {
		syntax = "."
	}
	tokens, dotTokens := gatekit.ImportedAs(file, "go/token")
	if dotTokens {
		tokens = "."
	}
	var out []string
	for _, decl := range file.Decls {
		fn, isFunc := decl.(*ast.FuncDecl)
		if !isFunc || fn.Body == nil {
			continue
		}
		if inspectsConcatenations(fn.Body, syntax, tokens) && readsStringText(fn.Body, syntax, tokens) {
			out = append(out, "func "+fn.Name.Name)
		}
	}
	return out
}

// inspectsConcatenations reports whether body hands ast.Inspect a visitor that
// matches *ast.BinaryExpr and asks for token.ADD.
func inspectsConcatenations(body *ast.BlockStmt, syntax, tokens string) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall || len(call.Args) != 2 || importedName(call.Fun, syntax) != "Inspect" {
			return !found
		}
		if visitor, isFunc := call.Args[1].(*ast.FuncLit); isFunc {
			found = found || (matchesSyntaxType(visitor.Body, syntax, "BinaryExpr") && namesToken(visitor.Body, tokens, "ADD"))
		}
		return !found
	})
	return found
}

// matchesSyntaxType reports whether body asserts or switches a value to the
// named go/ast type.
func matchesSyntaxType(body ast.Node, syntax, name string) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.TypeAssertExpr:
			found = found || (typed.Type != nil && syntaxTypeName(typed.Type, syntax) == name)
		case *ast.CaseClause:
			for _, listed := range typed.List {
				found = found || syntaxTypeName(listed, syntax) == name
			}
		}
		return !found
	})
	return found
}

func namesToken(body ast.Node, tokens, name string) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		if expr, isExpr := node.(ast.Expr); isExpr && tokens != "" && importedName(expr, tokens) == name {
			found = true
		}
		return !found
	})
	return found
}

// stringTextReaders are the calls that turn a syntax node into its text.
var stringTextReaders = map[string]bool{
	"LiteralText": true, "TextOf": true, "ConcatenatedString": true, "StringExpr": true,
	"SQLStatementsOf": true, "SQLStatementsWithConstants": true, "Unquote": true,
}

// readsStringText reports whether body reads the text of a string literal:
// it names token.STRING or *ast.BasicLit, or calls a reader of text.
func readsStringText(body ast.Node, syntax, tokens string) bool {
	if namesToken(body, tokens, "STRING") || matchesSyntaxType(body, syntax, "BasicLit") {
		return true
	}
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		if call, isCall := node.(*ast.CallExpr); isCall && stringTextReaders[calledName(call)] {
			found = true
		}
		return !found
	})
	return found
}

func takesASyntaxNode(sig *ast.FuncType, syntax string) bool {
	if sig.Params == nil {
		return false
	}
	for _, param := range sig.Params.List {
		if isSyntaxNodeType(param.Type, syntax) {
			return true
		}
	}
	return false
}

// isSyntaxNodeType matches ast.Expr, ast.Node and *ast.BasicLit — the three
// spellings a reader of this shape takes.
//
// The package identifier is the file's own alias for go/ast, resolved from its
// imports. As the literal `ast` it missed a file that aliases the import — a
// clean tree over exactly the shape this census exists to find. As ANY
// identifier it read `yaml.Node` as a syntax node, and a walk over a YAML
// document is not a second answer to what string a Go expression holds.
func isSyntaxNodeType(expr ast.Expr, syntax string) bool {
	switch syntaxTypeName(expr, syntax) {
	case "Expr", "Node", "BasicLit":
		return true
	}
	return false
}

// syntaxTypeName is the go/ast type expr spells, through a pointer, or "".
func syntaxTypeName(expr ast.Expr, syntax string) string {
	if star, isStar := expr.(*ast.StarExpr); isStar {
		expr = star.X
	}
	return importedName(expr, syntax)
}

// importedName is the name expr selects from the package the file imports as
// qualifier, or "". A dot import spells the name unqualified.
func importedName(expr ast.Expr, qualifier string) string {
	switch typed := expr.(type) {
	case *ast.SelectorExpr:
		if pkg, qualified := typed.X.(*ast.Ident); qualified && pkg.Name == qualifier {
			return typed.Sel.Name
		}
	case *ast.Ident:
		if qualifier == "." {
			return typed.Name
		}
	}
	return ""
}

// TestNothingShadowsTheStringConversion holds the premise gatekit.StringExpr's
// conversion arm rests on.
//
// It folds `string(x)` as the identity, matching the callee by NAME because a
// syntactic reader has no types to ask. `string` is a predeclared identifier
// rather than a keyword, so a package could declare its own — and the reader
// would then read that function's result as its argument's text. Nothing here
// does, and this is what says so instead of the reader assuming it.
func TestNothingShadowsTheStringConversion(t *testing.T) {
	t.Parallel()
	var findings []string
	walked := 0
	eachGoFileInTheModule(t, func(path string, file *ast.File) {
		walked++
		for _, decl := range file.Decls {
			if named := declaresString(decl); named != "" {
				findings = append(findings, path+": "+named)
			}
		}
	})
	// Pinned for the same reason its sibling is: a census whose finding is an
	// ABSENCE passes by finding nothing, which is also what it does once it has
	// stopped looking.
	if walked < goFileFloor {
		t.Fatalf("the walk reached only %d Go file(s), and this census is pinned at %d — a walk "+
			"that stopped reaching them reports a clean tree in the same words as a tree with "+
			"nothing left to fix", walked, goFileFloor)
	}
	if len(findings) > 0 {
		t.Errorf("%d declaration(s) shadow `string`:\n\t%s\n\n"+
			"gatekit.StringExpr folds `string(x)` as the identity on its argument, matching the "+
			"callee by name because a syntactic reader has no types to ask. A package that "+
			"declares its own `string` makes that fold read a function's RESULT as its "+
			"argument's text.", len(findings), strings.Join(findings, "\n\t"))
	}
}

// declaresString names the declaration if it binds the identifier `string`.
func declaresString(decl ast.Decl) string {
	switch typed := decl.(type) {
	case *ast.FuncDecl:
		if typed.Recv == nil && typed.Name.Name == "string" {
			return "func string"
		}
	case *ast.GenDecl:
		for _, spec := range typed.Specs {
			switch bound := spec.(type) {
			case *ast.ValueSpec:
				for _, name := range bound.Names {
					if name.Name == "string" {
						return typed.Tok.String() + " string"
					}
				}
			case *ast.TypeSpec:
				if bound.Name.Name == "string" {
					return "type string"
				}
			}
		}
	}
	return ""
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
			// A file that aliases the import spells the same type, and a
			// matcher that missed it would leave that reader invisible.
			name: "an aliased import is the same syntax node",
			source: "package p\nimport goast \"go/ast\"\nfunc fold(e goast.Expr) (string, bool) {\n" +
				"\treturn fold(e)\n}\n",
			want: 1,
		},
		{
			name: "a dot import spells the same type unqualified",
			source: "package p\nimport . \"go/ast\"\nfunc fold(e Expr) (string, bool) {\n" +
				"\treturn fold(e)\n}\n",
			want: 1,
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
			parsed, err := parser.ParseFile(gatekit.SourceFileSet(), "probe.go", tc.source, 0)
			if err != nil {
				t.Fatalf("parsing the probe: %v", err)
			}
			if got := stringFoldersIn(parsed); len(got) != tc.want {
				t.Errorf("the detector found %d reader(s), want %d: %v", len(got), tc.want, got)
			}
		})
	}
}

// The collector census, from the other end. Each private collector it replaced
// is planted in its own shape and must be found. A walk that only reads
// literals, or only meets `+` chains, must not be.
func TestTheCollectorCensusSeesEachShapeASecondCollectorIsWrittenIn(t *testing.T) {
	t.Parallel()
	const head = "package p\nimport (\n\t\"go/ast\"\n\t\"go/token\"\n)\n"
	for _, probe := range []struct {
		name, source string
		want         int
	}{
		{
			name: "a type switch that joins a chain and reads each literal",
			source: head + "func collect(f *ast.File) (out []string) {\n\tast.Inspect(f, func(n ast.Node) bool {\n" +
				"\t\tswitch x := n.(type) {\n\t\tcase *ast.BinaryExpr:\n\t\t\treturn x.Op != token.ADD\n" +
				"\t\tcase *ast.BasicLit:\n\t\t\tout = append(out, x.Value)\n\t\t}\n\t\treturn true\n\t})\n\treturn out\n}\n",
			want: 1,
		},
		{
			name: "an assertion that hands each chain to the shared fold, keyed by position",
			source: head + "func collect(f *ast.File, out map[token.Pos]string) {\n\tast.Inspect(f, func(n ast.Node) bool {\n" +
				"\t\tif b, ok := n.(*ast.BinaryExpr); ok && b.Op == token.ADD {\n" +
				"\t\t\tout[b.Pos()], _ = gatekit.ConcatenatedString(b)\n\t\t}\n\t\treturn true\n\t})\n}\n",
			want: 1,
		},
		{
			name: "an aliased import is the same walk",
			source: "package p\nimport (\n\tgoast \"go/ast\"\n\tgotoken \"go/token\"\n)\n" +
				"func collect(f *goast.File) {\n\tgoast.Inspect(f, func(n goast.Node) bool {\n" +
				"\t\tb, ok := n.(*goast.BinaryExpr)\n\t\treturn ok && b.Op == gotoken.ADD && b.X.(*goast.BasicLit) != nil\n\t})\n}\n",
			want: 1,
		},
		{
			name: "a walk over chains that reads no text",
			source: head + "func calls(f *ast.File) (n int) {\n\tast.Inspect(f, func(node ast.Node) bool {\n" +
				"\t\tif b, ok := node.(*ast.BinaryExpr); ok && b.Op == token.ADD {\n\t\t\tn++\n\t\t}\n\t\treturn true\n\t})\n\treturn n\n}\n",
			want: 0,
		},
		{
			name: "a walk over literals that never meets a chain",
			source: head + "func lits(f *ast.File) (out []string) {\n\tast.Inspect(f, func(n ast.Node) bool {\n" +
				"\t\tif l, ok := n.(*ast.BasicLit); ok && l.Kind == token.STRING {\n\t\t\tout = append(out, l.Value)\n\t\t}\n" +
				"\t\treturn true\n\t})\n\treturn out\n}\n",
			want: 0,
		},
	} {
		t.Run(probe.name, func(t *testing.T) {
			t.Parallel()
			parsed, err := parser.ParseFile(token.NewFileSet(), "probe.go", probe.source, 0)
			if err != nil {
				t.Fatalf("parsing the probe: %v", err)
			}
			if got := statementCollectorsIn(parsed); len(got) != probe.want {
				t.Errorf("the census found %d collector(s), want %d: %v", len(got), probe.want, got)
			}
		})
	}
}
