// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"errors"
	"slices"

	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// sendOutbound fronts the three sends. Each stays its own tool because each is
// its own approval kind and its own tier floor.
type sendOutbound struct{ routed }

func (sendOutbound) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "send_outbound", Title: "Send a message", Version: toolVersionV1,
		Description: sendOutboundCopy.render(),
		Instead:     sendOutboundCopy.Instead,
		InputSchema: schema(`{"type":"object","properties":{
			"as":{"description":"email_reply answers a recorded mail thread, email_new starts one filed on links, chat_reply answers a captured chat"}}}`),
	}
}

func (sendOutbound) routeField() string { return "as" }

func (sendOutbound) routeTargets() map[string]string {
	return map[string]string{"email_reply": "send_email", "email_new": "send_company_email", "chat_reply": "send_message"}
}

// draftOutbound fronts drafting one message and drafting a follow-up per
// slipping deal.
type draftOutbound struct{ routed }

func (draftOutbound) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "draft_outbound", Title: "Draft an email or follow-ups", Version: toolVersionV1,
		Description: draftOutboundCopy.render(),
		Instead:     draftOutboundCopy.Instead,
		InputSchema: schema(`{"type":"object","properties":{
			"for":{"description":"one_message drafts one email; slipping_deals drafts a follow-up per slipping deal"}}}`),
	}
}

func (draftOutbound) routeField() string { return "for" }

func (draftOutbound) routeTargets() map[string]string {
	return map[string]string{"one_message": "draft_email", "slipping_deals": "draft_follow_ups_for"}
}

// resolveDuplicates fronts settling a flagged pair and merging two records.
// The ids a call names say which it is. A view can therefore call it with
// either tool's own arguments.
type resolveDuplicates struct{ routed }

func (resolveDuplicates) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "resolve_duplicates", Title: "Merge two records or dismiss a flagged pair", Version: toolVersionV1,
		Description: resolveDuplicatesCopy.render(),
		Instead:     resolveDuplicatesCopy.Instead,
		InputSchema: schema(`{"type":"object","properties":{
			"candidate_id":{"description":"A flagged pair's candidate_id, to dismiss or reopen it; leave out when merging"},
			"source_id":{"description":"The record merged away (archived, redirected to the survivor); leave out with candidate_id"},
			"member_ids":{"description":"The contacts of one case that are one person; leave out with candidate_id"}}}`),
	}
}

const (
	settleFlaggedPair = "decide_duplicate"
	mergeTwoRecords   = "merge_records"
)

func (resolveDuplicates) routeTargets() map[string]string {
	return map[string]string{"dismiss": settleFlaggedPair, "merge": mergeTwoRecords}
}

var errResolveNamesOneWay = &BadArgsError{Cause: errors.New(
	"name candidate_id to settle a flagged pair, source_id and target_id to merge, " +
		"or contact_id and member_ids to merge a contact case, one of them")}

// chooseTarget sends a pair dismissal to decide_duplicate, and both merges, of
// two records or of a contact case, to merge_records.
func (resolveDuplicates) chooseTarget(members map[string]json.RawMessage) (string, error) {
	dropNullMembers(members, "candidate_id", "source_id", "target_id", "contact_id", "member_ids")
	_, pair := members["candidate_id"]
	named := func(keys ...string) bool {
		return slices.ContainsFunc(keys, func(key string) bool { _, ok := members[key]; return ok })
	}
	merge := named("source_id", "target_id", "contact_id", "member_ids")
	switch {
	case pair && !merge:
		return settleFlaggedPair, nil
	case !pair && merge:
		return mergeTwoRecords, nil
	}
	return "", errResolveNamesOneWay
}
