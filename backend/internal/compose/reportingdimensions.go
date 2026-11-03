// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// The product, country, industry and value basis a selection names narrow one
// population for every reader that can honour them. A reader that cannot
// answers unsupported with the dimension named, so a filtered chip never sits
// above an unfiltered figure.

const reportingValueBasisAll = crmcontracts.ReportingValueBasisAll

// reportingDimensionMetrics are the metrics whose readers apply every
// dimension. Any other metric answers unsupported while one is set.
var reportingDimensionMetrics = []crmcontracts.ReportingMetricID{
	reportingBookingsWon, reportingClosedWinRate, reportingOpenPipeline, reportingStageAge, reportingQualifiedPipelineCreated, reportingBookingsMix,
	reportingQualifiedOpportunitiesCreated, reportingInitiativePipeline,
	reportingLeadsCreated, reportingLeadStatusMoves, reportingLeadsPromoted, reportingLeadFirstResponse, reportingLeadsAwaitingResponse,
}

// reportingValueBasisMetrics are the readers of won value, the only figures a
// value basis other than `all` can change. An open figure has no allocation.
var reportingValueBasisMetrics = []crmcontracts.ReportingMetricID{reportingBookingsWon, reportingBookingsMix}

func reportingDimensioned(frame crmcontracts.ReportingContext) bool {
	return frame.ProductId != nil || frame.InitiativeId != nil || frame.Country != nil || frame.Industry != nil || reportingValueBasis(frame) != reportingValueBasisAll
}

func reportingValueBasis(frame crmcontracts.ReportingContext) crmcontracts.ReportingValueBasis {
	if frame.ValueBasis == nil || *frame.ValueBasis == "" {
		return reportingValueBasisAll
	}
	return *frame.ValueBasis
}

// reportingDimensionGap is the coverage of a reader that cannot honour the
// selection's dimensions.
func reportingDimensionGap(frame crmcontracts.ReportingContext, metric crmcontracts.ReportingMetricID) (crmcontracts.ReportingCoverage, bool) {
	said := reporting.ExplanationCopy(frame)
	if !slices.Contains(reportingDimensionMetrics, metric) && reportingDimensioned(frame) {
		return reportingGap("unsupported", said.Say(reportingCoverageWords.Dimension)), true
	}
	if !slices.Contains(reportingValueBasisMetrics, metric) && reportingValueBasis(frame) != reportingValueBasisAll {
		return reportingGap("unsupported", said.Say(reportingCoverageWords.ValueBasisOpen)), true
	}
	return crmcontracts.ReportingCoverage{}, false
}

// reportingDimensionContext resolves the selection's dimensions onto the
// frame. The product needs the product read grant, as the catalog does, and
// one that does not exist is a missing row.
func reportingDimensionContext(ctx context.Context, tx pgx.Tx, selection crmcontracts.ReportingSelection, out *crmcontracts.ReportingContext) error {
	out.Country, out.Industry = selection.Country, selection.Industry
	if selection.ValueBasis != nil && *selection.ValueBasis != reportingValueBasisAll {
		out.ValueBasis = selection.ValueBasis
	}
	if err := reportingInitiativeContext(ctx, tx, selection, out); err != nil {
		return err
	}
	if selection.ProductId == nil {
		return nil
	}
	if !auth.ReadGranted(ctx, "product") {
		return apperrors.ErrPermissionDenied
	}
	var name string
	err := tx.QueryRow(ctx, `SELECT name FROM product WHERE id = $1 AND archived_at IS NULL`, *selection.ProductId).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("the selected product is not a live product: %w", apperrors.ErrNotFound)
	}
	if err != nil {
		return err
	}
	out.ProductId, out.ProductLabel = selection.ProductId, &name
	return nil
}

// dealDimensionSQL narrows deal rows aliased `alias` to the frame's product,
// country and industry. Country and industry are read through the company
// the reader may open, so a company outside their row scope records nothing.
func dealDimensionSQL(ctx context.Context, frame crmcontracts.ReportingContext, b *reportingBindings, alias string) (string, error) {
	where := ""
	if frame.ProductId != nil {
		where += " AND EXISTS (SELECT 1 FROM deal_product dp WHERE dp.deal_id=" + alias + ".id AND dp.product_id=" + b.add(*frame.ProductId) + ")"
	}
	company, err := companyDimensionSQL(ctx, frame, b, alias+".company_id", "")
	return where + company, err
}

// leadDimensionSQL narrows lead rows aliased `alias`. A lead is in a product's
// population through its own product or the product of the deal it was
// promoted into. A lead with no company keeps the industry a rep typed.
func leadDimensionSQL(ctx context.Context, frame crmcontracts.ReportingContext, b *reportingBindings, alias string) (string, error) {
	where := ""
	if frame.ProductId != nil {
		product := b.add(*frame.ProductId)
		where += " AND (" + alias + ".product_id=" + product + " OR EXISTS (SELECT 1 FROM deal_product dp WHERE dp.deal_id=" + alias + ".qualified_deal_id AND dp.product_id=" + product + "))"
	}
	company, err := companyDimensionSQL(ctx, frame, b, alias+".company_id", alias+".industry")
	return where + company, err
}

// companyDimensionSQL reads the company's address country and industry through
// storekit.LinkedCompanyColumn, as the deal and lead list filters do. So the
// filter needs the company read grant and follows the company row scope.
func companyDimensionSQL(ctx context.Context, frame crmcontracts.ReportingContext, b *reportingBindings, companyID, ownIndustry string) (string, error) {
	where := ""
	if frame.Country != nil {
		country, err := storekit.LinkedCompanyColumn(ctx, companyID, "address_country", b.arg)
		if err != nil {
			return "", err
		}
		where += " AND " + country + "=" + b.add(*frame.Country)
	}
	if frame.Industry != nil {
		linked, err := storekit.LinkedCompanyColumn(ctx, companyID, "industry", b.arg)
		if err != nil {
			return "", err
		}
		industry := b.add(*frame.Industry)
		company := linked + "=" + industry
		if ownIndustry != "" {
			company = "(" + company + " OR (" + companyID + " IS NULL AND " + ownIndustry + "=" + industry + "))"
		}
		where += " AND " + company
	}
	return where, nil
}

// allocationValueSQL is the base-currency value of a won deal's allocations
// that the frame names: one product's, one component kind's, or both. An
// allocation in another currency than the deal's has no known value.
func allocationValueSQL(frame crmcontracts.ReportingContext, b *reportingBindings, alias string) string {
	where := "dp.deal_id=" + alias + ".id AND dp.amount_minor IS NOT NULL"
	if frame.ProductId != nil {
		where += " AND dp.product_id=" + b.add(*frame.ProductId)
	}
	if basis := reportingValueBasis(frame); basis != reportingValueBasisAll {
		where += " AND dp.component_kind=" + b.add(string(basis))
	}
	return "(SELECT " + allocationConvertedSQL("SUM(dp.amount_minor)", "bool_or(dp.currency IS DISTINCT FROM "+alias+".currency)", alias, b.add(frame.Currency)) + " FROM deal_product dp WHERE " + where + ")"
}

// allocationConvertedSQL converts an allocation amount in the deal's currency
// to the base currency, bound as `base`. It uses the rate the deal froze at
// its close and the minor-unit scales of both currencies, as
// deals.BaseValueSQL does. A deal with no frozen rate has no known value.
func allocationConvertedSQL(amount, foreign, alias, base string) string {
	digits := func(currency string) string {
		return "power(10::numeric, coalesce((SELECT d.digits FROM currency_minor_digits d WHERE d.currency=" + currency + "),2))"
	}
	return "CASE WHEN " + amount + " IS NULL OR " + foreign + " THEN NULL" +
		" WHEN " + alias + ".currency=" + base + " THEN " + amount +
		" WHEN " + alias + ".fx_rate_to_base IS NULL THEN NULL" +
		" ELSE round(" + amount + "::numeric*" + alias + ".fx_rate_to_base*" + digits(base) + "/" + digits(alias+".currency") + ")::bigint END"
}
