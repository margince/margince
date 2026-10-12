// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"errors"

	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// decideApprovals fronts deciding one staged action and deciding a bundle. The
// id a call names says which it is, so there is no field to choose by.
type decideApprovals struct{ routed }

func (decideApprovals) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "decide_approvals", Title: "Approve or reject staged actions", Version: toolVersionV1,
		Description: decideApprovalsCopy.render(),
		Instead:     decideApprovalsCopy.Instead,
		InputSchema: schema(`{"type":"object","properties":{
			"staged_action_id":{"description":"One staged action; leave out when naming bundle_id"},
			"bundle_id":{"description":"Every still-waiting proposal one act staged; leave out when naming staged_action_id"},
			"reason":{"description":"Why, in the decider's own words; with bundle_id it is recorded against every member"}}}`),
	}
}

const (
	decideOne    = "decide_approval"
	decideBundle = "decide_approval_bundle"
)

func (decideApprovals) routeTargets() map[string]string {
	return map[string]string{"one": decideOne, "bundle": decideBundle}
}

var errDecideNamesOneID = &BadArgsError{Cause: errors.New("name exactly one of staged_action_id or bundle_id")}

// chooseTarget treats a null id as absent, as the surface does everywhere, and
// drops it: the target refuses an argument it does not take.
func (decideApprovals) chooseTarget(members map[string]json.RawMessage) (string, error) {
	for _, id := range []string{"staged_action_id", "bundle_id"} {
		if isNull(members[id]) {
			delete(members, id)
		}
	}
	_, one := members["staged_action_id"]
	_, bundle := members["bundle_id"]
	switch {
	case one && !bundle:
		return decideOne, nil
	case bundle && !one:
		return decideBundle, nil
	}
	return "", errDecideNamesOneID
}
