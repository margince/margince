// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commsauthz

import "slices"

// RefusalRank orders refusals by what a sender must deal with first. An
// address that needs repair comes first, then the subject's own stop, then a
// footer finding, then missing evidence or permission. The preview, the review
// and the 409 all rank by it, so every surface names the same first refusal.
func RefusalRank(reasonCode string, footerFindings int) int {
	switch {
	case len(RemediesFor(reasonCode)) == 0:
		return 0
	case LevelForReason(reasonCode) == LevelSubject:
		return 1
	case reasonCode == ReasonFooterIncomplete || footerFindings > 0:
		return 2
	default:
		return 3
	}
}

// RankedForSender orders a preview's decisions for the sender. The ones that
// would refuse come first, by RefusalRank; the decision order stands within a
// rank.
func (s DecisionSet) RankedForSender() DecisionSet {
	ranked := slices.Clone(s.Decisions)
	slices.SortStableFunc(ranked, func(a, b Decision) int { return senderRank(a) - senderRank(b) })
	return DecisionSet{Decisions: ranked}
}

// senderRank places a decision that sends after every refusal.
func senderRank(d Decision) int {
	if !d.WouldRefuse(d.Mode) {
		return 4
	}
	return RefusalRank(d.ReasonCode, len(d.Evidence.FooterFindings))
}
