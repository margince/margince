// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// TechnicalReceiptChange keeps the durable audit format independent of enrichment inputs.
type TechnicalReceiptChange struct {
	Field  string `json:"field"`
	Before string `json:"before"`
	After  string `json:"after"`
}

func technicalReceiptChanges(changes []TechnicalChange) []TechnicalReceiptChange {
	receipts := make([]TechnicalReceiptChange, 0, len(changes))
	for _, change := range changes {
		after := change.Value
		if change.Kind == TechnicalGone {
			after = ""
		}
		receipts = append(receipts, TechnicalReceiptChange{Field: change.Field, Before: change.Previous, After: after})
	}
	return receipts
}
