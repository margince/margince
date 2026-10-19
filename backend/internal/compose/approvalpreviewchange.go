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
	case "advance_deal", "progress_deal", "update_record", "assign_owner", "lifecycle_change":
	default:
		return nil, false, nil
	}
	if approval.ProposedChange == nil {
		return nil, true, nil
	}
	raw, err := json.Marshal(approval.ProposedChange)
	if err != nil {
		return nil, true, err
	}
	var args map[string]json.RawMessage
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, true, apperrors.ErrInvalidArgument
	}
	if _, rest := args["operation"]; rest {
		body := args["body"]
		args = nil
		if err := json.Unmarshal(body, &args); err != nil {
			return nil, true, apperrors.ErrInvalidArgument
		}
	}
	switch approval.Kind {
	case "advance_deal", "progress_deal":
		changes := map[string]json.RawMessage{"stage_id": args["to_stage_id"]}
		for _, key := range []string{"lost_reason", "won_without_contract_reason", "won_without_contract_detail", "note"} {
			if value, ok := args[key]; ok {
				changes[key] = value
			}
		}
		return changes, true, nil
	case "lifecycle_change":
		return map[string]json.RawMessage{"lifecycle": args["proposed_lifecycle"]}, true, nil
	case "assign_owner":
		return map[string]json.RawMessage{"owner_id": args["owner_id"]}, true, nil
	default:
		if fields, ok := args["fields"]; ok {
			args = nil
			if err := json.Unmarshal(fields, &args); err != nil {
				return nil, true, apperrors.ErrInvalidArgument
			}
		}
		_, covered := args["owner_id"]
		return args, covered, nil
	}
}
