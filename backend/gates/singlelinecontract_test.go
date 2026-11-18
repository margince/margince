// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// Every text field the contract declares `x-single-line: true` must reach the
// request decoders as a generated field carrying the tag
// datasource.RejectUnsafeSingleLine reads. A declaration with no tag is a name
// that still accepts line breaks.
func TestEverySingleLineFieldIsTaggedForTheDecoders(t *testing.T) {
	t.Parallel()
	contract, err := gatekit.ReadFile(contractPath)
	if err != nil {
		t.Fatal(err)
	}
	generated, err := gatekit.ReadFile("internal/contracts/api_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	declared := strings.Count(string(contract), "x-single-line: true")
	tagged := strings.Count(string(generated), datasource.SingleLineTag+`:\"true\"`) +
		strings.Count(string(generated), datasource.SingleLineTag+`:"true"`)
	if declared < 30 {
		t.Fatalf("the contract declares %d single-line fields; the scan no longer reads them", declared)
	}
	if tagged < declared {
		t.Errorf("the contract declares %d single-line fields and the generated structs tag %d: "+
			"a declaration on a schema the generator cannot tag is a promise no decoder keeps", declared, tagged)
	}
}

// A Create schema and its Update schema name one field. When the create side
// is single-line, the update side must be too, or a rename accepts what the
// create refused. Pairs come from the CreateXRequest / UpdateXRequest naming,
// so a future pair is held without being listed here.
func TestAnUpdateSchemaKeepsTheSingleLineRuleOfItsCreateSchema(t *testing.T) {
	t.Parallel()
	raw, err := gatekit.ReadFile(contractPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	pairs := 0
	for _, create := range singleLineSchemas(&doc) {
		name := strings.TrimSuffix(strings.TrimPrefix(create.name, "Create"), "Request")
		update, ok := slProperties(&doc, "Update"+name+"Request")
		if !strings.HasPrefix(create.name, "Create") || !strings.HasSuffix(create.name, "Request") || !ok {
			continue
		}
		pairs++
		for _, property := range create.properties {
			if has, present := update[property]; present && !has {
				t.Errorf("Update%sRequest.%s lacks x-single-line though %s declares it", name, property, create.name)
			}
		}
	}
	if pairs < 10 {
		t.Fatalf("found %d create/update pairs with single-line fields; the scan no longer reads them", pairs)
	}
}

type singleLineSchema struct {
	name       string
	properties []string
}

func slSchemas(doc *yaml.Node) *yaml.Node {
	node := doc.Content[0]
	for _, key := range []string{"components", "schemas"} {
		next := slMapping(node, key)
		if next == nil {
			return nil
		}
		node = next
	}
	return node
}

func slMapping(node *yaml.Node, key string) *yaml.Node {
	if node == nil {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func isSingleLine(property *yaml.Node) bool {
	flag := slMapping(property, "x-single-line")
	return flag != nil && flag.Value == "true"
}

// slProperties maps each property of a named schema to whether it is
// single-line.
func slProperties(doc *yaml.Node, name string) (map[string]bool, bool) {
	properties := slMapping(slMapping(slSchemas(doc), name), "properties")
	if properties == nil {
		return nil, false
	}
	out := map[string]bool{}
	for i := 0; i+1 < len(properties.Content); i += 2 {
		out[properties.Content[i].Value] = isSingleLine(properties.Content[i+1])
	}
	return out, true
}

func singleLineSchemas(doc *yaml.Node) []singleLineSchema {
	var out []singleLineSchema
	schemas := slSchemas(doc)
	for i := 0; i+1 < len(schemas.Content); i += 2 {
		properties, _ := slProperties(doc, schemas.Content[i].Value)
		var marked []string
		for property, single := range properties {
			if single {
				marked = append(marked, property)
			}
		}
		if len(marked) > 0 {
			out = append(out, singleLineSchema{name: schemas.Content[i].Value, properties: marked})
		}
	}
	return out
}
