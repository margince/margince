// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// The subject-request queue is human-only in the contract because it is
// human-only in the store.
//
// consent's requireDSRAdmin refuses every principal that is not a HUMAN, and
// it does so before it checks admin — so an agent tool pointed at this queue
// can only ever be refused. An operation advertised on the tool surface that
// can only 403 is not merely useless: it is a door the contract is already
// holding open, and the day somebody relaxes the store gate the policy would
// let agents straight through without anyone deciding that.
//
// Two of the three operations here were marked human-only and the third was
// not, which is the drift this gate exists to catch — the miss was invisible
// because nothing compares the contract's answer with the store's.
//
// The contract is PARSED rather than pattern-matched: writing the same
// operation block-style instead of inline changes nothing about the document,
// and a regex that walked onto a neighbouring path would report a confident
// wrong answer about a list it was no longer reading.

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	dsrContract   = "api/crm.yaml"
	dsrPathPrefix = "/data-subject-requests"
	// dsrOperationFloor is what the queue declared when this gate was
	// written. A census that reads zero operations — a renamed path, a
	// restructured document — would otherwise certify nothing while passing.
	dsrOperationFloor = 3
)

func TestTheSubjectRequestQueueIsHumanOnlyInTheContract(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(dsrContract)
	if err != nil {
		t.Fatal(err)
	}
	// Loosely typed on purpose: a path item carries `parameters` (a sequence)
	// beside its verbs, which no fixed struct for an operation can hold.
	var doc struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}

	seen := 0
	for path, operations := range doc.Paths {
		if !strings.HasPrefix(path, dsrPathPrefix) {
			continue
		}
		for method, raw := range operations {
			operation, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			// `parameters` and `summary` sit beside the verbs on a path item;
			// only the entries carrying an operationId are operations.
			id, named := operation["operationId"].(string)
			if !named || id == "" {
				continue
			}
			seen++
			if access, _ := operation["x-agent-access"].(string); access != "human-only" {
				t.Errorf("%s %s (%s) is x-agent-access %q — the store refuses every non-human caller, so this can only ever 403",
					strings.ToUpper(method), path, id, access)
			}
			if operation["x-mcp-tool"] != nil {
				t.Errorf("%s %s (%s) still declares an x-mcp-tool — a human-only operation that names a tool puts it in the catalogue for agents to call and be refused",
					strings.ToUpper(method), path, id)
			}
		}
	}
	if seen < dsrOperationFloor {
		t.Fatalf("read %d operations under %s, want at least %d: this gate finds them by path prefix, so a rename leaves it certifying nothing",
			seen, dsrPathPrefix, dsrOperationFloor)
	}
}
