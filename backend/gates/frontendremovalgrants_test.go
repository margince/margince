// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The browser draws Remove on an edge from its own table of removal grants, and
// the server admits the archive from dealseat.RemovalGrants. If the two
// disagree, a seat sees a Remove the server refuses, or misses one it may use.
// This compares both, for every relationship kind the contract names.

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/dealseat"
)

const (
	frontendRemovalGrants = "../frontend/src/components/relationships.edges.tsx"
	relationshipKindsGen  = "internal/contracts/api_gen.go"
)

var (
	tsRemovalTable   = regexp.MustCompile(`(?s)const REMOVAL_GRANTS: [^=]+= \{(.*?)\};`)
	tsRemovalDefault = regexp.MustCompile(`const DEFAULT_REMOVAL_GRANTS: [^=]+= \[([^\]]*)\];`)
	tsRemovalEntry   = regexp.MustCompile(`["']?(\w+)["']?:\s*\[([^\]]*)\]`)
	tsGrantWord      = regexp.MustCompile(`["'](\w+)["']`)
	contractEdgeKind = regexp.MustCompile(`\bRelationshipKind\w+\s+RelationshipKind = "(\w+)"`)
)

func TestTheFrontendRemovalRuleMirrorsTheServers(t *testing.T) {
	t.Parallel()
	source, err := gatekit.ReadFile(frontendRemovalGrants)
	if err != nil {
		t.Fatalf("reading the frontend removal table: %v", err)
	}
	text := tsComment.ReplaceAllString(string(source), " ")
	table, fallback := tsRemovalTable.FindStringSubmatch(text), tsRemovalDefault.FindStringSubmatch(text)
	if table == nil || fallback == nil {
		t.Fatalf("%s no longer declares REMOVAL_GRANTS and DEFAULT_REMOVAL_GRANTS as literals; this gate reads a shape that is gone", frontendRemovalGrants)
	}
	inTS := map[string][]string{}
	for _, entry := range tsRemovalEntry.FindAllStringSubmatch(table[1], -1) {
		inTS[entry[1]] = grantWords(entry[2])
	}
	if len(inTS) == 0 {
		t.Fatal("no entries parsed out of REMOVAL_GRANTS; a gate that reads nothing agrees with everything")
	}

	generated, err := gatekit.ReadFile(relationshipKindsGen)
	if err != nil {
		t.Fatalf("reading the contract's relationship kinds: %v", err)
	}
	kinds := map[string]bool{}
	for _, m := range contractEdgeKind.FindAllStringSubmatch(string(generated), -1) {
		kinds[m[1]] = true
	}
	if !kinds["deal_stakeholder"] || len(kinds) < 5 {
		t.Fatalf("read %d relationship kinds out of %s, which cannot be the contract's list", len(kinds), relationshipKindsGen)
	}
	for kind := range kinds {
		browser, listed := inTS[kind]
		if !listed {
			browser = grantWords(fallback[1])
		}
		server := make([]string, 0, 2)
		for _, action := range dealseat.RemovalGrants(kind) {
			server = append(server, string(action))
		}
		slices.Sort(server)
		if !slices.Equal(browser, server) {
			t.Errorf("%s: the server removes with %v and the browser draws Remove for %v", kind, server, browser)
		}
	}
	for kind := range inTS {
		if !kinds[kind] {
			t.Errorf("REMOVAL_GRANTS names %q, which is no relationship kind of the contract", kind)
		}
	}
}

// grantWords reads a TypeScript list of grant names, sorted.
func grantWords(list string) []string {
	var out []string
	for _, m := range tsGrantWord.FindAllStringSubmatch(list, -1) {
		out = append(out, strings.ToLower(m[1]))
	}
	slices.Sort(out)
	return out
}
