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
// The count is pinned rather than zero because the form to move these to is
// open. That choice decides the generated TypeScript for all of them, and this
// holds the set from growing meanwhile.
//
// Counted as mapping keys of the parsed document, so a flow-style member counts
// and the word in a description or a YAML comment does not.

import (
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
