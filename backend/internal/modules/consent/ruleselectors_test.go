// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// TestTheRecipientCountryIsTheFirstCodeTheRecordStates covers the order of the
// sources and what counts as a stated country.
func TestTheRecipientCountryIsTheFirstCodeTheRecordStates(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		candidates []RecipientCountry
		want       RecipientCountry
	}{
		{"the contact's own address, stored upper case",
			[]RecipientCountry{{Code: "DE", Source: CountryFromContactAddress}, {Code: "VN", Source: CountryFromEmployerAddress}},
			RecipientCountry{Code: "de", Source: CountryFromContactAddress}},
		{"a free-text country is skipped, never mapped to a code",
			[]RecipientCountry{{Code: "Germany", Source: CountryFromContactAddress}, {Code: "VN", Source: CountryFromEmployerAddress}},
			RecipientCountry{Code: "vn", Source: CountryFromEmployerAddress}},
		{"no stated country is unknown", nil, RecipientCountry{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := firstUsableCountry(tc.candidates); got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestNoCodeAndAnUncoveredCodeBothResolveToTheBaseline: the baseline is the
// answer ok=false, for a recipient whose country is unknown and for one whose
// country no pack covers.
func TestNoCodeAndAnUncoveredCodeBothResolveToTheBaseline(t *testing.T) {
	t.Parallel()
	if _, _, ok := applicableRules(); ok {
		t.Error("no code resolved to a pack")
	}
	if _, _, ok := applicableRules(""); ok {
		t.Error("an empty code resolved to a pack")
	}
	if _, applied, ok := applicableRules("yq"); ok || len(applied.Codes) != 0 {
		t.Errorf("a code no pack declares resolved to %+v", applied)
	}
}

// TestEachSelectorContributesItsOwnDisclosures pins the split. Who the
// controller is and who answers about the data come from the sender's pack.
// The advertising obligations come from the recipient's.
func TestEachSelectorContributesItsOwnDisclosures(t *testing.T) {
	t.Parallel()
	declared := []messagingrules.Disclosure{
		{Kind: messagingrules.ControllerIdentity},
		{Kind: messagingrules.PrivacyContact},
		{Kind: messagingrules.ObjectionRoute, MarketingOnly: true},
		{Kind: messagingrules.AdvertiserContact, MarketingOnly: true},
	}
	sender := appendOwed(nil, declared, true)
	if len(sender) != 2 || sender[0].Kind != messagingrules.ControllerIdentity || sender[1].Kind != messagingrules.PrivacyContact {
		t.Errorf("the sender's pack contributed %+v, want the controller identity and the privacy contact", sender)
	}
	recipient := appendOwed(nil, declared, false)
	if len(recipient) != 2 || recipient[0].Kind != messagingrules.ObjectionRoute || recipient[1].Kind != messagingrules.AdvertiserContact {
		t.Errorf("a recipient's pack contributed %+v, want the objection route and the advertiser contact", recipient)
	}
}

// TestTwoRecipientsOweOneLinePerKind: one body goes to both, so a kind both
// packs declare is owed once, and on every message if either pack says so.
func TestTwoRecipientsOweOneLinePerKind(t *testing.T) {
	t.Parallel()
	owed := appendOwed(nil, []messagingrules.Disclosure{{Kind: messagingrules.ObjectionRoute, MarketingOnly: true}}, false)
	owed = appendOwed(owed, []messagingrules.Disclosure{{Kind: messagingrules.ObjectionRoute}}, false)
	if len(owed) != 1 {
		t.Fatalf("owed %+v, want one objection route", owed)
	}
	if owed[0].MarketingOnly {
		t.Error("a pack owing the route on every message was narrowed to advertising by the other recipient's pack")
	}
}
