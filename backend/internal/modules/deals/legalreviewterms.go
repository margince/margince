// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The terms a legal approval binds to: the deal's amount and currency, and
// every legally material field of its current offer, lines included.
//
// One read serves both halves: the pack shows every hashed term but two ids.
// A changed product or buyer id only asks legal again, never less. The caller
// holds the deal row lock, and every offer write takes it first
// (lockOfferDeal). So no offer edit lands between the read and the commit.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// reviewTerms is what legal approved. The JSON is the fingerprint's input, so
// a field leaves the hash only by a `json:"-"` that says why.
type reviewTerms struct {
	AmountMinor *int64    `json:"amount_minor"`
	Currency    *string   `json:"currency"`
	CompanyID   *ids.UUID `json:"company_id"`
	// Offer is the one the pack shows. Binding is every other offer the buyer
	// has been sent or has accepted, so a second binding offer changes the
	// terms too.
	Offer   *offerTerms  `json:"offer"`
	Binding []offerTerms `json:"binding"`
}

type offerTerms struct {
	ID             ids.UUID  `json:"id"`
	OfferNumber    string    `json:"offer_number"`
	Revision       int       `json:"revision"`
	BuyerCompanyID *ids.UUID `json:"buyer_company_id"`
	// Status and the PDF move without changing what the offer says: a buyer
	// accepting the approved terms must not send the deal back to legal.
	Status     string          `json:"-"`
	HasPDF     bool            `json:"-"`
	ValidUntil *time.Time      `json:"-"`
	ValidDay   *string         `json:"valid_until"`
	Currency   string          `json:"currency"`
	NetMinor   int64           `json:"net_minor"`
	TaxMinor   int64           `json:"tax_minor"`
	GrossMinor int64           `json:"gross_minor"`
	IntroText  *string         `json:"intro_text"`
	TermsText  *string         `json:"terms_text"`
	Lines      []offerLineTerm `json:"lines"`
}

// offerLineTerm is one line as the buyer reads it. Numerics travel as their
// own text, so 1.50 and 1.5 hash as Postgres prints them, never as a float.
type offerLineTerm struct {
	Position       int       `json:"position"`
	ProductID      *ids.UUID `json:"product_id"`
	Description    string    `json:"description"`
	Unit           *string   `json:"unit"`
	Quantity       string    `json:"quantity"`
	UnitPriceMinor int64     `json:"unit_price_minor"`
	DiscountPct    *string   `json:"discount_pct"`
	TaxRate        *string   `json:"tax_rate"`
	BillingModel   *string   `json:"billing_model"`
	IntervalMonths *int      `json:"billing_interval_months"`
	IntervalCount  *int      `json:"interval_count"`
	ProposalState  string    `json:"proposal_state"`
}

// readReviewTerms reads the terms past the caller's field masks: they are
// hashed and compared, and only the offer, which the requester reads, is shown.
func readReviewTerms(ctx context.Context, tx pgx.Tx, dealID ids.DealID) (reviewTerms, error) {
	var out reviewTerms
	if err := tx.QueryRow(ctx, `SELECT amount_minor, currency, company_id FROM deal WHERE id = $1`, dealID).
		Scan(&out.AmountMinor, &out.Currency, &out.CompanyID); err != nil {
		return out, notFoundOr(err, "read the deal's terms")
	}
	current, err := readOfferTerms(ctx, tx, `
		 WHERE deal_id = $1 AND archived_at IS NULL AND status <> 'superseded'
		 ORDER BY CASE status WHEN 'accepted' THEN 0 WHEN 'sent' THEN 1 WHEN 'draft' THEN 2 ELSE 3 END,
		          created_at DESC, id DESC
		 LIMIT 1`, dealID)
	if err != nil {
		return out, err
	}
	if len(current) == 1 {
		out.Offer = &current[0]
	}
	binding, err := readOfferTerms(ctx, tx, `
		 WHERE deal_id = $1 AND archived_at IS NULL AND status IN ('sent', 'accepted')
		 ORDER BY id`, dealID)
	// The shown offer is hashed once, as Offer, so sending the approved draft
	// leaves the terms as they were.
	out.Binding = slices.DeleteFunc(binding, func(o offerTerms) bool {
		return out.Offer != nil && o.ID == out.Offer.ID
	})
	return out, err
}

// fingerprint hashes the terms. Equal terms hash equal however they were read.
func (t reviewTerms) fingerprint() (string, error) {
	raw, err := json.Marshal(t)
	if err != nil {
		return "", fmt.Errorf("encode the reviewed terms: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// readOfferTerms reads the offers a fixed clause selects, each with its
// lines. The pack's offer is the newest live one not superseded, preferring an
// accepted one, then a sent one, then a draft.
func readOfferTerms(ctx context.Context, tx pgx.Tx, clause string, dealID ids.DealID) ([]offerTerms, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, offer_number, revision, buyer_company_id, status, pdf_asset_ref IS NOT NULL,
		       valid_until, valid_until::text, currency, net_minor, tax_minor, gross_minor,
		       intro_text, terms_text
		  FROM offer`+clause, dealID)
	if err != nil {
		return nil, fmt.Errorf("read the deal's offers: %w", err)
	}
	offers, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (offerTerms, error) {
		var o offerTerms
		err := row.Scan(&o.ID, &o.OfferNumber, &o.Revision, &o.BuyerCompanyID, &o.Status, &o.HasPDF,
			&o.ValidUntil, &o.ValidDay, &o.Currency, &o.NetMinor, &o.TaxMinor, &o.GrossMinor,
			&o.IntroText, &o.TermsText)
		return o, err
	})
	if err != nil {
		return nil, fmt.Errorf("read the deal's offers: %w", err)
	}
	for i := range offers {
		if offers[i].Lines, err = offerLineTerms(ctx, tx, offers[i].ID); err != nil {
			return nil, err
		}
	}
	if offers == nil {
		offers = []offerTerms{}
	}
	return offers, nil
}

func offerLineTerms(ctx context.Context, tx pgx.Tx, offerID ids.UUID) ([]offerLineTerm, error) {
	rows, err := tx.Query(ctx, `
		SELECT position, product_id, description, unit, quantity::text, unit_price_minor,
		       discount_pct::text, tax_rate::text, billing_model,
		       billing_interval_months, interval_count, proposal_state
		  FROM offer_line_item
		 WHERE offer_id = $1
		 ORDER BY position, id`, offerID)
	if err != nil {
		return nil, fmt.Errorf("read the offer's lines: %w", err)
	}
	lines, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (offerLineTerm, error) {
		var l offerLineTerm
		err := row.Scan(&l.Position, &l.ProductID, &l.Description, &l.Unit, &l.Quantity,
			&l.UnitPriceMinor, &l.DiscountPct, &l.TaxRate, &l.BillingModel,
			&l.IntervalMonths, &l.IntervalCount, &l.ProposalState)
		return l, err
	})
	if err != nil {
		return nil, fmt.Errorf("read the offer's lines: %w", err)
	}
	if lines == nil {
		lines = []offerLineTerm{}
	}
	return lines, nil
}

// lockOfferDeal takes the row lock of the deal an offer belongs to, before
// the offer's own. The legal gate reads a deal's offers under that lock. So an
// offer edit lands before the gate reads, or waits for it to commit.
func lockOfferDeal(ctx context.Context, tx pgx.Tx, offerID ids.OfferID) error {
	if _, err := tx.Exec(ctx, `
		SELECT 1 FROM deal WHERE id = (SELECT deal_id FROM offer WHERE id = $1) FOR UPDATE`, offerID); err != nil {
		return fmt.Errorf("lock the offer's deal: %w", err)
	}
	return nil
}
