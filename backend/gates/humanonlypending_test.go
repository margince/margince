// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H1

package gates

// An agent may do what its user may do in the app, unless security says no.
// Every human-only operation a signed-in user reaches says why in the contract,
// as x-human-only-reason: security, covered by another tool, or pending, which
// tools/gen-agentpolicy requires. This holds the count still waiting to be opened.

import "testing"

// pendingHumanOnlyCeiling is the number of operations a user reaches in the
// app that their agent cannot reach yet. It equals today's count, with no
// headroom. A new operation cannot join the backlog unnoticed, and each change
// that opens operations to agents lowers it.
const pendingHumanOnlyCeiling = 256

const pendingReason = "pending"

func TestNoOperationJoinsThePendingHumanOnlyBacklog(t *testing.T) {
	t.Parallel()
	declared := humanOnlyOperationsInContract(t)
	if len(declared) == 0 {
		t.Fatal("no operation in api/crm.yaml is human-only, so a pending count of zero would prove nothing")
	}
	pending := 0
	for _, op := range contractOperations(t) {
		if op["x-agent-access"] == humanOnlyAccess && op["x-human-only-reason"] == pendingReason {
			pending++
		}
	}
	switch {
	case pending > pendingHumanOnlyCeiling:
		t.Errorf("%d human-only operations are pending, over the ceiling of %d.\n\tA new operation a user "+
			"reaches in the app must reach their agent too: give it x-mcp-tool, or declare why security keeps "+
			"it from agents (x-human-only-reason: security).", pending, pendingHumanOnlyCeiling)
	case pending < pendingHumanOnlyCeiling:
		t.Errorf("%d human-only operations are pending, under the ceiling of %d.\n\tLower "+
			"pendingHumanOnlyCeiling to %d in this change, so the opened operations cannot close again "+
			"unnoticed.", pending, pendingHumanOnlyCeiling, pending)
	}
}

// contractOperations reads the operation objects of api/crm.yaml.
func contractOperations(t *testing.T) []map[string]any {
	t.Helper()
	paths, ok := loadContract(t)["paths"].(map[string]any)
	if !ok {
		t.Fatal("api/crm.yaml carries no paths")
	}
	var ops []map[string]any
	for _, item := range paths {
		operations, ok := item.(map[string]any)
		if !ok {
			continue
		}
		for method, raw := range operations {
			if op, ok := raw.(map[string]any); ok && httpMethods[method] {
				ops = append(ops, op)
			}
		}
	}
	return ops
}
