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
// Roots: the value of every json.Marshal, json.MarshalIndent and
// Encoder.Encode call in a package under internal/ where any non-test file
// imports the model port. A value may be typed by a literal, a declaration, or
// the result of a function or method this module declares. The walk follows
// fields, embedded structs, pointers, slices, map values and type definitions
// across this module's packages. A named integer type is an integer.
//
// Blind spots (H2): a value typed only by a range, a generic or a function
// value ends the walk at that root. An interface field ends it at that field.
// The floor of known roots fails a resolver that stopped resolving.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"slices"
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

// promptFuncKey names a function, or a method by its receiver's type name.
type promptFuncKey struct{ dir, recv, name string }

// promptFuncDecl is one function declaration and the file that holds it.
type promptFuncDecl struct {
	fn   *ast.FuncDecl
	file *ast.File
}

// promptRoot is a type to walk, with the file and package directory whose
// imports qualify it.
type promptRoot struct {
	file *ast.File
	dir  string
	typ  ast.Expr
}

// minorUnitSources parses package directories on demand and indexes their
// type declarations. A walk uses it to follow a field into another package.
type minorUnitSources struct {
	moduleRoot string
	parse      func(path string) (*ast.File, error)
	files      map[string][]*ast.File
	types      map[promptTypeKey]promptTypeDecl
	funcs      map[promptFuncKey]promptFuncDecl
	errs       []error
}

// promptTypeDecl is one type declaration and the file whose imports qualify it.
type promptTypeDecl struct {
	spec *ast.TypeSpec
	file *ast.File
	dir  string
}

func newMinorUnitSources(moduleRoot string, parse func(string) (*ast.File, error)) *minorUnitSources {
	return &minorUnitSources{moduleRoot: moduleRoot, parse: parse, files: map[string][]*ast.File{},
		types: map[promptTypeKey]promptTypeDecl{}, funcs: map[promptFuncKey]promptFuncDecl{}}
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
		if fn, isFunc := decl.(*ast.FuncDecl); isFunc {
			p.funcs[promptFuncKey{dir, receiverName(fn), fn.Name.Name}] = promptFuncDecl{fn: fn, file: file}
			continue
		}
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

// heldType is the named type a value's pointers, slices and maps hold.
func heldType(expr ast.Expr) ast.Expr {
	for {
		switch typ := expr.(type) {
		case *ast.StarExpr:
			expr = typ.X
		case *ast.ArrayType:
			expr = typ.Elt
		case *ast.MapType:
			expr = typ.Value
		default:
			return expr
		}
	}
}

func stripStars(expr ast.Expr) ast.Expr {
	for {
		star, isStar := expr.(*ast.StarExpr)
		if !isStar {
			return expr
		}
		expr = star.X
	}
}

// integerAmount reports whether a field's type encodes as integers: an integer,
// a named integer type, or a pointer, slice, array or map of them.
func (p *minorUnitSources) integerAmount(file *ast.File, dir string, expr ast.Expr, depth int) bool {
	if depth > 8 {
		return false
	}
	switch typ := expr.(type) {
	case *ast.StarExpr:
		return p.integerAmount(file, dir, typ.X, depth+1)
	case *ast.ArrayType:
		return p.integerAmount(file, dir, typ.Elt, depth+1)
	case *ast.MapType:
		return p.integerAmount(file, dir, typ.Value, depth+1)
	case *ast.Ident:
		if strings.TrimPrefix(strings.TrimRight(typ.Name, "0123456789"), "u") == "int" {
			return true
		}
	}
	decl, found := p.lookup(file, dir, expr)
	return found && p.integerAmount(decl.file, decl.dir, decl.spec.Type, depth+1)
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
			if name.IsExported() && (strings.HasSuffix(name.Name, "Minor") || strings.HasSuffix(key, "_minor")) &&
				w.packages.integerAmount(file, dir, field.Type, 0) {
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

// rootsIn types the value of each json.Marshal, json.MarshalIndent and
// Encoder.Encode call in a file, where the resolver can.
func (p *minorUnitSources) rootsIn(file *ast.File, dir string) []promptRoot {
	jsonName, _ := gatekit.ImportedAs(file, "encoding/json")
	if jsonName == "" {
		return nil
	}
	var roots []promptRoot
	for _, decl := range file.Decls {
		fn, isFunc := decl.(*ast.FuncDecl)
		if !isFunc || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, isCall := n.(*ast.CallExpr)
			if !isCall {
				return true
			}
			var value ast.Expr
			switch {
			case len(call.Args) == 1 && (isQualifiedCall(call, jsonName, "Marshal") || isEncode(fn, call, jsonName)):
				value = call.Args[0]
			case len(call.Args) == 3 && isQualifiedCall(call, jsonName, "MarshalIndent"):
				value = call.Args[0]
			}
			if target := redecoded(fn, call, jsonName); target != nil {
				value = target
			}
			if value != nil {
				if root, found := p.valueType(file, dir, fn, value); found {
					roots = append(roots, root)
				}
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

// redecoded is the target a function decodes a marshalled value back into, or
// nil. Such a round trip converts one shape to another, and what leaves the
// function is the decoded shape.
func redecoded(fn *ast.FuncDecl, marshal *ast.CallExpr, jsonName string) ast.Expr {
	var encoded string
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if assign, isAssign := n.(*ast.AssignStmt); isAssign && len(assign.Rhs) == 1 && assign.Rhs[0] == marshal {
			if ident, isIdent := assign.Lhs[0].(*ast.Ident); isIdent {
				encoded = ident.Name
			}
		}
		return encoded == ""
	})
	var target ast.Expr
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, isCall := n.(*ast.CallExpr)
		if encoded != "" && isCall && len(call.Args) == 2 && isQualifiedCall(call, jsonName, "Unmarshal") {
			if ident, isIdent := call.Args[0].(*ast.Ident); isIdent && ident.Name == encoded {
				target = call.Args[1]
			}
		}
		return target == nil
	})
	return target
}

// isEncode reports an Encode on a json.NewEncoder, called on the constructor
// directly or on a name the function assigned it to.
func isEncode(fn *ast.FuncDecl, call *ast.CallExpr, jsonName string) bool {
	sel, isSel := call.Fun.(*ast.SelectorExpr)
	if !isSel || sel.Sel.Name != "Encode" {
		return false
	}
	if made, isCall := sel.X.(*ast.CallExpr); isCall {
		return isQualifiedCall(made, jsonName, "NewEncoder")
	}
	name, isIdent := sel.X.(*ast.Ident)
	if !isIdent {
		return false
	}
	encoder := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		assign, isAssign := n.(*ast.AssignStmt)
		if !isAssign || len(assign.Lhs) != len(assign.Rhs) {
			return !encoder
		}
		for i, lhs := range assign.Lhs {
			ident, isLHS := lhs.(*ast.Ident)
			made, isCall := assign.Rhs[i].(*ast.CallExpr)
			if isLHS && ident.Name == name.Name && isCall && isQualifiedCall(made, jsonName, "NewEncoder") {
				encoder = true
			}
		}
		return !encoder
	})
	return encoder
}

// valueType reads the type of an encoded value. It knows a composite literal,
// a name declared in the function, and the result of a function or method this
// module declares. Anything else answers false.
func (p *minorUnitSources) valueType(file *ast.File, dir string, fn *ast.FuncDecl, value ast.Expr) (promptRoot, bool) {
	switch v := value.(type) {
	case *ast.CompositeLit:
		return promptRoot{file, dir, v.Type}, v.Type != nil
	case *ast.ParenExpr:
		return p.valueType(file, dir, fn, v.X)
	case *ast.UnaryExpr:
		if v.Op == token.AND {
			return p.valueType(file, dir, fn, v.X)
		}
	case *ast.Ident:
		return p.declaredRoot(file, dir, fn, v.Name)
	case *ast.CallExpr:
		return p.callResult(file, dir, fn, v)
	}
	return promptRoot{}, false
}

func (p *minorUnitSources) declaredRoot(file *ast.File, dir string, fn *ast.FuncDecl, name string) (promptRoot, bool) {
	for _, list := range []*ast.FieldList{fn.Recv, fn.Type.Params} {
		if list == nil {
			continue
		}
		for _, field := range list.List {
			for _, ident := range field.Names {
				if ident.Name == name {
					return promptRoot{file, dir, field.Type}, true
				}
			}
		}
	}
	var root promptRoot
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.ValueSpec:
			for _, ident := range stmt.Names {
				if ident.Name == name && stmt.Type != nil {
					root, found = promptRoot{file, dir, stmt.Type}, true
				}
			}
		case *ast.AssignStmt:
			for i, lhs := range stmt.Lhs {
				if ident, isIdent := lhs.(*ast.Ident); isIdent && ident.Name == name && len(stmt.Lhs) == len(stmt.Rhs) {
					root, found = p.valueType(file, dir, fn, stmt.Rhs[i])
				}
			}
		}
		return !found
	})
	return root, found
}

// callResult is the first result type of a called function or method. A
// selector is a method on a value the function declared, else a function of
// the module package the file imports under that qualifier.
func (p *minorUnitSources) callResult(file *ast.File, dir string, fn *ast.FuncDecl, call *ast.CallExpr) (promptRoot, bool) {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return p.resultOf(dir, "", fun.Name)
	case *ast.SelectorExpr:
		if receiver, found := p.valueType(file, dir, fn, fun.X); found {
			decl, declared := p.lookup(receiver.file, receiver.dir, stripStars(receiver.typ))
			if !declared {
				return promptRoot{}, false
			}
			return p.resultOf(decl.dir, decl.spec.Name.Name, fun.Sel.Name)
		}
		if qualifier, isIdent := fun.X.(*ast.Ident); isIdent {
			if imported := importDirFor(file, qualifier.Name); imported != "" {
				return p.resultOf(filepath.Join(p.moduleRoot, imported), "", fun.Sel.Name)
			}
		}
	}
	return promptRoot{}, false
}

func (p *minorUnitSources) resultOf(dir, recv, name string) (promptRoot, bool) {
	p.load(dir)
	decl, found := p.funcs[promptFuncKey{dir, recv, name}]
	if !found || decl.fn.Type.Results == nil || len(decl.fn.Type.Results.List) == 0 {
		return promptRoot{}, false
	}
	return promptRoot{decl.file, dir, decl.fn.Type.Results.List[0].Type}, true
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
	// A value of a declared tool result's shape is judged by toolWalk, which
	// knows the display the registry adds beside its amounts. The shapes come
	// from what the tools declare, so a new tool joins that walk and leaves this one.
	toolResults := packages.toolResultTypes(filepath.Join(moduleRoot, toolResultDir))
	for _, dir := range dirs {
		files := packages.load(dir)
		if !slices.ContainsFunc(files, func(file *ast.File) bool { return importsPath(file, promptModelPort) }) {
			continue
		}
		for _, file := range files {
			for _, root := range packages.rootsIn(file, dir) {
				if decl, found := packages.lookup(root.file, root.dir, heldType(root.typ)); found {
					if toolResults[promptTypeKey{decl.dir, decl.spec.Name.Name}] {
						continue
					}
					rootNames[decl.dir+"."+decl.spec.Name.Name] = true
				}
				walk.seen = map[promptTypeKey]bool{}
				walk.typeExpr(root.file, root.dir, root.typ)
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
	for path, source := range map[string]string{
		"internal/brief/brief.go":        promptBriefFixture,
		"internal/brief/variants.go":     promptVariantsFixture,
		"internal/brief/helper.go":       promptHelperFixture,
		"internal/money/money.go":        promptMoneyFixture,
		"internal/modules/agents/out.go": promptToolFixture,
	} {
		writePromptFixture(t, filepath.Join(root, path), source)
	}
	findings, resolved := promptMinorUnitFindings(t, root, func(path string) (*ast.File, error) {
		return parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	})
	brief, money := filepath.Join(root, "internal/brief"), filepath.Join(root, "internal/money")
	var want []string
	for _, name := range []string{"Assigned.AssignedMinor", "Built.TaxMinor", "DealIn.AmountMinor", "Embedded.FeeMinor",
		"Encoded.EncMinor", "Helper.ExtraMinor", "Indented.IndentMinor", "Input.Tagged", "Made.CostMinor",
		"Variants.ByStageMinor", "Variants.DoubleMinor", "Variants.PriceMinor"} {
		want = append(want, brief+"."+name)
	}
	want = append(want, money+".Line.TotalMinor", money+".Sum.SumMinor")
	if got := fmt.Sprint(findings); got != fmt.Sprint(want) {
		t.Errorf("findings = %v, want %v", findings, want)
	}
	var wantRoots []string
	for _, name := range []string{"Assigned", "Built", "Encoded", "Helper", "Indented", "Input", "Made", "Plan", "Variants"} {
		wantRoots = append(wantRoots, brief+"."+name)
	}
	wantRoots = append(wantRoots, money+".Sum")
	if got := fmt.Sprint(resolved); got != fmt.Sprint(wantRoots) {
		t.Errorf("resolved roots = %v, want %v", got, wantRoots)
	}
}

// promptBriefFixture is the package's one file that imports the model port.
const promptBriefFixture = `package brief

import (
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/ports/model"
	"github.com/margince/margince/backend/internal/money"
)

type Input struct {
	Deal    *DealIn
	Lines   []money.Line
	Hidden  int64 ` + "`json:\"-\"`" + `
	Tagged  int64 ` + "`json:\"net_minor\"`" + `
	Label   string
}

type DealIn struct {
	AmountMinor int64
	SafeMinor   int64 ` + "`json:\"-\"`" + `
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
`

// promptVariantsFixture plants each way of typing an encoded value, and a
// round trip into a tool shape, which the tool walk judges instead.
const promptVariantsFixture = `package brief

import (
	"encoding/json"
	"io"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/money"
)

type Cents int64

type Variants struct {
	PriceMinor   Cents
	DoubleMinor  **int64
	ByStageMinor map[string]int64
	Embedded
}

type Embedded struct{ FeeMinor int64 }

type Built struct{ TaxMinor int64 }

func build() Built { return Built{} }

type builder struct{}

type Made struct{ CostMinor int64 }

func (builder) Build() Made { return Made{} }

type Indented struct{ IndentMinor int64 }

type Encoded struct{ EncMinor int64 }

type Assigned struct{ AssignedMinor int64 }

type Converted struct{ RoundMinor int64 }

func encodeVariants(w io.Writer) {
	json.Marshal(Variants{})
	json.Marshal(build())
	b := builder{}
	json.Marshal(b.Build())
	json.Marshal(money.Total())
	json.MarshalIndent(Indented{}, "", " ")
	json.NewEncoder(w).Encode(Encoded{})
	enc := json.NewEncoder(w)
	enc.Encode(Assigned{})
	raw, _ := json.Marshal(Converted{})
	out := []agents.Shape{}
	json.Unmarshal(raw, &out)
}
`

// promptHelperFixture encodes from a file that does not import the port.
const promptHelperFixture = `package brief

import "encoding/json"

type Helper struct{ ExtraMinor int64 }

func helper() { json.Marshal(Helper{}) }
`

const promptMoneyFixture = `package money

type Line struct {
	TotalMinor *int64
	unitMinor  int64
}

type Sum struct{ SumMinor int64 }

func Total() Sum { return Sum{} }
`

// promptToolFixture declares one tool result shape.
const promptToolFixture = `package agents

type Shape struct {
	Currency    string ` + "`json:\"currency\"`" + `
	AmountMinor int64  ` + "`json:\"amount_minor\"`" + `
}

func schemaFor[T any]() []byte { return nil }

var shapes = schemaFor[Shape]()
`

func writePromptFixture(t *testing.T, path, source string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("making fixture dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("writing fixture %s: %v", path, err)
	}
}
