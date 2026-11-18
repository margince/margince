// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"slices"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// rankRefusals orders refusals by what a sender must deal with first. An
// address that needs repair comes first, then the subject's own stop, then a
// footer finding, then missing evidence or permission. The decision order
// stands within a rank. The review and the 409 both rank here.
func rankRefusals(refusals []RefusedRecipient) []RefusedRecipient {
	slices.SortStableFunc(refusals, func(a, b RefusedRecipient) int { return refusalRank(a) - refusalRank(b) })
	return refusals
}

func refusalRank(r RefusedRecipient) int {
	switch {
	case len(commsauthz.RemediesFor(r.ReasonCode)) == 0:
		return 0
	case commsauthz.LevelForReason(r.ReasonCode) == commsauthz.LevelSubject:
		return 1
	case r.ReasonCode == commsauthz.ReasonFooterIncomplete || len(r.FooterFindings) > 0:
		return 2
	default:
		return 3
	}
}
