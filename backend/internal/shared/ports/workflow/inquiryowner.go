// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package workflow

import (
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// AcceptedInquiryOwner reads the rule that accepted a website inquiry from the
// activity's raw payload. Nil means no rule has accepted it yet.
func AcceptedInquiryOwner(raw *map[string]any) (*ids.UUID, error) {
	if raw == nil {
		return nil, nil //nolint:nilnil // an activity with no payload has no accepting rule, and that is no failure
	}
	value, present := (*raw)["handling_automation_id"]
	if !present || value == nil {
		return nil, nil //nolint:nilnil // a payload that names no rule is an inquiry nobody accepted yet
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
