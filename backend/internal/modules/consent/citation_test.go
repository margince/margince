// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"sync"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

var (
	citedOn   = time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	reviewDue = citedOn.AddDate(1, 0, 0)
	lawEnds   = citedOn.AddDate(0, 6, 0)
	lawStarts = citedOn.AddDate(0, 2, 0)
	// reviewTomorrow dates a review the day after the lookups below that ask
	// before it.
	reviewTomorrow = citedOn.AddDate(0, 1, 1)
)

// citingPacks registers two packs that cite the same refusal code under
// different law, so a lookup that ignored the recipient would answer wrong.
// Once per process: the registry refuses a second registration.
var citingPacks = sync.OnceValue(func() [3]jurisdiction.Code {
	reviewed := messagingrules.Provenance{SourceCheckedAt: citedOn, ReviewedAt: citedOn, ReviewDue: reviewDue, Reviewer: "pack maintainers"}
	instruments := []messagingrules.Instrument{
		{Name: "Act A", Provision: "§1", SourceURL: "https://example.org/a"},
		{Name: "Act B", SourceURL: "https://example.org/b", ExpiresFrom: lawEnds},
		{Name: "Act C", SourceURL: "https://example.org/c", EffectiveFrom: lawStarts},
	}
	messagingrules.Register(messagingrules.Rules{
		Jurisdiction: "ka", Version: 1, Instruments: instruments, Provenance: reviewed,
		Citations: []messagingrules.Citation{
			{Code: commsauthz.ReasonFrequencyCapReached, Instrument: "Act A"},
			{Code: DraftForeignAdvertiser, Territory: messagingrules.SenderElsewhere, Instrument: "Act A", Provision: "§9"},
			{Code: commsauthz.ReasonNoMarketingConsent, Instrument: "Act B", Provision: "§2"},
			{Code: commsauthz.ReasonConsentWithdrawn, Instrument: "Act C", Provision: "§3"},
		},
	})
	messagingrules.Register(messagingrules.Rules{
		Jurisdiction: "kb", Version: 1, Instruments: instruments, Provenance: reviewed,
		Citations: []messagingrules.Citation{
			{Code: commsauthz.ReasonFrequencyCapReached, Instrument: "Act A", Provision: "§5"},
		},
	})
	tomorrow := reviewed
	tomorrow.ReviewedAt = reviewTomorrow
	messagingrules.Register(messagingrules.Rules{
		Jurisdiction: "kd", Version: 1, Instruments: instruments, Provenance: tomorrow,
		Citations: []messagingrules.Citation{{Code: commsauthz.ReasonFrequencyCapReached, Instrument: "Act A"}},
	})
	return [3]jurisdiction.Code{"ka", "kb", "kd"}
})

func TestACitationIsResolvedAgainstTheRecipientsPackAndSender(t *testing.T) {
	t.Parallel()
	packs := citingPacks()
	a, b, reviewedLater := packs[0], packs[1], packs[2]
	soon := citedOn.AddDate(0, 1, 0)
	for _, tc := range []struct {
		name      string
		code      string
		where     citationContext
		want      citationOutcome
		provision string
	}{
		{
			"the instrument's own provision fills an empty one", commsauthz.ReasonFrequencyCapReached,
			citationContext{Recipient: RecipientCountry{Code: a}, At: soon},
			citationShown, "§1",
		},
		{
			"the same code cites other law in another market", commsauthz.ReasonFrequencyCapReached,
			citationContext{Recipient: RecipientCountry{Code: b}, At: soon},
			citationShown, "§5",
		},
		{
			"a recipient no pack covers has none", commsauthz.ReasonFrequencyCapReached,
			citationContext{Recipient: RecipientCountry{Code: "yq"}, At: soon},
			citationNotDeclared, "",
		},
		{
			"a code the pack does not cite has none", commsauthz.ReasonHardBounce,
			citationContext{Recipient: RecipientCountry{Code: a}, At: soon},
			citationNotDeclared, "",
		},
		{
			"a sender established here is not cited as foreign", DraftForeignAdvertiser,
			citationContext{Recipient: RecipientCountry{Code: a}, Sender: a, At: soon},
			citationNotDeclared, "",
		},
		{
			"a sender established elsewhere is", DraftForeignAdvertiser,
			citationContext{Recipient: RecipientCountry{Code: a}, Sender: b, At: soon},
			citationShown, "§9",
		},
		{
			"an unknown sender counts as elsewhere, as the finding does", DraftForeignAdvertiser,
			citationContext{Recipient: RecipientCountry{Code: a}, At: soon},
			citationShown, "§9",
		},
		{
			"an overdue review withholds the pack's citations", commsauthz.ReasonFrequencyCapReached,
			citationContext{Recipient: RecipientCountry{Code: a}, At: reviewDue.Add(time.Second)},
			citationReviewOverdue, "",
		},
		{
			"a review due this instant is not yet overdue", commsauthz.ReasonFrequencyCapReached,
			citationContext{Recipient: RecipientCountry{Code: a}, At: reviewDue},
			citationShown, "§1",
		},
		{
			"an expired law withholds its own citation", commsauthz.ReasonNoMarketingConsent,
			citationContext{Recipient: RecipientCountry{Code: a}, At: lawEnds},
			citationLawExpired, "",
		},
		{
			"a law in force is cited", commsauthz.ReasonNoMarketingConsent,
			citationContext{Recipient: RecipientCountry{Code: a}, At: soon},
			citationShown, "§2",
		},
		{
			"a review dated tomorrow approves nothing today", commsauthz.ReasonFrequencyCapReached,
			citationContext{Recipient: RecipientCountry{Code: reviewedLater}, At: soon},
			citationNotYetReviewed, "",
		},
		{
			"the same pack cites once its review instant has come", commsauthz.ReasonFrequencyCapReached,
			citationContext{Recipient: RecipientCountry{Code: reviewedLater}, At: reviewTomorrow},
			citationShown, "§1",
		},
		{
			"a law not yet in force is not", commsauthz.ReasonConsentWithdrawn,
			citationContext{Recipient: RecipientCountry{Code: a}, At: soon},
			citationNotInForce, "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, outcome := citationFor(tc.code, tc.where)
			if outcome != tc.want || got.Provision != tc.provision {
				t.Errorf("got %q %+v, want %q with provision %q", outcome, got, tc.want, tc.provision)
			}
			if outcome == citationShown && got.SourceURL == "" {
				t.Errorf("a shown citation carries no source: %+v", got)
			}
		})
	}
}
