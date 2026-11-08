// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

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

// livesIn places the env's contact in a country by its postal address.
func (e *resolveEnv) livesIn(t *testing.T, code string) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE contact SET address_country = upper($2) WHERE id = $1`, e.contact, code); err != nil {
		t.Fatalf("placing the contact in %s: %v", code, err)
	}
}

// Marketing without consent to a recipient whose country requires prior
// consent is refused with a reason nobody sends past, an admin included. A
// recipient in a country that keeps the exception keeps the refusal it
// answers. Codes no pack in the tree claims, so a real pack cannot collide.
//
// Mutation: drop the requireConsentWhereNothingStandsIn call from the contact
// arm of decideOneIn and the consent-only cases keep no_marketing_consent.
func TestNoExceptionStandsInForMarketingConsentWhereTheCountryRequiresIt(t *testing.T) {
	messagingrules.Register(messagingrules.Rules{Jurisdiction: "zq", Version: 1, ConsentOnly: true})
	messagingrules.Register(messagingrules.Rules{Jurisdiction: "zr", Version: 1})

	for _, tc := range []struct {
		name, country string
		req           commsauthz.Request
		want          string
	}{
		{"legacy marketing, consent only", "zq", commsauthz.Request{LegacyPurposeKey: "newsletter"}, commsauthz.ReasonMarketingConsentRequired},
		{"claimed marketing, consent only", "zq", commsauthz.Request{Context: commsauthz.CategoryMarketing}, commsauthz.ReasonMarketingConsentRequired},
		{"legacy marketing, exception kept", "zr", commsauthz.Request{LegacyPurposeKey: "newsletter"}, commsauthz.ReasonNoMarketingConsent},
		{"legacy marketing, no country", "", commsauthz.Request{LegacyPurposeKey: "newsletter"}, commsauthz.ReasonNoMarketingConsent},
		{"claimed marketing, exception kept", "zr", commsauthz.Request{Context: commsauthz.CategoryMarketing}, commsauthz.ReasonNoEvidence},
		// An unproven follow-up is a missing fact, not missing consent: the rep
		// records it, so it keeps its own reason in every country.
		{"unproven follow-up, consent only", "zq", commsauthz.Request{Context: commsauthz.CategoryRequestedFollowup}, commsauthz.ReasonNoEvidence},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := setupResolve(t)
			e.seedPurpose(t, "newsletter", "marketing")
			if tc.country != "" {
				e.livesIn(t, tc.country)
			}
			d := e.decide(t, tc.req)
			if d.Verdict == commsauthz.VerdictAllow || d.ReasonCode != tc.want {
				t.Fatalf("decision = %s %q, want a refusal for %q", d.Verdict, d.ReasonCode, tc.want)
			}
			byAdmin := (commsauthz.DecisionSet{Decisions: []commsauthz.Decision{d}}).ExceptionMayAnswer(true)
			if wantException := tc.want != commsauthz.ReasonMarketingConsentRequired; byAdmin != wantException {
				t.Errorf("an admin's exception answers it = %v, want %v", byAdmin, wantException)
			}
		})
	}
}

// A standing override for marketing does not stand in for consent either.
//
// Mutation: move the requireConsentWhereNothingStandsIn call below the override
// arm and the override allows the send.
func TestAnOverrideDoesNotStandInForConsentWhereTheCountryRequiresIt(t *testing.T) {
	messagingrules.Register(messagingrules.Rules{Jurisdiction: "zs", Version: 1, ConsentOnly: true})
	e := setupResolve(t)
	e.seedPurpose(t, "newsletter", "marketing")
	e.livesIn(t, "zs")
	e.seedOverride(t, "marketing", false)

	d := e.decide(t, commsauthz.Request{LegacyPurposeKey: "newsletter"})
	if d.Verdict == commsauthz.VerdictAllow || d.ReasonCode != commsauthz.ReasonMarketingConsentRequired {
		t.Fatalf("decision = %s %q, want marketing refused for want of the consent the country requires",
			d.Verdict, d.ReasonCode)
	}
}

// A lead filed under a company in such a country is held to the same rule.
//
// Mutation: drop the requireConsentWhereNothingStandsIn call from the lead arm
// of decideOneIn and the lead keeps no_marketing_consent.
func TestALeadIsHeldToConsentWhereTheCountryRequiresIt(t *testing.T) {
	messagingrules.Register(messagingrules.Rules{Jurisdiction: "zt", Version: 1, ConsentOnly: true})
	e := setupLeadConsent(t)
	company := ids.NewV7()
	if _, err := e.owner.Exec(e.ctx, `INSERT INTO company (id, display_name, source, captured_by, address_country)
		VALUES ($1, 'Lead Co', 'manual', 'human:x', 'ZT')`, company); err != nil {
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
	if d.ReasonCode != commsauthz.ReasonMarketingConsentRequired {
		t.Fatalf("reason = %q, want %q for a lead in a consent-only country", d.ReasonCode, commsauthz.ReasonMarketingConsentRequired)
	}
}
