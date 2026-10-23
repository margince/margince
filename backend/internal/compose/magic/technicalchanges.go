// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

import (
	"encoding/json"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
)

func technicalFieldChanges(e entry) []crmcontracts.MagicFieldChange {
	var evidence struct {
		Technical struct {
			Changes []contacts.TechnicalReceiptChange `json:"field_changes"`
		} `json:"technical"`
	}
	if err := json.Unmarshal(e.Evidence, &evidence); err != nil {
		// Legacy evidence without a readable delta cannot establish prior values.
		return nil
	}
	changes := make([]crmcontracts.MagicFieldChange, 0, len(evidence.Technical.Changes))
	for _, change := range evidence.Technical.Changes {
		changes = append(changes, crmcontracts.MagicFieldChange{
			Field: change.Field, Before: change.Before, After: change.After,
		})
	}
	return changes
}
