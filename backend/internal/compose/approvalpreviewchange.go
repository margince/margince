// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func commercialChanges(approval crmcontracts.Approval) (map[string]json.RawMessage, bool, error) {
	switch approval.Kind {
	case deals.StageProgressionKind, operationMergeTags, operationSetUpPartner, toolArchiveRecord, operationEmitFlowEvent, toolCreateRecord, singleRelinkTool, operationRelinkActivities, operationRelinkThread, operationPromoteLead, operationDemoteLead, operationSendMessage, operationInviteMeeting, operationBookMeeting, enrichProposalKind, operationDisqualifyLead, previewSendEmail, previewSendCompanyEmail, previewAdvanceDeal, previewProgressDeal, operationAdvanceProjectPhase, toolUpdateRecord, "assign_owner", lifecycleProposalKind:
	default:
		return nil, false, nil
	}
	if approval.ProposedChange == nil {
		return nil, true, nil
	}
	args, err := commercialArguments(approval)
	if err != nil {
		return nil, true, err
	}
	switch approval.Kind {
	case operationDisqualifyLead:
		return map[string]json.RawMessage{previewFieldStatus: json.RawMessage(`"disqualified"`)}, true, nil
	case operationMergeTags, operationSetUpPartner, toolArchiveRecord, operationEmitFlowEvent, toolCreateRecord, singleRelinkTool, operationRelinkActivities, operationRelinkThread, operationPromoteLead, operationDemoteLead, operationSendMessage, operationInviteMeeting, operationBookMeeting, enrichProposalKind, previewSendEmail, previewSendCompanyEmail:
		return map[string]json.RawMessage{}, true, nil
	case previewAdvanceDeal, previewProgressDeal, deals.StageProgressionKind:
		changes := map[string]json.RawMessage{fieldStageID: args["to_stage_id"]}
		keys := []string{"won_without_contract_reason", "won_without_contract_detail"}
		if approval.Kind != deals.StageProgressionKind {
			keys = append(keys, "lost_reason", bulkParamNote)
		}
		for _, key := range keys {
			if value, ok := args[key]; ok {
				changes[key] = value
			}
		}
		return changes, true, nil
	case operationAdvanceProjectPhase:
		return map[string]json.RawMessage{fieldPhase: args["to_phase"]}, true, nil
	case lifecycleProposalKind:
		return map[string]json.RawMessage{previewLifecycle: args["proposed_lifecycle"]}, true, nil
	case "assign_owner":
		owner, present := args[bulkParamOwner]
		if !present {
			return nil, true, apperrors.ErrInvalidArgument
		}
		return map[string]json.RawMessage{bulkParamOwner: owner}, true, nil
	default:
		if fields, ok := args["fields"]; ok {
			args = nil
			if err := json.Unmarshal(fields, &args); err != nil {
				return nil, true, apperrors.ErrInvalidArgument
			}
		}
		return args, true, nil
	}
}

func commercialArguments(approval crmcontracts.Approval) (map[string]json.RawMessage, error) {
	raw, err := json.Marshal(approval.ProposedChange)
	if err != nil {
		return nil, err
	}
	var args map[string]json.RawMessage
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, apperrors.ErrInvalidArgument
	}
	if _, rest := args["operation"]; rest {
		body := args["body"]
		args = nil
		if err := json.Unmarshal(body, &args); err != nil {
			return nil, apperrors.ErrInvalidArgument
		}
	}
	return args, nil
}

func commercialRequest[T any](approval crmcontracts.Approval) (T, error) {
	var empty T
	args, err := commercialArguments(approval)
	if err != nil {
		return empty, err
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return empty, err
	}
	return commandBody[T](raw)
}

func previewRouteMatches(args map[string]any, tool, recordType string, id ids.UUID, method string) bool {
	operation, valid := args["operation"].(string)
	if !valid {
		return false
	}
	path, valid := args["path"].(string)
	if !valid {
		return false
	}
	for route, policy := range agentPolicies {
		if policy.Op == operation && policy.Tool == tool && string(policy.RecordType) == recordType && strings.ReplaceAll(route, "{id}", id.String()) == method+" "+path {
			return true
		}
	}
	return false
}
