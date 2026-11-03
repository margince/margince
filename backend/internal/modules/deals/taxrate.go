// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The tax rate sheet: a standard and a reduced rate per country, each from the
// day it takes effect. A rate is added and never changed or removed, like an
// FX rate, so the sheet is the record of what was in force when. The table's
// trigger refuses an UPDATE or DELETE; no role holds either verb.

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	taxRateObject    = "tax_rate"
	taxCategoryField = "tax_category"
	validFromField   = "valid_from"
	// taxRateLimit is the highest rate the sheet takes, below numeric(5,2)'s bound.
	taxRateLimit = "100"
)

var (
	countryShape = regexp.MustCompile(`^[a-z]{2}$`)
	// taxRateShape is a percent with at most two decimals, as the column holds.
	taxRateShape = regexp.MustCompile(`^[0-9]{1,2}(\.[0-9]{1,2})?$`)
	// taxCategories are the categories a product may be in. "exempt" has no
	// rate on the sheet: an exempt line is taxed at 0.
	taxCategories = []string{"standard", "reduced", "exempt"}
)

// KnownCountry answers whether a code is an assigned country.
type KnownCountry func(code string) (bool, error)

// TaxRate is one recorded rate.
type TaxRate = crmcontracts.TaxRate

// RecordTaxRateInput is one rate to add to the sheet.
type RecordTaxRateInput struct {
	Country, Category, Rate string
	ValidFrom               time.Time
}

// ListTaxRates answers the sheet, or one country's part of it.
func (s *Store) ListTaxRates(ctx context.Context, country *string) ([]TaxRate, error) {
	if err := auth.Require(ctx, taxRateObject, principal.ActionRead); err != nil {
		return nil, err
	}
	filter := ""
	if country != nil {
		filter = strings.ToLower(*country)
		if !countryShape.MatchString(filter) {
			return nil, httperr.Validation("country", "invalid_country", "country is an ISO 3166-1 alpha-2 code, for example de")
		}
	}
	var out []TaxRate
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, country, category, rate::text, valid_from, created_at FROM tax_rate
			 WHERE $1 = '' OR country = $1
			 ORDER BY country, category, valid_from DESC`, filter)
		if err != nil {
			return fmt.Errorf("list tax rates: %w", err)
		}
		out, err = pgx.CollectRows(rows, scanTaxRate)
		return err
	})
	if out == nil {
		out = []TaxRate{}
	}
	return out, err
}

func scanTaxRate(row pgx.CollectableRow) (TaxRate, error) {
	var r TaxRate
	var id ids.UUID
	var category string
	var validFrom time.Time
	err := row.Scan(&id, &r.Country, &category, &r.Rate, &validFrom, &r.CreatedAt)
	r.Id = openapi_types.UUID(id)
	r.Category = crmcontracts.TaxRateCategory(category)
	r.ValidFrom = openapi_types.Date{Time: validFrom}
	return r, err
}

// RecordTaxRate adds one rate to the sheet, and answers how many products the
// rate classified. Only the first rate of a category of the installation's own
// country classifies any. It puts every unclassified live product whose
// default_tax_rate equals it into that category. Products have no country, so
// a rate for another country says nothing about them.
func (s *Store) RecordTaxRate(ctx context.Context, in RecordTaxRateInput) (TaxRate, int, error) {
	if err := auth.Require(ctx, taxRateObject, principal.ActionCreate); err != nil {
		return TaxRate{}, 0, err
	}
	in, err := normalizeTaxRate(in)
	if err != nil {
		return TaxRate{}, 0, err
	}
	// A rate is never removed, so a country that does not exist must never
	// reach the sheet.
	known, err := s.installation.KnownCountry(in.Country)
	if err != nil {
		return TaxRate{}, 0, err
	}
	if !known {
		return TaxRate{}, 0, httperr.Validation("country", "invalid_country", "country is not an assigned ISO 3166-1 alpha-2 code")
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return TaxRate{}, 0, err
	}
	var out TaxRate
	var classified int
	err = s.Tx(ctx, func(tx pgx.Tx) error {
		// One sheet line at a time, so two first rates of one category cannot
		// both believe they are first.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('tax_rate:' || $1 || ':' || $2, 0))`,
			in.Country, in.Category); err != nil {
			return fmt.Errorf("lock the tax rate sheet: %w", err)
		}
		earliest, err := s.refusePastTaxRate(ctx, tx, in)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			INSERT INTO tax_rate (country, category, rate, valid_from, captured_by)
			VALUES ($1, $2, $3::numeric, $4, $5)
			RETURNING id, country, category, rate::text, valid_from, created_at`,
			in.Country, in.Category, in.Rate, in.ValidFrom, by)
		if err != nil {
			return fmt.Errorf("record a tax rate: %w", err)
		}
		recorded, err := pgx.CollectExactlyOneRow(rows, scanTaxRate)
		if storekit.IsUniqueViolation(err) {
			return fmt.Errorf("a %s rate for %s already starts on %s: %w", in.Category, in.Country,
				in.ValidFrom.Format(time.DateOnly), apperrors.ErrConflict)
		}
		if err != nil {
			return fmt.Errorf("record a tax rate: %w", err)
		}
		out = recorded
		if _, err := storekit.Audit(ctx, tx, "create", taxRateObject, ids.UUID(out.Id), nil, map[string]any{
			"country": out.Country, "category": string(out.Category), "rate": out.Rate, validFromField: in.ValidFrom.Format(time.DateOnly),
		}); err != nil {
			return fmt.Errorf("audit a tax rate: %w", err)
		}
		if earliest != nil {
			return nil
		}
		classified, err = s.classifyByFirstRate(ctx, tx, in)
		return err
	})
	return out, classified, err
}

// normalizeTaxRate checks the rate's shape before a connection is taken.
func normalizeTaxRate(in RecordTaxRateInput) (RecordTaxRateInput, error) {
	in.Country = strings.ToLower(strings.TrimSpace(in.Country))
	if !countryShape.MatchString(in.Country) {
		return in, httperr.Validation("country", "invalid_country", "country is an ISO 3166-1 alpha-2 code, for example de")
	}
	if in.Category != "standard" && in.Category != "reduced" {
		return in, httperr.Validation("category", "invalid", "choose standard or reduced; an exempt product has no rate")
	}
	in.Rate = strings.TrimSpace(in.Rate)
	if !taxRateShape.MatchString(in.Rate) {
		return in, httperr.Validation("rate", "invalid", "the rate is a percent below "+taxRateLimit+" with at most two decimals, for example 19 or 7.70")
	}
	return in, nil
}

// refusePastTaxRate refuses a rate dated before today in the installation's
// zone, unless it starts before every rate of its country and category. That
// extends the history back without changing what any sent offer was taxed
// under. It answers the earliest rate's day, nil when there is none yet.
func (s *Store) refusePastTaxRate(ctx context.Context, tx pgx.Tx, in RecordTaxRateInput) (*time.Time, error) {
	var earliest *time.Time
	if err := tx.QueryRow(ctx, `SELECT min(valid_from) FROM tax_rate WHERE country = $1 AND category = $2`,
		in.Country, in.Category).Scan(&earliest); err != nil {
		return nil, fmt.Errorf("read the earliest tax rate: %w", err)
	}
	// Ungated: any seat that may record a rate may know which day it is.
	today, err := s.todayIn(ctx, tx, s.installation.TimezoneApplied)
	if err != nil {
		return nil, err
	}
	if !in.ValidFrom.Before(today) || earliest == nil || in.ValidFrom.Before(*earliest) {
		return earliest, nil
	}
	return nil, httperr.Validation(validFromField, "tax_rate_past", fmt.Sprintf(
		"valid_from cannot be in the past unless it is before the earliest %s %s rate (%s)",
		in.Country, in.Category, earliest.Format(time.DateOnly)))
}

// classifyByFirstRate puts the installation's unclassified products whose
// default rate equals a category's first rate into that category.
func (s *Store) classifyByFirstRate(ctx context.Context, tx pgx.Tx, in RecordTaxRateInput) (int, error) {
	home, err := s.installation.Country(ctx, tx)
	if err != nil {
		return 0, fmt.Errorf("read the installation's country: %w", err)
	}
	if home != in.Country {
		return 0, nil
	}
	rows, err := tx.Query(ctx, `
		UPDATE product SET tax_category = $1
		 WHERE tax_category IS NULL AND archived_at IS NULL AND default_tax_rate = $2::numeric
		RETURNING id`, in.Category, in.Rate)
	if err != nil {
		return 0, fmt.Errorf("classify products by a first tax rate: %w", err)
	}
	classified, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return 0, err
	}
	for _, id := range classified {
		if _, err := storekit.Audit(ctx, tx, "update", "product", id,
			map[string]any{taxCategoryField: nil}, map[string]any{taxCategoryField: in.Category}); err != nil {
			return 0, fmt.Errorf("audit a product's tax category: %w", err)
		}
	}
	return len(classified), nil
}

// taxCategoryFilter is the product list's `WHERE` for one category, binding
// the category through arg. "unclassified" is the products with none.
func taxCategoryFilter(category string, arg func(any) int) string {
	if category == "unclassified" {
		return "tax_category IS NULL"
	}
	return storekit.SQLf(taxCategoryField+" = $%d", arg(category))
}

// checkTaxCategory refuses a category the product column does not admit.
func checkTaxCategory(category string) error {
	if slices.Contains(taxCategories, category) {
		return nil
	}
	return httperr.Validation(taxCategoryField, "invalid", "choose standard, reduced or exempt")
}

// ListTaxRates implements GET /tax-rates.
func (h Handlers) ListTaxRates(w http.ResponseWriter, r *http.Request, params crmcontracts.ListTaxRatesParams) {
	rates, err := h.store.ListTaxRates(r.Context(), params.Country)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.TaxRateListResponse{Data: rates})
}

// RecordTaxRate implements POST /tax-rates.
func (h Handlers) RecordTaxRate(w http.ResponseWriter, r *http.Request) {
	var req crmcontracts.RecordTaxRateRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	rate, classified, err := h.store.RecordTaxRate(r.Context(), RecordTaxRateInput{
		Country: req.Country, Category: string(req.Category), Rate: req.Rate, ValidFrom: req.ValidFrom.Time,
	})
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, crmcontracts.TaxRateRecorded{Rate: rate, ClassifiedProducts: classified})
}
