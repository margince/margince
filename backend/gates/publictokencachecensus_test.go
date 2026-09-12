// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

// The no-store census covers every route the contract publishes on the two
// anonymous token prefixes.
//
// publictokencache_integration_test.go asserts that /v1/public/preferences/*
// and /v1/public/confirm/* answer every request with Cache-Control: no-store,
// because each of their answers is derived from a bearer token sitting in
// somebody's mailbox. That test drives a fixed list of verb+route pairs, and a
// fixed list is exactly the thing that goes stale: publish a sixth pair and the
// census keeps passing while silently covering less of its subject than it
// claims. Under-recognition leaves no failing assertion behind, which is why
// this gate derives the subject from the contract instead of restating it.
//
// This gate proves COVERAGE, not the header. The header is proven by the
// integration test, against the running stack, where it is actually set.

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/shared/kernel/capabilitypath"
)

// tokenPrefixes are the anonymous surfaces whose every answer must be
// uncacheable, read from the package that decides which paths carry a
// credential rather than restated here — a second list would drift the first
// time a prefix is added there.
//
// The contract spells paths without the /v1 mount, so the prefixes are trimmed
// to match what crm.yaml actually declares.
func tokenPrefixes() []string {
	var prefixes []string
	for _, p := range capabilitypath.CredentialPrefixes() {
		// The booking page's slug is in capabilitypath because it must not be
		// logged — it is the only admission check on a route that WRITES. But
		// it is a public identifier the host hands out, not a mailed token, so
		// its answers disclose the host's availability rather than a named
		// contact's record, and no case in the census drives them.
		//
		// noStoreOnCredentialPaths does stamp them, and compose's own
		// TestEveryCredentialPathAnswersUncacheable asserts that for every
		// prefix in the list, this one included. What is deliberately not
		// claimed here is a per-route census over live booking state.
		if strings.Contains(p, "/booking/") {
			continue
		}
		prefixes = append(prefixes, strings.TrimPrefix(p, "/v1"))
	}
	return prefixes
}

// httpVerbs is the set a path item may declare. Anything outside it (parameters,
// summary) is not an operation and is not part of the census.
// The full OpenAPI 3.1 set, trace included: a verb missing from this map is a
// published operation the census silently stops seeing, which is the shape of
// under-recognition this gate exists to prevent.
var httpVerbs = map[string]bool{
	"get": true, "put": true, "post": true, "patch": true,
	"delete": true, "head": true, "options": true, "trace": true,
}

func TestTheNoStoreCensusCoversEveryPublicTokenRoute(t *testing.T) {
	t.Parallel()

	published := publishedTokenOperations(t)
	if len(published) == 0 {
		t.Fatal("the contract publishes no route under the anonymous token prefixes, so this " +
			"gate is reading the wrong subject and would pass however little the census covered")
	}

	covered := coveredTokenOperations(t)
	for _, op := range published {
		if !covered[op] {
			t.Errorf("%s is published on an anonymous token prefix but no case in "+
				"publictokencache_integration_test.go drives it, so nothing proves its answer "+
				"carries Cache-Control: no-store. Add a case naming the token state it answers in.",
				op)
		}
	}

	// The reverse direction: a case naming a route the contract no longer
	// publishes is dead weight that makes the census look larger than it is.
	for op := range covered {
		if !slicesContains(published, op) {
			t.Errorf("publictokencache_integration_test.go drives %s, which the contract no "+
				"longer publishes on an anonymous token prefix. Remove the case or restore the route.",
				op)
		}
	}
}

// publishedTokenOperations reads every verb+path the contract declares under the
// token prefixes, as "METHOD /path".
func publishedTokenOperations(t *testing.T) []string {
	t.Helper()
	var doc struct {
		Paths map[string]map[string]yaml.Node `yaml:"paths"`
	}
	raw, err := os.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing the contract: %v", err)
	}

	var ops []string
	for path, item := range doc.Paths {
		if !hasAnyPrefix(path, tokenPrefixes()) {
			continue
		}
		for verb := range item {
			if httpVerbs[strings.ToLower(verb)] {
				ops = append(ops, strings.ToUpper(verb)+" "+path)
			}
		}
	}
	sort.Strings(ops)
	return ops
}

// coveredTokenOperations reads the census file's own declaration of what it
// drives. The list is a comment block rather than the case table, because the
// table's paths carry live tokens and query strings that no static reader can
// resolve back to a contract path — a match on those would be a match on
// spelling, and the first case whose URL was built differently would read as a
// missing route.
//
// The declaration is therefore the claim, and the integration test is what makes
// it true. Both fail loudly on their own terms: this gate when the claim stops
// matching the contract, the integration test when a claimed answer loses its
// header.
func coveredTokenOperations(t *testing.T) map[string]bool {
	t.Helper()
	const census = "internal/compose/integration/publictokencache_integration_test.go"
	raw, err := os.ReadFile(census)
	if err != nil {
		t.Fatalf("reading the census: %v", err)
	}

	// Each declared pair is written as "//   COVERS: GET /public/confirm/{token}".
	declared := regexp.MustCompile(`(?m)^//\s+COVERS:\s+([A-Z]+)\s+(\S+)\s*$`)
	matches := declared.FindAllStringSubmatch(string(raw), -1)
	if len(matches) == 0 {
		t.Fatalf("%s declares no COVERS line, so this gate has nothing to compare against the "+
			"contract and would pass while covering nothing", census)
	}

	covered := make(map[string]bool, len(matches))
	for _, m := range matches {
		covered[m[1]+" "+m[2]] = true
	}
	return covered
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func slicesContains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
