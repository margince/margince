// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import crmcontracts "github.com/margince/margince/backend/internal/contracts"

const sourceDecay = crmcontracts.WorklistItemSource("relationship_decay")

// hasReason reports whether a row already states one fact.
func hasReason(item crmcontracts.WorklistItem, kind crmcontracts.WorklistReasonKind) bool {
	for _, because := range item.Because {
		if because.Kind == kind {
			return true
		}
	}
	return false
}
