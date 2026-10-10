// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

//go:build !integration

package gates

// The deals module refuses a minor-unit price above values.MaxExactInteger.
// The contract publishes no maximum for it. This gate lists each place and
// fails if the contract gains a maximum that differs from the code's.

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"os"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

type figureMaximum struct {
	file, constant string
	path           []string
}

const priceMaximumFile = "internal/shared/kernel/values/exactinteger.go"

var priceMaxima = []figureMaximum{
	{priceMaximumFile, "MaxExactInteger", schemaPath("components", "schemas", "OfferLineItemInput", "properties", "unit_price_minor")},
	{priceMaximumFile, "MaxExactInteger", schemaPath("components", "schemas", "UpdateOfferLineItemRequest", "properties", "unit_price_minor")},
	{priceMaximumFile, "MaxExactInteger", schemaPath("components", "schemas", "CreateProductRequest", "properties", "unit_price_minor")},
	{priceMaximumFile, "MaxExactInteger", schemaPath("components", "schemas", "UpdateProductRequest", "properties", "unit_price_minor")},
}

func TestAPriceMaximumTheCodeAppliesIsTheContractsOrIsRecordedAsUnstated(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing the contract: %v", err)
	}
	for _, limit := range priceMaxima {
		applied := constantExpressionValue(t, limit.file, limit.constant)
		schema := schemaNode(t, doc, limit.path)
		published, stated := schema["maximum"].(int)
		if stated && int64(published) != applied {
			t.Errorf("%s applies %d and api/crm.yaml publishes maximum %d at %v", limit.constant, applied, published, limit.path)
		}
		if !stated {
			// Recorded, not hidden: the code refuses above this and the contract is silent.
			t.Logf("%s refuses above %d; api/crm.yaml states no maximum at %v", limit.constant, applied, limit.path)
		}
	}
}

func schemaNode(t *testing.T, doc map[string]any, path []string) map[string]any {
	t.Helper()
	var node any = doc
	for _, key := range path {
		next, ok := node.(map[string]any)
		if !ok {
			t.Fatalf("api/crm.yaml has no %v: the gate's subject moved; repoint it", path)
		}
		if node, ok = next[key]; !ok {
			t.Fatalf("api/crm.yaml has no %q on the way to %v: the gate's subject moved; repoint it", key, path)
		}
	}
	schema, ok := node.(map[string]any)
	if !ok {
		t.Fatalf("api/crm.yaml %v is not a schema node", path)
	}
	return schema
}

// constantExpressionValue evaluates a package constant written as an integer
// expression of literals, such as 1<<53 - 1.
func constantExpressionValue(t *testing.T, file, name string) int64 {
	t.Helper()
	parsed, err := gatekit.ParseFile(file, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", file, err)
	}
	for _, decl := range parsed.Decls {
		general, ok := decl.(*ast.GenDecl)
		if !ok || general.Tok != token.CONST {
			continue
		}
		for _, spec := range general.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, ident := range value.Names {
				if ident.Name != name {
					continue
				}
				result, err := types.Eval(token.NewFileSet(), nil, token.NoPos, types.ExprString(value.Values[i]))
				if err != nil {
					t.Fatalf("%s in %s is not a constant expression of literals: %v", name, file, err)
				}
				n, exact := constant.Int64Val(result.Value)
				if !exact {
					t.Fatalf("%s in %s does not fit an int64", name, file)
				}
				return n
			}
		}
	}
	t.Fatalf("%s declares no %s", file, name)
	return 0
}
