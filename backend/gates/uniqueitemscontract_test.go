// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// Every list the contract declares `uniqueItems: true` must reach the request
// decoders as a tagged generated field. The tag is the only thing
// datasource.RejectRepeatedItems reads. A list the generator cannot tag (a
// top-level array schema, an array of arrays) is a promise no decoder keeps.
//
// Compared as sets of Schema.property names, so a missing tag cannot be hidden
// by an extra one elsewhere.
func TestEveryUniqueItemsListIsTaggedForTheDecoders(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	declared := map[string]bool{}
	uniqueItemsDeclarations(&doc, nil, declared)
	if len(declared) == 0 {
		t.Fatal("found no `uniqueItems: true` in the contract; the scan no longer reads it")
	}
	tagged := uniqueTaggedFields(t, "internal/contracts/api_gen.go")
	for _, site := range sortedKeys(declared) {
		if !tagged[site] {
			t.Errorf("%s declares uniqueItems and no generated field carries `%s:\"items\"` for it; "+
				"declare the list as a property of a named schema, or the decoders accept its repeats",
				site, datasource.UniqueItemsTag)
		}
	}
	for _, field := range sortedKeys(tagged) {
		if !declared[field] {
			t.Errorf("the generated field %s is tagged unique but no schema property of that name declares uniqueItems", field)
		}
	}
}

// uniqueItemsDeclarations records each `uniqueItems: true` on a named schema's
// property as Schema.property. Anywhere else it records the full document
// path, which no generated field can match.
func uniqueItemsDeclarations(node *yaml.Node, path []string, into map[string]bool) {
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			uniqueItemsDeclarations(child, path, into)
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, val := node.Content[i], node.Content[i+1]
			if key.Value == "uniqueItems" && val.Kind == yaml.ScalarNode && val.Value == "true" {
				into[uniqueItemsSite(path)] = true
			}
			uniqueItemsDeclarations(val, append(slices.Clone(path), key.Value), into)
		}
	case yaml.SequenceNode:
		for i, child := range node.Content {
			uniqueItemsDeclarations(child, append(slices.Clone(path), strconv.Itoa(i)), into)
		}
	}
}

func uniqueItemsSite(path []string) string {
	if len(path) == 5 && path[0] == "components" && path[1] == "schemas" && path[3] == "properties" {
		return path[2] + "." + path[4]
	}
	return strings.Join(path, ".")
}

// uniqueTaggedFields names each generated struct field tagged unique as
// Type.json_name.
func uniqueTaggedFields(t *testing.T, path string) map[string]bool {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	tagged := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}
		shape, ok := spec.Type.(*ast.StructType)
		if !ok {
			return true
		}
		for _, field := range shape.Fields.List {
			if field.Tag == nil {
				continue
			}
			tag, err := strconv.Unquote(field.Tag.Value)
			if err != nil {
				t.Fatalf("unreadable struct tag %s: %v", field.Tag.Value, err)
			}
			if reflect.StructTag(tag).Get(datasource.UniqueItemsTag) == "items" {
				name, _, _ := strings.Cut(reflect.StructTag(tag).Get("json"), ",")
				tagged[spec.Name.Name+"."+name] = true
			}
		}
		return true
	})
	return tagged
}
