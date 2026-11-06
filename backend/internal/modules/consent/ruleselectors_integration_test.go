// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

func (e *channelConsentEnv) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func (e *channelConsentEnv) countryOf(t *testing.T, who ruleSubject) RecipientCountry {
	t.Helper()
	var got RecipientCountry
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		var err error
		got, err = recipientCountryTx(e.ctx, tx, who)
		return err
	}); err != nil {
		t.Fatalf("resolving the recipient's country: %v", err)
	}
	return got
}

// TestARecipientsCountryComesFromTheirRecord walks the sources in order, and
// the lead takes the same route through the record it has.
func TestARecipientsCountryComesFromTheirRecord(t *testing.T) {
	e := setupChannelConsent(t)
	contact := ruleSubject{Kind: entityContact, ID: e.contact.String()}

	e.exec(t, `UPDATE contact SET address_country = 'VN' WHERE id = $1`, e.contact)
	if got := e.countryOf(t, contact); got != (RecipientCountry{Code: "vn", Source: CountryFromContactAddress}) {
		t.Errorf("own address: got %+v, want vn from the contact's address", got)
	}

	company := ids.NewV7()
	e.exec(t, `INSERT INTO company (id, display_name, source, captured_by, address_country)
		VALUES ($1, 'Employer GmbH', 'manual', 'human:x', 'AT')`, company)
	e.exec(t, `INSERT INTO relationship (kind, contact_id, company_id, is_current_primary, source, captured_by)
		VALUES ('employment', $1, $2, true, 'manual', 'human:x')`, e.contact, company)
	e.exec(t, `UPDATE contact SET address_country = NULL WHERE id = $1`, e.contact)
	if got := e.countryOf(t, contact); got != (RecipientCountry{Code: "at", Source: CountryFromEmployerAddress}) {
		t.Errorf("no own address: got %+v, want at from the current primary employer", got)
	}

	lead := ids.NewV7()
	e.exec(t, `INSERT INTO lead (id, full_name, email, company_id, source, captured_by)
		VALUES ($1, 'Lena Lead', 'lena@lead.test', $2, 'inbound', 'human:x')`, lead, company)
	if got := e.countryOf(t, ruleSubject{Kind: entityLead, ID: lead.String()}); got != (RecipientCountry{Code: "at", Source: CountryFromLeadCompany}) {
		t.Errorf("lead: got %+v, want at from the lead's company", got)
	}
}

// TestAnAddressSuffixIsNeverACountry: an address on a country's domain, with no
// record behind it, has no country. Reading one off the domain would put a
// foreign pack on a guess.
//
// Mutation: map the domain's last label to a code in recipientCountryOf and this
// fails.
func TestAnAddressSuffixIsNeverACountry(t *testing.T) {
	e := setupChannelConsent(t)
	var got RecipientCountry
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		var err error
		got, err = recipientCountryOf(e.ctx, tx, connector.Recipient{Email: "anna@firma.vn"})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got.Known() {
		t.Errorf("an address on a .vn domain resolved to %+v", got)
	}
}

// TestTheSendingEntityPicksTheDataProtectionCountry: a sender assigned to an
// entity sends under its country; one assigned to none sends for the default
// entity, which takes the installation's.
func TestTheSendingEntityPicksTheDataProtectionCountry(t *testing.T) {
	e := setupChannelConsent(t)
	store := e.store.WithInstallationCountry(
		InstallationCountryFunc(func(context.Context, pgx.Tx) (jurisdiction.Code, error) { return "de", nil }))
	countryFor := func(sender ids.UUID) jurisdiction.Code {
		t.Helper()
		var got jurisdiction.Code
		if err := store.db.Tx(e.ctx, func(tx pgx.Tx) error {
			var err error
			got, err = store.dataProtectionCountryTx(e.ctx, tx, sender)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return got
	}

	if got := countryFor(e.user); got != "de" {
		t.Errorf("a sender with no entity sent under %q, want the default entity's de", got)
	}
	if got := countryFor(ids.Nil); got != "de" {
		t.Errorf("automatic work sent under %q, want the default entity's de", got)
	}
	entity := ids.NewV7()
	e.exec(t, `INSERT INTO legal_entity (id, display_name, legal_name, country, captured_by)
		VALUES ($1, 'Branch', 'Branch Ltd', 'vn', 'human:x')`, entity)
	e.exec(t, `INSERT INTO legal_entity_member (legal_entity_id, user_id, assigned_by)
		VALUES ($1, $2, 'human:x')`, entity, e.user)
	if got := countryFor(e.user); got != "vn" {
		t.Errorf("a sender of the vn entity sent under %q", got)
	}
}

// TestDisclosuresFollowTheirOwnSelectors drives both selectors through
// DisclosuresFor. The pack owes a controller identity (data protection) and an
// objection route on advertising (marketing).
func TestDisclosuresFollowTheirOwnSelectors(t *testing.T) {
	e := setupChannelConsent(t)
	messagingrules.Register(messagingrules.Rules{
		Jurisdiction: "zj", Version: 1,
		Disclosures: []messagingrules.Disclosure{
			{Kind: messagingrules.ControllerIdentity},
			{Kind: messagingrules.ObjectionRoute, MarketingOnly: true},
		},
	})
	stateParticulars(t, e, ControllerParticulars{
		LegalName: "Beispiel GmbH", PostalAddress: "Hauptstraße 1", ObjectionRoute: "Reply STOP.",
	})
	livesIn(t, e, "")
	senderIn := func(code jurisdiction.Code) *Store {
		return e.store.WithInstallationCountry(
			InstallationCountryFunc(func(context.Context, pgx.Tx) (jurisdiction.Code, error) { return code, nil }))
	}
	kinds := func(store *Store) []string {
		var out []string
		for _, line := range disclosures(t, e, store, commsauthz.CategoryMarketing) {
			out = append(out, line.Kind)
		}
		return out
	}

	if got := kinds(senderIn("zj")); len(got) != 1 || got[0] != string(messagingrules.ControllerIdentity) {
		t.Errorf("sender in zj, recipient with no country: owed %v, want only the controller identity", got)
	}
	e.exec(t, `UPDATE contact SET address_country = 'ZJ' WHERE id = $1`, e.contact)
	if got := kinds(senderIn("")); len(got) != 1 || got[0] != string(messagingrules.ObjectionRoute) {
		t.Errorf("sender under no pack, recipient in zj: owed %v, want only the objection route", got)
	}
}
