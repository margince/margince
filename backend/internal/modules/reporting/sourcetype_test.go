// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestKnownSourceTypesAreTheDeclaredConstants(t *testing.T) {
	t.Parallel()
	file, err := parser.ParseFile(token.NewFileSet(), "sourcetype.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	declared := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		if typ, ok := spec.Type.(*ast.Ident); !ok || typ.Name != "SourceType" {
			return true
		}
		for _, v := range spec.Values {
			value, err := strconv.Unquote(v.(*ast.BasicLit).Value)
			if err != nil {
				t.Fatal(err)
			}
			declared[value] = true
		}
		return true
	})
	if len(declared) != len(knownSourceTypes) {
		t.Errorf("SourceType declares %d values and knownSourceTypes holds %d", len(declared), len(knownSourceTypes))
	}
	for value := range declared {
		if _, ok := knownSourceTypes[value]; !ok {
			t.Errorf("SourceType declares %q and knownSourceTypes does not", value)
		}
	}
}

func TestAFactWithAnUnknownSourceIsRefusedBeforeItIsKept(t *testing.T) {
	t.Parallel()
	if err := requireKnownSourceType("deal"); err != nil {
		t.Errorf("deal was refused: %v", err)
	}
	for _, bad := range []string{"", "Deal", "widget"} {
		if err := requireKnownSourceType(bad); !errors.Is(err, apperrors.ErrInvalidArgument) {
			t.Errorf("%q: want ErrInvalidArgument, got %v", bad, err)
		}
	}
}
