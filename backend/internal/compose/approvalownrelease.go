// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "github.com/margince/margince/backend/internal/shared/kernel/principal"

// releaseTarget is one tool verb aimed at one record type — the pair an agent
// call is staged under, as the approval's kind and target type.
type releaseTarget struct {
	tool       string
	recordType agentRecordType
}

// agentStraightThrough maps every (verb, record type) the admission table
// names to whether EVERY route behind it auto-executes and spends no cap that
// leaves the workspace. Such a call was staged only because a person had edited
// a field it touched; any other was staged because its own route asks a human
// first — a deal close, a relink, a tag merge, a schema change, a webhook, a
// send — and stays that human's to release.
var agentStraightThrough = func() map[releaseTarget]bool {
	straight := make(map[releaseTarget]bool, len(agentPolicies))
	for _, pol := range agentPolicies {
		if pol.Tool == "" || pol.RecordType == "" {
			continue
		}
		key := releaseTarget{pol.Tool, pol.RecordType}
		allSoFar, seen := straight[key]
		route := pol.Tier == tierAutoExecute && !principal.Scope(pol.Scope).Egresses()
		straight[key] = route && (allSoFar || !seen)
	}
	return straight
}()

// undoableAgentRelease is approvals.UndoableRelease for agent-staged calls. A
// pair the admission table does not name — an unknown kind, or no target type
// — is not undoable, which keeps the stricter rule for it.
func undoableAgentRelease(kind, targetType string) bool {
	return agentStraightThrough[releaseTarget{kind, agentRecordType(targetType)}]
}
