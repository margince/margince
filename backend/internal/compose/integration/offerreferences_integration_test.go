// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An offer is anchored on its DEAL, and every seat of the workspace reads every
// deal — a deal is customer identity. The company the offer names is not:
// capture privacy makes a company private to the colleague who captured
// it. So an offer read handed back a reference the reader's own company
// read would refuse, which is the existence oracle dealreferences closed on the
// deal itself, arriving one table along (#2004).
//
// The buyer snapshot is the sharper half. It is the buyer's legal block frozen
// at send — display name, and where the record carries one, legal name — so an
// offer read disclosed strictly more than the id the deal read withholds.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/installseam"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// offerDeskCompanyPerms is the offer desk that may also read companies.
//
// offerDeskPerms carries no `company` grant at all, and VisibleSubset
// withholds every id when the object grant is missing — so a fixture built on
// it would pass whatever capture privacy did, which is the wrong subject. This
// set isolates the row-scope arm: the grant is present for both seats, and the
// only thing separating them is who the company was captured private to.
var offerDeskCompanyPerms = principal.Permissions{
	RoleKeys: []string{"deal_desk"},
	Objects: map[string]principal.ObjectGrant{
		"deal":                  {Create: true, Read: true, Update: true},
		"offer":                 {Create: true, Read: true, Update: true},
		"company":               {Read: true},
		"installation_settings": {Read: true},
	},
}

// offerDeskWideNoCompanyPerms is the seat the RENDER needs: workspace-wide
// authority over deals and offers, and no company grant at all.
//
// The render asks a harder question than the read — EnsureWritable on the deal,
// not EnsureVisible — so an own-scoped colleague never reaches the buyer block
// to be refused it. A deal desk operating across the workspace without CRM
// company access does, and it is the seat that would have printed the name.
var offerDeskWideNoCompanyPerms = principal.Permissions{
	RoleKeys: []string{"deal_desk"},
	RowScope: principal.RowScopeAll,
	Objects: map[string]principal.ObjectGrant{
		"deal":                  {Create: true, Read: true, Update: true},
		"offer":                 {Create: true, Read: true, Update: true},
		"installation_settings": {Read: true},
	},
}

// offerDeskWideCompanyPerms is that desk with the company grant back: it edits
// every deal, and opens every company capture privacy does not hold back from it.
var offerDeskWideCompanyPerms = principal.Permissions{
	RoleKeys: []string{"deal_desk"},
	RowScope: principal.RowScopeAll,
	Objects: map[string]principal.ObjectGrant{
		"deal":                  {Create: true, Read: true, Update: true},
		"offer":                 {Create: true, Read: true, Update: true},
		"company":               {Read: true},
		"installation_settings": {Read: true},
	},
}

// seedOfferOnAPrivateCompany is one sent offer whose buyer is capture-private to a
// colleague, created and sent by an admin who can see it.
//
// Sent, not draft: the snapshot only exists after a send, and it is the field
// that carries the name.
func seedOfferOnAPrivateCompany(t *testing.T, e *Env) (ids.OfferID, ids.DealID, ids.UUID) {
	t.Helper()
	// Created and sent by the colleague the company belongs to, before it
	// becomes private: that is the order this state actually arises in, and it
	// leaves a real snapshot for the refusals below to be about.
	offer, deal, privateCompany := draftOfferOnAColleaguesCompany(t, e)
	desk := e.As(e.Rep3, []ids.UUID{e.Team1}, offerDeskCompanyPerms)
	sent, err := e.Deals.SendOffer(desk, offer, nil)
	if err != nil {
		t.Fatalf("send offer: %v", err)
	}
	// The sender's own response still names the buyer — otherwise the refusal
	// below would hold against an offer that never had one.
	if sent.BuyerCompanyId == nil || sent.BuyerSnapshot == nil {
		t.Fatalf("the sent offer names no buyer for the seat that sent it (id=%v snapshot=%v) — the fixture proves nothing about withholding",
			sent.BuyerCompanyId, sent.BuyerSnapshot)
	}
	e.MakeCapturePrivate(t, "company", privateCompany, e.Rep3)
	return offer, deal, privateCompany
}

// draftOfferOnAColleaguesCompany is a draft offer on Rep3's deal for Meridian
// Labs, written by Rep3 while the company is still workspace-visible.
func draftOfferOnAColleaguesCompany(t *testing.T, e *Env) (ids.OfferID, ids.DealID, ids.UUID) {
	t.Helper()
	pipeline, open, _ := DealFixture(t, e)
	admin := e.Admin()

	// Workspace-visible while the admin links it, so the write passes its own
	// EnsureLinkTarget gate; capture privacy lands afterwards, which is the
	// order a connector-captured company reaches this state in anyway.
	company := e.SeedCompany(t, "Meridian Labs", &e.Rep3)
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Meridian renewal", pipeline, open, &e.Rep3))
	companyID := companyIDOf(company)
	if _, err := e.Deals.UpdateDeal(admin, deal, deals.UpdateDealInput{CompanyID: &companyID}); err != nil {
		t.Fatalf("linking the deal to its company: %v", err)
	}

	desk := e.As(e.Rep3, []ids.UUID{e.Team1}, offerDeskCompanyPerms)
	description, price := "Retainer", int64(10000)
	created, err := e.Deals.CreateOffer(desk, deal, deals.CreateOfferInput{
		Currency: "EUR", Source: "manual",
		LineItems: []deals.OfferLineInputRow{{
			Description: &description, Quantity: "1", UnitPriceMinor: &price,
		}},
	})
	if err != nil {
		t.Fatalf("create offer: %v", err)
	}
	return ids.From[ids.OfferKind](ids.UUID(created.Id)), deal, company
}

// A reader who cannot open the company gets the offer without it.
func TestAnOfferWithholdsABuyerTheReaderCannotOpen(t *testing.T) {
	e := Setup(t)
	offer, deal, _ := seedOfferOnAPrivateCompany(t, e)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, offerDeskCompanyPerms)

	got, err := e.Deals.GetOffer(rep, offer, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the offer: %v — the offer is anchored on a deal this seat reads, so the read itself must succeed", err)
	}
	if got.BuyerCompanyId != nil {
		t.Errorf("the offer names buyer company %v, which this seat's own company read would refuse — the id alone is an existence oracle over a capture-private company", *got.BuyerCompanyId)
	}
	if got.BuyerSnapshot != nil {
		t.Errorf("the offer carries the buyer snapshot %v — the frozen legal block names the company, which is strictly more than the id withheld beside it", *got.BuyerSnapshot)
	}

	// The LIST is a separate statement and was separately exposed.
	page, _, err := e.Deals.ListDealOffers(rep, deal, deals.ListDealOffersInput{})
	if err != nil {
		t.Fatalf("listing the deal's offers: %v", err)
	}
	if len(page) != 1 {
		t.Fatalf("the deal lists %d offer(s), want 1 — withholding a reference must not withhold the row", len(page))
	}
	if page[0].BuyerCompanyId != nil || page[0].BuyerSnapshot != nil {
		t.Errorf("the listed offer names its buyer (id=%v snapshot=%v)", page[0].BuyerCompanyId, page[0].BuyerSnapshot)
	}
}

// And the colleague who CAN open it still sees it, so the rule withholds from a
// reader rather than emptying the field for everyone.
func TestAnOfferKeepsABuyerTheReaderCanOpen(t *testing.T) {
	e := Setup(t)
	offer, _, _ := seedOfferOnAPrivateCompany(t, e)

	owner := e.As(e.Rep3, []ids.UUID{e.Team1}, offerDeskCompanyPerms)
	got, err := e.Deals.GetOffer(owner, offer, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the offer as the company's owner: %v", err)
	}
	if got.BuyerCompanyId == nil {
		t.Error("the offer withholds its buyer from the colleague the company is private TO — the rule is about who may open the record, not about hiding it from everybody")
	}
	if got.BuyerSnapshot == nil {
		t.Error("the offer withholds the buyer snapshot from the colleague who can open the company")
	}
}

// The snapshot the SEND froze is the real one, even when the sender is the seat
// the buyer is hidden FROM.
//
// This is what decides where the withholding goes. The tempting placement is
// readOffer — the one read every path shares — but that is also the read the
// write paths take their current state from: sendSnapshots freezes the buyer's
// legal block from the offer it is handed. Withheld there, this send would
// record a blank buyer, destroying the evidence rather than protecting it, and
// silently, because the offer would look sent.
//
// So the rule is applied where things LEAVE the server, and both halves are
// asserted here: the stored record names the company, and the response the
// sender gets does not.
func TestASendFreezesTheRealBuyerEvenWhenTheSenderCannotSeeIt(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	admin := e.Admin()

	// Private to Rep2 BEFORE the send, and the deal belongs to Rep3 — so the
	// seat doing the sending is not the seat the company is visible to.
	hidden := e.SeedCompany(t, "Ashgrove Holdings", &e.Rep2)
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Ashgrove renewal", pipeline, open, &e.Rep3))
	hiddenID := companyIDOf(hidden)
	if _, err := e.Deals.UpdateDeal(admin, deal, deals.UpdateDealInput{CompanyID: &hiddenID}); err != nil {
		t.Fatalf("linking the deal to its company: %v", err)
	}
	e.MakeCapturePrivate(t, "company", hidden, e.Rep2)

	desk := e.As(e.Rep3, []ids.UUID{e.Team1}, offerDeskCompanyPerms)
	description, price := "Retainer", int64(10000)
	created, err := e.Deals.CreateOffer(desk, deal, deals.CreateOfferInput{
		Currency: "EUR", Source: "manual",
		LineItems: []deals.OfferLineInputRow{{
			Description: &description, Quantity: "1", UnitPriceMinor: &price,
		}},
	})
	if err != nil {
		t.Fatalf("create offer: %v", err)
	}
	offer := ids.From[ids.OfferKind](ids.UUID(created.Id))
	sent, err := e.Deals.SendOffer(desk, offer, nil)
	if err != nil {
		t.Fatalf("send offer: %v", err)
	}

	// The response withholds it: the sender still cannot open that company.
	if sent.BuyerCompanyId != nil || sent.BuyerSnapshot != nil {
		t.Errorf("the send response names a buyer this seat cannot open (id=%v snapshot=%v)",
			sent.BuyerCompanyId, sent.BuyerSnapshot)
	}

	// And the record is real. A blank one here is worse than the disclosure:
	// the offer looks sent and nothing says who to.
	var frozen string
	ctx := e.Admin()
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT coalesce(buyer_snapshot::text, '') FROM offer WHERE id = $1`,
			offer.UUID).Scan(&frozen)
	}); err != nil {
		t.Fatalf("reading the stored snapshot: %v", err)
	}
	if !strings.Contains(frozen, hidden.String()) {
		t.Errorf("the stored buyer snapshot does not name the company the offer was sent to: %q — withholding reached the write path and destroyed the legal record", frozen)
	}
}

// The snapshot the SEND froze is the real one, whoever reads it afterwards.
//
// Withholding is a response rule, and the obvious place to apply it — the read
// every path shares — is also the read the write paths take their current state
// from. Applied there, a send by a seat that cannot see the buyer would have
// frozen a blank legal block: destroying the evidence rather than protecting
// it, and silently, since the offer would look sent.
func TestTheSendFreezesTheRealBuyerWhateverTheResponseWithholds(t *testing.T) {
	e := Setup(t)
	offer, _, privateCompany := seedOfferOnAPrivateCompany(t, e)

	var frozen string
	ctx := e.Admin()
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT coalesce(buyer_snapshot::text, '') FROM offer WHERE id = $1`,
			offer.UUID).Scan(&frozen)
	}); err != nil {
		t.Fatalf("reading the stored snapshot: %v", err)
	}
	if frozen == "" {
		t.Fatal("the offer stored no buyer snapshot at all — the legal record of who it was sent to is gone")
	}
	if !strings.Contains(frozen, privateCompany.String()) {
		t.Errorf("the stored buyer snapshot does not name the company the offer was sent to: %s", frozen)
	}
}

// The DOCUMENT says no more than the read does.
//
// PrepareRender gathers the buyer legal block the PDF prints — display name
// and, where the record carries one, legal name. That block is read from the
// live company while the offer is a draft and from the frozen snapshot
// once sent, and neither read asked whether this seat may open the company. So
// the API withheld the buyer while the PDF printed its name: the two halves of
// one offer disagreeing about the same record, with the more disclosing half
// being the one that gets stored and sent.
func TestARenderSaysNoMoreAboutTheBuyerThanTheReadDoes(t *testing.T) {
	e := Setup(t)
	offer, _, privateCompany := seedOfferOnAPrivateCompany(t, e)
	desk := e.As(e.Rep1, []ids.UUID{e.Team1}, offerDeskWideNoCompanyPerms)

	ingredients, err := e.Deals.PrepareRender(desk, offer)
	if err != nil {
		t.Fatalf("preparing the render: %v — the offer is anchored on a deal this seat writes, so the render itself must reach its ingredients", err)
	}
	if ingredients.Offer.BuyerCompanyId != nil {
		t.Errorf("the render's offer names buyer company %v, which this seat's own company read would refuse", *ingredients.Offer.BuyerCompanyId)
	}
	if ingredients.BuyerBlock != nil {
		t.Errorf("the buyer block %v reaches the document — it carries the company's display name and legal name, which is strictly more than the id the API withholds beside it",
			ingredients.BuyerBlock)
	}
	// Named so a future reader can see which company the block would have been
	// about; the assertions above are what fail.
	t.Logf("the buyer withheld from this seat is %v", privateCompany)
}

// And the colleague who CAN open the company still gets a complete document. A
// render that omitted the buyer for everyone would pass the case above while
// producing an offer PDF with no buyer on it, which is not a quieter answer but
// a broken one.
func TestARenderKeepsTheBuyerBlockForASeatThatCanOpenIt(t *testing.T) {
	e := Setup(t)
	offer, _, _ := seedOfferOnAPrivateCompany(t, e)
	owner := e.As(e.Rep3, []ids.UUID{e.Team1}, offerDeskCompanyPerms)

	ingredients, err := e.Deals.PrepareRender(owner, offer)
	if err != nil {
		t.Fatalf("preparing the render: %v", err)
	}
	if ingredients.Offer.BuyerCompanyId == nil {
		t.Error("the render's offer names no buyer for the seat that owns the company")
	}
	if ingredients.BuyerBlock == nil {
		t.Fatal("the document carries no buyer block for the seat that owns the company — the withholding emptied the field for everyone")
	}
	if ingredients.BuyerBlock["display_name"] == nil {
		t.Errorf("the buyer block names no company: %v", ingredients.BuyerBlock)
	}
}

// The DOWNLOAD says no more than the read does either.
//
// A render withholds the buyer from the seat rendering it, then stores the
// document for every reader of the offer. Printed by a colleague who could open
// the company, it names a buyer this reader's own read withholds — so the read
// drops the reference with the buyer, and the download answers the 404 an offer
// never rendered gets.
func TestADownloadSaysNoMoreAboutTheBuyerThanTheReadDoes(t *testing.T) {
	e := Setup(t)
	offer, deal, _ := seedOfferOnAPrivateCompany(t, e)
	owner := e.As(e.Rep3, []ids.UUID{e.Team1}, offerDeskCompanyPerms)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, offerDeskCompanyPerms)
	h := deals.NewHandlers(e.DB(), installseam.Deals()).WithBlobstore(blobstore.NewMemory())
	renderOfferAs(owner, t, h, offer)

	got, err := e.Deals.GetOffer(rep, offer, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the offer: %v", err)
	}
	if got.PdfAssetRef != nil {
		t.Errorf("the offer names its rendering %q to a seat that cannot open the buyer the document prints", *got.PdfAssetRef)
	}
	page, _, err := e.Deals.ListDealOffers(rep, deal, deals.ListDealOffersInput{})
	if err != nil || len(page) != 1 {
		t.Fatalf("listing the deal's offers = %d offer(s), %v; want the one offer", len(page), err)
	}
	if page[0].PdfAssetRef != nil {
		t.Errorf("the listed offer names its rendering %q", *page[0].PdfAssetRef)
	}

	requireNoRendering(t, downloadOfferPdfAs(rep, h, offer), "downloading as a seat that cannot open the buyer")

	// The control: the document refused above really does name the company, and
	// the seat that can open it still downloads it.
	served := downloadOfferPdfAs(owner, h, offer)
	if served.Code != http.StatusOK {
		t.Fatalf("downloading as the colleague who can open the buyer = %d %s, want 200", served.Code, served.Body.String())
	}
	if !bytes.Contains(served.Body.Bytes(), []byte("Meridian Labs")) {
		t.Error("the stored document does not print the buyer, so the refusal above withheld nothing")
	}
}

// A draft's buyer can change after it is rendered, and the stored document goes
// on printing the old one. So the change retires the rendering: kept, it would be
// judged by the NEW buyer's visibility and handed to a seat the old buyer is
// hidden from.
func TestANewBuyerRetiresTheRenderingThatPrintsTheOldOne(t *testing.T) {
	e := Setup(t)
	offer, _, privateCompany := draftOfferOnAColleaguesCompany(t, e)
	e.MakeCapturePrivate(t, "company", privateCompany, e.Rep3)
	owner := e.As(e.Rep3, []ids.UUID{e.Team1}, offerDeskCompanyPerms)
	desk := e.As(e.Rep1, []ids.UUID{e.Team1}, offerDeskWideCompanyPerms)
	blob := blobstore.NewMemory()
	h := deals.NewHandlers(e.DB(), installseam.Deals()).WithBlobstore(blob)
	stored := renderOfferAs(owner, t, h, offer)
	requireNoRendering(t, downloadOfferPdfAs(desk, h, offer), "downloading as a desk that cannot open the buyer")

	ashgrove := companyIDOf(e.SeedCompany(t, "Ashgrove Holdings", &e.Rep1))
	changed := changeBuyerAs(desk, t, h, offer, ashgrove)
	if changed.PdfAssetRef != nil {
		t.Errorf("the offer still names the rendering %q, which prints the buyer it no longer has", *changed.PdfAssetRef)
	}
	if _, _, err := blob.Get(context.Background(), stored); !errors.Is(err, blobstore.ErrNotFound) {
		t.Errorf("the retired rendering is still in the store (err=%v) — nothing names it, and it prints the old buyer", err)
	}
	requireNoRendering(t, downloadOfferPdfAs(desk, h, offer), "downloading after the buyer changed")

	// The control: the change retired the old document, not the feature. A render
	// after it prints the new buyer, and the desk downloads that one.
	renderOfferAs(desk, t, h, offer)
	served := downloadOfferPdfAs(desk, h, offer)
	if served.Code != http.StatusOK || !bytes.Contains(served.Body.Bytes(), []byte("Ashgrove Holdings")) {
		t.Fatalf("downloading the rendering made after the change = %d, want 200 printing the new buyer", served.Code)
	}
}

// failingDeleteStore refuses every delete: an object store that is down after the
// edit has already committed.
type failingDeleteStore struct{ blobstore.Store }

func (failingDeleteStore) Delete(context.Context, string) error {
	return errors.New("object store unavailable")
}

// Reclaiming the retired PDF is housekeeping after the edit commits, so a store
// that refuses the delete leaves an orphan and the edit still stands.
func TestABuyerChangeStandsWhenTheRetiredPdfCannotBeDeleted(t *testing.T) {
	e := Setup(t)
	offer, _, _ := draftOfferOnAColleaguesCompany(t, e)
	owner := e.As(e.Rep3, []ids.UUID{e.Team1}, offerDeskCompanyPerms)
	h := deals.NewHandlers(e.DB(), installseam.Deals()).WithBlobstore(failingDeleteStore{Store: blobstore.NewMemory()})
	renderOfferAs(owner, t, h, offer)

	ashgrove := companyIDOf(e.SeedCompany(t, "Ashgrove Holdings", &e.Rep3))
	if changed := changeBuyerAs(owner, t, h, offer, ashgrove); changed.PdfAssetRef != nil {
		t.Errorf("the edit answered the rendering %q it retired", *changed.PdfAssetRef)
	}
	got, err := e.Deals.GetOffer(owner, offer, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the offer back: %v", err)
	}
	if got.BuyerCompanyId == nil || ids.UUID(*got.BuyerCompanyId) != ashgrove.UUID || got.PdfAssetRef != nil {
		t.Errorf("the committed edit did not stand: buyer=%v ref=%v", got.BuyerCompanyId, got.PdfAssetRef)
	}
}

// changeBuyerAs sends the buyer change through the real handler and answers the
// offer it returned.
func changeBuyerAs(ctx context.Context, t *testing.T, h deals.Handlers, offer ids.OfferID, buyer ids.CompanyID) crmcontracts.Offer {
	t.Helper()
	rec := httptest.NewRecorder()
	h.UpdateOffer(rec, httptest.NewRequest(http.MethodPatch, "/v1/offers/"+offer.String(),
		strings.NewReader(`{"buyer_company_id":"`+buyer.String()+`"}`)).WithContext(ctx),
		crmcontracts.Id(offer.UUID), crmcontracts.UpdateOfferParams{})
	if rec.Code != http.StatusOK {
		t.Fatalf("changing the draft's buyer = %d %s, want 200", rec.Code, rec.Body.String())
	}
	var changed crmcontracts.Offer
	if err := json.Unmarshal(rec.Body.Bytes(), &changed); err != nil {
		t.Fatalf("decoding the changed offer: %v", err)
	}
	return changed
}

// renderOfferAs renders through the real handler and answers the ref it stored.
func renderOfferAs(ctx context.Context, t *testing.T, h deals.Handlers, offer ids.OfferID) string {
	t.Helper()
	rec := httptest.NewRecorder()
	h.RenderOffer(rec, httptest.NewRequest(http.MethodPost, "/v1/offers/"+offer.String()+"/render", nil).WithContext(ctx),
		crmcontracts.Id(offer.UUID), crmcontracts.RenderOfferParams{})
	if rec.Code != http.StatusOK {
		t.Fatalf("rendering the offer = %d %s, want 200", rec.Code, rec.Body.String())
	}
	var rendered crmcontracts.Offer
	if err := json.Unmarshal(rec.Body.Bytes(), &rendered); err != nil || rendered.PdfAssetRef == nil {
		t.Fatalf("the render named no ref it stored (err=%v): %s", err, rec.Body.String())
	}
	return *rendered.PdfAssetRef
}

func downloadOfferPdfAs(ctx context.Context, h deals.Handlers, offer ids.OfferID) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.DownloadOfferPdf(rec, httptest.NewRequest(http.MethodGet, "/v1/offers/"+offer.String()+"/pdf", nil).WithContext(ctx),
		crmcontracts.Id(offer.UUID))
	return rec
}

// requireNoRendering holds a download to the answer an offer never rendered gets.
func requireNoRendering(t *testing.T, rec *httptest.ResponseRecorder, what string) {
	t.Helper()
	var problem AnyMap
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("%s = %d, not a problem document: %v", what, rec.Code, err)
	}
	if rec.Code != http.StatusNotFound || problem["code"] != "not_found" {
		t.Fatalf("%s = %d %v, want 404 not_found", what, rec.Code, problem["code"])
	}
}
