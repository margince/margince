// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"slices"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// rankRefusals orders refusals by commsauthz.RefusalRank. The decision order
// stands within a rank. The review and the 409 both rank here.
func rankRefusals(refusals []RefusedRecipient) []RefusedRecipient {
	slices.SortStableFunc(refusals, func(a, b RefusedRecipient) int { return refusalRank(a) - refusalRank(b) })
	return refusals
}

func refusalRank(r RefusedRecipient) int {
	return commsauthz.RefusalRank(r.ReasonCode, len(r.FooterFindings))
}
