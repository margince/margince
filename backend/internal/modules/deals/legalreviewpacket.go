// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The review pack: the deal as it stood when legal was asked.
//
// Built from the requester's own reads, so the pack carries nothing the
// requester could not open. The deal is read through their field masks. The
// documents come through the activities reader under their principal.
//
// It freezes facts, not names. A document, the deal, its company and owner
// are kept as ids. Their names are read when a reviewer opens the pack, so
// erasing or renaming the source leaves no copy here.
// The storage type is not the wire type, so a contract rename never breaks an
// older pack.

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type reviewPacket struct {
	Deal          packetDeal       `json:"deal"`
	ValueMinor    *int64           `json:"value_minor,omitempty"`
	ValueCurrency *string          `json:"value_currency,omitempty"`
	Offer         *packetOffer     `json:"offer,omitempty"`
	Binding       []packetOffer    `json:"binding,omitempty"`
	History       []packetStage    `json:"history"`
	Documents     []PacketDocument `json:"documents"`
	// Terms fingerprints what legal approved (legalreviewterms.go). The stage
	// gate compares it with the deal at the move. It never leaves the server.
	Terms string `json:"terms"`
}

type packetDeal struct {
	ID                ids.UUID   `json:"id"`
	CompanyID         *ids.UUID  `json:"company_id,omitempty"`
	AmountMinor       *int64     `json:"amount_minor,omitempty"`
	Currency          *string    `json:"currency,omitempty"`
	StageID           ids.UUID   `json:"stage_id"`
	StageName         string     `json:"stage_name"`
	ExpectedCloseDate *time.Time `json:"expected_close_date,omitempty"`
	OwnerID           *ids.UUID  `json:"owner_id,omitempty"`
}

type packetOffer struct {
	ID          ids.UUID          `json:"id"`
	OfferNumber string            `json:"offer_number"`
	Revision    int               `json:"revision"`
	Status      string            `json:"status"`
	NetMinor    int64             `json:"net_minor"`
	TaxMinor    int64             `json:"tax_minor"`
	GrossMinor  int64             `json:"gross_minor"`
	Currency    string            `json:"currency"`
	ValidUntil  *time.Time        `json:"valid_until,omitempty"`
	IntroText   *string           `json:"intro_text,omitempty"`
	TermsText   *string           `json:"terms_text,omitempty"`
	HasPDF      bool              `json:"has_pdf"`
	Lines       []packetOfferLine `json:"lines"`
}

type packetOfferLine struct {
	Position       int     `json:"position"`
	Description    string  `json:"description"`
	Unit           *string `json:"unit,omitempty"`
	Quantity       string  `json:"quantity"`
	UnitPriceMinor int64   `json:"unit_price_minor"`
	DiscountPct    *string `json:"discount_pct,omitempty"`
	TaxRate        *string `json:"tax_rate,omitempty"`
	BillingModel   *string `json:"billing_model,omitempty"`
	IntervalMonths *int    `json:"billing_interval_months,omitempty"`
	IntervalCount  *int    `json:"interval_count,omitempty"`
	Staged         bool    `json:"staged,omitempty"`
}

type packetStage struct {
	StageName string    `json:"stage_name"`
	EnteredAt time.Time `json:"entered_at"`
}

func (p reviewPacket) marshal() ([]byte, error) {
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("encode the review pack: %w", err)
	}
	return raw, nil
}

// buildReviewPacket reads the pack inside the request's transaction.
func (s *Store) buildReviewPacket(
	ctx context.Context, tx pgx.Tx, dealID ids.DealID, docs []PacketDocument,
) (reviewPacket, error) {
	var out reviewPacket
	deal, err := readDealForCaller(ctx, tx, dealID, storekit.LiveOnly, nil)
	if err != nil {
		return out, err
	}
	if out.Deal, err = packetDealOf(ctx, tx, deal); err != nil {
		return out, err
	}
	if out.ValueMinor, out.ValueCurrency, err = s.priceForQueue(ctx, tx, deal); err != nil {
		return out, err
	}
	terms, err := readReviewTerms(ctx, tx, dealID)
	if err != nil {
		return out, err
	}
	if terms.Offer != nil {
		out.Offer = packetOfferOf(*terms.Offer)
	}
	for _, o := range terms.Binding {
		out.Binding = append(out.Binding, *packetOfferOf(o))
	}
	if out.Terms, err = terms.fingerprint(); err != nil {
		return out, err
	}
	if out.History, err = stagesEntered(ctx, tx, dealID); err != nil {
		return out, err
	}
	out.Documents = docs
	if out.Documents == nil {
		out.Documents = []PacketDocument{}
	}
	return out, nil
}

// packetDealOf copies the deal's facts and names its stage. The company comes
// off the masked read, so one the requester could not open is left out.
func packetDealOf(ctx context.Context, tx pgx.Tx, d crmcontracts.Deal) (packetDeal, error) {
	out := packetDeal{
		ID: ids.UUID(d.Id), AmountMinor: d.AmountMinor,
		CompanyID: (*ids.UUID)(d.CompanyId), OwnerID: (*ids.UUID)(d.OwnerId),
	}
	if d.Currency != nil {
		currency := string(*d.Currency)
		out.Currency = &currency
	}
	if d.ExpectedCloseDate != nil {
		out.ExpectedCloseDate = &d.ExpectedCloseDate.Time
	}
	if d.StageId != nil {
		out.StageID = ids.UUID(*d.StageId)
	}
	if !auth.ReadGranted(ctx, "company") {
		out.CompanyID = nil
	}
	err := tx.QueryRow(ctx, `SELECT name FROM stage WHERE id = $1`, out.StageID).Scan(&out.StageName)
	if err != nil {
		return out, fmt.Errorf("read the stage the review pack shows: %w", err)
	}
	return out, nil
}

// priceForQueue prices the deal's amount in the base currency on the request
// day. An open deal holds no base amount of its own, and the queue sorts
// deals of every currency on one scale. Unpriceable is no value, never zero.
func (s *Store) priceForQueue(
	ctx context.Context, tx pgx.Tx, d crmcontracts.Deal,
) (*int64, *string, error) {
	if d.AmountMinor == nil || d.Currency == nil {
		return nil, nil, nil
	}
	base, err := s.installation.BaseCurrency(ctx, tx)
	if err != nil {
		return nil, nil, err
	}
	priced, err := PriceAll(ctx, tx, NewFXRates(base, s.clock()),
		[]CurrencyAmount{{Minor: *d.AmountMinor, Currency: string(*d.Currency)}})
	if err != nil {
		return nil, nil, err
	}
	if !priced[0].Priced {
		return nil, nil, nil
	}
	return &priced[0].Minor, &base, nil
}

// stagesEntered is the pack's short history: each stage the deal entered,
// oldest first.
func stagesEntered(ctx context.Context, tx pgx.Tx, dealID ids.DealID) ([]packetStage, error) {
	rows, err := tx.Query(ctx, `
		SELECT s.name, h.changed_at
		  FROM deal_stage_history h JOIN stage s ON s.id = h.to_stage_id
		 WHERE h.deal_id = $1
		 ORDER BY h.changed_at, h.id`, dealID)
	if err != nil {
		return nil, fmt.Errorf("read the deal's stage history: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (packetStage, error) {
		var st packetStage
		err := row.Scan(&st.StageName, &st.EnteredAt)
		return st, err
	})
}

// packetOfferOf is the offer as the pack shows it, from the read the
// fingerprint hashed.
func packetOfferOf(o offerTerms) *packetOffer {
	out := &packetOffer{
		ID: o.ID, OfferNumber: o.OfferNumber, Revision: o.Revision, Status: o.Status,
		NetMinor: o.NetMinor, TaxMinor: o.TaxMinor, GrossMinor: o.GrossMinor, Currency: o.Currency,
		ValidUntil: o.ValidUntil, IntroText: o.IntroText, TermsText: o.TermsText, HasPDF: o.HasPDF,
		Lines: make([]packetOfferLine, 0, len(o.Lines)),
	}
	for _, l := range o.Lines {
		out.Lines = append(out.Lines, packetOfferLine{
			Position: l.Position, Description: l.Description, Unit: l.Unit, Quantity: l.Quantity,
			UnitPriceMinor: l.UnitPriceMinor, DiscountPct: l.DiscountPct, TaxRate: l.TaxRate,
			BillingModel: l.BillingModel, IntervalMonths: l.IntervalMonths, IntervalCount: l.IntervalCount,
			Staged: l.ProposalState == "staged",
		})
	}
	return out
}
