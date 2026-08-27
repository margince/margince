// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// A membership set built from a generated enum's own constants must hold every
// member of that enum.
//
// Spelling the constants reads as though the set cannot drift from crm.yaml —
// every value in it IS the contract's value, so no rename slips past. What that
// spelling does not carry is the SET: crm.yaml gains a member, the generator
// emits it, the enum's Valid() accepts it, and a map written this way keeps the
// members it had. The refusal that follows names a field and rejects a value
// the contract publishes.
//
// Only completeness is checked, because the other direction cannot fail: the
// corpus admits a set only when EVERY key names a constant of one enum, so a
// set that is in scope is a subset by construction and holding it whole is set
// equality. A stray key does not make the set wrong here — it takes the set out
// of this gate's reach, which is why the corpus rule is stated with the
// detector rather than left implicit.
//
// oapi-codegen emits Valid() and no member list, which is why these maps exist
// at all — a refusal has to render the vocabulary it accepted, and Valid()
// cannot enumerate. So the maps stay and this gate holds them whole.
//
// Completeness is the DEFAULT and a selection is the exception, because the two
// fail in opposite directions: a set meant to be closed and left short refuses a
// valid value silently, while a set meant to be a selection and widened here
// fails loudly on the next run. Over-collecting costs a reason; over-suppressing
// costs a vocabulary nobody notices went short.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const contractsImportPath = "github.com/margince/margince/backend/internal/contracts"

// contractsPkgDir is where the generated enums live, relative to the module root.
const contractsPkgDir = "internal/contracts"

// deliberateSelections ratifies a set that draws on a contract enum WITHOUT
// meaning to hold all of it. Keyed by the declaration, never by the file: two
// selections over two enums share a file in this tree, and a file-keyed entry
// would ratify the second one nobody read.
var deliberateSelections = gatekit.Waive(map[string]string{
	"internal/compose/enrichextract.go:legalPageFields":         "the fields a legal/imprint page can carry, which is a property of that page kind rather than of the cold-start vocabulary; widening it would send the extractor looking for an ICP on an imprint",
	"internal/compose/siteprofile.go:hardGateProfileFields":     "the fields whose absence blocks the profile gate, deliberately the smallest set that makes a profile usable; every further field is wanted, not required",
	"internal/compose/orgdossier/growthfitwrite.go:suggestions": "the one sentence nature a growth-fit suggestion may carry; a fact or an assessment written here would be a claim the dossier never evidenced",
})

// contractEnumIndex reads the generated package and answers, for a constant
// name, which enum declares it. Derived rather than listed: the enums are
// regenerated from crm.yaml and a hand-kept index would be the second copy this
// gate exists to refuse.
func contractEnumIndex(t *testing.T) (byName map[string]string, byEnum map[string][]string) {
	t.Helper()
	byName, byEnum = map[string]string{}, map[string][]string{}
	entries, err := os.ReadDir(contractsPkgDir)
	if err != nil {
		t.Fatalf("reading %s: %v", contractsPkgDir, err)
	}
	fset := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(contractsPkgDir, entry.Name()), nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", entry.Name(), err)
		}
		indexConsts(t, file, byName, byEnum)
	}
	if len(byEnum) == 0 {
		t.Fatalf("no generated enum constants found under %s — the derivation is broken, "+
			"and a gate deriving nothing judges nothing", contractsPkgDir)
	}
	return byName, byEnum
}

func indexConsts(t *testing.T, file *ast.File, byName map[string]string, byEnum map[string][]string) {
	t.Helper()
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			enum, ok := vs.Type.(*ast.Ident)
			if !ok {
				continue
			}
			for i, value := range vs.Values {
				lit, ok := value.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING || i >= len(vs.Names) {
					continue
				}
				byName[vs.Names[i].Name] = enum.Name
				byEnum[enum.Name] = append(byEnum[enum.Name], vs.Names[i].Name)
			}
		}
	}
}

// vocabularySet is one package-level `map[string]bool` whose every key names a
// constant of ONE generated enum.
type vocabularySet struct {
	path, name, enum string
	members          []string
}

func TestEveryClosedVocabularyOverAContractEnumHoldsAllOfIt(t *testing.T) {
	t.Parallel()
	byName, byEnum := contractEnumIndex(t)
	scope := gatekit.Scope{
		Roots:   []string{"internal/compose", "internal/modules"},
		Subject: func(_ string, file *ast.File) bool { return len(vocabularySetsIn("", file, byName)) > 0 },
		Exempt: gatekit.Waive(map[string]string{
			contractsPkgDir + "/api_gen.go": "the generated source of the vocabularies themselves; a set here would be the contract, not a copy of it",
		}),
	}
	found := 0
	for _, parsed := range scope.Files(t) {
		for _, set := range vocabularySetsIn(parsed.Path, parsed.File, byName) {
			found++
			key := fmt.Sprintf("%s:%s", set.path, set.name)
			whole := byEnum[set.enum]
			missing := missingFrom(set.members, whole)
			if len(missing) == 0 {
				if deliberateSelections.Waived(t, key) {
					t.Errorf("%s %s is ratified as a deliberate selection over %s and now holds "+
						"every member of it. A waiver over a set that is complete ratifies nothing "+
						"and hides the next member that goes missing: drop the entry.",
						set.path, set.name, set.enum)
				}
				continue
			}
			if deliberateSelections.Waived(t, key) {
				continue
			}
			t.Errorf("%s %s draws on the generated enum %s and holds %d of its %d members, "+
				"missing %v.\n\nA set spelled out of the contract's constants cannot drift on a "+
				"VALUE and says nothing about the SET: crm.yaml grew and this did not, so a value "+
				"the contract publishes is refused here. Add the members, or ratify the set as a "+
				"deliberate selection in deliberateSelections with what the narrowing buys.",
				set.path, set.name, set.enum, len(set.members), len(whole), missing)
		}
	}
	if found == 0 {
		t.Error("no vocabulary set found anywhere under the roots — the detector has stopped " +
			"seeing its subject, and a gate that judges nothing reads exactly like a clean tree")
	}
	deliberateSelections.AssertAllMatched(t)
}

// vocabularySetsIn returns every package-level `map[string]bool` in file whose
// keys are ALL constants of one generated enum.
//
// All, not most: a map mixing contract constants with local strings is a
// vocabulary this gate cannot reason about — the local half has no member list
// to be complete against — and judging it on the contract half alone would
// report a shortfall that is not one.
func vocabularySetsIn(path string, file *ast.File, byName map[string]string) []vocabularySet {
	qualifier, dotImported := gatekit.ImportedAs(file, contractsImportPath)
	if qualifier == "" && !dotImported {
		return nil
	}
	var sets []vocabularySet
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, value := range vs.Values {
				lit, ok := value.(*ast.CompositeLit)
				if !ok || !isStringBoolMap(lit.Type) || i >= len(vs.Names) {
					continue
				}
				members, whole := enumKeysOf(lit, qualifier, dotImported, byName)
				if !whole || len(members) == 0 {
					continue
				}
				sets = append(sets, vocabularySet{
					path: path, name: vs.Names[i].Name,
					enum: byName[members[0]], members: members,
				})
			}
		}
	}
	return sets
}

func isStringBoolMap(expr ast.Expr) bool {
	mapType, ok := expr.(*ast.MapType)
	if !ok {
		return false
	}
	key, keyOK := mapType.Key.(*ast.Ident)
	value, valueOK := mapType.Value.(*ast.Ident)
	return keyOK && valueOK && key.Name == "string" && value.Name == "bool"
}

// enumKeysOf reports the enum constants the map's keys name, and whether EVERY
// key named one of a single enum.
func enumKeysOf(lit *ast.CompositeLit, qualifier string, dotImported bool, byName map[string]string) ([]string, bool) {
	var members []string
	enums := map[string]bool{}
	for _, element := range lit.Elts {
		kv, ok := element.(*ast.KeyValueExpr)
		if !ok {
			return nil, false
		}
		name, ok := contractConstNamed(kv.Key, qualifier, dotImported, byName)
		if !ok {
			return nil, false
		}
		members = append(members, name)
		enums[byName[name]] = true
	}
	return members, len(enums) == 1
}

// contractConstNamed unwraps the `string(crmcontracts.Member)` a map key is
// written as and answers which generated constant it names.
func contractConstNamed(expr ast.Expr, qualifier string, dotImported bool, byName map[string]string) (string, bool) {
	if call, ok := expr.(*ast.CallExpr); ok && len(call.Args) == 1 {
		if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "string" {
			expr = call.Args[0]
		}
	}
	switch node := expr.(type) {
	case *ast.SelectorExpr:
		id, ok := node.X.(*ast.Ident)
		if !ok || id.Name != qualifier {
			return "", false
		}
		_, known := byName[node.Sel.Name]
		return node.Sel.Name, known
	case *ast.Ident:
		if !dotImported {
			return "", false
		}
		_, known := byName[node.Name]
		return node.Name, known
	}
	return "", false
}

func missingFrom(have, whole []string) []string {
	held := map[string]bool{}
	for _, name := range have {
		held[name] = true
	}
	var missing []string
	for _, name := range whole {
		if !held[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	return missing
}
