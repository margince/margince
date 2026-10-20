// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func commercialChanges(approval crmcontracts.Approval) (map[string]json.RawMessage, bool, error) {
	switch approval.Kind {
	case previewAdvanceDeal, previewProgressDeal, toolUpdateRecord, "assign_owner", lifecycleProposalKind:
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
	case previewAdvanceDeal, previewProgressDeal:
		changes := map[string]json.RawMessage{fieldStageID: args["to_stage_id"]}
		for _, key := range []string{"lost_reason", "won_without_contract_reason", "won_without_contract_detail", bulkParamNote} {
			if value, ok := args[key]; ok {
				changes[key] = value
			}
		}
		return changes, true, nil
	case lifecycleProposalKind:
		return map[string]json.RawMessage{previewLifecycle: args["proposed_lifecycle"]}, true, nil
	case "assign_owner":
		return map[string]json.RawMessage{bulkParamOwner: args[bulkParamOwner]}, true, nil
	default:
		if fields, ok := args["fields"]; ok {
			args = nil
			if err := json.Unmarshal(fields, &args); err != nil {
				return nil, true, apperrors.ErrInvalidArgument
			}
		}
		_, covered := args[bulkParamOwner]
		return args, covered, nil
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
