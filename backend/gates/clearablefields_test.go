// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// The fields a restore says it can clear are the fields the stores clear.
//
// compose.clearableFields decides which nulls travel as clears and which are
// refused. The stores decide which ones they actually write. A field the
// reversal path names and a store does not clear is a restore that reports
// success and changes nothing — the one outcome worse than a refusal, because
// the person reads the confirmation and stops looking. A field a store clears
// and the reversal path does not name is a restore refused for no reason.
//
// This is exactly the shape of claim that rots: two lists in different packages
// that only agree on the day they were written. So the gate derives the store
// half from the source rather than restating it.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// clearableMapsByRecordType names the declaration in each module that lists what
// that record type can clear, and the record type it serves. The values are the
// record types themselves, not reasons anything is excused.
//
// Derived from the naming convention the stores share.
//
// gatekit:fixture the record type each module's clearable-column map serves
var clearableMapsByRecordType = map[string]string{
	"clearablePersonColumns":       "person",
	"clearableOrganizationColumns": "organization",
	"clearableLeadColumns":         "lead",
	"clearableDealColumns":         "deal",
	// A clear that writes TWO columns cannot travel in the single-column map,
	// and a census that read only that map would report a store clearing fewer
	// fields than it does — under-recognition, the one way this gate must not
	// break. Both declarations serve the deal, and the walk unions them.
	"dealClearPairs":          "deal",
	"clearableProjectColumns": "project",
}

// storeClearableFields walks the module sources for the clearable-column maps
// and returns the wire field names each one holds — the map KEYS, which are
// what a caller names, not the column values.
func storeClearableFields(t *testing.T) map[string][]string {
	t.Helper()
	found := map[string][]string{}
	// A map key the walk cannot reduce to a string — a constant in the key
	// position — is a field this census would silently not see. The keys are
	// wire field names and are declared as literals for exactly that reason.
	unreadable := map[string][]string{}
	roots := []string{
		filepath.Join("internal", "modules", "people"),
		filepath.Join("internal", "modules", "deals"),
		filepath.Join("internal", "modules", "projects"),
		filepath.Join("internal", "modules", "activities"),
	}
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatalf("read %s: %v", root, err)
		}
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			path := filepath.Join(root, entry.Name())
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			collectClearableMaps(file, found, unreadable)
		}
	}
	for recordType, where := range unreadable {
		t.Errorf("%s: %v declares a clearable field under a key this census cannot read "+
			"(a constant rather than a literal). The keys are wire field names and must "+
			"stay literals: a constant there is invisible here, and the census then "+
			"reports fewer fields than the store clears while still passing.",
			recordType, where)
	}
	return found
}

// collectClearableMaps reads the keys of each clearable-column map literal.
func collectClearableMaps(file *ast.File, into map[string][]string, unreadable map[string][]string) {
	ast.Inspect(file, func(node ast.Node) bool {
		decl, isFunc := node.(*ast.FuncDecl)
		if !isFunc {
			return true
		}
		recordType, serves := clearableMapsByRecordType[decl.Name.Name]
		if !serves {
			return true
		}
		ast.Inspect(decl.Body, func(inner ast.Node) bool {
			literal, isComposite := inner.(*ast.CompositeLit)
			if !isComposite {
				return true
			}
			for _, element := range literal.Elts {
				pair, isPair := element.(*ast.KeyValueExpr)
				if !isPair {
					continue
				}
				key, isString := pair.Key.(*ast.BasicLit)
				if !isString || key.Kind != token.STRING {
					// A key this walk cannot read is a field it cannot census,
					// and skipping it silently is how the gate comes to read a
					// smaller tree and still report PASS. Recorded as unreadable
					// so the assertion below names it instead.
					unreadable[recordType] = append(unreadable[recordType], decl.Name.Name)
					continue
				}
				field, err := strconv.Unquote(key.Value)
				if err != nil {
					unreadable[recordType] = append(unreadable[recordType], decl.Name.Name)
					continue
				}
				into[recordType] = append(into[recordType], field)
			}
			return false
		})
		return false
	})
}

func TestTheFieldsARestoreClearsAreTheFieldsTheStoresClear(t *testing.T) {
	t.Parallel()
	fromStores := storeClearableFields(t)
	if len(fromStores) == 0 {
		t.Fatal("no clearable-column map found in any module; the walk is broken, not the tree — " +
			"people/person.go declares one")
	}
	declared := composeClearableFields(t)

	for recordType, storeFields := range fromStores {
		sort.Strings(storeFields)
		named := append([]string(nil), declared[recordType]...)
		sort.Strings(named)
		if strings.Join(storeFields, ",") == strings.Join(named, ",") {
			continue
		}
		t.Errorf("%s: the reversal path says it can clear %v, the store clears %v.\n"+
			"\tA field named and not cleared is a restore that reports success and "+
			"writes nothing; one cleared and not named is a restore refused for no "+
			"reason.", recordType, named, storeFields)
	}
	for recordType := range declared {
		if len(declared[recordType]) > 0 && fromStores[recordType] == nil {
			t.Errorf("%s: the reversal path names clearable fields and no store declares any; "+
				"every one of them would be sent and silently dropped", recordType)
		}
	}
}

// composeClearableFields reads compose's own declaration out of its source. It
// is unexported there on purpose — nothing outside compose decides what a
// restore may clear — so the gate parses rather than imports.
func composeClearableFields(t *testing.T) map[string][]string {
	t.Helper()
	path := filepath.Join("internal", "compose", "clearablefields.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	declared := map[string][]string{}
	ast.Inspect(file, func(node ast.Node) bool {
		spec, isValue := node.(*ast.ValueSpec)
		if !isValue || len(spec.Names) != 1 || spec.Names[0].Name != "clearableFields" {
			return true
		}
		literal, isComposite := spec.Values[0].(*ast.CompositeLit)
		if !isComposite {
			return true
		}
		for _, element := range literal.Elts {
			// The key half of the same rule the field half states below: an
			// entry this reader skips is a record type left out of BOTH sides,
			// which the census then reports agreement on. Every shape it
			// cannot read fails by name rather than being passed over.
			pair, isPair := element.(*ast.KeyValueExpr)
			if !isPair {
				t.Fatal("compose.clearableFields holds an entry that is not key: value; this " +
					"census can only read a keyed literal, and skipping one would hide a divergence")
			}
			key, isLiteral := pair.Key.(*ast.BasicLit)
			if !isLiteral {
				t.Fatal("compose.clearableFields is keyed by a non-literal record type; spell " +
					"it as a quoted literal so the census can read it")
			}
			recordType, err := strconv.Unquote(key.Value)
			if err != nil {
				t.Fatalf("compose.clearableFields is keyed by %s, which is not a quoted record type: %v",
					key.Value, err)
			}
			fields, isList := pair.Value.(*ast.CompositeLit)
			if !isList {
				t.Fatalf("compose.clearableFields[%q] is not a field list literal; this census can "+
					"only read one, and skipping it would hide a divergence", recordType)
			}
			declared[recordType] = []string{}
			for _, item := range fields.Elts {
				// A field this reader cannot read is a field the census would
				// leave out of BOTH sides and still report agreement on — the
				// one failure mode a census must not have. It fails by name
				// instead, so the fix is to spell the field as a literal here
				// rather than to discover the divergence in production.
				value, isString := item.(*ast.BasicLit)
				if !isString {
					t.Fatalf("compose.clearableFields[%q] holds a non-literal field; this census can "+
						"only read literals, and skipping one would hide a divergence", recordType)
				}
				field, err := strconv.Unquote(value.Value)
				if err != nil {
					t.Fatalf("compose.clearableFields[%q] holds %s, which is not a quoted field name: %v",
						recordType, value.Value, err)
				}
				declared[recordType] = append(declared[recordType], field)
			}
		}
		return false
	})
	if len(declared) == 0 {
		t.Fatal("compose.clearableFields was not found; the gate would report agreement " +
			"between two empty sets")
	}
	return declared
}
