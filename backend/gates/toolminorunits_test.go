// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

//gate:kind prohibition H2

package gates

// No agent tool result carries a minor-unit amount a model cannot read in
// major units.
//
// A tool result reaches the model through Registry.Invoke, which adds
// `<name>_display` in major units beside each `<name>_minor` whose object names
// a currency, or sits under an enclosing base_currency (withReadableAmounts).
// So a tool result may carry `_minor` integers, unlike a prompt, but only where
// that pass finds a currency for them. An integer named for minor units fails
// when its JSON key is not spelled `_minor`, or when no currency is in scope.
// Scope follows the runtime: an object's own currency or base_currency, else
// the base_currency of an enclosing object, unless an object between them
// names a currency of its own.
//
// Roots: the type argument of every schemaFor[T]() under the agents module,
// which is the declared output shape of each tool. Every root must resolve.
//
// Blind spots (H2): a tool that returns raw JSON (a passthrough or a wire
// record) declares no Go shape, so only the runtime pass covers it. An
// interface field ends the walk at that field.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const toolResultDir = "internal/modules/agents"

// toolResultRoots is the type argument of each schemaFor[T]() in dir.
func (p *minorUnitSources) toolResultRoots(dir string) []promptRoot {
	var roots []promptRoot
	for _, file := range p.load(dir) {
		ast.Inspect(file, func(n ast.Node) bool {
			call, isCall := n.(*ast.CallExpr)
			if !isCall {
				return true
			}
			if index, isIndex := call.Fun.(*ast.IndexExpr); isIndex {
				if name, isIdent := index.X.(*ast.Ident); isIdent && name.Name == "schemaFor" {
					roots = append(roots, promptRoot{file, dir, index.Index})
				}
			}
			return true
		})
	}
	return roots
}

// toolResultTypes is every type a declared tool result is built from.
func (p *minorUnitSources) toolResultTypes(dir string) map[promptTypeKey]bool {
	reached := map[promptTypeKey]bool{}
	walk := &toolWalk{packages: p, findings: map[string]bool{}}
	for _, root := range p.toolResultRoots(dir) {
		walk.seen = map[toolSeenKey]bool{}
		walk.typeExpr(root.file, root.dir, root.typ, false)
		for seen := range walk.seen {
			reached[seen.key] = true
		}
	}
	return reached
}

// toolSeenKey visits a type once per scope: under an inherited currency, and
// without one, a field can pass in one and fail in the other.
type toolSeenKey struct {
	key       promptTypeKey
	inherited bool
}

// toolWalk follows a tool result's type and records each minor-unit integer
// the runtime pass cannot name a unit for.
type toolWalk struct {
	packages *minorUnitSources
	seen     map[toolSeenKey]bool
	findings map[string]bool
}

func (w *toolWalk) typeExpr(file *ast.File, dir string, expr ast.Expr, inherited bool) {
	switch typ := expr.(type) {
	case *ast.StarExpr:
		w.typeExpr(file, dir, typ.X, inherited)
	case *ast.ArrayType:
		w.typeExpr(file, dir, typ.Elt, inherited)
	case *ast.MapType:
		w.typeExpr(file, dir, typ.Value, inherited)
	case *ast.StructType:
		w.object(w.members(file, dir, "struct literal", typ, 0), inherited)
	case *ast.Ident, *ast.SelectorExpr:
		decl, found := w.packages.lookup(file, dir, typ)
		if !found {
			return
		}
		key := toolSeenKey{promptTypeKey{decl.dir, decl.spec.Name.Name}, inherited}
		if w.seen[key] {
			return
		}
		w.seen[key] = true
		if fields, isStruct := decl.spec.Type.(*ast.StructType); isStruct {
			w.object(w.members(decl.file, decl.dir, decl.dir+"."+decl.spec.Name.Name, fields, 0), inherited)
			return
		}
		w.typeExpr(decl.file, decl.dir, decl.spec.Type, inherited)
	}
}

// toolMember is one field of a JSON object, with the declaration that owns it.
type toolMember struct {
	file  *ast.File
	dir   string
	owner string
	field *ast.Field
}

// members lists one JSON object's fields: the struct's own and those of each
// struct it embeds untagged, whose fields encoding/json promotes.
func (w *toolWalk) members(file *ast.File, dir, owner string, fields *ast.StructType, depth int) []toolMember {
	var out []toolMember
	for _, field := range fields.Fields.List {
		if len(field.Names) == 0 && minorUnitJSONKey(field) == "" && depth < 8 {
			decl, found := w.packages.lookup(file, dir, stripStars(field.Type))
			if embedded, isStruct := decl.spec.Type.(*ast.StructType); found && isStruct {
				out = append(out, w.members(decl.file, decl.dir, decl.dir+"."+decl.spec.Name.Name, embedded, depth+1)...)
				continue
			}
		}
		out = append(out, toolMember{file, dir, owner, field})
	}
	return out
}

// object judges one JSON object's amounts and walks into its fields with the
// scope its children inherit.
func (w *toolWalk) object(members []toolMember, inherited bool) {
	own, base := false, false
	for _, m := range members {
		for _, name := range m.field.Names {
			switch toolJSONKey(m.field, name.Name) {
			case "currency":
				own = true
			case "base_currency":
				base = true
			}
		}
	}
	scoped := own || base || inherited
	childInherits := base || (inherited && !own)
	for _, m := range members {
		if minorUnitJSONKey(m.field) == "-" {
			continue
		}
		for _, name := range m.field.Names {
			key := toolJSONKey(m.field, name.Name)
			if !name.IsExported() || !(strings.HasSuffix(name.Name, "Minor") || strings.HasSuffix(key, "_minor")) ||
				!w.packages.integerAmount(m.file, m.dir, m.field.Type, 0) {
				continue
			}
			if !strings.HasSuffix(key, "_minor") || !scoped {
				w.findings[m.owner+"."+name.Name] = true
			}
		}
		w.typeExpr(m.file, m.dir, m.field.Type, childInherits)
	}
}

// toolJSONKey is the key encoding/json writes a field under.
func toolJSONKey(field *ast.Field, name string) string {
	if key := minorUnitJSONKey(field); key != "" {
		return key
	}
	return name
}

// toolMinorUnitFindings walks every tool result root under moduleRoot and
// returns the amounts with no readable unit, the roots it resolved, and the
// roots it could not.
func toolMinorUnitFindings(t *testing.T, moduleRoot string, parse func(string) (*ast.File, error)) (findings, resolved, unresolved []string) {
	t.Helper()
	packages := newMinorUnitSources(moduleRoot, parse)
	walk := &toolWalk{packages: packages, findings: map[string]bool{}}
	rootNames, missing := map[string]bool{}, map[string]bool{}
	for _, root := range packages.toolResultRoots(filepath.Join(moduleRoot, toolResultDir)) {
		decl, found := packages.lookup(root.file, root.dir, stripStars(root.typ))
		if !found {
			missing[types.ExprString(root.typ)] = true
			continue
		}
		rootNames[decl.dir+"."+decl.spec.Name.Name] = true
		walk.seen = map[toolSeenKey]bool{}
		walk.typeExpr(root.file, root.dir, root.typ, false)
	}
	if len(packages.errs) > 0 {
		t.Fatalf("reading Go source for tool results: %v", packages.errs)
	}
	return sortedKeys(walk.findings), sortedKeys(rootNames), sortedKeys(missing)
}

// toolRootFloor names tool results the resolver must reach. Each once sent a
// minor-unit amount to a model with no unit beside it.
var toolRootFloor = []string{
	"internal/modules/agents.ForecastMovementResult",
	"internal/modules/agents.ForecastReadingsResult",
	"internal/modules/agents.InputChecksResult",
}

func TestNoToolResultCarriesAnAmountWithoutItsUnit(t *testing.T) {
	t.Parallel()
	findings, resolved, unresolved := toolMinorUnitFindings(t, ".", func(path string) (*ast.File, error) {
		return gatekit.ParseFile(path, parser.SkipObjectResolution)
	})
	if len(unresolved) > 0 {
		t.Errorf("these tool result shapes did not resolve, so their amounts went unread: %v", unresolved)
	}
	reached := map[string]bool{}
	for _, name := range resolved {
		reached[name] = true
	}
	for _, name := range toolRootFloor {
		if !reached[name] {
			t.Errorf("the gate no longer resolves %s as a tool result; it resolved %d: %v", name, len(resolved), resolved)
		}
	}
	if len(findings) > 0 {
		t.Errorf("these tool result amounts reach the model in minor units with no unit it can read:\n  %s\n\n"+
			"Spell the JSON key `<name>_minor` and put a `currency`, or a `base_currency` the amount was "+
			"converted to, on the object or an enclosing one. Registry.Invoke then adds `<name>_display`.",
			strings.Join(findings, "\n  "))
	}
}

// TestTheToolMinorUnitDetectorSeesWhatItClaimsTo plants each scope rule and
// reads the answer.
func TestTheToolMinorUnitDetectorSeesWhatItClaimsTo(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writePromptFixture(t, filepath.Join(root, toolResultDir, "tools.go"), toolFixture)
	findings, resolved, unresolved := toolMinorUnitFindings(t, root, func(path string) (*ast.File, error) {
		return parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	})
	agents := filepath.Join(root, toolResultDir)
	want := []string{agents + ".Bare.CostMinor", agents + ".Line.NetMinor", agents + ".Unspelled.FeeMinor"}
	if got := fmt.Sprint(findings); got != fmt.Sprint(want) {
		t.Errorf("findings = %v, want %v", findings, want)
	}
	wantRoots := []string{agents + ".Bare", agents + ".Readings", agents + ".Row", agents + ".Unspelled"}
	if fmt.Sprint(resolved) != fmt.Sprint(wantRoots) || fmt.Sprint(unresolved) != "[Missing]" {
		t.Errorf("resolved %v and missed %v, want %v and [Missing]", resolved, unresolved, wantRoots)
	}
}

// toolFixture plants one case per rule. Line passes under Readings, which
// converts to EUR, and fails under Row, whose own currency stops inheritance.
const toolFixture = `package agents

type Readings struct {
	BaseCurrency string ` + "`json:\"base_currency\"`" + `
	TotalMinor   int64  ` + "`json:\"total_minor\"`" + `
	Landing      Landing
	Lines        []Line
	Embedded
}

type Landing struct{ AmountMinor int64 ` + "`json:\"amount_minor\"`" + ` }

type Embedded struct{ SpentMinor *int64 ` + "`json:\"spent_minor\"`" + ` }

type Row struct {
	Currency    string ` + "`json:\"currency\"`" + `
	AmountMinor int64  ` + "`json:\"amount_minor\"`" + `
	Line        Line
}

type Line struct{ NetMinor int64 ` + "`json:\"net_minor\"`" + ` }

type Bare struct{ CostMinor **int64 ` + "`json:\"cost_minor\"`" + ` }

type Cents int64

type Unspelled struct {
	Currency string ` + "`json:\"currency\"`" + `
	FeeMinor Cents  ` + "`json:\"fee\"`" + `
}

func schemaFor[T any]() []byte { return nil }

var shapes = [][]byte{schemaFor[Readings](), schemaFor[*Row](), schemaFor[Bare](), schemaFor[Unspelled](), schemaFor[Missing]()}
`
