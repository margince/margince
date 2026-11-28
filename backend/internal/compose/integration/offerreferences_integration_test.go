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
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// offerDeskCompanyPerms is the offer desk that may also read companies.
//
// OfferDeskPerms carries no `company` grant at all.
// VisibleSubset withholds every id when the object grant is missing.
// A fixture built on it would pass whatever capture privacy did.
// That is the wrong subject.
// This set isolates the row-scope arm. The grant is present for both seats.
// The only thing separating them is who the company was captured private to.
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

// A send by a seat that cannot open the buyer company is refused.
//
// The send freezes the buyer's display and legal name onto the offer.
// A seat that may not read that company must not record them. The
// refusal leaves the offer a draft with nothing frozen, and its message names
// no company.
func TestASendByASeatThatCannotOpenTheBuyerIsRefused(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	admin := e.Admin()

	hidden := e.SeedCompany(t, "Ashgrove Holdings", &e.Rep2)
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Ashgrove renewal", pipeline, open, &e.Rep3))
	hiddenID := companyIDOf(hidden)
	if _, err := e.Deals.UpdateDeal(admin, deal, deals.UpdateDealInput{CompanyID: &hiddenID}); err != nil {
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
	e.MakeCapturePrivate(t, "company", hidden, e.Rep2)
	offer := ids.From[ids.OfferKind](ids.UUID(created.Id))

	_, err = e.Deals.SendOffer(desk, offer, nil)
	requireBuyerNotReadable(t, err, "Ashgrove")
	requireStillADraftWithNoSnapshot(t, e, offer)
}

// requireBuyerNotReadable holds a refusal to the one answer for an unreadable
// buyer. The message must not repeat the company's name.
func requireBuyerNotReadable(t *testing.T, err error, companyName string) {
	t.Helper()
	var refused *deals.BuyerNotReadableError
	if !errors.As(err, &refused) || !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("answer = %v, want a BuyerNotReadableError that is a permission denial", err)
	}
	if strings.Contains(err.Error(), companyName) {
		t.Errorf("the refusal names the company: %v", err)
	}
}

func requireStillADraftWithNoSnapshot(t *testing.T, e *Env, offer ids.OfferID) {
	t.Helper()
	var status, frozen string
	ctx := e.Admin()
	if err := database.WithTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT status, coalesce(buyer_snapshot::text, '') FROM offer WHERE id = $1`,
			offer.UUID).Scan(&status, &frozen)
	}); err != nil {
		t.Fatalf("reading the stored offer: %v", err)
	}
	if status != "draft" || frozen != "" {
		t.Errorf("the refused send left status=%q snapshot=%q, want a draft with nothing frozen", status, frozen)
	}
}

// A seat with no company grant at all is refused the send of an offer that
// names a buyer.
func TestASendWithoutTheCompanyGrantIsRefused(t *testing.T) {
	e := Setup(t)
	offer, _, _ := draftOfferOnAColleaguesCompany(t, e)
	wide := e.As(e.Rep3, []ids.UUID{e.Team1}, offerDeskWideNoCompanyPerms)

	_, err := e.Deals.SendOffer(wide, offer, nil)
	requireBuyerNotReadable(t, err, "Meridian")
	requireStillADraftWithNoSnapshot(t, e, offer)
}

// The same seat sends an offer that names no company.
func TestASendWithoutTheCompanyGrantSucceedsForAnOfferWithNoBuyer(t *testing.T) {
	e := Setup(t)
	wide := e.As(e.Rep3, []ids.UUID{e.Team1}, offerDeskWideNoCompanyPerms)
	pipeline, open, _ := DealFixture(t, e)
	bare := ids.From[ids.DealKind](e.SeedDeal(t, "No account yet", pipeline, open, &e.Rep3))
	description, price := "Retainer", int64(10000)
	created, err := e.Deals.CreateOffer(wide, bare, deals.CreateOfferInput{
		Currency: "EUR", Source: "manual",
		LineItems: []deals.OfferLineInputRow{{Description: &description, Quantity: "1", UnitPriceMinor: &price}},
	})
	if err != nil {
		t.Fatalf("create an offer with no buyer: %v", err)
	}
	if _, err := e.Deals.SendOffer(wide, ids.From[ids.OfferKind](ids.UUID(created.Id)), nil); err != nil {
		t.Errorf("sending an offer with no buyer company without the company grant: %v", err)
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
	if err := database.WithTx(ctx, e.Pool, func(tx pgx.Tx) error {
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

// The render is refused for a seat that cannot open the buyer.
//
// A document without the buyer block names nobody. A seat with no company
// grant and a seat that capture privacy hides this company from are both
// refused the same way. The offer here is sent, so the block would come from
// the frozen snapshot, and the refusal holds for it too.
func TestARenderIsRefusedForASeatThatCannotOpenTheBuyer(t *testing.T) {
	e := Setup(t)
	offer, _, _ := seedOfferOnAPrivateCompany(t, e)

	for name, perms := range map[string]principal.Permissions{
		"no company grant":      offerDeskWideNoCompanyPerms,
		"company hidden by row": offerDeskWideCompanyPerms,
	} {
		t.Run(name, func(t *testing.T) {
			desk := e.As(e.Rep1, []ids.UUID{e.Team1}, perms)
			_, err := e.Deals.PrepareRender(desk, offer)
			requireBuyerNotReadable(t, err, "Meridian")
		})
	}
}

// The refusal reaches the HTTP caller as a 403 and stores nothing.
func TestTheRenderEndpointAnswers403ForABuyerTheSeatCannotOpen(t *testing.T) {
	e := Setup(t)
	offer, _, _ := seedOfferOnAPrivateCompany(t, e)
	desk := e.As(e.Rep1, []ids.UUID{e.Team1}, offerDeskWideNoCompanyPerms)
	h := deals.NewHandlers(e.DB(), installseam.Deals()).WithBlobstore(blobstore.NewMemory())

	rec := httptest.NewRecorder()
	h.RenderOffer(rec, httptest.NewRequest(http.MethodPost, "/v1/offers/"+offer.String()+"/render", nil).WithContext(desk),
		crmcontracts.Id(offer.UUID), crmcontracts.RenderOfferParams{})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("render = %d %s, want 403", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "Meridian") {
		t.Errorf("the refusal names the company: %s", rec.Body.String())
	}
	var ref string
	ctx := e.Admin()
	if err := database.WithTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT coalesce(pdf_asset_ref, '') FROM offer WHERE id = $1`, offer.UUID).Scan(&ref)
	}); err != nil || ref != "" {
		t.Errorf("the refused render stored a rendering: ref=%q err=%v", ref, err)
	}
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
	if len(ingredients.BuyerBlock) > 3 {
		t.Errorf("the buyer block carries more than the company id and its two names: %v", ingredients.BuyerBlock)
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
	if !strings.Contains(offerPDFText(t, served.Body.Bytes()), "Meridian Labs") {
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
	if served.Code != http.StatusOK || !strings.Contains(offerPDFText(t, served.Body.Bytes()), "Ashgrove Holdings") {
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

// An agent presents its granting user's own permissions, so it is refused the
// buyer that user is refused. No agent tool sends or renders an offer, so this
// holds the store entry points an agent principal could still reach.
func TestAnAgentWhoseUserCannotOpenTheBuyerIsRefusedASend(t *testing.T) {
	e := Setup(t)
	draft, _, _ := draftOfferOnAColleaguesCompany(t, e)

	noGrant := e.AgentFor(t, e.Rep3, []ids.UUID{e.Team1}, offerDeskWideNoCompanyPerms)
	_, err := e.Deals.SendOffer(noGrant, draft, nil)
	requireBuyerNotReadable(t, err, "Meridian")
	requireStillADraftWithNoSnapshot(t, e, draft)
}

func TestAnAgentWhoseUserCannotOpenTheBuyerIsRefusedARender(t *testing.T) {
	e := Setup(t)
	sent, _, _ := seedOfferOnAPrivateCompany(t, e)

	hidden := e.AgentFor(t, e.Rep1, []ids.UUID{e.Team1}, offerDeskWideCompanyPerms)
	_, err := e.Deals.PrepareRender(hidden, sent)
	requireBuyerNotReadable(t, err, "Meridian")
}

// An archived buyer stays readable: the company read serves it under
// include_archived to a seat that may open it, so render and send do too.
func TestAnArchivedBuyerCompanyStillRendersAndSendsForASeatThatCanOpenIt(t *testing.T) {
	e := Setup(t)
	offer, _, company := draftOfferOnAColleaguesCompany(t, e)
	if _, err := e.Contacts.ArchiveCompany(e.Admin(), companyIDOf(company), nil); err != nil {
		t.Fatalf("archiving the buyer company: %v", err)
	}
	desk := e.As(e.Rep3, []ids.UUID{e.Team1}, offerDeskCompanyPerms)

	ingredients, err := e.Deals.PrepareRender(desk, offer)
	if err != nil || ingredients.BuyerBlock["display_name"] != "Meridian Labs" {
		t.Fatalf("render with an archived buyer = %v, %v; want the buyer block", ingredients.BuyerBlock, err)
	}
	if _, err := e.Deals.SendOffer(desk, offer, nil); err != nil {
		t.Errorf("send with an archived buyer: %v", err)
	}
}
