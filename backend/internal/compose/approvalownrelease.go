// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// releaseTarget is one tool verb aimed at one record type — the pair an agent
// call is staged under, as the approval's kind and target type.
type releaseTarget struct {
	tool       string
	recordType agentRecordType
}

// agentStraightThrough maps every (verb, record type) the admission table
// names to whether EVERY route behind it auto-executes and spends no cap that
// leaves the workspace. Such a call was staged only because a human had edited
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

// filingGuard answers whether filings under a project could still be undone by
// a member, for the activities the call names. The activities store holds the
// predicate the undo itself refuses by.
type filingGuard interface {
	FilingsStayUndoable(ctx context.Context, activityIDs []ids.UUID, project ids.UUID) (bool, error)
}

// undoableAgentRelease is approvals.UndoableRelease for agent-staged calls. A
// tool whose tier turns on its arguments is judged by where THIS call resolves,
// because the policy's static "dynamic" says nothing about it and its target
// type may be the destination rather than the verb's record. A pair the
// admission table does not name — an unknown kind, or no target type — is not
// undoable, which keeps the stricter rule for it.
//
// A filing under a project is undoable by a member only while the activity is
// not restricted, held or under an open erasure request, so that destination is
// also judged by the state of the activities the call names; without a guard it
// is not undoable.
func undoableAgentRelease(guard filingGuard) approvals.UndoableRelease {
	return func(ctx context.Context, kind, targetType string, change json.RawMessage) bool {
		if undoable, decided := agents.ReleaseUndoableByDestination(kind, change); decided {
			return undoable && filingsStayUndoable(ctx, guard, change)
		}
		return agentStraightThrough[releaseTarget{kind, agentRecordType(targetType)}]
	}
}

// relinkedActivities is the slice of a staged relink the state check reads: the
// destination and the activities, named singly or as a set.
type relinkedActivities struct {
	ActivityID  *ids.UUID  `json:"activity_id"`
	ActivityIDs []ids.UUID `json:"activity_ids"`
	EntityType  string     `json:"entity_type"`
	EntityID    ids.UUID   `json:"entity_id"`
}

// filingsStayUndoable is true for every destination but a project, and for a
// project when the guard says the undo could still take each filing back.
func filingsStayUndoable(ctx context.Context, guard filingGuard, change json.RawMessage) bool {
	var call relinkedActivities
	if err := json.Unmarshal(change, &call); err != nil {
		return false
	}
	if call.EntityType != "project" {
		return true
	}
	if guard == nil {
		return false
	}
	named := call.ActivityIDs
	if call.ActivityID != nil {
		named = append(named, *call.ActivityID)
	}
	stay, err := guard.FilingsStayUndoable(ctx, named, call.EntityID)
	return err == nil && stay
}
