// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every too_long field refusal states the limit it refused at, so the screen
// can say "at most 2,000 characters" in the reader's language.
//
// The corpus is every "too_long" in the module: the literal and each use of a
// const holding it. Calls to httperr.TooLong and RequireWithin count toward
// the floor, since they no longer spell the code.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const tooLongCode = "too_long"

// tooLongNotAFieldRefusal names the consts that hold "too_long" for something
// other than a field refusal, keyed by import path and const name.
var tooLongNotAFieldRefusal = gatekit.Waive(map[string]string{
	modulePath + "/internal/modules/activities.TranscriptFailureTooLong": "a transcript's failure state, " +
		"stored on the reading and rendered by its own screen copy, not a refused input",
	modulePath + "/internal/compose/readerrendering.codeTooLong": "the reader-rendering refusal has its " +
		"own code set, localized in design-system/readerrendering.tsx",
})

// tooLongPackage is one parsed directory of the module.
type tooLongPackage struct {
	importPath string
	files      map[string]*ast.File
}

// fieldMaxShape is what a type's FieldMax method answers: a field of the
// receiver it returns, or zero when it can only answer 0.
type fieldMaxShape struct {
	field string
	zero  bool
}

// typedLiteral is one composite literal of a named type, where it stands.
type typedLiteral struct {
	at  string
	lit *ast.CompositeLit
}

// tooLongCensus is what the walk found across the module.
type tooLongCensus struct {
	t                   *testing.T
	limits              map[string]fieldMaxShape
	literals            map[string][]typedLiteral
	consts              map[string]bool
	sites, builderCalls int
	failures            []string
}

func TestEveryTooLongRefusalStatesItsLimit(t *testing.T) {
	t.Parallel()
	census := censusOfTooLong(t, parseModulePackages(t))
	for _, failure := range census.failures {
		t.Error(failure)
	}
	tooLongNotAFieldRefusal.AssertAllMatched(t)
	if census.sites < 15 || census.sites+census.builderCalls < 30 {
		t.Errorf("found %d \"too_long\" sites and %d TooLong/RequireWithin calls; the tree has more than 15 "+
			"and 30 in all, so the walk is reading too little", census.sites, census.builderCalls)
	}
}

// TestTheTooLongCensusSeesEveryShapeOfAMissingLimit plants each shape the
// walk must read, so a census that stops seeing one fails here first.
func TestTheTooLongCensusSeesEveryShapeOfAMissingLimit(t *testing.T) {
	t.Parallel()
	census := censusOfTooLong(t, plantedTooLongPackages(t, map[string]string{
		"example.test/codes": `package codes
const Code = "too_long"`,
		"example.test/alias": `package alias
import "example.test/codes"
const local = codes.Code
const again = local
func a() string { return again }`,
		"example.test/dotted": `package dotted
import . "example.test/codes"
func b() string { return Code }`,
		"example.test/typed": `package typed
type refusal struct{ Field, Code string; Max int }
func (e *refusal) FieldMax() int { return e.Max }
type zeroRefusal struct{ Code string }
func (e *zeroRefusal) FieldMax() int { return 0 }
type fixed struct{ Max int }
func (e *fixed) FieldFault() (string, string, string) { return "f", "too_long", "" }
func (e *fixed) FieldMax() int { return e.Max }
var sound = &refusal{Field: "f", Code: "too_long", Max: 10}
var zeroMax = &refusal{Field: "f", Code: "too_long", Max: 0}
var noMax = &refusal{Field: "f", Code: "too_long"}
var zeroMethod = &zeroRefusal{Code: "too_long"}
var unset = &fixed{}`,
	}))
	want := []string{
		"alias.go:5: this too_long reaches the wire with no limit",
		"dotted.go:3: this too_long reaches the wire with no limit",
		"typed.go:10: example.test/typed.refusal refuses with too_long and this literal sets no positive Max",
		"typed.go:11: example.test/typed.refusal refuses with too_long and this literal sets no positive Max",
		"typed.go:12: example.test/typed.zeroRefusal.FieldMax answers 0",
		"typed.go:7: example.test/typed.fixed.FieldMax answers its Max, and these literals set none: typed.go:13",
	}
	if len(census.failures) != len(want) {
		t.Errorf("planted census found %d failures, want %d:\n%s", len(census.failures), len(want),
			strings.Join(census.failures, "\n"))
	}
	for _, prefix := range want {
		if !slices.ContainsFunc(census.failures, func(f string) bool { return strings.HasPrefix(f, prefix) }) {
			t.Errorf("planted census missed %q; found:\n%s", prefix, strings.Join(census.failures, "\n"))
		}
	}
}

// plantedTooLongPackages parses one file per import path, named for its
// last segment.
func plantedTooLongPackages(t *testing.T, sources map[string]string) map[string]*tooLongPackage {
	t.Helper()
	packages := map[string]*tooLongPackage{}
	for importPath, src := range sources {
		name := path.Base(importPath) + ".go"
		file, err := parser.ParseFile(gatekit.SourceFileSet(), name, src, 0)
		if err != nil {
			t.Fatalf("parsing planted %s: %v", importPath, err)
		}
		packages[importPath] = &tooLongPackage{importPath: importPath, files: map[string]*ast.File{name: file}}
	}
	return packages
}

// censusOfTooLong reads every too_long site of packages.
func censusOfTooLong(t *testing.T, packages map[string]*tooLongPackage) tooLongCensus {
	census := tooLongCensus{
		t:        t,
		limits:   fieldMaxShapes(packages),
		literals: typedLiterals(packages),
		consts:   tooLongConsts(packages),
	}
	for _, pkg := range packages {
		for name, file := range pkg.files {
			census.read(pkg, name, file)
		}
	}
	return census
}

// parseModulePackages parses every hand-written Go file of the backend module,
// grouped by directory.
func parseModulePackages(t *testing.T) map[string]*tooLongPackage {
	t.Helper()
	packages := map[string]*tooLongPackage{}
	err := gatekit.WalkDir(".", func(file string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "testdata" || entry.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(file, ".go") || strings.HasSuffix(file, "_test.go") || strings.HasSuffix(file, "_gen.go") {
			return nil
		}
		parsed, err := gatekit.ParseFile(file, 0)
		if err != nil {
			return err
		}
		dir := filepath.ToSlash(filepath.Dir(file))
		if packages[dir] == nil {
			packages[dir] = &tooLongPackage{importPath: path.Join(modulePath, dir), files: map[string]*ast.File{}}
		}
		packages[dir].files[filepath.ToSlash(file)] = parsed
		return nil
	})
	if err != nil {
		t.Fatalf("walking the backend module: %v", err)
	}
	return packages
}

// fieldMaxShapes answers, for every "importPath.Type" with a FieldMax method,
// what that method returns.
func fieldMaxShapes(packages map[string]*tooLongPackage) map[string]fieldMaxShape {
	out := map[string]fieldMaxShape{}
	for _, pkg := range packages {
		for _, file := range pkg.files {
			for _, decl := range file.Decls {
				if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "FieldMax" && fn.Recv != nil {
					out[pkg.importPath+"."+receiverTypeName(fn)] = shapeOfFieldMax(fn)
				}
			}
		}
	}
	return out
}

// shapeOfFieldMax reads a one-statement FieldMax: `return 0` can name no
// limit, and `return e.Max` names the limit only where Max is set.
func shapeOfFieldMax(fn *ast.FuncDecl) fieldMaxShape {
	if fn.Body == nil || len(fn.Body.List) != 1 {
		return fieldMaxShape{}
	}
	ret, ok := fn.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return fieldMaxShape{}
	}
	if isNonPositiveLiteral(ret.Results[0]) {
		return fieldMaxShape{zero: true}
	}
	sel, ok := ret.Results[0].(*ast.SelectorExpr)
	if !ok {
		return fieldMaxShape{}
	}
	if recv, ok := sel.X.(*ast.Ident); ok && len(fn.Recv.List[0].Names) == 1 && fn.Recv.List[0].Names[0].Name == recv.Name {
		return fieldMaxShape{field: sel.Sel.Name}
	}
	return fieldMaxShape{}
}

// typedLiterals indexes every composite literal of a named type by that type.
func typedLiterals(packages map[string]*tooLongPackage) map[string][]typedLiteral {
	out := map[string][]typedLiteral{}
	for _, pkg := range packages {
		for name, file := range pkg.files {
			imports, _ := importAliases(file)
			ast.Inspect(file, func(n ast.Node) bool {
				if lit, ok := n.(*ast.CompositeLit); ok && lit.Type != nil {
					key := compositeTypeKey(lit, pkg.importPath, imports)
					at := name + ":" + strconv.Itoa(gatekit.SourceFileSet().Position(lit.Pos()).Line)
					out[key] = append(out[key], typedLiteral{at: at, lit: lit})
				}
				return true
			})
		}
	}
	return out
}

// tooLongConsts answers "importPath.Name" for every const that holds
// "too_long": the literal, or another such const by any spelling.
func tooLongConsts(packages map[string]*tooLongPackage) map[string]bool {
	out := map[string]bool{}
	for grew := true; grew; {
		grew = false
		for _, pkg := range packages {
			for _, file := range pkg.files {
				imports, dots := importAliases(file)
				ast.Inspect(file, func(n ast.Node) bool {
					spec, ok := n.(*ast.ValueSpec)
					if !ok {
						return true
					}
					for i, value := range spec.Values {
						if i >= len(spec.Names) {
							break
						}
						key := pkg.importPath + "." + spec.Names[i].Name
						if !out[key] && holdsTooLong(value, pkg.importPath, imports, dots, out) {
							out[key], grew = true, true
						}
					}
					return false
				})
			}
		}
	}
	return out
}

// holdsTooLong reports whether a const's value is "too_long" or names a const
// already known to hold it.
func holdsTooLong(value ast.Expr, self string, imports map[string]string, dots []string, consts map[string]bool) bool {
	switch node := value.(type) {
	case *ast.BasicLit:
		return isTooLongLiteral(node)
	case *ast.SelectorExpr:
		qualifier, ok := node.X.(*ast.Ident)
		return ok && consts[imports[qualifier.Name]+"."+node.Sel.Name]
	case *ast.Ident:
		return identConst(node.Name, self, dots, consts) != ""
	}
	return false
}

// identConst answers the key of a bare name that is a too_long const of this
// package or of a dot import, or "".
func identConst(name, self string, dots []string, consts map[string]bool) string {
	for _, owner := range append([]string{self}, dots...) {
		if consts[owner+"."+name] {
			return owner + "." + name
		}
	}
	return ""
}

// read classifies every too_long site of one file.
func (c *tooLongCensus) read(pkg *tooLongPackage, name string, file *ast.File) {
	imports, dots := importAliases(file)
	var stack []ast.Node
	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		stack = append(stack, n)
		if call, ok := n.(*ast.CallExpr); ok && isHTTPErrLengthBuilder(call, imports) {
			c.builderCalls++
		}
		key, ok := tooLongReference(n, stack, pkg.importPath, imports, dots, c.consts)
		if !ok {
			return true
		}
		c.sites++
		reason := c.unlimited(stack, pkg.importPath, imports)
		if reason != "" && !tooLongNotAFieldRefusal.Waived(c.t, key) {
			pos := gatekit.SourceFileSet().Position(n.Pos())
			c.failures = append(c.failures, name+":"+strconv.Itoa(pos.Line)+": "+reason)
		}
		return true
	})
}

// tooLongReference answers the key of a too_long site at n: the literal, a
// bare const of this package, or a qualified const of another.
func tooLongReference(
	n ast.Node, stack []ast.Node, self string, imports map[string]string, dots []string, consts map[string]bool,
) (string, bool) {
	if len(stack) > 1 {
		if _, declaring := stack[len(stack)-2].(*ast.ValueSpec); declaring {
			return "", false
		}
	}
	switch node := n.(type) {
	case *ast.BasicLit:
		return self + "." + tooLongCode, isTooLongLiteral(node)
	case *ast.SelectorExpr:
		qualifier, ok := node.X.(*ast.Ident)
		if !ok || imports[qualifier.Name] == "" {
			return "", false
		}
		key := imports[qualifier.Name] + "." + node.Sel.Name
		return key, consts[key]
	case *ast.Ident:
		if len(stack) > 1 {
			if parent, ok := stack[len(stack)-2].(*ast.SelectorExpr); ok && parent.Sel == node {
				return "", false
			}
		}
		key := identConst(node.Name, self, dots, consts)
		return key, key != ""
	}
	return "", false
}

// unlimitedTooLong answers why the site at the top of stack carries no limit,
// or "" when it carries one.
func (c *tooLongCensus) unlimited(stack []ast.Node, self string, imports map[string]string) string {
	fn := enclosingFuncDecl(stack)
	if fn != nil && fn.Name.Name == "TooLong" && fn.Recv == nil && strings.HasSuffix(self, "/platform/httperr") {
		return ""
	}
	if lit, ok := codeOfCompositeLiteral(stack); ok {
		typeKey := compositeTypeKey(lit, self, imports)
		shape, limited := c.limits[typeKey]
		switch {
		case !limited:
			return typeKey + " refuses with too_long and has no FieldMax method; add one so the 422 carries max"
		case shape.zero:
			return typeKey + ".FieldMax answers 0, which states no limit; return the limit"
		case shape.field != "" && !setsPositive(lit, shape.field):
			return typeKey + " refuses with too_long and this literal sets no positive " + shape.field + "; set it to the limit"
		}
		return ""
	}
	if fn != nil && fn.Name.Name == "FieldFault" && fn.Recv != nil && inReturn(stack) {
		return c.unlimitedType(self + "." + receiverTypeName(fn))
	}
	return "this too_long reaches the wire with no limit; use httperr.TooLong or httperr.RequireWithin, " +
		"or a typed refusal with a FieldMax method"
}

// unlimitedType answers why a type whose FieldFault always says too_long can
// reach the wire with no limit, or "".
func (c *tooLongCensus) unlimitedType(typeKey string) string {
	shape, limited := c.limits[typeKey]
	switch {
	case !limited:
		return typeKey + ".FieldFault returns too_long and the type has no FieldMax method; add one"
	case shape.zero:
		return typeKey + ".FieldMax answers 0, which states no limit; return the limit"
	case shape.field == "":
		return ""
	}
	var unset []string
	for _, site := range c.literals[typeKey] {
		if !setsPositive(site.lit, shape.field) {
			unset = append(unset, site.at)
		}
	}
	if len(unset) == 0 {
		return ""
	}
	return typeKey + ".FieldMax answers its " + shape.field + ", and these literals set none: " + strings.Join(unset, ", ")
}

// codeOfCompositeLiteral answers the composite literal whose Code key the site is.
func codeOfCompositeLiteral(stack []ast.Node) (*ast.CompositeLit, bool) {
	if len(stack) < 3 {
		return nil, false
	}
	pair, ok := stack[len(stack)-2].(*ast.KeyValueExpr)
	if !ok || pair.Value != stack[len(stack)-1] {
		return nil, false
	}
	key, ok := pair.Key.(*ast.Ident)
	if !ok || !strings.EqualFold(key.Name, "code") {
		return nil, false
	}
	lit, ok := stack[len(stack)-3].(*ast.CompositeLit)
	return lit, ok
}

// setsPositive reports whether the literal sets field to something other than
// a literal zero or less.
func setsPositive(lit *ast.CompositeLit, field string) bool {
	for _, elt := range lit.Elts {
		if pair, ok := elt.(*ast.KeyValueExpr); ok {
			if key, ok := pair.Key.(*ast.Ident); ok && key.Name == field {
				return !isNonPositiveLiteral(pair.Value)
			}
		}
	}
	return false
}

func isNonPositiveLiteral(expr ast.Expr) bool {
	if unary, ok := expr.(*ast.UnaryExpr); ok && unary.Op == token.SUB {
		return true
	}
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.INT {
		return false
	}
	n, err := strconv.ParseInt(lit.Value, 0, 64)
	return err == nil && n <= 0
}

func compositeTypeKey(lit *ast.CompositeLit, self string, imports map[string]string) string {
	switch typ := lit.Type.(type) {
	case *ast.Ident:
		return self + "." + typ.Name
	case *ast.SelectorExpr:
		if qualifier, ok := typ.X.(*ast.Ident); ok {
			return imports[qualifier.Name] + "." + typ.Sel.Name
		}
	}
	return self + ".<unnamed>"
}

func enclosingFuncDecl(stack []ast.Node) *ast.FuncDecl {
	for _, node := range slices.Backward(stack) {
		if fn, ok := node.(*ast.FuncDecl); ok {
			return fn
		}
	}
	return nil
}

func inReturn(stack []ast.Node) bool {
	for _, node := range slices.Backward(stack) {
		switch node.(type) {
		case *ast.ReturnStmt:
			return true
		case *ast.FuncDecl, *ast.FuncLit:
			return false
		}
	}
	return false
}

// importAliases maps each import's name in the file to its path, and lists
// the paths imported with a dot.
func importAliases(file *ast.File) (map[string]string, []string) {
	out := map[string]string{}
	var dots []string
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		name := gatekit.DeclaredPackageName(path)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		if name == "." {
			dots = append(dots, path)
			continue
		}
		out[name] = path
	}
	return out, dots
}

func isHTTPErrLengthBuilder(call *ast.CallExpr, imports map[string]string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	qualifier, ok := sel.X.(*ast.Ident)
	return ok && imports[qualifier.Name] == httperrPath && (sel.Sel.Name == "TooLong" || sel.Sel.Name == "RequireWithin")
}

func isTooLongLiteral(expr ast.Node) bool {
	lit, ok := expr.(*ast.BasicLit)
	return ok && lit.Kind == token.STRING && lit.Value == strconv.Quote(tooLongCode)
}
