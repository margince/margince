// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The SPA's table of routes that hold a model call open must be the contract's.
//
// The client counts a request to one of these routes as the agent working from
// the moment it leaves, which is what lights the AI-activity rail before the
// feed can carry the run; and it gives a proxy's failure on one a "the work may
// still be running" body. Both readers want exactly the operations whose
// handler calls a model and WAITS, and the contract says which those are
// (`x-waits-on-model`). The table was a hand-kept list of path suffixes for a
// long time, and it drifted in both directions without anything failing: it
// named a route that calls a data provider and no model, and it missed the
// meeting brief — a GET that runs two model calls on every open — so a person
// waited on the agent for the whole of it with the chrome reporting rest. That
// is the one failure this surface cannot show: an agent at rest looks exactly
// like an agent nobody is counting.
//
// The contract is the owner and the table its declared mirror, compared in
// both directions and on the value: a route marked there and absent here, a
// route here the contract does not mark, and an `always` on one side against
// an `on-miss` on the other all fail. The marker itself is a claim about a
// handler that this gate does not read — the same standing `x-agent-access`
// has — so the marker's own doc comment says to read the handler first.

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	modelRouteContract = "api/crm.yaml"
	modelRouteClient   = "../frontend/src/api/client.ts"
	modelRouteMarker   = "x-waits-on-model"
)

// modelWaitValues is the marker's closed vocabulary, which the client's own
// `ModelWait` union spells the same way: `always` counts the request the moment
// it leaves, `on-miss` once it has outlived a stored answer.
var modelWaitValues = map[string]bool{"always": true, "on-miss": true}

// tsModelRouteEntry reads one `"METHOD /path": "wait"` pair out of the table.
// The key is always quoted — it has a space in it — so there is one spelling to
// match, and the value's quotes are optional for the reason the sibling
// minor-unit gate gives: an entry the parser cannot see is one this gate
// silently agrees with.
var tsModelRouteEntry = regexp.MustCompile(`"([A-Z]+ /[^"]+)":\s*["']?([a-z-]+)["']?`)

func TestTheClientsModelRouteTableIsTheContracts(t *testing.T) {
	t.Parallel()
	inContract := contractModelRoutes(t)
	inClient := clientModelRoutes(t)

	for route, want := range inContract {
		got, present := inClient[route]
		switch {
		case !present:
			t.Errorf("%s is %s: %s in the contract and absent from %s, so a person waiting on it sees the rail report an agent at rest", route, modelRouteMarker, want, modelRouteClient)
		case got != want:
			t.Errorf("%s: the contract says %s, the client says %s — the two disagree about when the reader is waiting on the agent", route, want, got)
		}
	}
	for route, got := range inClient {
		if _, present := inContract[route]; !present {
			t.Errorf("%s is in %s as %q and the contract does not mark it %s — either mark the operation (after reading its handler) or drop the entry", route, modelRouteClient, got, modelRouteMarker)
		}
	}
}

// contractModelRoutes reads the marked operations out of the contract, keyed
// the way the client spells a route, with the marker's value.
func contractModelRoutes(t *testing.T) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(modelRouteContract)
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	// Loosely typed on purpose: a path item carries `parameters` beside its
	// verbs, which no fixed struct for an operation can hold.
	var doc struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing the contract: %v", err)
	}
	marked := map[string]string{}
	for path, item := range doc.Paths {
		for method, raw := range item {
			operation, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			value, present := operation[modelRouteMarker]
			if !present {
				continue
			}
			route := strings.ToUpper(method) + " " + path
			wait, isString := value.(string)
			if !isString || !modelWaitValues[wait] {
				t.Errorf("%s carries %s: %v, which is not one of always | on-miss", route, modelRouteMarker, value)
				continue
			}
			marked[route] = wait
		}
	}
	if len(marked) == 0 {
		t.Fatalf("no operation in %s carries %s — a gate that reads nothing agrees with everything", modelRouteContract, modelRouteMarker)
	}
	return marked
}

// clientModelRoutes is the client's table, read out of its source: the constant
// is module-private and exported by nothing, which is right for the client and
// leaves the text as the one thing this gate can read.
func clientModelRoutes(t *testing.T) map[string]string {
	t.Helper()
	source, err := os.ReadFile(modelRouteClient)
	if err != nil {
		t.Fatalf("reading the client: %v", err)
	}
	const marker = "MODEL_ROUTES"
	start := indexAfter(string(source), marker+": Readonly<Record<string, ModelWait>> = {")
	if start < 0 {
		t.Fatalf("%s no longer declares %s as an object literal — this gate is reading a shape that is gone", modelRouteClient, marker)
	}
	end := indexAfter(string(source)[start:], "};")
	if end < 0 {
		t.Fatalf("%s's %s literal is unterminated", modelRouteClient, marker)
	}
	table := tsComment.ReplaceAllString(string(source)[start:start+end], " ")
	entries := map[string]string{}
	for _, m := range tsModelRouteEntry.FindAllStringSubmatch(table, -1) {
		entries[m[1]] = m[2]
	}
	if len(entries) == 0 {
		t.Fatalf("no entries parsed out of %s's %s — a gate that reads nothing agrees with everything", modelRouteClient, marker)
	}
	return entries
}
