// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package workflow

import (
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func AcceptedInquiryOwner(raw *map[string]any) (*ids.UUID, error) {
	if raw == nil {
		return nil, nil
	}
	value, present := (*raw)["handling_automation_id"]
	if !present || value == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var owner *ids.UUID
	if err := json.Unmarshal(encoded, &owner); err != nil {
		return nil, err
	}
	return owner, nil
}
