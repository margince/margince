// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// A day-count bound published in the contract and enforced in Go is ONE rule
// with two spellings, so it is held here.
//
// The two halves fail in opposite and equally quiet directions. Widen the Go
// constant alone and the server accepts a span its own generated clients refuse
// to send, so the capability exists and nothing can reach it. Widen the
// contract alone and a client sends a span the server rejects, which reads to a
// caller as the feature being broken rather than as the request being out of
// range.
//
// Neither shows up anywhere else: the generated types carry `maximum` as prose
// in a doc comment, and no validator applies it — a fact the response-window
// handler already says out loud, which is why it re-checks the bound itself.
//
// Derived from the contract rather than listed, so a THIRD bounded day count
// added later either joins this table or is visibly absent from it.

import (
	"os"
	"regexp"
	"strconv"
	"testing"
)

// dayCaps names each published day bound and where the Go constant that must
// equal it is declared. The schema is where the number is published; the
// constant is where the refusal is made.
//
// The Go value is READ from source rather than repeated here. A number typed
// into this table would be a third copy of the same rule, and a gate holding
// two spellings together by introducing a third is not holding anything.
func dayCaps() []struct {
	schema   string
	file     string
	constant string
} {
	return []struct {
		schema   string
		file     string
		constant string
	}{
		{
			schema:   "DismissRelationshipNudgeRequest",
			file:     "internal/modules/people/nudgedismissal.go",
			constant: "nudgeDismissalMaxDays",
		},
	}
}

// constantValue reads one untyped integer constant out of a Go source file.
func constantValue(t *testing.T, file, name string) int {
	t.Helper()
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	found := regexp.MustCompile(`(?m)^const ` + regexp.QuoteMeta(name) + ` = (\d+)$`).
		FindStringSubmatch(string(raw))
	if found == nil {
		t.Fatalf("%s declares no `const %s = <integer>` — this gate's subject moved",
			file, name)
	}
	n, err := strconv.Atoi(found[1])
	if err != nil {
		t.Fatalf("%s declares %s as %q: %v", file, name, found[1], err)
	}
	return n
}

// maximumUnder finds the `maximum:` published under one schema name.
//
// A line scan rather than a YAML parse, and the anchor is the schema's own
// declaration line: the contract is one file of forty thousand lines, so a
// search for `maximum:` alone would find every bound in it and a parse would
// pull a dependency in for one integer.
func maximumUnder(t *testing.T, contract, schema string) int {
	t.Helper()
	at := regexp.MustCompile(`(?m)^    ` + regexp.QuoteMeta(schema) + `:$`).
		FindStringIndex(contract)
	if at == nil {
		t.Fatalf("schema %q is not declared in the contract — this gate's subject moved", schema)
	}
	// Bounded to the schema's own block: the next line at the same indent ends
	// it, so a `maximum` belonging to the schema after this one cannot be read
	// as this one's.
	rest := contract[at[1]:]
	if end := regexp.MustCompile(`(?m)^    [A-Za-z]`).FindStringIndex(rest); end != nil {
		rest = rest[:end[0]]
	}
	found := regexp.MustCompile(`(?m)^\s+maximum:\s*(\d+)\s*$`).FindStringSubmatch(rest)
	if found == nil {
		t.Fatalf("schema %q publishes no maximum, so the Go constant bounds a range "+
			"no client is told about", schema)
	}
	n, err := strconv.Atoi(found[1])
	if err != nil {
		t.Fatalf("schema %q publishes a maximum of %q: %v", schema, found[1], err)
	}
	return n
}

func TestPublishedDayCapsMatchTheConstantsThatRefuse(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatal(err)
	}
	contract := string(raw)

	for _, bound := range dayCaps() {
		t.Run(bound.constant, func(t *testing.T) {
			t.Parallel()

			published := maximumUnder(t, contract, bound.schema)
			enforced := constantValue(t, bound.file, bound.constant)
			if published != enforced {
				t.Errorf("%s publishes a maximum of %d and %s refuses past %d — "+
					"a client sending %d is told it may and is then refused",
					bound.schema, published, bound.constant, enforced, published)
			}
		})
	}
}
