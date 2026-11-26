// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

// `nullable:` in a document that declares OpenAPI 3.1, where the keyword does
// not exist.
//
// 3.1 dropped it for a type union. A reader of these lines is told the field
// may be null. A validator reading the same lines is told nothing, and neither
// says which the server meant.
//
// The count is pinned rather than zero because no 3.1 form serves a nullable
// `$ref` through both generators: `oneOf` and `anyOf` with a null member fail
// the Go build. So the uses stay until a generator can express one, and this
// holds the set from growing meanwhile.
//
// Counted as mapping keys of the parsed document, so a flow-style member counts
// and the word in a description or a YAML comment does not.

import (
	"fmt"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// nullableInContract is how many `nullable:` members api/crm.yaml holds today.
// It may go down and never up.
const nullableInContract = 82

func TestTheContractDeclaresOpenAPI31AndNullableIsInert(t *testing.T) {
	t.Parallel()
	raw, err := gatekit.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	doc := string(raw)

	// The pin means nothing in a 3.0 document, where the keyword is real.
	if !strings.Contains(doc, "openapi: 3.1") {
		t.Fatal("the contract no longer declares OpenAPI 3.1: this census is about a keyword " +
			"that version dropped, so it has to be re-read before it is trusted")
	}

	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		t.Fatalf("parsing the contract: %v", err)
	}
	found := nullableKeys(&root)
	switch {
	case found > nullableInContract:
		t.Errorf("api/crm.yaml holds %d `nullable:` members, up from %d. 3.1 has no such keyword: "+
			"write the field as a type union, or drop it where absence is the real contract",
			found, nullableInContract)
	case found < nullableInContract:
		t.Errorf("api/crm.yaml holds %d `nullable:` members, down from %d. Re-pin "+
			"nullableInContract to %d so the gain cannot be given back", found, nullableInContract, found)
	}
}

// nullableKeys counts the mapping keys named nullable anywhere under node.
func nullableKeys(node *yaml.Node) int {
	count := 0
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == "nullable" {
				count++
			}
		}
	}
	for _, child := range node.Content {
		count += nullableKeys(child)
	}
	return count
}

// TestNoSchemaNarrowsANullableReference refuses `allOf` beside a type union
// with null. It is the one 3.1 form that builds in both generators. The
// TypeScript it emits is `(T | null) & T`, which is just T. So the null is gone
// from the published type while the contract still says it may arrive.
func TestNoSchemaNarrowsANullableReference(t *testing.T) {
	t.Parallel()
	raw, err := gatekit.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		t.Fatalf("parsing the contract: %v", err)
	}
	if found := narrowingCompositions(&root); len(found) > 0 {
		t.Errorf("api/crm.yaml composes allOf with a null type union at %s. The generated "+
			"TypeScript drops the null. Write `allOf: [$ref]` with `nullable: true` instead",
			strings.Join(found, ", "))
	}
}

// The census above reads every spelling of the form it refuses: block and flow
// style, and a union that names null in any position.
func TestTheNarrowingCensusReadsEverySpelling(t *testing.T) {
	t.Parallel()
	for name, doc := range map[string]string{
		"block": "a:\n  allOf:\n    - $ref: '#/x'\n  type:\n    - object\n    - 'null'\n",
		"flow":  "a: { allOf: [{ $ref: '#/x' }], type: [object, 'null'] }\n",
		"first": "a: { allOf: [{ $ref: '#/x' }], type: ['null', object] }\n",
	} {
		var root yaml.Node
		if err := yaml.Unmarshal([]byte(doc), &root); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := narrowingCompositions(&root); len(got) != 1 {
			t.Errorf("%s spelling: found %d narrowing compositions, want 1", name, len(got))
		}
	}
	var plain yaml.Node
	if err := yaml.Unmarshal([]byte("a: { allOf: [{ $ref: '#/x' }], nullable: true }\n"), &plain); err != nil {
		t.Fatal(err)
	}
	if got := narrowingCompositions(&plain); len(got) != 0 {
		t.Errorf("the allowed nullable form was refused at %v", got)
	}
}

// narrowingCompositions names the line of every mapping that holds `allOf`
// and a `type` sequence containing null.
func narrowingCompositions(node *yaml.Node) []string {
	var found []string
	if node.Kind == yaml.MappingNode {
		composed, nullUnion := false, false
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, value := node.Content[i].Value, node.Content[i+1]
			composed = composed || key == "allOf"
			if key == "type" && value.Kind == yaml.SequenceNode {
				for _, member := range value.Content {
					nullUnion = nullUnion || member.Value == "null"
				}
			}
		}
		if composed && nullUnion {
			found = append(found, fmt.Sprintf("line %d", node.Line))
		}
	}
	for _, child := range node.Content {
		found = append(found, narrowingCompositions(child)...)
	}
	return found
}
