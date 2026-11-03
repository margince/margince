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
	reportingLeadsCreated, reportingLeadStatusMoves, reportingLeadsPromoted, reportingLeadFirstResponse, reportingLeadsAwaitingResponse,
}

// reportingValueBasisMetrics are the readers of won value, the only figures a
// value basis other than `all` can change. An open figure has no allocation.
var reportingValueBasisMetrics = []crmcontracts.ReportingMetricID{reportingBookingsWon, reportingBookingsMix}

func reportingDimensioned(frame crmcontracts.ReportingContext) bool {
	return frame.ProductId != nil || frame.Country != nil || frame.Industry != nil || reportingValueBasis(frame) != reportingValueBasisAll
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
// frame. A product the reader may not open, or that does not exist, is a
// missing row rather than a hint that it is there.
func reportingDimensionContext(ctx context.Context, tx pgx.Tx, selection crmcontracts.ReportingSelection, out *crmcontracts.ReportingContext) error {
	out.Country, out.Industry = selection.Country, selection.Industry
	if selection.ValueBasis != nil && *selection.ValueBasis != reportingValueBasisAll {
		out.ValueBasis = selection.ValueBasis
	}
	if selection.ProductId == nil {
		return nil
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
// country and industry. Country and industry are the customer company's, as
// the list vocabulary reads them.
func dealDimensionSQL(frame crmcontracts.ReportingContext, b *reportingBindings, alias string) string {
	where := ""
	if frame.ProductId != nil {
		where += " AND EXISTS (SELECT 1 FROM deal_product dp WHERE dp.deal_id=" + alias + ".id AND dp.product_id=" + b.add(*frame.ProductId) + ")"
	}
	return where + companyDimensionSQL(frame, b, alias+".company_id", "")
}

// leadDimensionSQL narrows lead rows aliased `alias`. A lead is in a product's
// population through its own product or the product of the deal it was
// promoted into. A lead with no company keeps the industry a rep typed.
func leadDimensionSQL(frame crmcontracts.ReportingContext, b *reportingBindings, alias string) string {
	where := ""
	if frame.ProductId != nil {
		product := b.add(*frame.ProductId)
		where += " AND (" + alias + ".product_id=" + product + " OR EXISTS (SELECT 1 FROM deal_product dp WHERE dp.deal_id=" + alias + ".qualified_deal_id AND dp.product_id=" + product + "))"
	}
	return where + companyDimensionSQL(frame, b, alias+".company_id", alias+".industry")
}

func companyDimensionSQL(frame crmcontracts.ReportingContext, b *reportingBindings, companyID, ownIndustry string) string {
	where := ""
	if frame.Country != nil {
		where += " AND EXISTS (SELECT 1 FROM company_address_country cac WHERE cac.company_id=" + companyID + " AND " + b.add(*frame.Country) + "=ANY(cac.countries))"
	}
	if frame.Industry != nil {
		industry := b.add(*frame.Industry)
		company := "EXISTS (SELECT 1 FROM company o WHERE o.id=" + companyID + " AND o.industry=" + industry + ")"
		if ownIndustry != "" {
			company = "(" + company + " OR (" + companyID + " IS NULL AND " + ownIndustry + "=" + industry + "))"
		}
		where += " AND " + company
	}
	return where
}

// allocationValueSQL is the base-currency value of a won deal's allocations
// that the frame names: one product's, one component kind's, or both. The
// allocation is converted with the ratio the deal's own frozen base amount
// gives, so it never needs a second rate read. An allocation in another
// currency than the deal's has no known value.
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
// to the base currency, bound as `base`, by the deal's own frozen ratio.
func allocationConvertedSQL(amount, foreign, alias, base string) string {
	return "CASE WHEN " + amount + " IS NULL OR " + foreign + " THEN NULL" +
		" WHEN " + alias + ".currency=" + base + " THEN " + amount +
		" WHEN " + alias + ".amount_minor IS NOT NULL AND " + alias + ".amount_minor<>0 AND " + alias + ".amount_minor_base IS NOT NULL" +
		" THEN round(" + amount + "::numeric*" + alias + ".amount_minor_base/" + alias + ".amount_minor)::bigint ELSE NULL END"
}
