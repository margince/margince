// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// A list's hidden count is read from the statement the list reads.
//
// A record page states how many entries row scope withheld from a section,
// and whom to ask. If the count and the list each wrote their own `FROM`, the
// two populations would drift, and the count would name entries the list never
// had. So both are built from one storekit.HiddenSection, made by one builder
// function. This holds that every builder feeds both a hidden count and a list,
// and that a hidden count never reads a section written inline.

import (
	"go/ast"
	"go/parser"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// sectionListMethods are the HiddenSection methods that render the list side.
var sectionListMethods = []string{"ListSQL", "VisibleFrom"}

// hiddenRecordsSections pairs each contract field of type HiddenRecords with
// the builder whose section answers it. The pairing is the one fact neither
// side states: both the contract's fields and the tree's builders are read
// from their owners, and each must match this table exactly.
//
// gatekit:fixture the contract fields a hidden-section builder answers
var hiddenRecordsSections = map[string]string{
	"Company360.contacts_hidden":                    "internal/modules/contacts.employmentSection",
	"Company360.billing_contacts_hidden":            "internal/modules/contacts.billingContactsSection",
	"CompanyFinanceSummary.billing_contacts_hidden": "internal/modules/contacts.billingContactsSection",
}

// sectionUse is what the tree does with one builder's sections.
type sectionUse struct {
	file            string
	counted, listed bool
}

func TestEveryHiddenCountSharesItsListsSection(t *testing.T) {
	t.Parallel()
	// Keyed by package directory and name, so two packages' builders of one
	// name are told apart.
	builders := map[string]*sectionUse{}
	bodies := map[*ast.FuncDecl]string{}
	err := gatekit.WalkDir("internal", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") ||
			strings.Contains(filepath.ToSlash(path), "/storekit/") {
			return err
		}
		file, err := gatekit.ParseFile(path, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		for _, decl := range file.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || fn.Body == nil {
				continue
			}
			dir := filepath.ToSlash(filepath.Dir(path))
			bodies[fn] = dir
			if returnsHiddenSection(fn) {
				builders[dir+"."+fn.Name.Name] = &sectionUse{file: filepath.ToSlash(path)}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(builders) == 0 {
		t.Fatal("this census found no function returning a storekit.HiddenSection, and the record pages build " +
			"several: the reader has stopped matching rather than the tree having changed")
	}
	for fn, dir := range bodies {
		readSectionUses(t, fn, dir, builders)
	}
	assertSectionsMatchTheContract(t, builders)
	for _, name := range slices.Sorted(maps.Keys(builders)) {
		use := builders[name]
		if !use.counted || !use.listed {
			t.Errorf("%s: %s builds a hidden section that feeds a hidden count %v and a list %v. "+
				"Read both from it, so the count is of the rows the list would have shown", use.file, name, use.counted, use.listed)
		}
	}
}

// returnsHiddenSection is whether fn's results include a HiddenSection.
func returnsHiddenSection(fn *ast.FuncDecl) bool {
	if fn.Type.Results == nil {
		return false
	}
	for _, field := range fn.Type.Results.List {
		if selector, isSelector := field.Type.(*ast.SelectorExpr); isSelector && selector.Sel.Name == "HiddenSection" {
			return true
		}
	}
	return false
}

// readSectionUses marks what fn does with each builder's result. A variable
// assigned from a builder call carries that builder to its later uses in fn.
func readSectionUses(t *testing.T, fn *ast.FuncDecl, dir string, builders map[string]*sectionUse) {
	t.Helper()
	bound := map[string]string{}
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if assign, isAssign := node.(*ast.AssignStmt); isAssign && len(assign.Rhs) == 1 {
			if name := builderCalled(assign.Rhs[0], dir, builders); name != "" {
				if ident, isIdent := assign.Lhs[0].(*ast.Ident); isIdent {
					bound[ident.Name] = name
				}
			}
		}
		call, isCall := node.(*ast.CallExpr)
		if !isCall {
			return true
		}
		selector, isSelector := call.Fun.(*ast.SelectorExpr)
		if !isSelector {
			return true
		}
		switch {
		case selector.Sel.Name == "ReadHiddenRows":
			if len(call.Args) < 3 {
				return true
			}
			name := sectionSource(call.Args[2], dir, bound, builders)
			if name == "" {
				t.Errorf("%s: ReadHiddenRows reads a section no builder made. Build it in one function "+
					"that the list reads too", fn.Name.Name)
				return true
			}
			builders[name].counted = true
		case slices.Contains(sectionListMethods, selector.Sel.Name):
			if name := sectionSource(selector.X, dir, bound, builders); name != "" {
				builders[name].listed = true
			}
		}
		return true
	})
}

// sectionSource is the builder an expression's section came from, or "".
func sectionSource(expr ast.Expr, dir string, bound map[string]string, builders map[string]*sectionUse) string {
	if ident, isIdent := expr.(*ast.Ident); isIdent {
		return bound[ident.Name]
	}
	return builderCalled(expr, dir, builders)
}

// builderCalled is the builder a call expression invokes, or "".
func builderCalled(expr ast.Expr, dir string, builders map[string]*sectionUse) string {
	call, isCall := expr.(*ast.CallExpr)
	if !isCall {
		return ""
	}
	ident, isIdent := call.Fun.(*ast.Ident)
	if !isIdent {
		return ""
	}
	key := dir + "." + ident.Name
	if _, isBuilder := builders[key]; isBuilder {
		return key
	}
	return ""
}

// assertSectionsMatchTheContract fails when a HiddenRecords field has no
// builder behind it, or a builder answers no field: a count the contract does
// not carry, or a field nothing fills.
func assertSectionsMatchTheContract(t *testing.T, builders map[string]*sectionUse) {
	t.Helper()
	fields := hiddenRecordsFields(t)
	if !slices.Equal(fields, slices.Sorted(maps.Keys(hiddenRecordsSections))) {
		t.Errorf("api/crm.yaml carries HiddenRecords on %v, and hiddenRecordsSections pairs %v. "+
			"Pair each field with the section builder that fills it", fields, slices.Sorted(maps.Keys(hiddenRecordsSections)))
	}
	paired := map[string]bool{}
	for _, builder := range hiddenRecordsSections {
		paired[builder] = true
		if builders[builder] == nil {
			t.Errorf("hiddenRecordsSections names %s, and no such function returns a storekit.HiddenSection", builder)
		}
	}
	for builder := range builders {
		if !paired[builder] {
			t.Errorf("%s builds a hidden section that no HiddenRecords field in api/crm.yaml carries", builder)
		}
	}
}

// hiddenRecordsFields is every "Schema.property" in the contract whose type is
// HiddenRecords, sorted.
func hiddenRecordsFields(t *testing.T) []string {
	t.Helper()
	components, _ := loadContractDocument(t)["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)
	if len(schemas) == 0 {
		t.Fatal("read no schemas from api/crm.yaml: the reader has stopped matching")
	}
	var out []string
	for name, raw := range schemas {
		schema, _ := raw.(map[string]any)
		properties, _ := schema["properties"].(map[string]any)
		for property, node := range properties {
			if refersToHiddenRecords(node) {
				out = append(out, name+"."+property)
			}
		}
	}
	slices.Sort(out)
	return out
}

// refersToHiddenRecords is whether a property node is a HiddenRecords, named
// directly or through allOf.
func refersToHiddenRecords(node any) bool {
	property, _ := node.(map[string]any)
	if ref, _ := property["$ref"].(string); strings.HasSuffix(ref, "/HiddenRecords") {
		return true
	}
	parts, _ := property["allOf"].([]any)
	for _, part := range parts {
		if refersToHiddenRecords(part) {
			return true
		}
	}
	return false
}
