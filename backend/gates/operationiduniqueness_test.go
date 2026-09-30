// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

//go:build !integration

package gates

// No two operations share an operationId.
//
// OpenAPI already requires it. Nothing in this tree held it, and eight gates
// key on it: each builds a map from operationId to the thing it is checking,
// and a duplicate silently collapses two operations into one. The map keeps
// whichever the walk reached last, the other is never examined, and every one
// of those gates reports PASS over the operation it dropped.
//
// That is the one direction a census must not break — it reads a smaller tree,
// reports PASS, and there is no failing assertion to notice. So the precondition
// belongs HERE, once, over the contract that owns it, rather than re-asserted in
// each consumer: a check repeated in a census reads to the next author as though
// the source gate does not exist.
//
// The generator is downstream of this and not covered by it. A unique contract
// can still produce a table with a repeated key if the emitter repeats one, and
// only a gate comparing the generator's input to its output can see that.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// operationDocument is one document that declares operations, and where they came
// from — the path is what a failure has to name, because the two halves of a
// collision are usually in different files.
type operationDocument struct {
	path string
	raw  []byte
}

// operationSite is one declaration of one operationId.
type operationSite struct {
	file   string
	path   string
	method string
}

func (s operationSite) String() string {
	return fmt.Sprintf("%s (%s %s)", s.file, strings.ToUpper(s.method), s.path)
}

// wantMinimumOperations is the floor beneath the count of operations this
// census finds, for the reason every floor in this directory has one: a walk
// that stops recognising the contract's shape finds nothing, and a corpus of
// zero has no duplicate in it. The contract carries several hundred; this is
// low enough never to need touching and high enough to catch a walk that reads
// only one document or only one key.
const wantMinimumOperations = 200

func TestNoTwoOperationsShareAnOperationID(t *testing.T) {
	t.Parallel()
	sites := map[string][]operationSite{}
	for _, file := range contractDocuments(t) {
		found := operationsIn(t, file)
		if len(found) == 0 {
			t.Errorf("%s declares no operation at all — this census read a document whose shape it does not "+
				"recognise, and a document it cannot read is one it cannot find a collision in", file.path)
			continue
		}
		for id, where := range found {
			sites[id] = append(sites[id], where...)
		}
	}

	total := 0
	for _, where := range sites {
		total += len(where)
	}
	if total < wantMinimumOperations {
		t.Fatalf("this census found %d operations across the contract, below the %d floor: it is reading less "+
			"than the tree declares, and a corpus that short has no collision in it to find",
			total, wantMinimumOperations)
	}

	for _, id := range sortedIDs(sites) {
		where := sites[id]
		if len(where) < 2 {
			continue
		}
		t.Errorf("operationId %q is declared %d times — %s. Two operations under one id collapse into one "+
			"wherever the contract is read by id, and every gate that does so then reports PASS over the one "+
			"it dropped rather than failing. Rename one",
			id, len(where), joinSites(where))
	}
}

// contractDocuments collects the documents that declare operations on the HTTP
// contract: the core one, and each extension's overlay of it.
//
// Discovered rather than listed. An extension is enabled by its presence under
// `extensions/`, so a listed corpus would leave the next unit's operations
// unexamined until somebody remembered to add it — and a collision between an
// extension and core is the likeliest one of all, since neither author reads
// the other's file.
func contractDocuments(t *testing.T) []operationDocument {
	t.Helper()
	// contractFile, not a second spelling of the same path: this package
	// already names the core contract, and two gates disagreeing about where
	// it lives is the shape that leaves one of them censusing nothing.
	paths := []string{contractFile}
	overlays, err := filepath.Glob(filepath.Join("..", "extensions", "*", "api", "crm.yaml"))
	if err != nil {
		t.Fatalf("looking for extension contract overlays: %v", err)
	}
	sort.Strings(overlays)
	paths = append(paths, overlays...)

	out := make([]operationDocument, 0, len(paths))
	for _, path := range paths {
		raw, err := os.ReadFile(path) //nolint:gosec // a path this test derived from the tree it censuses
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		out = append(out, operationDocument{path: path, raw: raw})
	}
	return out
}

// operationsIn reads one document's operation declarations, in either of the
// two shapes this tree writes: the core contract's `paths`, and an extension
// overlay's `actions[].update` against a JSONPath target.
func operationsIn(t *testing.T, file operationDocument) map[string][]operationSite {
	t.Helper()
	found := map[string][]operationSite{}
	for path, methods := range pathItemsIn(t, file) {
		for method, node := range methods {
			// A path item's own `parameters` sequence sits beside its methods
			// and is not an operation. Decoding it as one would fail the parse
			// rather than skip it, which is why the methods arrive as nodes.
			if method == "parameters" {
				continue
			}
			var op struct {
				OperationID string `yaml:"operationId"` //nolint:tagliatelle // OpenAPI's key, not ours to rename
			}
			if err := node.Decode(&op); err != nil {
				t.Fatalf("reading %s %s out of %s: %v", strings.ToUpper(method), path, file.path, err)
			}
			if op.OperationID == "" {
				continue
			}
			found[op.OperationID] = append(found[op.OperationID],
				operationSite{file: file.path, path: path, method: method})
		}
	}
	return found
}

// pathItemsIn resolves one document to path → method → operation, whichever
// shape it is written in.
func pathItemsIn(t *testing.T, file operationDocument) map[string]map[string]yaml.Node {
	t.Helper()
	var doc struct {
		Paths   map[string]map[string]yaml.Node `yaml:"paths"`
		Actions []struct {
			Target string               `yaml:"target"`
			Update map[string]yaml.Node `yaml:"update"`
			Remove bool                 `yaml:"remove"`
		} `yaml:"actions"`
	}
	if err := yaml.Unmarshal(file.raw, &doc); err != nil {
		t.Fatalf("parsing %s: %v", file.path, err)
	}
	if len(doc.Paths) > 0 {
		return doc.Paths
	}
	items := map[string]map[string]yaml.Node{}
	for _, action := range doc.Actions {
		// A `remove` action takes an operation away rather than declaring one,
		// and carries no `update` to read.
		if action.Remove || len(action.Update) == 0 {
			continue
		}
		path := overlayPath(action.Target)
		if items[path] == nil {
			items[path] = map[string]yaml.Node{}
		}
		// MERGED, not assigned. Two actions may target one path — an overlay
		// that adds a GET in one action and a POST in another is ordinary, and
		// so is a second overlay extending a path the first already touched.
		// Assigning here would drop every method but the last, and this census
		// would then report PASS over an operation it never read.
		for method, node := range action.Update {
			items[path][method] = node
		}
	}
	return items
}

// overlayPath reads the route out of an overlay target so a failure names the
// path a reader can find, rather than the JSONPath expression wrapping it.
// A target this does not recognise is reported as written, which is more useful
// than an empty string.
func overlayPath(target string) string {
	open := strings.Index(target, "['")
	close := strings.LastIndex(target, "']")
	if open < 0 || close <= open {
		return target
	}
	return target[open+2 : close]
}

func sortedIDs(sites map[string][]operationSite) []string {
	ids := make([]string, 0, len(sites))
	for id := range sites {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func joinSites(where []operationSite) string {
	parts := make([]string, 0, len(where))
	for _, site := range where {
		parts = append(parts, site.String())
	}
	sort.Strings(parts)
	return strings.Join(parts, " and ")
}

// What the census reads, on documents small enough to see whole.
//
// The census above runs over the real contract, where every case it must
// recognise is one a contributor happens to have written. These are the cases
// it has to survive whether anybody writes them or not — and the third is the
// one that made this test worth having: with the overlay's actions ASSIGNED
// rather than merged, a second action on the same path silently replaced the
// first, the census read one operation where two were declared, and it reported
// PASS over a planted collision. Falsified in exactly that shape.
func TestTheOperationCensusReadsBothContractShapes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		yaml string
		want []string
	}{{
		name: "the core contract's paths, with a path item's own parameters beside them",
		yaml: `
paths:
  /v1/deals:
    parameters:
      - $ref: '#/components/parameters/Sort'
    get:
      operationId: listDeals
    post:
      operationId: createDeal
`,
		want: []string{"createDeal", "listDeals"},
	}, {
		name: "an extension overlay's actions",
		yaml: `
overlay: 1.0.0
actions:
  - target: $.paths['/ext/u/quote']
    update:
      get:
        operationId: uQuote
`,
		want: []string{"uQuote"},
	}, {
		name: "two actions targeting one path, which is two operations and not one",
		yaml: `
overlay: 1.0.0
actions:
  - target: $.paths['/ext/u/quote']
    update:
      get:
        operationId: uQuote
  - target: $.paths['/ext/u/quote']
    update:
      delete:
        operationId: uWithdrawQuote
`,
		want: []string{"uQuote", "uWithdrawQuote"},
	}, {
		name: "a remove action, which takes an operation away rather than declaring one",
		yaml: `
overlay: 1.0.0
actions:
  - target: $.paths['/v1/deals']
    remove: true
`,
		want: nil,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			found := operationsIn(t, operationDocument{path: tc.name, raw: []byte(tc.yaml)})
			got := sortedIDs(found)
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("read %v, want %v — an operation this census cannot see is one it cannot find a "+
					"collision in, and it says so by passing", got, tc.want)
			}
		})
	}
}
