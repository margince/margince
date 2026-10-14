// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind shape H2

package gates

// The contract's list envelope has one shape, and httperr.withEmptyLists relies on it.
// It writes a nil page as `"data": []` by finding a struct with a `Data` slice.
// A list that spelled its page another way would go out as `null` when empty.
// This test asserts the shape over the generated contract: a new list shape
// either carries `Data []T` or fails here.

import (
	"go/ast"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// contractGen is the generated contract surface every response type is declared in.
const contractGen = "internal/contracts/api_gen.go"

func TestEveryListResponseCarriesADataSlice(t *testing.T) {
	t.Parallel()
	file, err := gatekit.ParseFile(contractGen, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", contractGen, err)
	}

	var checked int
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.TypeSpec)
		if !ok || !strings.HasSuffix(spec.Name.Name, "ListResponse") {
			return true
		}
		structType, ok := spec.Type.(*ast.StructType)
		if !ok {
			return true
		}
		checked++
		for _, field := range structType.Fields.List {
			for _, name := range field.Names {
				if name.Name != "Data" {
					continue
				}
				if _, isSlice := field.Type.(*ast.ArrayType); !isSlice {
					t.Errorf("%s.Data is not a slice; httperr.withEmptyLists would not turn a nil page "+
						"into an empty array", spec.Name.Name)
				}
				return true
			}
		}
		t.Errorf("%s declares no Data field; httperr.withEmptyLists reads the page off `Data` and "+
			"would send a nil page as null", spec.Name.Name)
		return true
	})

	// A population of zero would pass every assertion above while proving
	// nothing — the shape this whole file exists to refuse.
	if checked == 0 {
		t.Fatalf("no *ListResponse types found in %s; the census matched nothing and can only pass", contractGen)
	}
}
