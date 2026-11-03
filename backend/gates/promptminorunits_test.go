// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

//gate:kind prohibition H2

package gates

// No struct that is JSON-encoded into a model prompt carries a money amount in
// minor units.
//
// A model reads 1850000 next to "EUR" as 1,850,000 euros. The grounding check
// then accepts the figure, because the same digits are in the source. The fix
// is to tag the integer `json:"-"` and write the amount through
// values.MajorUnits in a MarshalJSON.
//
// Roots: the value of every json.Marshal call in a non-test file under
// internal/ that imports the model port. The walk follows fields, pointers,
// slices, maps and type definitions across this module's packages.
//
// Blind spots (H2): a value typed only by a call, a range or a method ends
// the walk at that root. An interface field ends it at that field. The floor
// of known roots fails a resolver that stopped resolving.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	promptModulePath = "github.com/margince/margince/backend/"
	promptModelPort  = promptModulePath + "internal/shared/ports/model"
)

// promptTypeKey names a type declaration: the package directory and the name.
type promptTypeKey struct{ dir, name string }

// minorUnitSources parses package directories on demand and indexes their
// type declarations. A walk uses it to follow a field into another package.
type minorUnitSources struct {
	moduleRoot string
	parse      func(path string) (*ast.File, error)
	files      map[string][]*ast.File
	types      map[promptTypeKey]promptTypeDecl
	errs       []error
}

// promptTypeDecl is one type declaration and the file whose imports qualify it.
type promptTypeDecl struct {
	spec *ast.TypeSpec
	file *ast.File
	dir  string
}

func newMinorUnitSources(moduleRoot string, parse func(string) (*ast.File, error)) *minorUnitSources {
	return &minorUnitSources{moduleRoot: moduleRoot, parse: parse,
		files: map[string][]*ast.File{}, types: map[promptTypeKey]promptTypeDecl{}}
}

func (p *minorUnitSources) load(dir string) []*ast.File {
	if files, seen := p.files[dir]; seen {
		return files
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		p.errs = append(p.errs, err)
	}
	var files []*ast.File
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, parseErr := p.parse(filepath.Join(dir, name))
		if parseErr != nil {
			p.errs = append(p.errs, parseErr)
			continue
		}
		files = append(files, file)
		p.index(dir, file)
	}
	p.files[dir] = files
	return files
}

func (p *minorUnitSources) index(dir string, file *ast.File) {
	for _, decl := range file.Decls {
		gen, isGen := decl.(*ast.GenDecl)
		if !isGen || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			if typeSpec, isType := spec.(*ast.TypeSpec); isType {
				p.types[promptTypeKey{dir, typeSpec.Name.Name}] = promptTypeDecl{spec: typeSpec, file: file, dir: dir}
			}
		}
	}
}

// lookup finds a declared type by name, in dir itself or, for a qualified
// name, in the module package the file imports under that qualifier.
func (p *minorUnitSources) lookup(file *ast.File, dir string, expr ast.Expr) (promptTypeDecl, bool) {
	switch typ := expr.(type) {
	case *ast.Ident:
		p.load(dir)
		decl, found := p.types[promptTypeKey{dir, typ.Name}]
		return decl, found
	case *ast.SelectorExpr:
		qualifier, isIdent := typ.X.(*ast.Ident)
		if !isIdent {
			return promptTypeDecl{}, false
		}
		imported := importDirFor(file, qualifier.Name)
		if imported == "" {
			return promptTypeDecl{}, false
		}
		target := filepath.Join(p.moduleRoot, imported)
		p.load(target)
		decl, found := p.types[promptTypeKey{target, typ.Sel.Name}]
		return decl, found
	}
	return promptTypeDecl{}, false
}

// importDirFor turns a file's qualifier into the module-relative directory it
// imports, or "" for a package outside this module.
func importDirFor(file *ast.File, qualifier string) string {
	for _, spec := range file.Imports {
		importPath := strings.Trim(spec.Path.Value, `"`)
		name := filepath.Base(importPath)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		if name == qualifier && strings.HasPrefix(importPath, promptModulePath) {
			return strings.TrimPrefix(importPath, promptModulePath)
		}
	}
	return ""
}

// promptWalk follows one root's type through its fields and records every
// minor-unit field that would be encoded.
type promptWalk struct {
	packages *minorUnitSources
	seen     map[promptTypeKey]bool
	findings map[string]bool
}

func (w *promptWalk) typeExpr(file *ast.File, dir string, expr ast.Expr) {
	switch typ := expr.(type) {
	case *ast.StarExpr:
		w.typeExpr(file, dir, typ.X)
	case *ast.ArrayType:
		w.typeExpr(file, dir, typ.Elt)
	case *ast.MapType:
		w.typeExpr(file, dir, typ.Value)
	case *ast.StructType:
		w.structFields(file, dir, "struct literal", typ)
	case *ast.Ident, *ast.SelectorExpr:
		decl, found := w.packages.lookup(file, dir, typ)
		if !found {
			return
		}
		key := promptTypeKey{decl.dir, decl.spec.Name.Name}
		if w.seen[key] {
			return
		}
		w.seen[key] = true
		if fields, isStruct := decl.spec.Type.(*ast.StructType); isStruct {
			w.structFields(decl.file, decl.dir, decl.dir+"."+decl.spec.Name.Name, fields)
			return
		}
		w.typeExpr(decl.file, decl.dir, decl.spec.Type)
	}
}

func (w *promptWalk) structFields(file *ast.File, dir, owner string, fields *ast.StructType) {
	for _, field := range fields.Fields.List {
		key := minorUnitJSONKey(field)
		if key == "-" {
			continue
		}
		for _, name := range field.Names {
			if name.IsExported() && isInteger(field.Type) &&
				(strings.HasSuffix(name.Name, "Minor") || strings.HasSuffix(key, "_minor")) {
				w.findings[owner+"."+name.Name] = true
			}
		}
		w.typeExpr(file, dir, field.Type)
	}
}

func minorUnitJSONKey(field *ast.Field) string {
	if field.Tag == nil {
		return ""
	}
	tag, err := strconv.Unquote(field.Tag.Value)
	if err != nil {
		return ""
	}
	key, _, _ := strings.Cut(reflect.StructTag(tag).Get("json"), ",")
	return key
}

func isInteger(expr ast.Expr) bool {
	if star, isStar := expr.(*ast.StarExpr); isStar {
		expr = star.X
	}
	ident, isIdent := expr.(*ast.Ident)
	return isIdent && strings.TrimPrefix(strings.TrimRight(ident.Name, "0123456789"), "u") == "int"
}

// promptRootTypes returns, per file, the type of each value passed to json.Marshal
// that the gate could type from syntax, keyed for the census floor.
func promptRootTypes(file *ast.File) map[ast.Expr]ast.Expr {
	jsonName, _ := gatekit.ImportedAs(file, "encoding/json")
	roots := map[ast.Expr]ast.Expr{}
	if jsonName == "" {
		return roots
	}
	for _, decl := range file.Decls {
		fn, isFunc := decl.(*ast.FuncDecl)
		if !isFunc || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, isCall := n.(*ast.CallExpr)
			if !isCall || len(call.Args) != 1 || !isQualifiedCall(call, jsonName, "Marshal") {
				return true
			}
			if typ := typeOfValue(fn, call.Args[0]); typ != nil {
				roots[call.Args[0]] = typ
			}
			return true
		})
	}
	return roots
}

func isQualifiedCall(call *ast.CallExpr, qualifier, name string) bool {
	sel, isSel := call.Fun.(*ast.SelectorExpr)
	if !isSel || sel.Sel.Name != name {
		return false
	}
	pkg, isIdent := sel.X.(*ast.Ident)
	return isIdent && pkg.Name == qualifier
}

// typeOfValue reads the type of a marshalled value from syntax. It knows a
// composite literal and a name declared as a parameter, receiver, `var x T` or
// `x := T{…}` in the same function. Anything else answers nil.
func typeOfValue(fn *ast.FuncDecl, value ast.Expr) ast.Expr {
	switch v := value.(type) {
	case *ast.CompositeLit:
		return v.Type
	case *ast.UnaryExpr:
		if v.Op == token.AND {
			return typeOfValue(fn, v.X)
		}
	case *ast.Ident:
		return declaredType(fn, v.Name)
	}
	return nil
}

func declaredType(fn *ast.FuncDecl, name string) ast.Expr {
	for _, list := range []*ast.FieldList{fn.Recv, fn.Type.Params} {
		if list == nil {
			continue
		}
		for _, field := range list.List {
			for _, ident := range field.Names {
				if ident.Name == name {
					return field.Type
				}
			}
		}
	}
	var found ast.Expr
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.ValueSpec:
			for _, ident := range stmt.Names {
				if ident.Name == name && stmt.Type != nil {
					found = stmt.Type
				}
			}
		case *ast.AssignStmt:
			for i, lhs := range stmt.Lhs {
				if ident, isIdent := lhs.(*ast.Ident); isIdent && ident.Name == name && i < len(stmt.Rhs) {
					if typ := typeOfValue(fn, stmt.Rhs[i]); typ != nil && found == nil {
						found = typ
					}
				}
			}
		}
		return found == nil
	})
	return found
}

// promptMinorUnitFindings walks every prompt root under moduleRoot/internal
// and returns the minor-unit fields they would encode, and the types it
// resolved as roots.
func promptMinorUnitFindings(t *testing.T, moduleRoot string, parse func(string) (*ast.File, error)) (findings, resolved []string) {
	t.Helper()
	root := filepath.Join(moduleRoot, "internal")
	packages := newMinorUnitSources(moduleRoot, parse)
	walk := &promptWalk{packages: packages, findings: map[string]bool{}}
	rootNames := map[string]bool{}
	var dirs []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err == nil && entry.IsDir() {
			dirs = append(dirs, path)
		}
		return err
	})
	if err != nil {
		t.Fatalf("walking %s for prompt encoders: %v", root, err)
	}
	for _, dir := range dirs {
		for _, file := range packages.load(dir) {
			if !importsPath(file, promptModelPort) {
				continue
			}
			for _, typ := range promptRootTypes(file) {
				if decl, found := packages.lookup(file, dir, typ); found {
					rootNames[decl.dir+"."+decl.spec.Name.Name] = true
				}
				walk.seen = map[promptTypeKey]bool{}
				walk.typeExpr(file, dir, typ)
			}
		}
	}
	if len(packages.errs) > 0 {
		t.Fatalf("reading Go source for prompt encoders: %v", packages.errs)
	}
	return sortedKeys(walk.findings), sortedKeys(rootNames)
}

func importsPath(file *ast.File, importPath string) bool {
	for _, spec := range file.Imports {
		if strings.Trim(spec.Path.Value, `"`) == importPath {
			return true
		}
	}
	return false
}

// promptRootFloor names prompt roots the resolver must reach. Each is a brief
// that once sent minor units to a model.
var promptRootFloor = []string{
	"internal/compose/companybrief.Input",
	"internal/compose/contactbrief.Input",
	"internal/compose/meetingbrief.Input",
	"internal/compose/meetingbrief.PlanPrompt",
}

func TestNoPromptCarriesAMinorUnitAmount(t *testing.T) {
	t.Parallel()
	findings, resolved := promptMinorUnitFindings(t, ".", func(path string) (*ast.File, error) {
		return gatekit.ParseFile(path, parser.SkipObjectResolution)
	})
	reached := map[string]bool{}
	for _, name := range resolved {
		reached[name] = true
	}
	for _, name := range promptRootFloor {
		if !reached[name] {
			t.Errorf("the gate no longer resolves %s as a prompt root; it resolved only %d roots: %v",
				name, len(resolved), resolved)
		}
	}
	if len(findings) > 0 {
		t.Errorf("these fields reach a model prompt as a minor-unit integer, which a model reads as an "+
			"amount a hundred times too large:\n  %s\n\nTag the field `json:\"-\"` and write the amount "+
			"in major units from a MarshalJSON through values.MajorUnits, as meetingbrief.DealIn does.",
			strings.Join(findings, "\n  "))
	}
}

// TestThePromptMinorUnitDetectorSeesWhatItClaimsTo plants the shapes the
// census above must catch and the ones it must not, and reads its answer.
func TestThePromptMinorUnitDetectorSeesWhatItClaimsTo(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writePromptFixture(t, filepath.Join(root, "internal/brief/brief.go"), `package brief

import (
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/ports/model"
	"github.com/margince/margince/backend/internal/money"
)

type Input struct {
	Deal    *DealIn
	Lines   []money.Line
	Hidden  int64 `+"`json:\"-\"`"+`
	Tagged  int64 `+"`json:\"net_minor\"`"+`
	Label   string
}

type DealIn struct {
	AmountMinor int64
	SafeMinor   int64 `+"`json:\"-\"`"+`
	NameMinor   string
}

type Plan struct{ Deal DealIn }

func encode(in Input) model.Message {
	raw, _ := json.Marshal(in)
	return model.Message{Content: string(raw)}
}

func encodePlan() string {
	plan := Plan{}
	raw, _ := json.Marshal(&plan)
	return string(raw)
}
`)
	writePromptFixture(t, filepath.Join(root, "internal/money/money.go"), `package money

type Line struct {
	TotalMinor *int64
	unitMinor  int64
}
`)
	findings, resolved := promptMinorUnitFindings(t, root, func(path string) (*ast.File, error) {
		return parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	})
	brief := filepath.Join(root, "internal/brief")
	want := []string{
		brief + ".DealIn.AmountMinor",
		brief + ".Input.Tagged",
		filepath.Join(root, "internal/money") + ".Line.TotalMinor",
	}
	if got := fmt.Sprint(findings); got != fmt.Sprint(want) {
		t.Errorf("findings = %v, want %v", findings, want)
	}
	if got, wantRoots := fmt.Sprint(resolved), fmt.Sprint([]string{brief + ".Input", brief + ".Plan"}); got != wantRoots {
		t.Errorf("resolved roots = %v, want %v", got, wantRoots)
	}
}

func writePromptFixture(t *testing.T, path, source string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("making fixture dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("writing fixture %s: %v", path, err)
	}
}
