// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An offer line with a tax category takes its rate from the tax rate sheet,
// for the country of the offer's legal entity. A draft takes today's rate and
// the send takes the send day's. A typed rate stays as it was typed.

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// lineTaxDeskPerms keeps the sheet and the rate card and writes offers, as the
// seeded admin does. A send reads the installation's zone and base currency.
var lineTaxDeskPerms = principal.Permissions{
	RoleKeys: []string{"admin"},
	Objects: map[string]principal.ObjectGrant{
		"tax_rate":              {Create: true, Read: true},
		"product":               {Create: true, Read: true, Update: true, Delete: true},
		"deal":                  {Create: true, Read: true, Update: true},
		"offer":                 {Create: true, Read: true, Update: true},
		"company":               {Read: true},
		"installation_settings": {Read: true},
	},
	RowScope: principal.RowScopeAll,
}

// lineTaxNoon is a fixed midday, the same calendar day in every zone an
// installation may name.
var lineTaxNoon = time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)

// lineTaxFixture is a German installation whose first standard rate of 19
// classified one product, with a further Austrian rate of 20 on the sheet.
type lineTaxFixture struct {
	e       *Env
	desk    context.Context
	dealID  ids.UUID
	product ids.ProductID
}

func newLineTaxFixture(t *testing.T) lineTaxFixture {
	t.Helper()
	e := Setup(t)
	e.Deals.SetClock(func() time.Time { return lineTaxNoon })
	e.WsExec(t, `INSERT INTO setting (key, value, updated_at) VALUES ('installation.country', '"de"'::jsonb, now())`)
	desk := e.As(e.AdminUser, nil, lineTaxDeskPerms)
	product := seedTaxedProduct(desk, t, e, "Consulting", 19)
	since := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, rate := range []struct{ country, rate string }{{"de", "19"}, {"at", "20"}} {
		if _, _, err := record(desk, e, rate.country, "standard", rate.rate, since); err != nil {
			t.Fatalf("record the %s standard rate: %v", rate.country, err)
		}
	}
	pipeline, open, _ := DealFixture(t, e)
	dealID := e.SeedDeal(t, "Tax deal", pipeline, open, &e.AdminUser)
	return lineTaxFixture{e: e, desk: desk, dealID: dealID, product: product}
}

// offer creates a draft with one line for the product and one typed 16 line.
func (f lineTaxFixture) offer(t *testing.T, entity *ids.LegalEntityID) crmcontracts.Offer {
	t.Helper()
	typed, desc, price := "16", "Old contract", int64(10000)
	return renderOneLineOffer(f.desk, t, f.e, f.dealID, deals.CreateOfferInput{
		LegalEntityID: entity,
		LineItems: []deals.OfferLineInputRow{
			{ProductID: &f.product, Quantity: "1"},
			{Description: &desc, Quantity: "1", UnitPriceMinor: &price, TaxRate: &typed},
		},
	})
}

// rates answers each line's rate and category, in position order.
func rates(offer crmcontracts.Offer) [][2]string {
	var out [][2]string
	for _, line := range *offer.LineItems {
		category := "none"
		if line.TaxCategory != nil {
			category = string(*line.TaxCategory)
		}
		out = append(out, [2]string{formatRate(line.TaxRate), category})
	}
	return out
}

func formatRate(rate float64) string { return strconv.FormatFloat(rate, 'f', -1, 64) }

func offerIDOf(offer crmcontracts.Offer) ids.OfferID {
	return ids.From[ids.OfferKind](ids.UUID(offer.Id))
}

func TestALineTakesTheRateOfItsEntitysCountry(t *testing.T) {
	f := newLineTaxFixture(t)
	vienna := seedVienna(t, f.e).ID

	german := f.offer(t, nil)
	austrian := f.offer(t, &vienna)
	if got := rates(german); got[0] != [2]string{"19", "standard"} || got[1] != [2]string{"16", "none"} {
		t.Errorf("the default entity's lines read %v, want 19 standard and the typed 16", got)
	}
	if got := rates(austrian); got[0] != [2]string{"20", "standard"} || got[1] != [2]string{"16", "none"} {
		t.Errorf("Vienna's lines read %v, want Austria's 20 standard and the typed 16", got)
	}
	if *austrian.GrossMinor != 12000+11600 {
		t.Errorf("Vienna's gross is %d, want 120.00 + 116.00", *austrian.GrossMinor)
	}

	home := ids.From[ids.LegalEntityKind](defaultEntityID(t, f.e))
	moved, _, err := f.e.Deals.UpdateOffer(f.desk, offerIDOf(austrian), deals.UpdateOfferInput{LegalEntityID: &home})
	if err != nil {
		t.Fatalf("move the draft to the default entity: %v", err)
	}
	if got := rates(moved); got[0][0] != "19" || got[1][0] != "16" || *moved.GrossMinor != 11900+11600 {
		t.Errorf("after the move the lines read %v, gross %d; want Germany's 19 and the typed 16", got, *moved.GrossMinor)
	}

	lines := *german.LineItems
	standard, seven := "standard", "7"
	patched, err := f.e.Deals.UpdateOfferLineItem(f.desk, offerIDOf(german), ids.UUID(lines[1].Id),
		deals.UpdateOfferLineInput{TaxCategory: &standard})
	if err != nil || rates(patched)[1] != [2]string{"19", "standard"} {
		t.Errorf("giving the typed line a category reads %v (%v), want 19 standard", rates(patched), err)
	}
	patched, err = f.e.Deals.UpdateOfferLineItem(f.desk, offerIDOf(german), ids.UUID(lines[0].Id),
		deals.UpdateOfferLineInput{TaxRate: &seven})
	if err != nil || rates(patched)[0] != [2]string{"7", "none"} {
		t.Errorf("typing a rate on the product line reads %v (%v), want 7 and no category", rates(patched), err)
	}

	both, category := "19", "standard"
	_, err = f.e.Deals.AddOfferLineItem(f.desk, offerIDOf(german), deals.OfferLineInputRow{
		ProductID: &f.product, Quantity: "1", TaxRate: &both, TaxCategory: &category,
	})
	if _, ok := errors.AsType[*deals.TaxRateAndCategoryError](err); !ok {
		t.Errorf("a line naming a rate and a category answered %v, want tax_rate_and_category", err)
	}
	reduced := "reduced"
	_, err = f.e.Deals.AddOfferLineItem(f.desk, offerIDOf(german), deals.OfferLineInputRow{
		ProductID: &f.product, Quantity: "1", TaxCategory: &reduced,
	})
	if _, ok := errors.AsType[*deals.TaxRateMissingError](err); !ok {
		t.Errorf("a reduced line with no reduced rate on the sheet answered %v, want tax_rate_missing", err)
	}
}

func TestTheSendDayPicksTheRate(t *testing.T) {
	f := newLineTaxFixture(t)
	sentToday := f.offer(t, nil)
	draft := f.offer(t, nil)
	if _, err := f.e.Deals.SendOffer(f.desk, offerIDOf(sentToday), nil); err != nil {
		t.Fatalf("send today: %v", err)
	}

	tomorrow := lineTaxNoon.AddDate(0, 0, 1)
	if _, _, err := record(f.desk, f.e, "de", "standard", "21", tomorrow.Truncate(24*time.Hour)); err != nil {
		t.Fatalf("record tomorrow's rate: %v", err)
	}
	f.e.Deals.SetClock(func() time.Time { return tomorrow })
	sent, err := f.e.Deals.SendOffer(f.desk, offerIDOf(draft), nil)
	if err != nil {
		t.Fatalf("send tomorrow: %v", err)
	}
	if got := rates(sent); got[0] != [2]string{"21", "standard"} || got[1] != [2]string{"16", "none"} {
		t.Errorf("the draft sent tomorrow reads %v, want tomorrow's 21 and the typed 16", got)
	}
	if *sent.GrossMinor != 12100+11600 {
		t.Errorf("the draft sent tomorrow has gross %d, want 121.00 + 116.00", *sent.GrossMinor)
	}
	earlier, err := f.e.Deals.GetOffer(f.desk, offerIDOf(sentToday), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if got := rates(earlier); got[0][0] != "19" || *earlier.GrossMinor != 11900+11600 {
		t.Errorf("the offer sent today reads %v, gross %d; want the 19 it was sent with", got, *earlier.GrossMinor)
	}
	if got := f.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1
		AND evidence ? 'lines_retaxed' AND after ->> 'gross_minor' = '23700'`, sent.Id); got != 1 {
		t.Errorf("%d send audit rows name the retaxed line and the new gross, want 1", got)
	}
	if got := f.e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope #>> '{entity,id}' = $1::text
		AND envelope ->> 'type' = 'offer.sent' AND envelope #>> '{payload,gross_minor}' = '23700'`, sent.Id); got != 1 {
		t.Errorf("%d offer.sent events state the new gross, want 1", got)
	}
}

func TestASendWithNoRateForTheCountryIsRefused(t *testing.T) {
	f := newLineTaxFixture(t)
	draft := f.offer(t, nil)
	// The installation moves to a country the sheet holds no rate for, and the
	// default entity names none of its own.
	f.e.WsExec(t, `UPDATE legal_entity SET country = NULL WHERE is_default`)
	f.e.WsExec(t, `UPDATE setting SET value = '"fr"'::jsonb WHERE key = 'installation.country'`)

	_, err := f.e.Deals.SendOffer(f.desk, offerIDOf(draft), nil)
	missing, ok := errors.AsType[*deals.TaxRateMissingError](err)
	if !ok || missing.Country != "fr" {
		t.Fatalf("sending with no French rate answered %v, want tax_rate_missing for fr", err)
	}
	still, err := f.e.Deals.GetOffer(f.desk, offerIDOf(draft), storekit.LiveOnly)
	if err != nil || still.Status != crmcontracts.OfferStatusDraft || rates(still)[0][0] != "19" {
		t.Errorf("the refused offer is %s with %v (%v), want an unchanged draft", still.Status, rates(still), err)
	}
}

func TestADraftedLineOnAClassifiedProductTakesTheSheetsRate(t *testing.T) {
	f := newLineTaxFixture(t)
	draft := f.offer(t, nil)
	evidence := deals.StagedOfferLineEvidence{Snippet: "they want the consulting", SourceID: "activity:1"}
	staged, err := f.e.Deals.AddStagedOfferLines(f.desk, offerIDOf(draft), []deals.StagedOfferLineInput{
		{Description: "Consulting", Quantity: "1", UnitPriceMinor: 10000, TaxRate: "16", ProductID: &f.product, Evidence: evidence, PriceGrounded: true},
		{Description: "Travel", Quantity: "1", UnitPriceMinor: 10000, TaxRate: "7", Evidence: evidence, PriceGrounded: true},
	})
	if err != nil {
		t.Fatalf("stage the drafted lines: %v", err)
	}
	onProduct, onNothing := staged[0], staged[1]
	if onProduct.TaxRate != 19 || onProduct.TaxCategory == nil || *onProduct.TaxCategory != "standard" {
		t.Errorf("the line on the classified product took %v (%v), want the sheet's 19 standard", onProduct.TaxRate, onProduct.TaxCategory)
	}
	if onNothing.TaxRate != 7 || onNothing.TaxCategory != nil {
		t.Errorf("the line on no product took %v (%v), want the drafter's 7 and no category", onNothing.TaxRate, onNothing.TaxCategory)
	}
	if _, err := f.e.Deals.AcceptOfferLineItem(f.desk, offerIDOf(draft), ids.UUID(onProduct.Id)); err != nil {
		t.Fatalf("accept the drafted line: %v", err)
	}

	tomorrow := lineTaxNoon.AddDate(0, 0, 1)
	if _, _, err := record(f.desk, f.e, "de", "standard", "21", tomorrow.Truncate(24*time.Hour)); err != nil {
		t.Fatalf("record tomorrow's rate: %v", err)
	}
	f.e.Deals.SetClock(func() time.Time { return tomorrow })
	sent, err := f.e.Deals.SendOffer(f.desk, offerIDOf(draft), nil)
	if err != nil {
		t.Fatalf("send tomorrow: %v", err)
	}
	if got := rates(sent); got[2] != [2]string{"21", "standard"} || got[3] != [2]string{"7", "none"} {
		t.Errorf("after the send the drafted lines read %v, want tomorrow's 21 standard and the drafter's 7", got[2:])
	}
}

func TestARevisionTakesTodaysRates(t *testing.T) {
	f := newLineTaxFixture(t)
	sent := f.offer(t, nil)
	if _, err := f.e.Deals.SendOffer(f.desk, offerIDOf(sent), nil); err != nil {
		t.Fatalf("send today: %v", err)
	}
	tomorrow := lineTaxNoon.AddDate(0, 0, 1)
	if _, _, err := record(f.desk, f.e, "de", "standard", "21", tomorrow.Truncate(24*time.Hour)); err != nil {
		t.Fatalf("record tomorrow's rate: %v", err)
	}
	f.e.Deals.SetClock(func() time.Time { return tomorrow })
	revision, err := f.e.Deals.RegenerateOffer(f.desk, offerIDOf(sent))
	if err != nil {
		t.Fatalf("regenerate: %v", err)
	}
	if got := rates(revision); got[0] != [2]string{"21", "standard"} || got[1] != [2]string{"16", "none"} {
		t.Errorf("the revision reads %v, want today's 21 standard and the typed 16 as copied", got)
	}
	if *revision.GrossMinor != 12100+11600 {
		t.Errorf("the revision's gross is %d, want 121.00 + 116.00", *revision.GrossMinor)
	}
}

func TestALinePatchAuditsTheCategoryItReplaced(t *testing.T) {
	f := newLineTaxFixture(t)
	if _, _, err := record(f.desk, f.e, "de", "reduced", "7", time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("record the reduced rate: %v", err)
	}
	draft := f.offer(t, nil)
	line := ids.UUID((*draft.LineItems)[0].Id)
	reduced, typed := "reduced", "10"
	for _, patch := range []deals.UpdateOfferLineInput{{TaxCategory: &reduced}, {TaxRate: &typed}} {
		if _, err := f.e.Deals.UpdateOfferLineItem(f.desk, offerIDOf(draft), line, patch); err != nil {
			t.Fatalf("patch the line: %v", err)
		}
	}
	for _, step := range []struct{ was, now string }{{`"standard"`, `"reduced"`}, {`"reduced"`, `null`}} {
		if got := f.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1
			AND evidence -> 'line_before' -> 'tax_category' = $2::jsonb
			AND evidence -> 'line_after' -> 'tax_category' = $3::jsonb`, draft.Id, step.was, step.now); got != 1 {
			t.Errorf("%d audit rows record the category moving from %s to %s, want 1", got, step.was, step.now)
		}
	}
}
