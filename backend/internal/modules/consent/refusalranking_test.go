// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// A sender is told first about what they must repair, then about the
// subject's own stop, then about the footer, then about missing evidence.
// Within one rank the decision order stands.
func TestRefusalsAreRankedByWhatTheSenderMustDealWithFirst(t *testing.T) {
	refusals := rankRefusals([]RefusedRecipient{
		{Address: "no-consent@a.test", ReasonCode: commsauthz.ReasonNoMarketingConsent},
		{Address: "footer@a.test", ReasonCode: commsauthz.ReasonFooterIncomplete},
		{Address: "no-evidence@a.test", ReasonCode: commsauthz.ReasonNoEvidence},
		{Address: "objected@a.test", ReasonCode: commsauthz.ReasonObjection},
		{Address: "bounced@a.test", ReasonCode: commsauthz.ReasonHardBounce},
		{Address: "withdrew@a.test", ReasonCode: commsauthz.ReasonConsentWithdrawn},
	})
	got := make([]string, 0, len(refusals))
	for _, r := range refusals {
		got = append(got, r.Address)
	}
	want := []string{
		"bounced@a.test", "objected@a.test", "withdrew@a.test", "footer@a.test",
		"no-consent@a.test", "no-evidence@a.test",
	}
	if !slices.Equal(got, want) {
		t.Errorf("ranked %v, want %v", got, want)
	}
}
