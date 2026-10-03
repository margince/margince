// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "github.com/margince/margince/backend/internal/shared/kernel/principal"

// agentToolEgresses maps every tool verb the agent admission table names to
// whether any operation it backs spends a cap that leaves the workspace. An
// agent call is staged under its tool verb as the approval kind.
var agentToolEgresses = func() map[string]bool {
	egresses := make(map[string]bool, len(agentPolicies))
	for _, pol := range agentPolicies {
		if pol.Tool == "" {
			continue
		}
		egresses[pol.Tool] = egresses[pol.Tool] || principal.Scope(pol.Scope).Egresses()
	}
	return egresses
}()

// undoableAgentRelease is approvals.UndoableRelease for agent-staged calls: a
// verb the admission table knows, spending no egressing cap. A webhook
// subscription is the one write-scoped record that opens a channel out — every
// later event is delivered to an address the agent chose — and the contract
// carries no marker that says so, so it is named here. An unknown kind is not
// undoable, which keeps the stricter rule for it.
func undoableAgentRelease(kind, targetType string) bool {
	if targetType == string(recordTypeWebhookSubscription) {
		return false
	}
	egresses, known := agentToolEgresses[kind]
	return known && !egresses
}
