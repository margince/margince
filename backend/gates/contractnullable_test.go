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
// open on #8407. That choice decides the generated TypeScript for all of them.
//
// What this holds meanwhile is that the set stops growing. It stood at nine
// when that issue was filed and reads higher now.
//
// Derived from the document, not a list of fields. A list grows by hand, and
// the field added without touching it is the one this catches.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// nullableInContract is how many `nullable:` members api/crm.yaml holds today.
// It may go down and never up.
const nullableInContract = 59

var nullableMember = regexp.MustCompile(`(?m)^\s*nullable:\s`)

func TestTheContractDeclaresOpenAPI31AndNullableIsInert(t *testing.T) {
	raw, err := os.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	doc := string(raw)

	// The pin means nothing in a 3.0 document, where the keyword is real.
	if !strings.Contains(doc, "openapi: 3.1") {
		t.Fatal("the contract no longer declares OpenAPI 3.1: this census is about a keyword " +
			"that version dropped, so it has to be re-read before it is trusted")
	}

	found := len(nullableMember.FindAllString(doc, -1))
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
