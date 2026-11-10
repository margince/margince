// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// AcceptedInquiryOwner reads the occurrence's saved policy, including paused policies.
func AcceptedInquiryOwner(raw *map[string]any) (*ids.UUID, error) {
	if raw == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var receipt struct {
		Policy *ids.UUID `json:"handling_automation_id"`
	}
	if err := json.Unmarshal(encoded, &receipt); err != nil {
		return nil, err
	}
	return receipt.Policy, nil
}
