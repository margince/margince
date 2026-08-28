// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// The corpus the gates in this package judge, and the recognisers they judge it
// with.
//
// Both are here for the same reason. A gate that walks the module its own way
// is a second copy of the walk, and the two drift until one of them reads a
// smaller tree than the other while still reporting PASS. A gate that decides
// "does this function build a T" its own way is a second copy of the same
// question, and every copy sees only the spellings its author happened to
// write: a review of seven gates in this package found each recognising the
// literal its author had in front of them and none recognising `var x T`,
// `new(T)`, or a zero literal filled in afterwards.
//
// So the gates rewritten here share one walk and one set of recognisers, and
// each says what it is asking rather than how to look for it. Two older gates in
// this package still walk the directory themselves — dedupeorg_seam_test.go, and
// projectanchorgate_test.go, which parses one filename. Converting them is worth
// doing and is not done here; naming them is the difference between a scope and
// a claim that the next author would grep, find, and stop looking behind.

// moduleFile is one of the module's own non-test sources, parsed.
type moduleFile struct {
	name string
	fset *token.FileSet
	file *ast.File
}

var (
	corpusOnce  sync.Once
	corpusFiles []moduleFile
	corpusErr   error
)

// moduleFiles parses the module's own non-test sources, once per test binary.
//
// Test sources are left out deliberately: a literal a test builds is a fixture,
// and holding a fixture to the shape of a shipped surface would refuse cases
// written to exercise one field at a time.
func moduleFiles(t *testing.T) []moduleFile {
	t.Helper()
	corpusOnce.Do(func() {
		entries, err := os.ReadDir(".")
		if err != nil {
			corpusErr = err
			return
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			fset := token.NewFileSet()
			file, perr := parser.ParseFile(fset, name, nil, parser.ParseComments)
			if perr != nil {
				corpusErr = perr
				return
			}
			corpusFiles = append(corpusFiles, moduleFile{name: name, fset: fset, file: file})
		}
	})
	if corpusErr != nil {
		t.Fatalf("reading the module's sources: %v", corpusErr)
	}
	// A census that reads nothing is the one failure that looks like success,
	// so the corpus refuses to be empty rather than letting each caller
	// remember to check.
	if len(corpusFiles) == 0 {
		t.Fatal("the module directory holds no non-test Go source, so every gate reading this " +
			"corpus judged nothing")
	}
	return corpusFiles
}

// forEachModuleFile hands each parsed source to visit.
func forEachModuleFile(t *testing.T, visit func(name string, fset *token.FileSet, file *ast.File)) {
	t.Helper()
	for _, parsed := range moduleFiles(t) {
		visit(parsed.name, parsed.fset, parsed.file)
	}
}

// forEachModuleFunc hands each function declaration with a body to visit.
// Every gate here that judges functions wants exactly this population, and
// spelling the decl loop at each one is how a gate comes to skip method values,
// or generic declarations, that another gate reads.
func forEachModuleFunc(t *testing.T, visit func(parsed moduleFile, fn *ast.FuncDecl)) {
	t.Helper()
	for _, parsed := range moduleFiles(t) {
		for _, decl := range parsed.file.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || fn.Body == nil || fn.Name == nil {
				continue
			}
			visit(parsed, fn)
		}
	}
}

// takesA reports whether fn is handed the named type.
//
// It is `holdersOf` asked as a yes/no, and it must stay that way: two walks of
// the same receiver-and-parameter list would answer this question and "which
// identifier holds one" differently the first time either learned a new shape,
// and the gates here ask both.
func takesA(fn *ast.FuncDecl, typeName string) bool {
	return len(holdersOf(fn, typeName)) > 0
}

// holdersOf names the identifiers in fn that hold a value of the named type.
//
// Three ways a function comes to hold one, and a gate that knows fewer than all
// three judges a smaller population while reporting a clean module:
//
//	func (t T) …              the receiver — a method ON the type reads its
//	                          fields with exactly the authority a function
//	                          taking one does
//	func f(t T)               a parameter, including *T
//	func f(ts []T) { for _, t := range ts … }
//	                          an element of a slice of them, which is how this
//	                          module's workflow steps actually receive touches
//
// The third is not a corner. Both lead-SLA workflow steps range over a
// `[]leadResponseTouch`, so a gate reading only receivers and parameters cannot
// see the two functions its own header is about.
func holdersOf(fn *ast.FuncDecl, typeName string) map[string]bool {
	held := map[string]bool{}
	fields := []*ast.Field{}
	if fn.Recv != nil {
		fields = append(fields, fn.Recv.List...)
	}
	if fn.Type.Params != nil {
		fields = append(fields, fn.Type.Params.List...)
	}
	collections := map[string]bool{}
	for _, field := range fields {
		switch {
		case typeText(field.Type) == typeName:
			for _, name := range field.Names {
				held[name.Name] = true
			}
		case isSliceOf(field.Type, typeName):
			for _, name := range field.Names {
				collections[name.Name] = true
			}
		}
	}
	if len(collections) == 0 || fn.Body == nil {
		return held
	}
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		loop, isRange := node.(*ast.RangeStmt)
		if !isRange || loop.Value == nil {
			return true
		}
		source, isIdent := loop.X.(*ast.Ident)
		if !isIdent || !collections[source.Name] {
			return true
		}
		if name, namesValue := loop.Value.(*ast.Ident); namesValue && name.Name != "_" {
			held[name.Name] = true
		}
		return true
	})
	return held
}

// isSliceOf reports a `[]T` or `[]*T` of the named type.
func isSliceOf(expr ast.Expr, typeName string) bool {
	slice, isSlice := expr.(*ast.ArrayType)
	return isSlice && slice.Len == nil && typeText(slice.Elt) == typeName
}

// readsFieldOf answers where fn selects the named field ON A VALUE OF THE NAMED
// TYPE, or an invalid position when it does not.
//
// The type matters, and matching the field name alone is the defect this
// replaces. Selecting `.FullName` on the candidate is reading what the ladder
// matched on; selecting it on the LEAD is a second reading. Selecting `.source`
// on some other struct that happens to have one is neither, and a gate that
// counted it would tell an author to ratify a read that never happened.
func readsFieldOf(fn *ast.FuncDecl, typeName, field string) token.Pos {
	var at token.Pos
	for _, pos := range fieldReadsOf(fn, typeName, field) {
		at = pos
		break
	}
	return at
}

// fieldReadsOf reports every position at which fn reads the named field off a
// value of the named type, excluding reads taken by ADDRESS: `&t.capturedBy` in
// a row Scan fills the field rather than asking it anything, and counting it
// would report the builder as a reader of every rule it populates.
func fieldReadsOf(fn *ast.FuncDecl, typeName, field string) []token.Pos {
	holders := holdersOf(fn, typeName)
	if len(holders) == 0 || fn.Body == nil {
		return nil
	}
	addressed := map[ast.Node]bool{}
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if unary, isUnary := node.(*ast.UnaryExpr); isUnary && unary.Op == token.AND {
			addressed[unary.X] = true
		}
		return true
	})
	var found []token.Pos
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		sel, isSel := node.(*ast.SelectorExpr)
		if !isSel || sel.Sel == nil || addressed[ast.Node(sel)] {
			return true
		}
		if field != "" && sel.Sel.Name != field {
			return true
		}
		if base, isIdent := sel.X.(*ast.Ident); isIdent && holders[base.Name] {
			found = append(found, sel.Sel.Pos())
		}
		return true
	})
	return found
}

// fieldsReadOf names which fields of the named type fn reads.
func fieldsReadOf(fn *ast.FuncDecl, typeName string) map[string]bool {
	holders := holdersOf(fn, typeName)
	if len(holders) == 0 || fn.Body == nil {
		return nil
	}
	addressed := map[ast.Node]bool{}
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if unary, isUnary := node.(*ast.UnaryExpr); isUnary && unary.Op == token.AND {
			addressed[unary.X] = true
		}
		return true
	})
	read := map[string]bool{}
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		sel, isSel := node.(*ast.SelectorExpr)
		if !isSel || sel.Sel == nil || addressed[ast.Node(sel)] {
			return true
		}
		if base, isIdent := sel.X.(*ast.Ident); isIdent && holders[base.Name] {
			read[sel.Sel.Name] = true
		}
		return true
	})
	return read
}

// sortedKeys names a map's keys in a stable order, so a failure reads the same
// on every run.
func sortedKeys[V any](values map[string]V) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// constructs reports whether body produces a POPULATED value of the named type.
//
// Populated, not merely mentioned, and by any of the spellings Go offers:
//
//	T{Field: v}          a literal with elements
//	&T{Field: v}         the same, addressed
//	var x T; x.F = v     a zero value filled in afterwards
//	x := T{}; x.F = v    the same, written as a literal
//	x := new(T); x.F = v the same, written as a pointer
//
// A bare zero value with nothing assigned to it is NOT construction: it is the
// error return every function that can fail has, and counting it would report
// every error path as a second derivation. What separates the two is whether a
// field is ever set on it, which is the question a reader asks too.
func constructs(body *ast.BlockStmt, typeName string) bool {
	if populatedLiteral(body, typeName) {
		return true
	}
	zeroed := zeroValuesOf(body, typeName)
	if len(zeroed) == 0 {
		return false
	}
	return anyFieldAssignedTo(body, zeroed)
}

// populatedLiteral reports a composite literal of the type carrying at least
// one element.
func populatedLiteral(body *ast.BlockStmt, typeName string) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		lit, isLit := node.(*ast.CompositeLit)
		if isLit && lit.Type != nil && typeText(lit.Type) == typeName && len(lit.Elts) > 0 {
			found = true
		}
		return !found
	})
	return found
}

// zeroValuesOf names the local variables that start life as a zero value of the
// type: `var x T`, `x := T{}`, and `x := new(T)`.
func zeroValuesOf(body *ast.BlockStmt, typeName string) map[string]bool {
	zeroed := map[string]bool{}
	ast.Inspect(body, func(node ast.Node) bool {
		switch stmt := node.(type) {
		case *ast.DeclStmt:
			gen, isGen := stmt.Decl.(*ast.GenDecl)
			if !isGen || gen.Tok != token.VAR {
				return true
			}
			for _, spec := range gen.Specs {
				value, isValue := spec.(*ast.ValueSpec)
				if !isValue || value.Type == nil || typeText(value.Type) != typeName {
					continue
				}
				for _, name := range value.Names {
					zeroed[name.Name] = true
				}
			}
		case *ast.AssignStmt:
			for i, rhs := range stmt.Rhs {
				if i >= len(stmt.Lhs) || !isZeroValueOf(rhs, typeName) {
					continue
				}
				if name, isIdent := stmt.Lhs[i].(*ast.Ident); isIdent {
					zeroed[name.Name] = true
				}
			}
		}
		return true
	})
	return zeroed
}

// isZeroValueOf reports whether expr is `T{}` or `new(T)`.
func isZeroValueOf(expr ast.Expr, typeName string) bool {
	switch value := expr.(type) {
	case *ast.CompositeLit:
		return value.Type != nil && typeText(value.Type) == typeName && len(value.Elts) == 0
	case *ast.UnaryExpr:
		return value.Op == token.AND && isZeroValueOf(value.X, typeName)
	case *ast.CallExpr:
		ident, isIdent := value.Fun.(*ast.Ident)
		if !isIdent || ident.Name != "new" || len(value.Args) != 1 {
			return false
		}
		return typeText(value.Args[0]) == typeName
	}
	return false
}

// anyFieldAssignedTo reports whether any of the named variables has a field
// written to it — `x.Field = v`, `x.Field += v`, and the same through a
// pointer, which parses identically.
func anyFieldAssignedTo(body *ast.BlockStmt, names map[string]bool) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		assign, isAssign := node.(*ast.AssignStmt)
		if !isAssign {
			return true
		}
		for _, lhs := range assign.Lhs {
			sel, isSel := lhs.(*ast.SelectorExpr)
			if !isSel {
				continue
			}
			if base, isIdent := sel.X.(*ast.Ident); isIdent && names[base.Name] {
				found = true
			}
		}
		return !found
	})
	return found
}

// mutatesResultOf reports whether body takes the value the named function
// returns and then writes a field on it.
//
// This is the shape neither "builds a T" nor "guards a T" can see, and it is a
// derivation just the same: a function that calls the one builder and then
// changes what came back has produced a second answer, in a form that reads at
// the call site as though it were still the first.
func mutatesResultOf(body *ast.BlockStmt, callee string) bool {
	held := map[string]bool{}
	ast.Inspect(body, func(node ast.Node) bool {
		assign, isAssign := node.(*ast.AssignStmt)
		if !isAssign {
			return true
		}
		for _, rhs := range assign.Rhs {
			if !callsNamed(rhs, callee) {
				continue
			}
			// A multi-value call binds its results positionally, and this
			// walk has no type information to say which name is the one.
			// Taking all of them widens the question from "the value" to
			// "anything this call returned", which is the safe direction: it
			// can only over-report a derivation, and nothing in this module
			// writes a field on a returned error or id.
			for _, lhs := range assign.Lhs {
				if name, isIdent := lhs.(*ast.Ident); isIdent && name.Name != "_" {
					held[name.Name] = true
				}
			}
		}
		return true
	})
	if len(held) == 0 {
		return false
	}
	return anyFieldAssignedTo(body, held)
}

// callsNamed reports whether expr is a call of the named function, written
// bare or as a selector — `leadPersonCandidate(…)` and `s.leadPersonCandidate(…)`
// are the same function reached two ways.
func callsNamed(expr ast.Expr, name string) bool {
	call, isCall := expr.(*ast.CallExpr)
	if !isCall {
		return false
	}
	switch target := call.Fun.(type) {
	case *ast.Ident:
		return target.Name == name
	case *ast.SelectorExpr:
		return target.Sel != nil && target.Sel.Name == name
	}
	return false
}

// fieldsOf names the fields the given struct declares, in source order.
//
// Derived rather than listed: a gate naming the fields it judges is a second
// copy of the struct, and the copy is short from the moment a field is added —
// silently, because the gate goes on passing over the fields it does know.
func fieldsOf(t *testing.T, typeName string) []string {
	t.Helper()
	var fields []string
	forEachModuleFile(t, func(_ string, _ *token.FileSet, file *ast.File) {
		ast.Inspect(file, func(node ast.Node) bool {
			spec, isSpec := node.(*ast.TypeSpec)
			if !isSpec || spec.Name.Name != typeName {
				return true
			}
			structure, isStruct := spec.Type.(*ast.StructType)
			if !isStruct || structure.Fields == nil {
				return true
			}
			for _, field := range structure.Fields.List {
				for _, name := range field.Names {
					fields = append(fields, name.Name)
				}
			}
			return false
		})
	})
	if len(fields) == 0 {
		t.Fatalf("this module declares no struct %s with fields, so a gate judging its fields "+
			"has nothing to judge", typeName)
	}
	return fields
}

// packageStrings indexes the module's package-level string constants and
// variables by name, so a census reading a query can resolve one written as an
// identifier.
//
// A query held in a `const` and passed by name is not a rarer shape than an
// inline literal; it is the shape a long query takes. A census that reads only
// the literals inside a function body cannot see it, and what it cannot see
// leaves the population entirely — which is not a lenient judgement, it is no
// judgement.
//
// Resolved to a FIXED POINT rather than in traversal order. `const q = "UPDATE
// " + table` is a valid declaration whether `table` is declared above it or
// below, and a single pass leaves the second case unresolved — an ordering of
// the source file deciding what a gate can see.
func packageStrings(t *testing.T) map[string]string {
	t.Helper()
	pending := map[string]ast.Expr{}
	for _, parsed := range moduleFiles(t) {
		for _, decl := range parsed.file.Decls {
			gen, isGen := decl.(*ast.GenDecl)
			if !isGen || (gen.Tok != token.CONST && gen.Tok != token.VAR) {
				continue
			}
			collectDeclaredStrings(gen, pending)
		}
	}
	known := map[string]string{}
	// Each round resolves only names whose dependencies resolved in an earlier
	// one, so `known` grows strictly until a round adds nothing. It is bounded
	// by `pending`, which is what makes that terminate — not the absence of
	// cycles, which a package that compiles cannot contain anyway.
	for progress := true; progress; {
		progress = false
		for name, expr := range pending {
			if _, done := known[name]; done {
				continue
			}
			if text, isText := gatekit.StringExpr(expr, known, gatekit.FoldStrict); isText {
				known[name] = text
				progress = true
			}
		}
	}
	return known
}

// collectDeclaredStrings records each name in a const or var block against the
// expression it takes its value from.
//
// A grouped const may omit its expression list and INHERIT the one above it —
// `const ( a = "x"; b )` gives b the value of a. Skipping a spec with no values
// drops that name silently, so the last expression list seen in the block is
// carried forward, which is what the language does.
func collectDeclaredStrings(gen *ast.GenDecl, into map[string]ast.Expr) {
	var inherited []ast.Expr
	for _, spec := range gen.Specs {
		value, isValue := spec.(*ast.ValueSpec)
		if !isValue {
			continue
		}
		values := value.Values
		if len(values) == 0 && gen.Tok == token.CONST {
			values = inherited
		} else if len(values) > 0 {
			inherited = values
		}
		for i, name := range value.Names {
			if i < len(values) {
				into[name.Name] = values[i]
			}
		}
	}
}

// concatenatedText joins the strings of a `+` chain, and names the literal
// nodes it consumed so they are not also read on their own.
//
// A name is resolved against the strings the package declares, because a query
// assembled from a table constant says the table's name just as plainly as one
// that spells it out. An operand that resolves to nothing — a column list
// variable, a fmt verb — is replaced by a SPACE rather than dropped, so two
// literals either side of it do not fuse into a word appearing in neither.
func concatenatedText(expr ast.Expr, known map[string]string) (string, []ast.Node) {
	var parts []ast.Node
	var text strings.Builder
	var walk func(ast.Expr)
	walk = func(node ast.Expr) {
		switch operand := node.(type) {
		case *ast.BinaryExpr:
			if operand.Op != token.ADD {
				text.WriteString(" ")
				return
			}
			walk(operand.X)
			walk(operand.Y)
		case *ast.BasicLit:
			value, isText := gatekit.LiteralText(operand)
			if !isText {
				text.WriteString(" ")
				return
			}
			parts = append(parts, ast.Node(operand))
			text.WriteString(value)
		default:
			if value, isKnown := gatekit.StringExpr(operand, known, gatekit.FoldStrict); isKnown {
				text.WriteString(value)
				return
			}
			text.WriteString(" ")
		}
	}
	walk(expr)
	if len(parts) == 0 {
		return "", nil
	}
	return text.String(), parts
}
