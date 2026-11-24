// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// The marketing mode of a country's pack, through the real engine. The codes
// are ones no pack in the tree claims.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// Where advertising is opt-out, a recipient who has said nothing is written to,
// and any refusal on record still stops it.
//
// Mutation: drop the marketing.OptOut branch from marketingVerdict and the
// first case keeps no_marketing_consent.
func TestAnOptOutCountryAllowsAdvertisingUntilARefusal(t *testing.T) {
	testMessagingRules(t, messagingrules.Rules{Jurisdiction: "zo", Version: 1, MarketingMode: messagingrules.MarketingOptOut})

	e := setupResolve(t)
	e.seedPurpose(t, "newsletter", "marketing")
	e.livesIn(t, "zo")

	d := e.decide(t, commsauthz.Request{LegacyPurposeKey: "newsletter"})
	if d.Verdict != commsauthz.VerdictAllow || d.ReasonCode != commsauthz.ReasonAllowed {
		t.Fatalf("decision = %s %q, want advertising allowed where the recipient has not refused", d.Verdict, d.ReasonCode)
	}
	if d.Basis != "" {
		t.Errorf("basis = %q, want none: no recorded basis names allowed until refused", d.Basis)
	}

	e.suppress(t, "marketing_objection")
	d = e.decide(t, commsauthz.Request{LegacyPurposeKey: "newsletter"})
	if d.Verdict != commsauthz.VerdictDeny || d.ReasonCode != commsauthz.ReasonObjection {
		t.Fatalf("decision = %s %q, want the unsubscribe on file to refuse", d.Verdict, d.ReasonCode)
	}
}

// An opt-in pack, stated or left to its default, and a country no pack covers
// keep requiring consent, so the mode changes nothing where it is not declared.
//
// Mutation: make packRulesFor read optOut from any pack and the first case allows.
func TestAnOptInCountryStillRequiresConsent(t *testing.T) {
	testMessagingRules(t, messagingrules.Rules{Jurisdiction: "zp", Version: 1, MarketingMode: messagingrules.MarketingOptIn})
	testMessagingRules(t, messagingrules.Rules{Jurisdiction: "zy", Version: 1})
	testMessagingRules(t, messagingrules.Rules{Jurisdiction: "zo", Version: 1, MarketingMode: messagingrules.MarketingOptOut})

	for _, country := range []string{"zp", "zy", ""} {
		e := setupResolve(t)
		e.seedPurpose(t, "newsletter", "marketing")
		if country != "" {
			e.livesIn(t, country)
		}
		d := e.decide(t, commsauthz.Request{LegacyPurposeKey: "newsletter"})
		if d.Verdict == commsauthz.VerdictAllow {
			t.Errorf("country %q: advertising without consent was allowed", country)
		}
	}
}

// A message that is not advertising never reads the mode.
func TestTheMarketingModeLeavesTransactionalMailAlone(t *testing.T) {
	testMessagingRules(t, messagingrules.Rules{Jurisdiction: "zo", Version: 1, MarketingMode: messagingrules.MarketingOptOut})

	e := setupResolve(t)
	e.seedPurpose(t, "invoice", "transactional")
	e.livesIn(t, "zo")

	d := e.decide(t, commsauthz.Request{LegacyPurposeKey: "invoice"})
	if d.ReasonCode != commsauthz.ReasonLegacyTransactionalUnevidenced {
		t.Fatalf("reason = %q, want %q: an invoice with nothing behind it is not helped by the mode",
			d.ReasonCode, commsauthz.ReasonLegacyTransactionalUnevidenced)
	}
}

// The composer asks for a country only while an opt-out pack could turn the
// refusal into an allow, and the answer, once saved on the contact, does.
//
// Mutation: drop the AsksCountry assignment in decidePreviewTx and the first
// check fails.
func TestAnUnknownCountryIsAskedForAndTheAnswerChangesTheVerdict(t *testing.T) {
	testMessagingRules(t, messagingrules.Rules{Jurisdiction: "zo", Version: 1, MarketingMode: messagingrules.MarketingOptOut})

	e := setupResolve(t)
	e.seedPurpose(t, "newsletter", "marketing")

	before := e.preview(t)
	if before.Verdict == commsauthz.VerdictAllow || !before.AsksCountry {
		t.Fatalf("before the answer: %s, asks = %v; want a refusal that asks for the country", before.Verdict, before.AsksCountry)
	}

	e.livesIn(t, "zo")
	after := e.preview(t)
	if after.Verdict != commsauthz.VerdictAllow || after.AsksCountry {
		t.Fatalf("after the answer: %s, asks = %v; want an allow and no question", after.Verdict, after.AsksCountry)
	}

	e.livesIn(t, "zp")
	known := e.preview(t)
	if known.AsksCountry {
		t.Error("a recorded country that is not opt-out was asked about again")
	}
}

func (e *resolveEnv) preview(t *testing.T) commsauthz.Decision {
	t.Helper()
	set, err := e.gate.Preview(e.ctx, commsauthz.Request{
		LegacyPurposeKey: "newsletter", Recipients: []connector.Recipient{{Email: e.address}},
	})
	if err != nil {
		t.Fatalf("previewing: %v", err)
	}
	if len(set.Decisions) != 1 {
		t.Fatalf("%d decisions, want one", len(set.Decisions))
	}
	return set.Decisions[0]
}

// A lead filed under a company in an opt-out country is written to as well.
//
// Mutation: drop the packRulesFor(country).optOut test from decideLeadOnItsRecord
// and the lead keeps no_marketing_consent.
func TestALeadInAnOptOutCountryIsWrittenToUntilARefusal(t *testing.T) {
	testMessagingRules(t, messagingrules.Rules{Jurisdiction: "zo", Version: 1, MarketingMode: messagingrules.MarketingOptOut})
	e := setupLeadConsent(t)
	company := ids.NewV7()
	if _, err := e.owner.Exec(e.ctx, `INSERT INTO company (id, display_name, source, captured_by, address_country)
		VALUES ($1, 'Lead Co', 'manual', 'human:x', 'ZO')`, company); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(e.ctx, `UPDATE lead SET company_id = $2 WHERE id = $1`, e.lead, company); err != nil {
		t.Fatal(err)
	}
	tx, err := e.store.db.Pool().Begin(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rolling back: %v", err)
		}
	}()

	d, err := NewGate(e.store).decideOne(e.ctx, tx, connector.Recipient{Email: e.leadEmail},
		commsauthz.Request{LegacyPurposeKey: "newsletter"}, commsauthz.PhaseTransmit)
	if err != nil {
		t.Fatalf("deciding: %v", err)
	}
	if d.Verdict != commsauthz.VerdictAllow {
		t.Fatalf("decision = %s %q, want a lead in an opt-out country allowed", d.Verdict, d.ReasonCode)
	}
}
