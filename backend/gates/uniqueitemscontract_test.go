// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"strconv"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// Every list the contract declares `uniqueItems: true` must reach the request
// decoders as a tagged generated field. The tag is the only thing
// datasource.RejectRepeatedItems reads. A list the generator cannot tag (a
// top-level array schema, an array of arrays) is a promise no decoder keeps.
func TestEveryUniqueItemsListIsTaggedForTheDecoders(t *testing.T) {
	raw, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	declared := uniqueItemsDeclarations(&doc)
	if declared == 0 {
		t.Fatal("found no `uniqueItems: true` in the contract; the scan no longer reads it")
	}
	tagged := uniqueTaggedFields(t, "internal/contracts/api_gen.go")
	if declared != tagged {
		t.Fatalf("the contract declares %d uniqueItems lists and the generated types tag %d fields `%s:\"items\"`; "+
			"move a list the generator cannot tag into a named schema's property, or the decoders accept its repeats",
			declared, tagged, datasource.UniqueItemsTag)
	}
}

func uniqueItemsDeclarations(node *yaml.Node) int {
	count := 0
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, val := node.Content[i], node.Content[i+1]
			if key.Value == "uniqueItems" && val.Kind == yaml.ScalarNode && val.Value == "true" {
				count++
			}
		}
	}
	for _, child := range node.Content {
		count += uniqueItemsDeclarations(child)
	}
	return count
}

func uniqueTaggedFields(t *testing.T, path string) int {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	ast.Inspect(file, func(n ast.Node) bool {
		field, ok := n.(*ast.Field)
		if !ok || field.Tag == nil {
			return true
		}
		tag, err := strconv.Unquote(field.Tag.Value)
		if err != nil {
			t.Fatalf("unreadable struct tag %s: %v", field.Tag.Value, err)
		}
		if reflect.StructTag(tag).Get(datasource.UniqueItemsTag) == "items" {
			count++
		}
		return true
	})
	return count
}
