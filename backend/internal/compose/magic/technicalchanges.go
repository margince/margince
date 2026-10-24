// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

import (
	"encoding/json"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
)

// technicalFieldChanges is what a lookup's receipt recorded as moved. A
// lookup writes no company column, so its images are empty and the fields
// live in the evidence. An older receipt without them yields nothing rather
// than a guessed earlier value.
func technicalFieldChanges(e entry) []crmcontracts.MagicFieldChange {
	var evidence struct {
		Technical struct {
			Changes []contacts.TechnicalReceiptChange `json:"field_changes"`
		} `json:"technical"`
	}
	if json.Unmarshal(e.Evidence, &evidence) != nil {
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
