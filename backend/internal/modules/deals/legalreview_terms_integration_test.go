// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

// An approval binds to every legally material field of the current offer.
// Each field class is edited through the offers module's own writer. Each edit
// sends the deal back to legal. A send changes no term, so it does not.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// offerCtx is the requester with the offer verbs the edits need.
func (e *reviewEnv) offerCtx() context.Context {
	return humanWith(e.ws, e.admin, map[string]principal.ObjectGrant{
		"deal":     {Create: true, Read: true, Update: true},
		"offer":    {Create: true, Read: true, Update: true},
		"pipeline": {Create: true, Read: true, Update: true},
		"product":  {Read: true},
		"company":  {Read: true},
	})
}

// approvedDealWithOffer seeds a gated deal with a one-line draft offer, and an
// approval of it. It answers the deal, the offer, its line and the next stage.
func approvedDealWithOffer(t *testing.T, e *reviewEnv) (ids.DealID, ids.OfferID, ids.UUID, ids.StageID) {
	t.Helper()
	dealID, _, toID := gatedDeal(t, e)
	ctx := e.offerCtx()
	description, price, tax := "Implementation", int64(500_000), "19.00"
	terms, valid := "Payment within 30 days.", "2099-12-31"
	offer, err := e.store.CreateOffer(ctx, dealID, CreateOfferInput{
		Currency: "EUR", Source: "manual", TermsText: &terms, ValidUntil: &valid,
		LineItems: []OfferLineInputRow{{Description: &description, Quantity: "2", UnitPriceMinor: &price, TaxRate: &tax}},
	})
	if err != nil {
		t.Fatalf("seeding an offer: %v", err)
	}
	if offer.LineItems == nil || len(*offer.LineItems) != 1 {
		t.Fatalf("the seeded offer carried %v lines, want one", offer.LineItems)
	}
	e.answer(t, e.request(t, dealID, nil), ReviewApproved)
	return dealID, ids.From[ids.OfferKind](ids.UUID(offer.Id)), ids.UUID((*offer.LineItems)[0].Id), toID
}

func TestAnApprovalLapsesWhenAnyTermOfTheOfferChanges(t *testing.T) {
	for name, edit := range map[string]func(*reviewEnv, ids.OfferID, ids.UUID) error{
		"terms text": func(e *reviewEnv, offer ids.OfferID, _ ids.UUID) error {
			text := "Payment within 90 days."
			_, _, err := e.store.UpdateOffer(e.offerCtx(), offer, UpdateOfferInput{TermsText: &text})
			return err
		},
		"validity": func(e *reviewEnv, offer ids.OfferID, _ ids.UUID) error {
			valid := "2099-06-30"
			_, _, err := e.store.UpdateOffer(e.offerCtx(), offer, UpdateOfferInput{ValidUntil: &valid})
			return err
		},
		"line description": func(e *reviewEnv, offer ids.OfferID, line ids.UUID) error {
			text := "Implementation and training"
			_, err := e.store.UpdateOfferLineItem(e.offerCtx(), offer, line, UpdateOfferLineInput{Description: &text})
			return err
		},
		"quantity and so the totals": func(e *reviewEnv, offer ids.OfferID, line ids.UUID) error {
			quantity := "3"
			_, err := e.store.UpdateOfferLineItem(e.offerCtx(), offer, line, UpdateOfferLineInput{Quantity: &quantity})
			return err
		},
		"tax allocation": func(e *reviewEnv, offer ids.OfferID, line ids.UUID) error {
			tax := "7.00"
			_, err := e.store.UpdateOfferLineItem(e.offerCtx(), offer, line, UpdateOfferLineInput{TaxRate: &tax})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := setupReviewEnv(t)
			dealID, offer, line, toID := approvedDealWithOffer(t, e)
			if err := edit(e, offer, line); err != nil {
				t.Fatalf("editing the %s: %v", name, err)
			}
			var required *LegalReviewRequiredError
			if err := e.advance(dealID, toID); !errors.As(err, &required) || !required.TermsChanged {
				t.Fatalf("after the %s changed the advance answered %v, want the terms-changed refusal", name, err)
			}
		})
	}
}

// Sending the approved offer changes no term, so the approval stands.
func TestSendingTheApprovedOfferKeepsTheApproval(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, offer, _, toID := approvedDealWithOffer(t, e)
	if _, err := e.store.SendOffer(e.offerCtx(), offer, nil); err != nil {
		t.Fatalf("sending the offer: %v", err)
	}
	if err := e.advance(dealID, toID); err != nil {
		t.Fatalf("sending the approved offer lapsed the approval: %v", err)
	}
}

// No offer at approval and one added since are different terms.
func TestAnOfferAddedAfterTheApprovalLapsesIt(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, toID := gatedDeal(t, e)
	e.answer(t, e.request(t, dealID, nil), ReviewApproved)
	description, price, tax := "Implementation", int64(500_000), "19.00"
	if _, err := e.store.CreateOffer(e.offerCtx(), dealID, CreateOfferInput{
		Currency: "EUR", Source: "manual",
		LineItems: []OfferLineInputRow{{Description: &description, Quantity: "1", UnitPriceMinor: &price, TaxRate: &tax}},
	}); err != nil {
		t.Fatalf("adding an offer: %v", err)
	}
	var required *LegalReviewRequiredError
	if err := e.advance(dealID, toID); !errors.As(err, &required) || !required.TermsChanged {
		t.Fatalf("an offer added after the approval answered %v, want the terms-changed refusal", err)
	}
}

// Once the approved offer is accepted, a second offer sent to the buyer binds
// too. Legal has not read it.
func TestASecondBindingOfferLapsesTheApproval(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, toID := gatedDeal(t, e)
	first := e.seedOffer(t, dealID, "Implementation")
	if _, err := e.store.SendOffer(e.offerCtx(), first, nil); err != nil {
		t.Fatalf("sending the first offer: %v", err)
	}
	if _, err := e.store.AcceptOffer(e.offerCtx(), first, nil); err != nil {
		t.Fatalf("accepting the first offer: %v", err)
	}
	e.answer(t, e.request(t, dealID, nil), ReviewApproved)
	second := e.seedOffer(t, dealID, "Extra licences")
	if _, err := e.store.SendOffer(e.offerCtx(), second, nil); err != nil {
		t.Fatalf("sending the second offer: %v", err)
	}
	var required *LegalReviewRequiredError
	if err := e.advance(dealID, toID); !errors.As(err, &required) || !required.TermsChanged {
		t.Fatalf("a second offer sent after the approval answered %v, want the terms-changed refusal", err)
	}
}

// Re-addressing the approved offer to another company changes who is bound.
func TestReaddressingTheApprovedOfferLapsesIt(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, offer, _, toID := approvedDealWithOffer(t, e)
	other := ids.NewV7()
	if _, err := e.owner.Exec(t.Context(), `
		INSERT INTO company (id, display_name, source, captured_by, owner_id) VALUES ($1, 'Other Buyer', 'manual', 'human:fixture', $2)`,
		other, e.admin); err != nil {
		t.Fatal(err)
	}
	buyer := ids.From[ids.CompanyKind](other)
	if _, _, err := e.store.UpdateOffer(e.offerCtx(), offer, UpdateOfferInput{BuyerCompanyID: &buyer}); err != nil {
		t.Fatalf("re-addressing the offer: %v", err)
	}
	var required *LegalReviewRequiredError
	if err := e.advance(dealID, toID); !errors.As(err, &required) || !required.TermsChanged {
		t.Fatalf("a re-addressed offer answered %v, want the terms-changed refusal", err)
	}
}

func (e *reviewEnv) seedOffer(t *testing.T, dealID ids.DealID, description string) ids.OfferID {
	t.Helper()
	price, tax := int64(500_000), "19.00"
	offer, err := e.store.CreateOffer(e.offerCtx(), dealID, CreateOfferInput{
		Currency: "EUR", Source: "manual",
		LineItems: []OfferLineInputRow{{Description: &description, Quantity: "1", UnitPriceMinor: &price, TaxRate: &tax}},
	})
	if err != nil {
		t.Fatalf("seeding an offer: %v", err)
	}
	return ids.From[ids.OfferKind](ids.UUID(offer.Id))
}

// The pack shows what the approval binds to: the intro, each line's billing
// term, and every other offer the buyer holds.
func TestThePackShowsEveryTermTheApprovalBindsTo(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, _ := gatedDeal(t, e)
	intro, description, price, tax := "Dear buyer,", "Support", int64(100_000), "19.00"
	recurring, months, periods := "recurring", 1, 36
	created, err := e.store.CreateOffer(e.offerCtx(), dealID, CreateOfferInput{
		Currency: "EUR", Source: "manual", IntroText: &intro,
		LineItems: []OfferLineInputRow{{
			Description: &description, Quantity: "1", UnitPriceMinor: &price, TaxRate: &tax,
			BillingModel: &recurring, BillingIntervalMonths: &months, IntervalCount: &periods,
		}},
	})
	if err != nil {
		t.Fatalf("seeding the recurring offer: %v", err)
	}
	shown := ids.From[ids.OfferKind](ids.UUID(created.Id))
	if _, err := e.store.SendOffer(e.offerCtx(), shown, nil); err != nil {
		t.Fatalf("sending the recurring offer: %v", err)
	}
	if _, err := e.store.AcceptOffer(e.offerCtx(), shown, nil); err != nil {
		t.Fatalf("accepting the recurring offer: %v", err)
	}
	second := e.seedOffer(t, dealID, "Extra licences")
	if _, err := e.store.SendOffer(e.offerCtx(), second, nil); err != nil {
		t.Fatalf("sending the second offer: %v", err)
	}
	got, err := e.store.GetLegalReview(humanWith(e.ws, e.reviewer, map[string]principal.ObjectGrant{
		legalReviewEntity: {Read: true}, "deal": {Read: true}, "offer": {Read: true},
	}), e.request(t, dealID, nil))
	if err != nil {
		t.Fatalf("reading the review: %v", err)
	}
	offer := got.Packet.Offer
	if offer == nil || offer.IntroText == nil || *offer.IntroText != intro || len(offer.Lines) != 1 {
		t.Fatalf("the pack showed the offer %+v, want the intro and one line", offer)
	}
	line := offer.Lines[0]
	if line.BillingModel == nil || *line.BillingModel != recurring || line.IntervalCount == nil || *line.IntervalCount != periods {
		t.Fatalf("the pack showed the line %+v, want a recurring line over %d periods", line, periods)
	}
	if got.Packet.Binding == nil || len(*got.Packet.Binding) != 1 || ids.UUID((*got.Packet.Binding)[0].Id) != second.UUID {
		t.Fatalf("the pack showed binding offers %v, want the second sent offer", got.Packet.Binding)
	}
}
