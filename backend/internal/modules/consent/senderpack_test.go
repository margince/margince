// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
	"github.com/margince/margince/backend/pkg/extension/messaging"
)

// senderPacks registers a recipient pack that grants legitimate interests and
// an existing-customer exception, and a sender pack that grants neither and
// declares a shorter reply window. Both cite the lawful-basis finding and a
// marketing finding, each under its own law.
var senderPacks = sync.OnceValue(func() [2]jurisdiction.Code {
	instruments := []messagingrules.Instrument{
		{Name: "Recipient Act", SourceURL: "https://example.org/recipient"},
		{Name: "Sender Act", SourceURL: "https://example.org/sender"},
	}
	messagingrules.Register(messagingrules.Rules{
		Jurisdiction: "kr", Version: 1, Instruments: instruments,
		MarketingExceptions: []messagingrules.MarketingException{{Kind: messaging.ExistingCustomer, RequiresSaleEvidence: true}},
		Citations: []messagingrules.Citation{
			{Code: commsauthz.ReasonNoEvidence, Instrument: "Recipient Act", Provision: "Art. 6"},
			{Code: commsauthz.ReasonObjection, Instrument: "Recipient Act", Provision: "Art. 21"},
		},
	})
	messagingrules.Register(messagingrules.Rules{
		Jurisdiction: "ks", Version: 1, Instruments: instruments,
		NoLegitimateInterests: true, ReplyWindow: 30 * 24 * time.Hour,
		Citations: []messagingrules.Citation{
			{Code: commsauthz.ReasonNoEvidence, Instrument: "Sender Act", Provision: "Art. 9"},
			{Code: commsauthz.ReasonObjection, Instrument: "Sender Act", Provision: "Art. 11"},
		},
	})
	return [2]jurisdiction.Code{"kr", "ks"}
})

func TestTheSendersPackBindsTheLawfulBasisBesideTheRecipients(t *testing.T) {
	t.Parallel()
	packs := senderPacks()
	recipient, sender := packs[0], packs[1]

	cases := []struct {
		name    string
		country RecipientCountry
		noLI    bool
	}{
		{"recipient alone grants legitimate interests", RecipientCountry{Code: recipient}, false},
		{"a sender without them takes them away", RecipientCountry{Code: recipient, Sender: sender}, true},
		{"an unknown recipient is judged under the sender's pack", RecipientCountry{Sender: sender}, true},
		{"the same country is one pack", RecipientCountry{Code: recipient, Sender: recipient}, false},
	}
	for _, tc := range cases {
		rules, _, found := applicableRules(tc.country.dataProtectionCodes()...)
		if !found || rules.NoLegitimateInterests != tc.noLI {
			t.Errorf("%s: found=%v NoLegitimateInterests=%v, want found and %v", tc.name, found, rules.NoLegitimateInterests, tc.noLI)
		}
	}
}

func TestTheSendersPackShortensTheWindowButGrantsNoMarketingException(t *testing.T) {
	t.Parallel()
	packs := senderPacks()
	alone := packRulesFor(RecipientCountry{Code: packs[0]})
	both := packRulesFor(RecipientCountry{Code: packs[0], Sender: packs[1]})
	if alone.reply != defaultReplyWindow || both.reply != 30*24*time.Hour {
		t.Errorf("reply window: alone %v, with sender %v; want the default, then the sender's 30 days", alone.reply, both.reply)
	}
	if both.marketingException == nil {
		t.Error("the recipient's existing-customer exception was dropped: the sender's marketing rules do not follow its mail")
	}
	if packRulesFor(RecipientCountry{Sender: packs[1]}).marketingException != nil {
		t.Error("a recipient with no country got the sender's marketing exception")
	}
}

func TestAFindingCitesTheSendersLawOnlyWhereItsDataProtectionSettlesIt(t *testing.T) {
	t.Parallel()
	packs := senderPacks()
	recipient, sender := packs[0], packs[1]
	law := func(instrument, provision string) commsauthz.Provision {
		url := "https://example.org/recipient"
		if instrument == "Sender Act" {
			url = "https://example.org/sender"
		}
		return commsauthz.Provision{Instrument: instrument, Provision: provision, SourceURL: url}
	}
	at := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)

	both := provisionsShown(citationContext{Recipient: RecipientCountry{Code: recipient}, Sender: sender, At: at})
	if want := []commsauthz.Provision{law("Recipient Act", "Art. 6"), law("Sender Act", "Art. 9")}; !slices.Equal(both[commsauthz.ReasonNoEvidence], want) {
		t.Errorf("lawful-basis finding cites %v, want the recipient's law then the sender's %v", both[commsauthz.ReasonNoEvidence], want)
	}
	if want := []commsauthz.Provision{law("Recipient Act", "Art. 21")}; !slices.Equal(both[commsauthz.ReasonObjection], want) {
		t.Errorf("marketing finding cites %v, want the recipient's law alone %v", both[commsauthz.ReasonObjection], want)
	}

	unknown := provisionsShown(citationContext{Sender: sender, At: at})
	if want := []commsauthz.Provision{law("Sender Act", "Art. 9")}; !slices.Equal(unknown[commsauthz.ReasonNoEvidence], want) {
		t.Errorf("unknown recipient: lawful-basis finding cites %v, want the sender's law %v", unknown[commsauthz.ReasonNoEvidence], want)
	}
	if got := unknown[commsauthz.ReasonObjection]; len(got) != 0 {
		t.Errorf("unknown recipient: marketing finding cites the sender's marketing law %v", got)
	}

	home := provisionsShown(citationContext{Recipient: RecipientCountry{Code: sender}, Sender: sender, At: at})
	if got := home[commsauthz.ReasonNoEvidence]; len(got) != 1 {
		t.Errorf("sender and recipient in one country cite %v, want its law once", got)
	}
}
