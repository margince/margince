// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An offer's language decides the text its new lines take from a product. That
// is the product's translation into it where there is one, else the product's
// own name. The text is copied once, when the line is added.

import (
	"errors"
	"maps"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// offerLanguagePerms is the desk grant plus the product and template grants
// the language cases need.
var offerLanguagePerms = func() principal.Permissions {
	perms := offerRenderDeskPerms
	perms.Objects = maps.Clone(offerRenderDeskPerms.Objects)
	perms.Objects["product"] = principal.ObjectGrant{Create: true, Read: true, Update: true}
	perms.Objects["offer_template"] = principal.ObjectGrant{Create: true, Read: true}
	return perms
}()

// seedRetainer is a product named in English with a German translation.
func seedRetainer(t *testing.T, e *Env) ids.ProductID {
	t.Helper()
	desk := e.As(e.AdminUser, nil, offerLanguagePerms)
	product, err := e.Deals.CreateProduct(desk, deals.CreateProductInput{Name: "Retainer", UnitPriceMinor: 50000, Currency: "EUR"})
	if err != nil {
		t.Fatalf("create the product: %v", err)
	}
	id := ids.From[ids.ProductKind](ids.UUID(product.Id))
	if _, err := e.Deals.PutProductTranslation(desk, id, "de", "Pauschale", new("Monatliche Betreuung")); err != nil {
		t.Fatalf("translate the product: %v", err)
	}
	return id
}

func productLine(product ids.ProductID) []deals.OfferLineInputRow {
	return []deals.OfferLineInputRow{{ProductID: &product, Quantity: "1"}}
}

func lineTexts(offer crmcontracts.Offer) []string {
	var out []string
	for _, line := range *offer.LineItems {
		out = append(out, line.Description)
	}
	return out
}

func TestAnOfferTakesItsEntitysLanguageAndProductTextInIt(t *testing.T) {
	e := Setup(t)
	vienna := seedVienna(t, e)
	if _, err := e.Contacts.UpdateLegalEntity(e.Admin(), vienna.ID, contacts.LegalEntityInput{DefaultLanguage: new("de")}, nil); err != nil {
		t.Fatalf("give Vienna a language: %v", err)
	}
	product := seedRetainer(t, e)
	pipeline, open, _ := DealFixture(t, e)
	dealID := e.SeedDeal(t, "Language deal", pipeline, open, &e.Rep1)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, offerLanguagePerms)

	created := renderOneLineOffer(rep, t, e, dealID, deals.CreateOfferInput{LineItems: productLine(product)})
	if created.Language == nil || *created.Language != "de" {
		t.Fatalf("the offer's language is %v, want Vienna's de", created.Language)
	}
	if got := lineTexts(created); len(got) != 1 || got[0] != "Pauschale" {
		t.Fatalf("the line reads %q, want the German name", got)
	}

	desk := e.As(e.AdminUser, nil, offerLanguagePerms)
	if _, err := e.Deals.PutProductTranslation(desk, product, "de", "Betreuungspauschale", nil); err != nil {
		t.Fatalf("retranslate: %v", err)
	}
	offerID := ids.From[ids.OfferKind](ids.UUID(created.Id))
	added, err := e.Deals.AddOfferLineItem(rep, offerID, productLine(product)[0])
	if err != nil {
		t.Fatalf("add a second line: %v", err)
	}
	if got := lineTexts(added); len(got) != 2 || got[0] != "Pauschale" || got[1] != "Betreuungspauschale" {
		t.Errorf("after the retranslation the lines read %q, want the first unchanged and the second new", got)
	}
}

func TestALanguageWithoutATranslationTakesTheProductsName(t *testing.T) {
	e := Setup(t)
	product := seedRetainer(t, e)
	pipeline, open, _ := DealFixture(t, e)
	dealID := e.SeedDeal(t, "Fallback deal", pipeline, open, &e.Rep1)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, offerLanguagePerms)

	created := renderOneLineOffer(rep, t, e, dealID, deals.CreateOfferInput{Language: new("fr"), LineItems: productLine(product)})
	if got := lineTexts(created); len(got) != 1 || got[0] != "Retainer" {
		t.Errorf("a French offer's line reads %q, want the product's own name", got)
	}
	if _, err := e.Deals.CreateOffer(rep, ids.From[ids.DealKind](dealID), deals.CreateOfferInput{
		Currency: "EUR", Source: "manual", Language: new("FRA"),
	}); err == nil {
		t.Error("a language that is not an ISO 639-1 code was accepted")
	}
}

// With no language on the entity, the offer takes its template's; with no
// template, the default template locale's.
func TestAnOfferWithoutAnEntityLanguageTakesItsTemplates(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	dealID := e.SeedDeal(t, "Template language deal", pipeline, open, &e.Rep1)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, offerLanguagePerms)
	tmpl, err := e.Deals.CreateOfferTemplate(rep, deals.CreateOfferTemplateInput{Name: "English", Locale: "en-GB"})
	if err != nil {
		t.Fatalf("create the template: %v", err)
	}
	templateID := ids.From[ids.OfferTemplateKind](ids.UUID(tmpl.Id))
	english := renderOneLineOffer(rep, t, e, dealID, deals.CreateOfferInput{TemplateID: &templateID})
	if english.Language == nil || *english.Language != "en" {
		t.Errorf("an offer on an English template has language %v, want en", english.Language)
	}
	plain := renderOneLineOffer(rep, t, e, dealID, deals.CreateOfferInput{})
	if plain.Language == nil || *plain.Language != "de" {
		t.Errorf("an offer with no template has language %v, want the default template's de", plain.Language)
	}
}

func TestADraftChangesItsLanguageAndASentOfferDoesNot(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	dealID := e.SeedDeal(t, "Language change deal", pipeline, open, &e.Rep1)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, offerLanguagePerms)
	created := renderOneLineOffer(rep, t, e, dealID, deals.CreateOfferInput{})
	offerID := ids.From[ids.OfferKind](ids.UUID(created.Id))
	moved, _, err := e.Deals.UpdateOffer(rep, offerID, deals.UpdateOfferInput{Language: new("en")})
	if err != nil || moved.Language == nil || *moved.Language != "en" {
		t.Fatalf("change a draft's language: %v, now %v", err, moved.Language)
	}
	if _, err := e.Deals.SendOffer(rep, offerID, nil); err != nil {
		t.Fatalf("send: %v", err)
	}
	_, _, err = e.Deals.UpdateOffer(rep, offerID, deals.UpdateOfferInput{Language: new("de")})
	if _, ok := errors.AsType[*deals.OfferNotDraftError](err); !ok {
		t.Errorf("changing a sent offer's language answered %v, want offer_not_draft", err)
	}
}

// A product's translations are read with product:read, changed with
// product:update, and every change leaves an audit row on the product.
func TestProductTranslationsFollowTheProductGrant(t *testing.T) {
	e := Setup(t)
	product := seedRetainer(t, e)
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, offerRenderDeskPerms)
	if _, err := e.Deals.PutProductTranslation(reader, product, "fr", "Forfait", nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a seat without product:update answered %v, want permission denied", err)
	}
	desk := e.As(e.AdminUser, nil, offerLanguagePerms)
	if err := e.Deals.DeleteProductTranslation(desk, product, "de"); err != nil {
		t.Fatalf("remove the German text: %v", err)
	}
	listed, err := e.Deals.ListProductTranslations(desk, product)
	if err != nil || len(listed) != 0 {
		t.Errorf("after removing it the product lists %v (%v), want none", listed, err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'product' AND entity_id = $1 AND action = 'update'`,
		ids.UUID(product.UUID)); got != 2 {
		t.Errorf("the product carries %d update audit rows, want one per translation change", got)
	}
}

// Changing a draft's language leaves the lines it already has, and the lines
// added after take product text in the new language.
func TestAChangedLanguageReachesOnlyTheLinesAddedAfterIt(t *testing.T) {
	e := Setup(t)
	product := seedRetainer(t, e)
	pipeline, open, _ := DealFixture(t, e)
	dealID := e.SeedDeal(t, "Language switch deal", pipeline, open, &e.Rep1)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, offerLanguagePerms)
	created := renderOneLineOffer(rep, t, e, dealID, deals.CreateOfferInput{Language: new("en"), LineItems: productLine(product)})
	offerID := ids.From[ids.OfferKind](ids.UUID(created.Id))
	if _, _, err := e.Deals.UpdateOffer(rep, offerID, deals.UpdateOfferInput{Language: new("de")}); err != nil {
		t.Fatalf("switch the draft to German: %v", err)
	}
	added, err := e.Deals.AddOfferLineItem(rep, offerID, productLine(product)[0])
	if err != nil {
		t.Fatalf("add a line: %v", err)
	}
	if got := lineTexts(added); len(got) != 2 || got[0] != "Retainer" || got[1] != "Pauschale" {
		t.Errorf("after the switch the lines read %q, want the English line kept and the new one German", got)
	}
}

// A template locale written with `_`, or one with no ISO 639-1 language,
// still gives the offer a valid language rather than a refused row.
func TestATemplateLocaleOfAnyShapeGivesAValidLanguage(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	dealID := e.SeedDeal(t, "Locale shape deal", pipeline, open, &e.Rep1)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, offerLanguagePerms)
	tmpl, err := e.Deals.CreateOfferTemplate(rep, deals.CreateOfferTemplateInput{Name: "Austria", Locale: "de-AT"})
	if err != nil {
		t.Fatalf("create the template: %v", err)
	}
	templateID := ids.From[ids.OfferTemplateKind](ids.UUID(tmpl.Id))
	for locale, want := range map[string]string{"de_AT": "de", "1x_unknown_99": "de", "en_GB": "en"} {
		e.WsExec(t, `UPDATE offer_template SET locale = $2 WHERE id = $1`, templateID, locale)
		created := renderOneLineOffer(rep, t, e, dealID, deals.CreateOfferInput{TemplateID: &templateID})
		if created.Language == nil || *created.Language != want {
			t.Errorf("template locale %q gave language %v, want %s", locale, created.Language, want)
		}
	}
}

// Two writes to one product's translation run one after the other, so each
// audit row's before-image is the text the write replaced.
func TestConcurrentTranslationWritesAuditTheTextTheyReplaced(t *testing.T) {
	e := Setup(t)
	product := seedRetainer(t, e)
	desk := e.As(e.AdminUser, nil, offerLanguagePerms)
	names := []string{"Forfait A", "Forfait B"}
	done := make(chan error, len(names))
	for _, name := range names {
		go func() {
			_, err := e.Deals.PutProductTranslation(desk, product, "fr", name, nil)
			done <- err
		}()
	}
	for range names {
		if err := <-done; err != nil {
			t.Fatalf("translate: %v", err)
		}
	}
	rows := e.WsCount(t, `
		SELECT count(*) FROM audit_log later JOIN audit_log earlier
		    ON earlier.entity_id = later.entity_id AND earlier.id < later.id
		 WHERE later.entity_id = $1 AND later.before -> 'translation_fr' IS NOT NULL
		   AND jsonb_typeof(later.before -> 'translation_fr') = 'object'
		   AND later.before -> 'translation_fr' = earlier.after -> 'translation_fr'`, ids.UUID(product.UUID))
	if rows != 1 {
		t.Errorf("%d audit rows replace the text the other write stored, want exactly one: the second write's before-image is the first write's result", rows)
	}
}
