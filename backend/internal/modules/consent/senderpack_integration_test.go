// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// The sending entity's data-protection pack beside the recipient's. The pack
// pair from recipientpack_integration_test.go stands in: one pack without
// legitimate interests, one with them.

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
)

// installedIn wires the installation's country, which the default entity
// takes, onto the env's store.
func installedIn(e *resolveEnv, code *jurisdiction.Code) {
	e.gate = e.gate.WithInstallationCountry(
		InstallationCountryFunc(func(context.Context, pgx.Tx) (jurisdiction.Code, error) { return *code, nil }))
}

// TestTheSendersPackWithoutLegitimateInterestsBindsEveryRecipient: a reply
// rests on legitimate interests under the recipient's own pack. Sent from an
// entity whose pack has none, the same reply has no basis, also to a recipient
// whose country is unknown.
//
// Mutation: read applicableRules(subject.Country.Code) in settlePackBasisTx and
// the second and third checks fail.
func TestTheSendersPackWithoutLegitimateInterestsBindsEveryRecipient(t *testing.T) {
	p := registerPackPair(t)
	e := setupResolve(t)
	sender := p.installation
	installedIn(e, &sender)
	placeContact(t, e.owner, e.contact, p.installation)
	e.capturedMail(t, "thread-matter", "cc", true)
	request := commsauthz.Request{ThreadKey: "thread-matter"}

	if d := e.decide(t, request); d.LawfulBasisMissing || d.Basis == "" {
		t.Fatalf("both packs grant legitimate interests, decision = %+v, want a basis", d)
	}
	sender = p.recipient
	if d := e.decide(t, request); !d.LawfulBasisMissing {
		t.Errorf("the sender's pack grants no legitimate interests, decision = %+v, want the basis marked missing", d)
	}
	if _, err := e.owner.Exec(context.Background(), `UPDATE contact SET address_country = NULL WHERE id = $1`, e.contact); err != nil {
		t.Fatal(err)
	}
	if d := e.decide(t, request); !d.LawfulBasisMissing {
		t.Errorf("a recipient with no country, decision = %+v, want the sender's pack to mark the basis missing", d)
	}
}

// TestTheWarningCanBeConfirmedForAStricterEntityThanTheDefault: the default
// entity's pack grants legitimate interests, a further entity's does not. The
// confirmation names no entity, so it is kept for a send from the stricter one.
//
// Mutation: judge the confirmation under the default entity alone and the
// second confirmation is dropped.
func TestTheWarningCanBeConfirmedForAStricterEntityThanTheDefault(t *testing.T) {
	p := registerPackPair(t)
	e := setupResolve(t)
	home := p.installation
	installedIn(e, &home)
	placeContact(t, e.owner, e.contact, p.installation)
	e.capturedMail(t, "thread-matter", "cc", true)
	e.ctx = withActivityCreate(e.ctx)
	subject := []ConfirmedSubject{{Kind: entityContact, ID: e.contact.UUID}}

	if got, err := e.store.ConfirmSendWithoutLawfulBasis(e.ctx, subject); err != nil || len(got) != 0 {
		t.Fatalf("one entity granting legitimate interests confirmed %v (%v), want nothing to confirm", got, err)
	}
	branch := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO legal_entity (id, display_name, legal_name, country, jurisdiction, captured_by)
		VALUES ($1, 'Branch', 'Branch Ltd', 'zz', $2, 'human:x')`, branch, string(p.recipient)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO legal_entity_member (legal_entity_id, user_id, assigned_by)
		VALUES ($1, $2, 'human:x')`, branch, e.user); err != nil {
		t.Fatal(err)
	}
	if got, err := e.store.ConfirmSendWithoutLawfulBasis(e.ctx, subject); err != nil || len(got) != 1 {
		t.Errorf("confirming with a stricter entity in the installation kept %v (%v), want the record", got, err)
	}
}

// TestTransmitJudgesUnderTheSenderCountryStagingRecorded: an entity whose
// country changes while the message waits does not change the law the message
// is judged under at transmit.
//
// Mutation: drop the staged sender in transmitCountries and this fails.
func TestTransmitJudgesUnderTheSenderCountryStagingRecorded(t *testing.T) {
	e := setupResolve(t)
	installation := jurisdiction.Code("de")
	installedIn(e, &installation)
	delivery := e.plantDelivery(t)
	stageDelivery(t, e, delivery)

	var recorded string
	if err := e.owner.QueryRow(context.Background(), `
		SELECT evidence->>'sender_country' FROM communication_decision
		 WHERE delivery_id = $1 AND phase = 'staging' LIMIT 1`, delivery).Scan(&recorded); err != nil {
		t.Fatalf("reading the staged sender: %v", err)
	}
	if recorded != "de" {
		t.Fatalf("staging recorded sender country %q, want de", recorded)
	}

	installation = "vn"
	countries, err := e.gate.transmitCountries(e.ctx, commsauthz.TransmitRequest{
		DeliveryID: delivery, Recipients: []connector.Recipient{{Email: e.address}},
	}, false)
	if err != nil {
		t.Fatalf("resolving the transmit countries: %v", err)
	}
	if len(countries) != 1 || countries[0].Sender != "de" {
		t.Errorf("transmit judged under sender %+v, want the staged de", countries)
	}
}

// TestTheLawsInForceAreTheEntitiesJurisdictionsNotTheirAddresses: a further
// entity registered at an address no pack covers still puts its jurisdiction's
// pack in force for the particulars.
func TestTheLawsInForceAreTheEntitiesJurisdictionsNotTheirAddresses(t *testing.T) {
	p := registerPackPair(t)
	e := setupResolve(t)
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO legal_entity (id, display_name, legal_name, country, jurisdiction, captured_by)
		VALUES ($1, 'Branch', 'Branch Ltd', 'zz', $2, 'human:x')`, ids.NewV7(), string(p.recipient)); err != nil {
		t.Fatal(err)
	}
	tx, err := e.owner.Begin(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(context.Background()); err != nil {
			t.Error(err)
		}
	}()
	codes, err := e.store.countriesInForceTx(e.ctx, tx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(codes, p.recipient) || slices.Contains(codes, "zz") {
		t.Errorf("laws in force = %v, want the branch's jurisdiction %q and not its address country", codes, p.recipient)
	}
}
