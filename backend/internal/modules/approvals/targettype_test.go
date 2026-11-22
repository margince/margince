// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestKnownTargetTypesAreTheDeclaredConstants(t *testing.T) {
	t.Parallel()
	file, err := parser.ParseFile(token.NewFileSet(), "targettype.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	declared := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		if typ, ok := spec.Type.(*ast.Ident); !ok || typ.Name != "TargetType" {
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
	if len(declared) != len(knownTargetTypes) {
		t.Errorf("TargetType declares %d values and knownTargetTypes holds %d", len(declared), len(knownTargetTypes))
	}
	for value := range declared {
		if _, ok := knownTargetTypes[value]; !ok {
			t.Errorf("TargetType declares %q and knownTargetTypes does not", value)
		}
	}
}

func TestEveryClassifiedTargetTypeIsAKnownTargetType(t *testing.T) {
	t.Parallel()
	for _, classified := range ClassifiedTargetTypes() {
		if err := requireKnownTargetType("target type", classified); err != nil {
			t.Errorf("%s has a visibility rule but cannot be staged: %v", classified, err)
		}
	}
}

func TestAStagingRefusesATypeTheCheckWouldRefuse(t *testing.T) {
	t.Parallel()
	for _, accepted := range []string{"", "deal", "stage", "ext_visit_log_visit"} {
		if err := requireKnownTargetType("target type", accepted); err != nil {
			t.Errorf("%q was refused: %v", accepted, err)
		}
	}
	for _, refused := range []string{"widget", "Deal", "ext:visit-log:visit", "ext_Bad"} {
		if err := requireKnownTargetType("target type", refused); !errors.Is(err, apperrors.ErrInvalidArgument) {
			t.Errorf("%q: want ErrInvalidArgument, got %v", refused, err)
		}
	}
}
