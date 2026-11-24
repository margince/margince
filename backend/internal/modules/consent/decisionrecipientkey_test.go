// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The stored key of an address is the form the cap count compares, so two
// spellings of one mailbox are one key. A channel id keeps its case.
func TestADecisionKeysAnAddressTheWayTheCapCountsIt(t *testing.T) {
	t.Parallel()
	typed := connector.Recipient{Email: " Ada@Example.COM "}
	if got, want := decisionRecipientKey(typed), normalizeCapAddress(typed.Email); got != want || got != "ada@example.com" {
		t.Errorf("an address is keyed %q, want the cap's own form %q", got, want)
	}
	channel := connector.Recipient{Channel: &connector.ChannelIdentity{Provider: "telegram", ChannelUserID: "AbC123"}}
	if got := decisionRecipientKey(channel); got != "telegram:AbC123" {
		t.Errorf("a channel id is keyed %q, want it as supplied", got)
	}
}

// A delivery staged by a release that stored the address as typed still finds
// its claim when it is transmitted.
func TestAStagedClaimIsFoundUnderTheSpellingItWasStoredIn(t *testing.T) {
	t.Parallel()
	r := connector.Recipient{Email: "Ada@Example.com"}
	stored := map[string]stagedClaim{"Ada@Example.com": {purposeKey: "typed"}}
	if got := stagedClaimOf(stored, r).purposeKey; got != "typed" {
		t.Errorf("the claim stored as typed read %q, want it found", got)
	}
	folded := map[string]stagedClaim{"ada@example.com": {purposeKey: "folded"}, "Ada@Example.com": {purposeKey: "typed"}}
	if got := stagedClaimOf(folded, r).purposeKey; got != "folded" {
		t.Errorf("the claim read %q, want the folded key to win", got)
	}
}
