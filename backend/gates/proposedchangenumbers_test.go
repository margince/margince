// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

//go:build !integration

package gates

// A proposed change is decoded by diffhash, never by a plain Unmarshal.
//
// encoding/json decodes every number into a float64 when the destination is `any`,
// and a float64 holds an integer exactly only below 2^53. Above that the decode is
// lossy AND SILENT: 9007199254740993 becomes 9007199254740992.
//
// For a proposed change that is not a display problem. The canonical bytes take the
// diff_hash that staging reads as "this exact call is already pending", so two
// different changes collide and the second is dropped as a duplicate of the first;
// and the canonical bytes are what travels, so modify-then-approve re-decodes them
// and a resumed agent run acts on the rounded number.
//
// diffhash.DecodeObject keeps each number as its exact decimal text. This refuses
// the spelling that does not — in the packages that handle a proposed change, where
// an `any`-shaped decode of one is the defect rather than a style.

import (
	"go/ast"
	"go/token"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// changeHandlers are the packages a proposed change passes through untyped. The
// contract declares proposed_change open by kind, so these cannot narrow it to a
// struct — which is exactly why the decode has to be the lossless one.
var changeHandlers = []string{
	"internal/modules/approvals",
	"internal/shared/kernel/diffhash",
}

// lossyDecodeFloor is how many json.Unmarshal calls these packages hold in all. A
// census that reads none of them would report PASS over the defect.
const lossyDecodeFloor = 3

func TestNoProposedChangeIsDecodedThroughFloat64(t *testing.T) {
	t.Parallel()

	tree := moduleRoot(t)
	seen := 0
	for _, pkg := range changeHandlers {
		for _, path := range goSourceFiles(t, tree+"/"+pkg) {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			file, err := gatekit.ParseFile(path, 0)
			if err != nil {
				t.Fatalf("parsing %s: %v", path, err)
			}
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || !isJSONUnmarshal(call) {
					return true
				}
				seen++
				if len(call.Args) != 2 {
					return true
				}
				if !decodesIntoAny(file, call.Args[1]) {
					return true
				}
				t.Errorf("%s:%d decodes into an untyped value with json.Unmarshal — a proposed "+
					"change decoded that way rounds every integer above 2^53 to a float64, and "+
					"the rounded value is what takes the diff_hash and what a resumed run acts "+
					"on. Use diffhash.DecodeObject", path, gatekit.SourceFileSet().Position(call.Pos()).Line)
				return true
			})
		}
	}
	if seen < lossyDecodeFloor {
		t.Fatalf("read %d json.Unmarshal calls across %v, fewer than the %d they hold — the census "+
			"is reading less than the tree", seen, changeHandlers, lossyDecodeFloor)
	}
}

func isJSONUnmarshal(call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Unmarshal" {
		return false
	}
	pkg, ok := selector.X.(*ast.Ident)
	return ok && pkg.Name == "json"
}

// decodesIntoAny reports whether the destination is an untyped map OR a bare `any`
// — both make the decode lossy, and the bare one is what this census first missed:
// it flagged only map[string]any, and editscope compared two changes by decoding
// each into `any` and re-marshalling, so two amounts differing above 2^53 compared
// EQUAL while the gate passed green over it.
//
// A concrete struct is sound and stays sound: encoding/json fills an int64 field
// from the literal digits.
func decodesIntoAny(file *ast.File, destination ast.Expr) bool {
	unary, ok := destination.(*ast.UnaryExpr)
	if !ok || unary.Op != token.AND {
		return false
	}
	name, ok := unary.X.(*ast.Ident)
	if !ok {
		return false
	}
	declared := false
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for _, declaredName := range spec.Names {
			if declaredName.Name == name.Name && (isAnyMap(spec.Type) || isBareAny(spec.Type)) {
				declared = true
			}
		}
		return !declared
	})
	return declared
}

// isBareAny matches `var v any`, the destination editscope used.
func isBareAny(t ast.Expr) bool {
	name, ok := t.(*ast.Ident)
	return ok && (name.Name == "any" || name.Name == "interface{}")
}

func isAnyMap(t ast.Expr) bool {
	m, ok := t.(*ast.MapType)
	if !ok {
		return false
	}
	key, keyOK := m.Key.(*ast.Ident)
	value, valueOK := m.Value.(*ast.Ident)
	return keyOK && key.Name == "string" && valueOK && (value.Name == "any" || value.Name == "interface{}")
}
