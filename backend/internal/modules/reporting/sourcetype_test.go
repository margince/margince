// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	openapi_types "github.com/oapi-codegen/runtime/types"

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

// countingTx stands in for the database: the only call a contribution write
// makes is Exec, and the test asks how many it made.
type countingTx struct {
	pgx.Tx
	execs int
}

func (c *countingTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	c.execs++
	return pgconn.CommandTag{}, nil
}

func TestAnEditionKeepsNoContributionWhenOneFactNamesAnUnknownSource(t *testing.T) {
	t.Parallel()
	good := Fact{Metric: "m", SourceType: string(SourceDeal)}
	bad := Fact{Metric: "m", SourceType: "widget"}
	edition := openapi_types.UUID{}

	tx := &countingTx{}
	err := writeEditionContributions(context.Background(), tx, edition, []Fact{good, bad})
	if !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("want ErrInvalidArgument, got %v", err)
	}
	if tx.execs != 0 {
		t.Errorf("%d statements ran before the unknown source was refused, want none", tx.execs)
	}

	// A bad fact after more than one chunk of good ones still writes nothing.
	long := make([]Fact, 0, 251)
	for range 250 {
		long = append(long, good)
	}
	long = append(long, bad)
	late := &countingTx{}
	if err := writeEditionContributions(context.Background(), late, edition, long); !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("want ErrInvalidArgument for the late bad fact, got %v", err)
	}
	if late.execs != 0 {
		t.Errorf("%d statements ran before the late unknown source was refused, want none", late.execs)
	}

	clean := &countingTx{}
	if err := writeEditionContributions(context.Background(), clean, edition, []Fact{good}); err != nil {
		t.Fatalf("a known source was refused: %v", err)
	}
	if clean.execs != 1 {
		t.Errorf("a clean edition ran %d statements, want 1", clean.execs)
	}
}
