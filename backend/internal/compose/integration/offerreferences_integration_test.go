// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An offer is anchored on its DEAL, and every seat of the workspace reads every
// deal — a deal is customer identity. The organization the offer names is not:
// capture privacy makes an organization private to the colleague who captured
// it. So an offer read handed back a reference the reader's own organization
// read would refuse, which is the existence oracle dealreferences closed on the
// deal itself, arriving one table along (#2004).
//
// The buyer snapshot is the sharper half. It is the buyer's legal block frozen
// at send — display name, and where the record carries one, legal name — so an
// offer read disclosed strictly more than the id the deal read withholds.

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// offerDeskCompanyPerms is the offer desk that may also read companies.
//
// offerDeskPerms carries no `organization` grant at all, and VisibleSubset
// withholds every id when the object grant is missing — so a fixture built on
// it would pass whatever capture privacy did, which is the wrong subject. This
// set isolates the row-scope arm: the grant is present for both seats, and the
// only thing separating them is who the company was captured private to.
var offerDeskCompanyPerms = principal.Permissions{
	RoleKeys: []string{"deal_desk"},
	Objects: map[string]principal.ObjectGrant{
		"deal":                  {Create: true, Read: true, Update: true},
		"offer":                 {Create: true, Read: true, Update: true},
		"organization":          {Read: true},
		"installation_settings": {Read: true},
	},
}

// seedOfferOnAPrivateOrg is one sent offer whose buyer is capture-private to a
// colleague, created and sent by an admin who can see it.
//
// Sent, not draft: the snapshot only exists after a send, and it is the field
// that carries the name.
func seedOfferOnAPrivateOrg(t *testing.T, e *Env) (ids.OfferID, ids.DealID, ids.UUID) {
	t.Helper()
	pipeline, open, _ := DealFixture(t, e)
	admin := e.Admin()

	// Workspace-visible while the admin links it, so the write passes its own
	// EnsureLinkTarget gate; capture privacy lands afterwards, which is the
	// order a connector-captured company reaches this state in anyway.
	privateOrg := e.SeedOrg(t, "Meridian Labs", &e.Rep3)
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Meridian renewal", pipeline, open, &e.Rep3))
	orgID := orgIDOf(privateOrg)
	if _, err := e.Deals.UpdateDeal(admin, deal, deals.UpdateDealInput{OrganizationID: &orgID}); err != nil {
		t.Fatalf("linking the deal to its organization: %v", err)
	}

	// Created and sent by the colleague the organization belongs to, before it
	// becomes private: that is the order this state actually arises in, and it
	// leaves a real snapshot for the refusals below to be about.
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
	// The sender's own response still names the buyer — otherwise the refusal
	// below would hold against an offer that never had one.
	if sent.BuyerOrgId == nil || sent.BuyerSnapshot == nil {
		t.Fatalf("the sent offer names no buyer for the seat that sent it (id=%v snapshot=%v) — the fixture proves nothing about withholding",
			sent.BuyerOrgId, sent.BuyerSnapshot)
	}
	e.MakeCapturePrivate(t, "organization", privateOrg, e.Rep3)
	return offer, deal, privateOrg
}

// A reader who cannot open the organization gets the offer without it.
func TestAnOfferWithholdsABuyerTheReaderCannotOpen(t *testing.T) {
	e := Setup(t)
	offer, deal, _ := seedOfferOnAPrivateOrg(t, e)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, offerDeskCompanyPerms)

	got, err := e.Deals.GetOffer(rep, offer, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the offer: %v — the offer is anchored on a deal this seat reads, so the read itself must succeed", err)
	}
	if got.BuyerOrgId != nil {
		t.Errorf("the offer names buyer organization %v, which this seat's own organization read would refuse — the id alone is an existence oracle over a capture-private company", *got.BuyerOrgId)
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
	if page[0].BuyerOrgId != nil || page[0].BuyerSnapshot != nil {
		t.Errorf("the listed offer names its buyer (id=%v snapshot=%v)", page[0].BuyerOrgId, page[0].BuyerSnapshot)
	}
}

// And the colleague who CAN open it still sees it, so the rule withholds from a
// reader rather than emptying the field for everyone.
func TestAnOfferKeepsABuyerTheReaderCanOpen(t *testing.T) {
	e := Setup(t)
	offer, _, _ := seedOfferOnAPrivateOrg(t, e)

	owner := e.As(e.Rep3, []ids.UUID{e.Team1}, offerDeskCompanyPerms)
	got, err := e.Deals.GetOffer(owner, offer, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the offer as the organization's owner: %v", err)
	}
	if got.BuyerOrgId == nil {
		t.Error("the offer withholds its buyer from the colleague the organization is private TO — the rule is about who may open the record, not about hiding it from everybody")
	}
	if got.BuyerSnapshot == nil {
		t.Error("the offer withholds the buyer snapshot from the colleague who can open the organization")
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
	hidden := e.SeedOrg(t, "Ashgrove Holdings", &e.Rep2)
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Ashgrove renewal", pipeline, open, &e.Rep3))
	hiddenID := orgIDOf(hidden)
	if _, err := e.Deals.UpdateDeal(admin, deal, deals.UpdateDealInput{OrganizationID: &hiddenID}); err != nil {
		t.Fatalf("linking the deal to its organization: %v", err)
	}
	e.MakeCapturePrivate(t, "organization", hidden, e.Rep2)

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
	if sent.BuyerOrgId != nil || sent.BuyerSnapshot != nil {
		t.Errorf("the send response names a buyer this seat cannot open (id=%v snapshot=%v)",
			sent.BuyerOrgId, sent.BuyerSnapshot)
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
	offer, _, privateOrg := seedOfferOnAPrivateOrg(t, e)

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
	if !strings.Contains(frozen, privateOrg.String()) {
		t.Errorf("the stored buyer snapshot does not name the organization the offer was sent to: %s", frozen)
	}
}
