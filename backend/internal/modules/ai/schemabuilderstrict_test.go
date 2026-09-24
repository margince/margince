// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// The shared schema builder emits nothing the strict profile refuses.
//
// shared/schema draws its line at this allowlist: a builder keyword outside it
// would turn every schema using that keyword into one OpenAI-shaped endpoints
// are sent unenforced, and one Anthropic may refuse. So every constructor is
// composed here, at every depth it can nest to, and the result is held to
// schemaAllowsStrict.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/schema"
)

// composedConstructors are the Node constructors the test below composes. The
// census after it reads the builder's source, so a constructor added there and
// not here fails rather than going unasked.
var composedConstructors = []string{"Array", "Enum", "Integer", "Number", "Object", "Optional", "Record", "String"}

func TestTheStrictCensusComposesEveryNodeConstructor(t *testing.T) {
	sources, err := filepath.Glob("../../shared/schema/*.go")
	if err != nil || len(sources) == 0 {
		t.Fatalf("listing the schema builder's sources: %d found, %v", len(sources), err)
	}
	found := map[string]bool{}
	for _, source := range sources {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), source, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", source, err)
		}
		for _, decl := range file.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || fn.Recv != nil || !fn.Name.IsExported() || fn.Type.Results == nil {
				continue
			}
			for _, result := range fn.Type.Results.List {
				if ident, isIdent := result.Type.(*ast.Ident); isIdent && ident.Name == "Node" {
					found[fn.Name.Name] = true
				}
			}
		}
	}
	if got := slices.Sorted(maps.Keys(found)); !slices.Equal(got, composedConstructors) {
		t.Errorf("the builder's Node constructors are %v and the strict census composes %v — "+
			"compose the new one below before relying on its output being strict-eligible", got, composedConstructors)
	}
}

func TestEverySchemaTheBuilderComposesIsStrictEligible(t *testing.T) {
	leaves := map[string]schema.Node{
		"string":    schema.String(),
		"number":    schema.Number(),
		"integer":   schema.Integer(),
		"enum":      schema.Enum("a", "b"),
		"described": schema.String().Describe("what it means"),
	}
	for name, leaf := range leaves {
		shapes := map[string]schema.Node{
			"record": schema.Record(
				schema.Field("value", leaf),
				schema.Field("optional", schema.Optional(leaf)),
				schema.Field("list", schema.Array(leaf)),
				schema.Field("nested", schema.Array(schema.Record(
					schema.Field("inner", schema.Optional(schema.Array(leaf))),
				))),
			),
			"object": schema.Object(map[string]schema.Node{
				"value": leaf, "optional": schema.Optional(leaf),
			}, "value", "optional"),
		}
		for shape, node := range shapes {
			raw := schema.Must(node)
			if !schemaAllowsStrict(raw) {
				t.Errorf("%s of %s is not strict-eligible: %s", shape, name, raw)
			}
		}
	}
}

// The inverse, so the test above cannot pass by the walk admitting anything:
// an Object that leaves a property out of required is the builder's one
// non-strict spelling, and it must be refused.
func TestAnObjectWithAnUnrequiredPropertyIsNotStrictEligible(t *testing.T) {
	raw := schema.Must(schema.Object(map[string]schema.Node{
		"id": schema.String(), "reply": schema.Enum("positive"),
	}, "id"))
	if schemaAllowsStrict(raw) {
		t.Fatalf("an object leaving reply out of required was judged strict-eligible: %s", raw)
	}
}
