// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The legal entity an offer is issued by. It defaults to the author's entity,
// a draft may change it and a sent offer may not. The send freezes what the
// document prints about it.

import (
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedVienna describes a further entity with every field an offer prints, and
// assigns the rep to it.
func seedVienna(t *testing.T, e *Env) contacts.LegalEntity {
	t.Helper()
	admin := e.Admin()
	vienna, err := e.Contacts.CreateLegalEntity(admin, contacts.LegalEntityInput{
		DisplayName: new("Acme Wien"), LegalName: new("Acme Wien GmbH"), Country: new("at"),
		Address: contacts.LegalEntityAddress{Line1: new("Kärntner Ring 12"), PostalCode: new("1010"), City: new("Wien")},
		VatID:   new("ATU12345678"), RegisterCourt: new("HG Wien"), RegisterNumber: new("FN 123456 a"),
		BankAccountHolder: new("Acme Wien GmbH"), IBAN: new("AT611904300234573201"), BIC: new("BKAUATWW"),
	})
	if err != nil {
		t.Fatalf("create Vienna: %v", err)
	}
	e.WsExec(t, `UPDATE legal_entity SET logo_object_key = 'logos/wien.png' WHERE id = $1`, vienna.ID)
	if err := e.Contacts.AssignLegalEntityMember(admin, vienna.ID, ids.From[ids.UserKind](e.Rep1)); err != nil {
		t.Fatalf("assign the rep to Vienna: %v", err)
	}
	return vienna
}

func defaultEntityID(t *testing.T, e *Env) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := e.Pool.QueryRow(e.Admin(), `SELECT id FROM legal_entity WHERE is_default`).Scan(&id); err != nil {
		t.Fatalf("read the default entity: %v", err)
	}
	return id
}

func entityOf(offer crmcontracts.Offer) ids.UUID {
	if offer.LegalEntityId == nil {
		return ids.UUID{}
	}
	return ids.UUID(*offer.LegalEntityId)
}

func TestAnOfferIsIssuedByItsAuthorsEntityUntilItIsSent(t *testing.T) {
	e := Setup(t)
	vienna := seedVienna(t, e)
	pipeline, open, _ := DealFixture(t, e)
	dealID := e.SeedDeal(t, "Issuer deal", pipeline, open, &e.Rep1)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, offerRenderDeskPerms)

	created := renderOneLineOffer(rep, t, e, dealID, deals.CreateOfferInput{})
	if got := entityOf(created); got != vienna.ID.UUID {
		t.Fatalf("the rep's offer names entity %s, want their own entity %s", got, vienna.ID)
	}
	byAdmin := renderOneLineOffer(e.As(e.AdminUser, nil, offerRenderDeskPerms), t, e, dealID, deals.CreateOfferInput{})
	if got := entityOf(byAdmin); got != defaultEntityID(t, e) {
		t.Errorf("an unassigned author's offer names entity %s, want the default", got)
	}

	offerID := ids.From[ids.OfferKind](ids.UUID(created.Id))
	fallback := ids.From[ids.LegalEntityKind](defaultEntityID(t, e))
	moved, _, err := e.Deals.UpdateOffer(rep, offerID, deals.UpdateOfferInput{LegalEntityID: &fallback})
	if err != nil || entityOf(moved) != fallback.UUID {
		t.Fatalf("moving a draft to the default entity: %v, names %s", err, entityOf(moved))
	}
	unknown := ids.New[ids.LegalEntityKind]()
	if _, _, err := e.Deals.UpdateOffer(rep, offerID, deals.UpdateOfferInput{LegalEntityID: &unknown}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("an unknown entity answered %v, want a refused reference", err)
	}
	back := vienna.ID
	if _, _, err := e.Deals.UpdateOffer(rep, offerID, deals.UpdateOfferInput{LegalEntityID: &back}); err != nil {
		t.Fatalf("moving the draft back to Vienna: %v", err)
	}

	if _, err := e.Deals.SendOffer(rep, offerID, nil); err != nil {
		t.Fatalf("send: %v", err)
	}
	_, _, err = e.Deals.UpdateOffer(rep, offerID, deals.UpdateOfferInput{LegalEntityID: &fallback})
	if _, ok := errors.AsType[*deals.OfferNotDraftError](err); !ok {
		t.Errorf("changing a sent offer's entity answered %v, want offer_not_draft", err)
	}
}

func TestASentOfferKeepsTheIssuerItWasSentWith(t *testing.T) {
	e := Setup(t)
	vienna := seedVienna(t, e)
	pipeline, open, _ := DealFixture(t, e)
	dealID := e.SeedDeal(t, "Frozen issuer deal", pipeline, open, &e.Rep1)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, offerRenderDeskPerms)
	created := renderOneLineOffer(rep, t, e, dealID, deals.CreateOfferInput{})
	offerID := ids.From[ids.OfferKind](ids.UUID(created.Id))

	sent, err := e.Deals.SendOffer(rep, offerID, nil)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	snapshot := *sent.IssuerSnapshot
	for key, want := range map[string]any{
		"workspace_name": "Acme Wien GmbH", "legal_entity_id": vienna.ID.String(), "legal_name": "Acme Wien GmbH",
		"vat_id": "ATU12345678", "register_number": "FN 123456 a", "iban": "AT611904300234573201",
		"bic": "BKAUATWW", "country": "at", "logo_object_key": "logos/wien.png", "base_currency": "EUR",
	} {
		if snapshot[key] != want {
			t.Errorf("issuer_snapshot[%s] = %v, want %v", key, snapshot[key], want)
		}
	}

	if _, err := e.Contacts.UpdateLegalEntity(e.Admin(), vienna.ID, contacts.LegalEntityInput{
		LegalName: new("Acme Austria GmbH"), VatID: new("ATU99999999"),
	}, nil); err != nil {
		t.Fatalf("rename Vienna: %v", err)
	}
	ing, err := e.Deals.PrepareRender(rep, offerID)
	if err != nil {
		t.Fatalf("prepare render: %v", err)
	}
	if ing.Issuer.Name != "Acme Wien GmbH" || ing.Issuer.VatID != "ATU12345678" || ing.Issuer.LogoKey != "logos/wien.png" {
		t.Errorf("a sent offer renders issuer %+v, want the one it was sent with", ing.Issuer)
	}
	if len(ing.Issuer.Address) != 3 || ing.Issuer.Address[1] != "1010 Wien" {
		t.Errorf("the frozen address prints as %q", ing.Issuer.Address)
	}
}

// A draft of an installation with one entity prints what it printed before
// entities: the installation's name, from the default entity.
func TestADefaultEntityDraftIssuesUnderTheInstallationName(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	dealID := e.SeedDeal(t, "Default issuer deal", pipeline, open, &e.Rep1)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, offerRenderDeskPerms)
	created := renderOneLineOffer(rep, t, e, dealID, deals.CreateOfferInput{})
	offerID := ids.From[ids.OfferKind](ids.UUID(created.Id))
	ing, err := e.Deals.PrepareRender(rep, offerID)
	if err != nil {
		t.Fatalf("prepare render: %v", err)
	}
	if ing.Issuer.Name != "Authz" {
		t.Errorf("a default-entity draft issues as %q, want the installation's name", ing.Issuer.Name)
	}
	sent, err := e.Deals.SendOffer(rep, offerID, nil)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if (*sent.IssuerSnapshot)["workspace_name"] != "Authz" {
		t.Errorf("the default entity's snapshot names %v, want the installation's name", (*sent.IssuerSnapshot)["workspace_name"])
	}
}
