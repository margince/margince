// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// The check digest of every shipped site, derived from the source. It covers
// the case's Evaluate method and the functions, values and types it names in
// this module. A method call cannot be resolved without types, so the walk takes
// every method of that name on a type it reached. A reached type counts by its
// own declaration. The types of its fields count only where judging code names
// them, so a field added to a store a case holds moves no digest.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
)

var updateSiteChecks = flag.Bool("update-site-checks", false,
	"rewrite testdata/site_checks.golden after a site's check changed; its records then read stale")

const (
	siteChecksPath = "testdata/site_checks.golden"
	// modulePath is this module's import path, and backendRoot the directory it
	// names as this package reaches it.
	modulePath  = "github.com/margince/margince/backend"
	backendRoot = "../../.."
)

// TestEverySiteCheckIsStamped holds the golden to the source of every shipped
// site's check. Editing what decides right from wrong moves that site's stamp,
// and its records read stale.
func TestEverySiteCheckIsStamped(t *testing.T) {
	census, err := compose.NewTaskCensus()
	if err != nil {
		t.Fatalf("building the census: %v", err)
	}
	corpus, err := LoadCorpus("corpus", census)
	if err != nil {
		t.Fatalf("loading the corpus: %v", err)
	}
	index := newSourceIndex(t)
	var lines []string
	for _, site := range census.All() {
		key := string(site.Task) + "/" + site.Variant
		root := caseTypeOf(t, census, corpus, site.Task, site.Variant)
		if root == nil {
			t.Errorf("site %s has no scenario to prepare its case from, so its check cannot be found", key)
			continue
		}
		lines = append(lines, key+" "+index.checkDigest(t, root))
	}
	slices.Sort(lines)
	want := strings.Join(lines, "\n") + "\n"
	if *updateSiteChecks {
		if err := os.WriteFile(siteChecksPath, []byte(want), 0o600); err != nil {
			t.Fatalf("writing %s: %v", siteChecksPath, err)
		}
		return
	}
	if want != siteChecksGolden {
		t.Errorf("a site's check changed, or a site was added or removed, and %s does not say so; "+
			"run this test with -update-site-checks, which turns that site's records stale:\n%s",
			siteChecksPath, firstLineDiff(siteChecksGolden, want))
	}
}

// caseTypeOf prepares site's case from its first scenario and answers the
// concrete type whose Evaluate grades it.
func caseTypeOf(t *testing.T, census *aitasks.Registry, corpus []Scenario, task ai.Task, variant string) reflect.Type {
	t.Helper()
	for _, sc := range corpus {
		if sc.Task != string(task) || sc.Site != variant {
			continue
		}
		factory, ok := census.CaseFor(task, variant)
		if !ok {
			t.Fatalf("site %s/%s binds no case", task, variant)
		}
		prepared, err := factory.Prepare(json.RawMessage(sc.Fixture), json.RawMessage(sc.Expect.Answer))
		if err != nil {
			t.Fatalf("preparing %s: %v", sc.Name, err)
		}
		return reflect.TypeOf(prepared)
	}
	return nil
}

func firstLineDiff(got, want string) string {
	gotLines, wantLines := strings.Split(got, "\n"), strings.Split(want, "\n")
	for i := range max(len(gotLines), len(wantLines)) {
		var g, w string
		if i < len(gotLines) {
			g = gotLines[i]
		}
		if i < len(wantLines) {
			w = wantLines[i]
		}
		if g != w {
			return fmt.Sprintf("  committed: %s\n  derived:   %s", g, w)
		}
	}
	return ""
}

// sourcePackage is one module package parsed without comments.
type sourcePackage struct {
	path  string
	files []*ast.File
	// funcs and values are the top-level declarations by name; methods are
	// every method by name, whatever its receiver.
	funcs   map[string]*ast.FuncDecl
	values  map[string]ast.Node
	methods map[string][]*ast.FuncDecl
	// fileOf maps a declaration to its file, whose imports resolve it.
	fileOf map[ast.Node]*ast.File
}

type sourceIndex struct {
	fset *token.FileSet
	pkgs map[string]*sourcePackage
}

func newSourceIndex(t *testing.T) *sourceIndex {
	t.Helper()
	return &sourceIndex{fset: token.NewFileSet(), pkgs: map[string]*sourcePackage{}}
}

// load parses a module package once; a path outside the module answers nil.
func (x *sourceIndex) load(t *testing.T, path string) *sourcePackage {
	t.Helper()
	if pkg, seen := x.pkgs[path]; seen {
		return pkg
	}
	rel, inModule := strings.CutPrefix(path, modulePath+"/")
	if !inModule {
		x.pkgs[path] = nil
		return nil
	}
	dir := filepath.Join(backendRoot, filepath.FromSlash(rel))
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading package %s: %v", path, err)
	}
	pkg := &sourcePackage{
		path: path, funcs: map[string]*ast.FuncDecl{}, values: map[string]ast.Node{},
		methods: map[string][]*ast.FuncDecl{}, fileOf: map[ast.Node]*ast.File{},
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(x.fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		pkg.files = append(pkg.files, file)
		indexDecls(pkg, file)
	}
	x.pkgs[path] = pkg
	return pkg
}

func indexDecls(pkg *sourcePackage, file *ast.File) {
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			pkg.fileOf[d] = file
			if d.Recv == nil {
				pkg.funcs[d.Name.Name] = d
			} else {
				pkg.methods[d.Name.Name] = append(pkg.methods[d.Name.Name], d)
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				pkg.fileOf[spec] = file
				switch s := spec.(type) {
				case *ast.ValueSpec:
					for _, ident := range s.Names {
						pkg.values[ident.Name] = s
					}
				case *ast.TypeSpec:
					pkg.values[s.Name.Name] = s
				}
			}
		}
	}
}

// checkDigest is the digest of every declaration root's Evaluate reaches.
func (x *sourceIndex) checkDigest(t *testing.T, root reflect.Type) string {
	t.Helper()
	named := root
	if named.Kind() == reflect.Pointer {
		named = named.Elem()
	}
	pkg := x.load(t, named.PkgPath())
	if pkg == nil {
		t.Fatalf("case type %s is outside the module", named)
	}
	return x.digestOf(t, pkg, named.Name())
}

// digestOf is the digest of what the Evaluate of typeName in pkg reaches.
func (x *sourceIndex) digestOf(t *testing.T, pkg *sourcePackage, typeName string) string {
	t.Helper()
	walk := checkWalk{
		index: x, seen: map[ast.Node]string{}, types: map[string]bool{}, selected: map[string]bool{},
		fieldTypes: map[string][]string{}, fieldCalls: map[string]map[string]bool{},
	}
	for _, method := range pkg.methods["Evaluate"] {
		if receiverName(method) == typeName {
			walk.visit(t, pkg, method)
		}
	}
	if len(walk.seen) == 0 {
		t.Fatalf("case type %s declares no Evaluate method in %s", typeName, pkg.path)
	}
	walk.visitFuncFields(t, pkg, typeName)
	walk.resolveMethods(t)
	var material []string
	for node, id := range walk.seen {
		var src bytes.Buffer
		if err := format.Node(&src, x.fset, node); err != nil {
			t.Fatalf("formatting %s: %v", id, err)
		}
		material = append(material, id+"\n"+src.String())
	}
	slices.Sort(material)
	sum := sha256.Sum256([]byte(strings.Join(material, "\n")))
	return hex.EncodeToString(sum[:])
}

func receiverName(method *ast.FuncDecl) string {
	expr := method.Recv.List[0].Type
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.IndexExpr:
		if ident, ok := e.X.(*ast.Ident); ok {
			return ident.Name
		}
	}
	return ""
}

// checkWalk collects the declarations a check reaches, each with its id: the
// types it reached, by "package.Type", and every name it selected on a value.
type checkWalk struct {
	index *sourceIndex
	seen  map[ast.Node]string
	types map[string]bool
	// fieldTypes are the types of the fields of reached types, by field name, as
	// "package.Type"; an embedded field is named by its type. fieldCalls are the
	// methods judging code calls through a field, as in c.cache.accept(got). A
	// field type counts only through the methods called on that field.
	fieldTypes map[string][]string
	fieldCalls map[string]map[string]bool
	selected   map[string]bool
}

func (w *checkWalk) visit(t *testing.T, pkg *sourcePackage, node ast.Node) {
	t.Helper()
	if _, done := w.seen[node]; done {
		return
	}
	w.seen[node] = pkg.path + "." + declName(node)
	switch d := node.(type) {
	case *ast.TypeSpec:
		// The declaration is hashed. Its field types are kept as receivers a
		// selected method may be called on, and are not hashed for themselves.
		w.types[pkg.path+"."+d.Name.Name] = true
		w.noteFieldTypes(t, pkg, d)
		return
	case *ast.FuncDecl:
		if d.Recv != nil {
			w.visitName(t, pkg, receiverName(d))
		}
	}
	file := pkg.fileOf[node]
	ast.Inspect(node, func(n ast.Node) bool {
		switch e := n.(type) {
		case *ast.CallExpr:
			w.noteFieldCall(e)
		case *ast.SelectorExpr:
			if ident, ok := e.X.(*ast.Ident); ok {
				if imported := importPath(file, ident.Name); imported != "" {
					if other := w.index.load(t, imported); other != nil {
						w.visitName(t, other, e.Sel.Name)
					}
					return false
				}
			}
			w.selected[e.Sel.Name] = true
		case *ast.Ident:
			w.visitName(t, pkg, e.Name)
		}
		return true
	})
}

// noteFieldTypes keeps the named type of each field of spec, through a pointer.
// It loads the package that declares the type, so its methods can be resolved.
func (w *checkWalk) noteFieldTypes(t *testing.T, pkg *sourcePackage, spec *ast.TypeSpec) {
	t.Helper()
	fields, ok := spec.Type.(*ast.StructType)
	if !ok {
		return
	}
	for _, field := range fields.Fields.List {
		expr := field.Type
		if star, isPointer := expr.(*ast.StarExpr); isPointer {
			expr = star.X
		}
		var named, last string
		switch typed := expr.(type) {
		case *ast.Ident:
			named, last = pkg.path+"."+typed.Name, typed.Name
		case *ast.SelectorExpr:
			ident, isIdent := typed.X.(*ast.Ident)
			imported := ""
			if isIdent {
				imported = importPath(pkg.fileOf[spec], ident.Name)
			}
			if imported == "" || w.index.load(t, imported) == nil {
				continue
			}
			named, last = imported+"."+typed.Sel.Name, typed.Sel.Name
		default:
			continue
		}
		names := []string{last}
		if len(field.Names) > 0 {
			names = names[:0]
			for _, ident := range field.Names {
				names = append(names, ident.Name)
			}
		}
		for _, name := range names {
			w.fieldTypes[name] = append(w.fieldTypes[name], named)
		}
	}
}

// noteFieldCall keeps the method a call selects on a field, as accept in
// c.cache.accept(got), and the field it goes through.
func (w *checkWalk) noteFieldCall(call *ast.CallExpr) {
	method, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}
	field, ok := method.X.(*ast.SelectorExpr)
	if !ok {
		return
	}
	if w.fieldCalls[field.Sel.Name] == nil {
		w.fieldCalls[field.Sel.Name] = map[string]bool{}
	}
	w.fieldCalls[field.Sel.Name][method.Sel.Name] = true
}

// calledThroughAField answers whether judging code calls method on a field of
// a reached type whose type is receiver.
func (w *checkWalk) calledThroughAField(receiver, method string) bool {
	for field, methods := range w.fieldCalls {
		if methods[method] && slices.Contains(w.fieldTypes[field], receiver) {
			return true
		}
	}
	return false
}

// visitName takes the top-level function, variable, constant or type name
// declares in pkg.
func (w *checkWalk) visitName(t *testing.T, pkg *sourcePackage, name string) {
	t.Helper()
	// The blank name declares nothing a check can call.
	if name == "_" {
		return
	}
	if fn, ok := pkg.funcs[name]; ok {
		w.visit(t, pkg, fn)
	}
	if value, ok := pkg.values[name]; ok {
		w.visit(t, pkg, value)
	}
}

// resolveMethods takes every method a selected name can call: one of that
// name on a type the walk reached, until no new one appears.
func (w *checkWalk) resolveMethods(t *testing.T) {
	t.Helper()
	for grown := true; grown; {
		before := len(w.seen)
		for _, path := range slices.Sorted(maps.Keys(w.index.pkgs)) {
			pkg := w.index.pkgs[path]
			if pkg == nil {
				continue
			}
			for name := range w.selected {
				for _, method := range pkg.methods[name] {
					if w.types[pkg.path+"."+receiverName(method)] || w.calledThroughAField(pkg.path+"."+receiverName(method), name) {
						w.visit(t, pkg, method)
					}
				}
			}
		}
		grown = len(w.seen) > before
	}
}

// visitFuncFields takes what each composite literal of typeName assigns to a
// field: a validator a case's Prepare hands it is called through that field.
func (w *checkWalk) visitFuncFields(t *testing.T, pkg *sourcePackage, typeName string) {
	t.Helper()
	for _, file := range pkg.files {
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			if ident, ok := lit.Type.(*ast.Ident); !ok || ident.Name != typeName {
				return true
			}
			for _, elt := range lit.Elts {
				if kv, ok := elt.(*ast.KeyValueExpr); ok {
					if _, isFunc := kv.Value.(*ast.FuncLit); isFunc {
						w.seenLiteral(t, pkg, file, kv.Value)
					} else {
						w.visitExpr(t, pkg, file, kv.Value)
					}
				}
			}
			return true
		})
	}
}

// visitExpr takes the declarations a field's value names.
func (w *checkWalk) visitExpr(t *testing.T, pkg *sourcePackage, file *ast.File, expr ast.Expr) {
	t.Helper()
	ast.Inspect(expr, func(n ast.Node) bool {
		switch e := n.(type) {
		case *ast.SelectorExpr:
			if ident, ok := e.X.(*ast.Ident); ok {
				if imported := importPath(file, ident.Name); imported != "" {
					if other := w.index.load(t, imported); other != nil {
						w.visitName(t, other, e.Sel.Name)
					}
					return false
				}
			}
		case *ast.Ident:
			w.visitName(t, pkg, e.Name)
		}
		return true
	})
}

// seenLiteral takes a function literal assigned to a field as a declaration of
// its own, and what it reaches.
func (w *checkWalk) seenLiteral(t *testing.T, pkg *sourcePackage, file *ast.File, lit ast.Expr) {
	t.Helper()
	if _, done := w.seen[lit]; done {
		return
	}
	pos := w.index.fset.Position(lit.Pos())
	w.seen[lit] = fmt.Sprintf("%s.literal@%s", pkg.path, filepath.Base(pos.Filename))
	w.visitExpr(t, pkg, file, lit)
}

func declName(node ast.Node) string {
	switch d := node.(type) {
	case *ast.FuncDecl:
		if d.Recv != nil {
			return receiverName(d) + "." + d.Name.Name
		}
		return d.Name.Name
	case *ast.ValueSpec:
		return d.Names[0].Name
	case *ast.TypeSpec:
		return d.Name.Name
	}
	return fmt.Sprintf("%T", node)
}

// importPath is the path name imports in file, or empty when it names none.
func importPath(file *ast.File, name string) string {
	if file == nil {
		return ""
	}
	for _, spec := range file.Imports {
		path := strings.Trim(spec.Path.Value, `"`)
		alias := path[strings.LastIndex(path, "/")+1:]
		if spec.Name != nil {
			alias = spec.Name.Name
		}
		if alias == name {
			return path
		}
	}
	return ""
}

// A site's check digest is part of the grader third of each of its scenarios'
// stamps, so a record its old check graded reads stale.
func TestAChangedSiteCheckMovesOnlyTheGraderThird(t *testing.T) {
	sc := testScenario("basic", wideBands)
	stamp := func() string {
		stamps, err := ScenarioStamps(wsContext(t), []Scenario{sc}, testCensus(t))
		if err != nil {
			t.Fatalf("stamping: %v", err)
		}
		return stamps[sc.Name]
	}
	key := sc.Task + "/" + sc.Site
	before := stamp()
	siteChecks[key] = "a-check"
	t.Cleanup(func() { delete(siteChecks, key) })
	after := stamp()
	if parts := splitStampChange(before, after); !parts.Grader || parts.Case || parts.Prompt {
		t.Errorf("a new check moved %+v, want the grader third alone", parts)
	}
}

// fixtureDigest is the check digest of judgeCase in a package built from src.
func fixtureDigest(t *testing.T, src string) string {
	t.Helper()
	x := newSourceIndex(t)
	file, err := parser.ParseFile(x.fset, "fixture.go", "package fixture\n"+src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing the fixture: %v", err)
	}
	pkg := &sourcePackage{
		path: modulePath + "/fixture", files: []*ast.File{file}, funcs: map[string]*ast.FuncDecl{},
		values: map[string]ast.Node{}, methods: map[string][]*ast.FuncDecl{}, fileOf: map[ast.Node]*ast.File{},
	}
	indexDecls(pkg, file)
	x.pkgs[pkg.path] = pkg
	return x.digestOf(t, pkg, "judgeCase")
}

// A case that holds a store in a field and judges through a helper. A blank
// declaration names the store too, the way an interface check does, and
// Evaluate discards a result with the same blank name.
const judgeFixture = `
type store struct{ rows int }
var _ = store{}
type judgeCase struct {
	want  int
	cache store
}
func (c judgeCase) Evaluate(got int) bool {
	_, same := sameAnswer(c.want, got)
	return same
}
func sameAnswer(want, got int) (int, bool) { return got, want == got }
`

func TestAnEditToTheJudgingCodeMovesTheSiteCheck(t *testing.T) {
	before := fixtureDigest(t, judgeFixture)
	edited := strings.Replace(judgeFixture, "return got, want == got", "return got, want <= got", 1)
	if fixtureDigest(t, edited) == before {
		t.Error("an edit to the helper Evaluate judges through left the digest unchanged")
	}
	evaluate := strings.Replace(judgeFixture, "sameAnswer(c.want, got)", "sameAnswer(got, c.want)", 1)
	if fixtureDigest(t, evaluate) == before {
		t.Error("an edit to Evaluate left the digest unchanged")
	}
}

func TestAFieldOnAStoreTheCaseHoldsMovesNoSiteCheck(t *testing.T) {
	before := fixtureDigest(t, judgeFixture)
	edited := strings.Replace(judgeFixture, "type store struct{ rows int }", "type store struct{ rows, pages int }", 1)
	if fixtureDigest(t, edited) != before {
		t.Error("a field added to a store the case only holds moved the digest")
	}
}

// A case that judges through a method of a store it holds in a field.
const fieldMethodFixture = `
type store struct{ rows int }
func (s store) accept(got int) bool { return got > s.rows }
type judgeCase struct{ cache store }
func (c judgeCase) Evaluate(got int) bool { return c.cache.accept(got) }
`

func TestAnEditToAMethodTheCaseCallsThroughAFieldMovesTheSiteCheck(t *testing.T) {
	before := fixtureDigest(t, fieldMethodFixture)
	edited := strings.Replace(fieldMethodFixture, "return got > s.rows", "return got >= s.rows", 1)
	if fixtureDigest(t, edited) == before {
		t.Error("an edit to the field's method Evaluate judges through left the digest unchanged")
	}
}
