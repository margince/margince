// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//nolint:tagliatelle // Calendar provider field names are an external wire contract.
package graphcal

import (
	"encoding/json"
)

// InvitationRequest recovers the stable request identity from a provider echo.
func InvitationRequest(eventID string, raw []byte) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	var event struct {
		RequestID string `json:"transactionId"`
	}
	if err := json.Unmarshal(raw, &event); err != nil {
		return "", err
	}
	return event.RequestID, nil
}
