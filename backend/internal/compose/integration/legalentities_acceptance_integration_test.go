// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Two legal entities, a user in each, an offer from each. Everything a
// customer sees from a user follows that user's entity. That is the offer's
// issuer, tax rates and product language, the booking page and the signature.

import (
	"context"
	"maps"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// entityDeskPerms writes offers in the product's languages and keeps the
// tax rate sheet.
var entityDeskPerms = func() principal.Permissions {
	perms := offerLanguagePerms
	perms.Objects = maps.Clone(offerLanguagePerms.Objects)
	perms.Objects["tax_rate"] = principal.ObjectGrant{Create: true, Read: true}
	return perms
}()

// sendOneProductOffer creates and sends an offer with one line for product.
func sendOneProductOffer(ctx context.Context, t *testing.T, e *Env, dealID ids.UUID, product ids.ProductID) crmcontracts.Offer {
	t.Helper()
	created := renderOneLineOffer(ctx, t, e, dealID, deals.CreateOfferInput{LineItems: productLine(product)})
	sent, err := e.Deals.SendOffer(ctx, ids.From[ids.OfferKind](ids.UUID(created.Id)), nil)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	return sent
}

func TestEachUserFacesCustomersAsTheirOwnEntity(t *testing.T) {
	e := Setup(t)
	e.Deals.SetClock(func() time.Time { return lineTaxNoon })
	e.WsExec(t, `INSERT INTO setting (key, value, updated_at) VALUES ('installation.country', '"de"'::jsonb, now())`)
	vienna := seedVienna(t, e)
	if _, err := e.Contacts.UpdateLegalEntity(e.Admin(), vienna.ID, contacts.LegalEntityInput{
		DefaultLanguage: new("en"), Imprint: new("Acme Wien GmbH, FN 123456 a"),
		SignatureTemplate: new("<p>{name}</p><p>{imprint}</p>"),
	}, nil); err != nil {
		t.Fatalf("describe Vienna's language and signature: %v", err)
	}
	desk := e.As(e.AdminUser, nil, entityDeskPerms)
	since := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, rate := range []struct{ country, rate string }{{"de", "19"}, {"at", "20"}} {
		if _, _, err := record(desk, e, rate.country, "standard", rate.rate, since); err != nil {
			t.Fatalf("record the %s standard rate: %v", rate.country, err)
		}
	}
	product := seedRetainer(t, e)
	if _, err := e.Deals.UpdateProduct(desk, product, deals.UpdateProductInput{TaxCategory: new("standard")}); err != nil {
		t.Fatalf("classify the product: %v", err)
	}
	pipeline, open, _ := DealFixture(t, e)
	dealID := e.SeedDeal(t, "Two entities deal", pipeline, open, &e.Rep1)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, entityDeskPerms)

	fromVienna := sendOneProductOffer(rep, t, e, dealID, product)
	fromHome := sendOneProductOffer(desk, t, e, dealID, product)

	for _, tc := range []struct {
		who                string
		offer              crmcontracts.Offer
		issuer, logo, text string
		rate               float64
	}{
		{"the rep in Vienna", fromVienna, "Acme Wien GmbH", "logos/wien.png", "Retainer", 20},
		{"the admin at home", fromHome, "", "", "Pauschale", 19},
	} {
		snapshot := *tc.offer.IssuerSnapshot
		if tc.issuer != "" && snapshot["legal_name"] != tc.issuer {
			t.Errorf("%s's offer is issued by %v, want %s", tc.who, snapshot["legal_name"], tc.issuer)
		}
		if tc.issuer == "" && snapshot["legal_entity_id"] != defaultEntityID(t, e).String() {
			t.Errorf("%s's offer is issued by entity %v, want the default", tc.who, snapshot["legal_entity_id"])
		}
		if logo := snapshot["logo_object_key"]; (logo == "logos/wien.png") != (tc.logo == "logos/wien.png") {
			t.Errorf("%s's offer prints logo %v, want Vienna's only on Vienna's offer", tc.who, logo)
		}
		line := (*tc.offer.LineItems)[0]
		if line.Description != tc.text || line.TaxRate != tc.rate {
			t.Errorf("%s's line reads %q at %v%%, want %q at %v%%", tc.who, line.Description, line.TaxRate, tc.text, tc.rate)
		}
	}

	repID, adminID := ids.From[ids.UserKind](e.Rep1), ids.From[ids.UserKind](e.AdminUser)
	repName, repLogo, err := e.Contacts.PublicBookingBrand(e.Admin(), repID)
	if err != nil || repName != "Acme Wien" || repLogo == nil || !strings.Contains(*repLogo, vienna.ID.String()) {
		t.Errorf("the rep's booking page shows %q with logo %v (%v), want Vienna's", repName, repLogo, err)
	}
	homeName, _, err := e.Contacts.PublicBookingBrand(e.Admin(), adminID)
	if err != nil || homeName == "Acme Wien" {
		t.Errorf("the admin's booking page shows %q (%v), want the default entity's", homeName, err)
	}

	repSignature, err := e.Contacts.SignatureFor(rep, e.Rep1)
	if err != nil || repSignature.LogoKey != "logos/wien.png" || !strings.Contains(repSignature.Imprint, "FN 123456 a") {
		t.Errorf("the rep signs with logo %q and imprint %q (%v), want Vienna's", repSignature.LogoKey, repSignature.Imprint, err)
	}
	homeSignature, err := e.Contacts.SignatureFor(desk, e.AdminUser)
	if err != nil || homeSignature.LogoKey == "logos/wien.png" || homeSignature.Imprint != "" {
		t.Errorf("the admin signs with logo %q and imprint %q (%v), want nothing of Vienna's", homeSignature.LogoKey, homeSignature.Imprint, err)
	}
}
