// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Every overlay wire assembler stamps `writable`.
//
// Absent means NOT writable, so an assembler that forgets produces a record
// nothing complains about and every client draws read-only. That is what hid
// this for a release: four assemblers, a fail-closed default, and an absence
// that looked like a considered answer.
//
// Held over the SOURCE as well as by behaviour, because behaviour alone only
// covers the assemblers a case remembers to call — which is the same shape of
// omission one level up.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

const overlayWireFile = "overlaywire.go"

// TestEveryOverlayWireAssemblerStampsWritable reads the file for the functions
// that build a contract record and requires each to set the slot.
func TestEveryOverlayWireAssemblerStampsWritable(t *testing.T) {
	t.Parallel()

	file, err := parser.ParseFile(token.NewFileSet(), overlayWireFile, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", overlayWireFile, err)
	}

	assemblers := 0
	for _, decl := range file.Decls {
		fn, isFn := decl.(*ast.FuncDecl)
		if !isFn || fn.Body == nil || !strings.HasPrefix(fn.Name.Name, "overlayWire") {
			continue
		}
		if !carriesTheSlot(t, returnedTypeName(fn)) {
			continue
		}
		assemblers++
		if !stampsWritable(fn.Body) {
			t.Errorf("%s builds a record and never sets Writable — absent means NOT writable, so "+
				"every record it assembles reaches the client read-only and a caller entitled to "+
				"edit one is given no way to", fn.Name.Name)
		}
	}
	// A file this reads nothing out of certifies nothing. The four that exist
	// are the floor: fewer means the naming changed and this stopped looking.
	if assemblers < 4 {
		t.Errorf("found %d overlayWire* assembler(s) building a record that carries the slot, want "+
			"at least 4 — the scan is keyed on the name and on the contract type, so fewer means it "+
			"has stopped seeing its subject rather than that the subject shrank", assemblers)
	}
}

// stampsWritable reports whether a function body assigns the Writable slot.
func stampsWritable(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		kv, isKV := n.(*ast.KeyValueExpr)
		if !isKV {
			return true
		}
		if ident, isIdent := kv.Key.(*ast.Ident); isIdent && ident.Name == "Writable" {
			found = true
		}
		return true
	})
	return found
}

// returnedTypeName is the contract type an assembler builds, or "" when its
// first result is not a named contract type.
func returnedTypeName(fn *ast.FuncDecl) string {
	if fn.Type.Results == nil || len(fn.Type.Results.List) == 0 {
		return ""
	}
	sel, isSel := fn.Type.Results.List[0].Type.(*ast.SelectorExpr)
	if !isSel {
		return ""
	}
	return sel.Sel.Name
}

// carriesTheSlot reports whether that contract type declares Writable at all.
//
// Asked of the CONTRACT rather than of a list here: which records carry the
// slot is the contract's business, and a list would have to be corrected by
// whoever added the next one — who is exactly the author this test exists to
// catch. An assembler for a type with no such field is not a finding, and
// naming those by hand is how the scan would come to skip one that does.
func carriesTheSlot(t *testing.T, typeName string) bool {
	t.Helper()
	if typeName == "" {
		return false
	}
	for _, sample := range []any{
		crmcontracts.Person{},
		crmcontracts.Organization{},
		crmcontracts.Deal{},
		crmcontracts.Lead{},
		crmcontracts.Project{},
		crmcontracts.Activity{},
	} {
		v := reflect.TypeOf(sample)
		if v.Name() != typeName {
			continue
		}
		_, has := v.FieldByName("Writable")
		return has
	}
	return false
}
