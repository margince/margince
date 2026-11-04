// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// A line with a tax category takes the sheet's rate for the country of the
// offer's legal entity. A draft reads today's rate and the send reads the
// send day's, after which the line never changes. A typed rate has no
// category and stays. A rate the sheet lacks is refused, never guessed.

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// lineIDKey names a line in the evidence of the offer write it caused.
const lineIDKey = "line_id"

// exemptCategory is the category with no row on the sheet: it is taxed at 0.
const exemptCategory = "exempt"

// TaxRateMissingError refuses a line or a send whose category has no rate on
// the sheet for the offer's country and day.
type TaxRateMissingError struct {
	Country, Category string
	Day               time.Time
}

func (e *TaxRateMissingError) Error() string {
	country := e.Country
	if country == "" {
		country = "the offer's country, which neither its legal entity nor the installation names"
	}
	return fmt.Sprintf("no %s tax rate for %s on %s", e.Category, country, e.Day.Format(time.DateOnly))
}

// MessageFault names the condition and no field: the rate is data an admin
// records on the sheet, not an argument the caller sent.
func (e *TaxRateMissingError) MessageFault() (code, message string) {
	return "tax_rate_missing", e.Error() + "; an admin must record the rate under Settings, Tax rates first"
}

// TaxRateAndCategoryError refuses a line that names a category and a rate.
// The category asks for the sheet's rate, so a typed one contradicts it.
type TaxRateAndCategoryError struct{}

func (e *TaxRateAndCategoryError) Error() string {
	return "give a line a tax_category or a tax_rate, not both"
}

// FieldFault names the category, the field that asks for the sheet's rate.
func (e *TaxRateAndCategoryError) FieldFault() (field, code, message string) {
	return taxCategoryField, "tax_rate_and_category", e.Error()
}

// taxBasis is where and when an offer's category lines read their rate.
type taxBasis struct {
	// Country is lower case, empty when neither the entity nor the
	// installation names one.
	Country string
	Day     time.Time
}

// offerTaxBasis is the country of the offer's legal entity, else the
// installation's, on day. A nil entity is the default entity.
func (s *Store) offerTaxBasis(ctx context.Context, tx pgx.Tx, entity *ids.LegalEntityID, day time.Time) (taxBasis, error) {
	issuer, err := s.installation.IssuerEntity(ctx, tx, entity)
	if err != nil {
		return taxBasis{}, fmt.Errorf("read the issuing entity's country: %w", err)
	}
	country := strings.ToLower(strings.TrimSpace(issuer.Country))
	if country == "" {
		if country, err = s.installation.Country(ctx, tx); err != nil {
			return taxBasis{}, fmt.Errorf("read the installation's country: %w", err)
		}
	}
	return taxBasis{Country: country, Day: day}, nil
}

// lazyTaxBasis answers a tax basis on its first call and the same one after.
// An offer with no category line never reads the country or the day.
type lazyTaxBasis func() (taxBasis, error)

// draftTaxBasis is offerTaxBasis on today, in the installation's zone. The
// zone is read without the settings grant: writing a line needs only offer.
func (s *Store) draftTaxBasis(ctx context.Context, tx pgx.Tx, entity *ids.LegalEntityID) lazyTaxBasis {
	return sync.OnceValues(func() (taxBasis, error) {
		day, err := s.todayIn(ctx, tx, s.installation.TimezoneApplied)
		if err != nil {
			return taxBasis{}, err
		}
		return s.offerTaxBasis(ctx, tx, entity, day)
	})
}

// sheetTaxRate is the rate the sheet holds for category in basis: the latest
// row in force on the day. Exempt is 0 and needs no row.
func sheetTaxRate(ctx context.Context, tx pgx.Tx, basisOf lazyTaxBasis, category string) (string, error) {
	if category == exemptCategory {
		return zeroPct, nil
	}
	basis, err := basisOf()
	if err != nil {
		return "", err
	}
	missing := &TaxRateMissingError{Country: basis.Country, Category: category, Day: basis.Day}
	if basis.Country == "" {
		return "", missing
	}
	var rate string
	err = tx.QueryRow(ctx,
		`SELECT rate::text FROM tax_rate
		 WHERE country = $1 AND category = $2 AND valid_from <= $3
		 ORDER BY valid_from DESC LIMIT 1`,
		basis.Country, category, basis.Day).Scan(&rate)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", missing
	}
	if err != nil {
		return "", fmt.Errorf("read the %s tax rate: %w", category, err)
	}
	return rate, nil
}

// checkLineTaxCategory refuses a category a caller named beside a typed rate,
// or one no line can take. Naming none is always fine.
func checkLineTaxCategory(asked, typedRate *string) error {
	if asked == nil {
		return nil
	}
	if typedRate != nil {
		return &TaxRateAndCategoryError{}
	}
	return checkTaxCategory(*asked)
}

// stagedLineTax gives a drafted line grounded on a classified product that
// product's category and the sheet's rate. A line on no product, or on an
// unclassified one, keeps the rate the drafter proposed.
func stagedLineTax(ctx context.Context, tx pgx.Tx, basis lazyTaxBasis, in *StagedOfferLineInput) error {
	if in.ProductID == nil {
		return nil
	}
	var category *string
	err := tx.QueryRow(ctx,
		`SELECT tax_category FROM product WHERE id = $1 AND archived_at IS NULL`, *in.ProductID).Scan(&category)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.NameMissingReference("product_id", apperrors.ErrNotFound)
	}
	if err != nil || category == nil {
		return err
	}
	rate, err := sheetTaxRate(ctx, tx, basis, *category)
	if err != nil {
		return err
	}
	in.TaxRate, in.taxCategory = rate, category
	return nil
}

// patchedCategoryRate turns a patch's tax category into the rate the sheet
// holds for the draft today, so the patch writes both together.
func (s *Store) patchedCategoryRate(ctx context.Context, tx pgx.Tx, current crmcontracts.Offer, in UpdateOfferLineInput) (UpdateOfferLineInput, error) {
	if err := checkLineTaxCategory(in.TaxCategory, in.TaxRate); err != nil || in.TaxCategory == nil {
		return in, err
	}
	rate, err := sheetTaxRate(ctx, tx, s.draftTaxBasis(ctx, tx, legalEntityIDOf(current)), *in.TaxCategory)
	if err != nil {
		return in, err
	}
	in.TaxRate = &rate
	return in, nil
}

// retaxedLine is one category line whose rate the sheet moved.
type retaxedLine struct {
	ID       ids.UUID
	Was, Now string
}

// retaxCategoryLines reads every category line on the offer again against
// basis and writes the rates that moved. It answers the lines it changed; the
// caller recomputes the totals.
func retaxCategoryLines(ctx context.Context, tx pgx.Tx, offerID ids.OfferID, basis lazyTaxBasis) ([]retaxedLine, error) {
	rows, err := tx.Query(ctx,
		`SELECT id, tax_category, tax_rate::text, version FROM offer_line_item
		 WHERE offer_id = $1 AND tax_category IS NOT NULL ORDER BY position`, offerID)
	if err != nil {
		return nil, fmt.Errorf("read the offer's category lines: %w", err)
	}
	type categoryLine struct {
		id             ids.UUID
		category, rate string
		version        int64
	}
	lines, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (categoryLine, error) {
		var l categoryLine
		return l, row.Scan(&l.id, &l.category, &l.rate, &l.version)
	})
	if err != nil {
		return nil, fmt.Errorf("read the offer's category lines: %w", err)
	}
	var moved []retaxedLine
	for _, line := range lines {
		rate, err := sheetTaxRate(ctx, tx, basis, line.category)
		if err != nil {
			return nil, err
		}
		if rate == line.rate {
			continue
		}
		tag, err := tx.Exec(ctx,
			`UPDATE offer_line_item SET tax_rate = $2 WHERE id = $1 AND version = $3`, line.id, rate, line.version)
		if err != nil {
			return nil, fmt.Errorf("write the line's tax rate: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return nil, fmt.Errorf("the line changed while its tax rate was read: %w", apperrors.ErrConflict)
		}
		moved = append(moved, retaxedLine{ID: line.id, Was: line.rate, Now: rate})
	}
	return moved, nil
}

// retaxAtSend reads the offer's category lines again on the send day, and
// carries moved totals into current so the offer.sent payload states them.
func (s *Store) retaxAtSend(ctx context.Context, tx pgx.Tx, current *crmcontracts.Offer, sendDay time.Time) (offerRetax, error) {
	entity := legalEntityIDOf(*current)
	basis := sync.OnceValues(func() (taxBasis, error) { return s.offerTaxBasis(ctx, tx, entity, sendDay) })
	retax, err := retaxOffer(ctx, tx, ids.From[ids.OfferKind](ids.UUID(current.Id)), basis)
	if err != nil {
		return offerRetax{}, err
	}
	if len(retax.Moved) > 0 {
		current.NetMinor = &retax.Totals.After.NetMinor
		current.TaxMinor = &retax.Totals.After.TaxMinor
		current.GrossMinor = &retax.Totals.After.GrossMinor
	}
	return retax, nil
}

// retaxForEntity moves a draft's category lines to the rates of the legal
// entity an edit names, as of today. An edit naming no entity moves nothing.
func (s *Store) retaxForEntity(ctx context.Context, tx pgx.Tx, offerID ids.OfferID, entity *ids.LegalEntityID) (offerRetax, error) {
	if entity == nil {
		return offerRetax{}, nil
	}
	return retaxOffer(ctx, tx, offerID, s.draftTaxBasis(ctx, tx, entity))
}

// auditHeaderEdit audits a draft's header edit. An edit that names a legal
// entity first moves the category lines to that entity's rates, as of today.
func (s *Store) auditHeaderEdit(ctx context.Context, tx pgx.Tx, id ids.OfferID, p *storekit.Patch, entity *ids.LegalEntityID) error {
	retax, err := s.retaxForEntity(ctx, tx, id, entity)
	if err != nil {
		return err
	}
	before, after := retax.images(p.Before(), p.After())
	if _, err := storekit.AuditWithEvidence(ctx, tx, "update", "offer", id.UUID, before, after, retax.evidence()); err != nil {
		return fmt.Errorf("audit offer update: %w", err)
	}
	return nil
}

// offerRetax is what reading an offer's category lines again changed.
type offerRetax struct {
	Moved  []retaxedLine
	Totals offerTotalsChange
}

// retaxOffer reads the offer's category lines again against basis and
// recomputes the totals when a rate moved.
func retaxOffer(ctx context.Context, tx pgx.Tx, offerID ids.OfferID, basis lazyTaxBasis) (offerRetax, error) {
	moved, err := retaxCategoryLines(ctx, tx, offerID, basis)
	if err != nil || len(moved) == 0 {
		return offerRetax{}, err
	}
	totals, err := recomputeOfferTotals(ctx, tx, offerID)
	if err != nil {
		return offerRetax{}, err
	}
	return offerRetax{Moved: moved, Totals: totals}, nil
}

// images adds the offer's money columns to the write's audit images when the
// retax moved them.
//
//craft:ignore naked-any an audit image carries whatever SQL types its columns hold
func (r offerRetax) images(before, after map[string]any) (map[string]any, map[string]any) {
	if len(r.Moved) == 0 {
		return before, after
	}
	maps.Copy(before, offerTotalsImage(r.Totals.Before))
	maps.Copy(after, offerTotalsImage(r.Totals.After))
	return before, after
}

// evidence names the lines the retax moved, from which rate to which. A
// line's fields are context about the offer's write, never part of its image.
//
//craft:ignore naked-any the evidence is the jsonb object the audit seam stores
func (r offerRetax) evidence() map[string]any {
	if len(r.Moved) == 0 {
		return nil
	}
	lines := make([]map[string]any, 0, len(r.Moved))
	for _, line := range r.Moved {
		lines = append(lines, map[string]any{lineIDKey: line.ID, "tax_rate_was": line.Was, "tax_rate": line.Now})
	}
	return map[string]any{"lines_retaxed": lines}
}
