// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"encoding/json"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// webFormSubmissionOf answers a website form note's raw payload as the fields
// it was written from. Any other row, or a raw of another shape, answers nil:
// the body still carries the same facts as text.
//
//craft:ignore naked-any raw is the activity's free-form source payload column
func webFormSubmissionOf(sourceSystem *string, raw map[string]any) *crmcontracts.WebFormSubmission {
	if sourceSystem == nil || *sourceSystem != provenance.WebFormSource || raw == nil {
		return nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var sent crmcontracts.WebFormSubmission
	if err := json.Unmarshal(encoded, &sent); err != nil || sent.Email == "" {
		return nil
	}
	return &sent
}
