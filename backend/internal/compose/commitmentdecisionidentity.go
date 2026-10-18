// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "strings"

func resolvedPriorPromise(c Commitment, offer CommitmentTaskProposal) bool {
	if c.Span == "" || (c.Seat == nil && c.Theirs == nil) || offer.SourceActivityID != c.SourceActivityID {
		return false
	}
	matches := func(party string) bool {
		return offer.Locator == commitmentLocator(c.SourceActivityID, claimKindOurs, party, c.Span)
	}
	if matches("us") || matches("name:") || matches(unnamedPromiserKey("")) {
		return true
	}
	if !matches(unnamedPromiserKey(offer.Party)) {
		return false
	}
	reported, resolved := strings.TrimSpace(offer.Party), strings.TrimSpace(c.Party)
	if strings.EqualFold(reported, resolved) {
		return true
	}
	for _, word := range strings.Fields(resolved) {
		if strings.EqualFold(reported, word) {
			return true
		}
	}
	return false
}
