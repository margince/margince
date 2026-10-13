// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"errors"

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
			"channel":{"description":"email_reply answers a recorded mail thread, new_email starts one filed on links, chat_reply answers a captured chat"}}}`),
	}
}

func (sendOutbound) routeField() string { return "channel" }

func (sendOutbound) routeTargets() map[string]string {
	return map[string]string{"email_reply": "send_email", "new_email": "send_company_email", "chat_reply": "send_message"}
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
			"kind":{"description":"email drafts one message; follow_ups drafts one per deal in a segment"}}}`),
	}
}

func (draftOutbound) routeField() string { return "kind" }

func (draftOutbound) routeTargets() map[string]string {
	return map[string]string{"email": "draft_email", "follow_ups": "draft_follow_ups_for"}
}

// runImport fronts the two writing steps of an import: the dry run and the
// commit of the run it produced.
type runImport struct{ routed }

func (runImport) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "run_import", Title: "Import a spreadsheet", Version: toolVersionV1,
		Description: runImportCopy.render(),
		Instead:     runImportCopy.Instead,
		InputSchema: schema(`{"type":"object","properties":{
			"step":{"description":"preview checks the rows and writes nothing; commit writes a previewed run by run_id"}}}`),
	}
}

func (runImport) routeField() string { return "step" }

func (runImport) routeTargets() map[string]string {
	return map[string]string{"preview": "preview_import", "commit": "commit_import"}
}

// resolveDuplicates fronts settling a flagged pair and merging two records.
// The ids a call names say which it is, so there is no field to choose by,
// and a view can call it with either tool's own arguments.
type resolveDuplicates struct{ routed }

func (resolveDuplicates) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "resolve_duplicates", Title: "Merge two records or dismiss a flagged pair", Version: toolVersionV1,
		Description: resolveDuplicatesCopy.render(),
		Instead:     resolveDuplicatesCopy.Instead,
		InputSchema: schema(`{"type":"object","properties":{
			"candidate_id":{"description":"A flagged pair's candidate_id, to dismiss or reopen it; leave out when merging"},
			"source_id":{"description":"The record merged away (archived, redirected to the survivor); leave out with candidate_id"}}}`),
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
	"name candidate_id to settle a flagged pair, or source_id and target_id to merge, not both")}

func (resolveDuplicates) chooseTarget(members map[string]json.RawMessage) (string, error) {
	dropNullMembers(members, "candidate_id", "source_id", "target_id")
	_, pair := members["candidate_id"]
	_, source := members["source_id"]
	_, target := members["target_id"]
	switch {
	case pair && !source && !target:
		return settleFlaggedPair, nil
	case !pair && (source || target):
		return mergeTwoRecords, nil
	}
	return "", errResolveNamesOneWay
}
