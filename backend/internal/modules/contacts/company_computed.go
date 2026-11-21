// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The RD-T08 formula-field display rows GetCompany surfaces
// (RD-AC-6/RD-AC-7/RD-AC-N-1): one DB-computed row (open_pipeline, fed
// by the 0065 security_invoker view), plus four honest floor rows. The
// visibility gate is a pure in-memory permission check — the STATE-4
// absent-key case — never a database round trip.

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// notYetBuiltReason floors a computed field with no backend data model
// at all. servedByHierarchyRollupReason is the honest alternative for
// weighted_pipeline: poc-v1, unlike the poc-1 reference this ports,
// already serves that figure — GET /companies/{id}/hierarchy-rollup
// (arc 1b) — so "not_yet_built" would misstate the truth; the row still
// floors computable=false because it is not a DB-GENERATED artifact
// (RD-AC-6's own bar), it just isn't UNBUILT.
const (
	notYetBuiltReason             = "not_yet_built"
	servedByHierarchyRollupReason = "served_by_hierarchy_rollup"
	// awaitingFXReason floors open_pipeline when the view row EXISTS
	// (open deals reference this company) but its aggregate is
	// itself NULL: not one of those deals could be converted, because
	// the installation holds no rate on or before today for any
	// currency they are held in. Distinct from the genuine zero of an
	// company with no open deals at all, and — since the view
	// converts rather than reading a column that is null until close —
	// now the rare state its name always implied.
	awaitingFXReason = "awaiting_fx"
	// partialPipelineReason floors open_pipeline when SOME open deals reached
	// the total and others could not be priced. The sum is real but short, and
	// a short figure shown as a total is worse than none: the reader has no way
	// to see what is missing from it, and the number sits on the page looking
	// like the whole pipeline.
	partialPipelineReason = "partial_pipeline"
	// amountWithheldReason floors open_pipeline for a reader whose deal amount
	// or currency is masked: the total would sum what the mask hides.
	amountWithheldReason   = "amount_withheld"
	openPipelineFormulaSQL = "company_open_pipeline_rollup: SUM over deal WHERE status = 'open' AND company_id = <this company> AND archived_at IS NULL, each amount converted to the installation base currency at the latest fx_rate on or before today"
)

// openPipelineDependencies names every column the view's aggregate reads:
// what is summed, what its WHERE gates participation on, what the rate
// lookup matches and orders by, and the setting the base currency comes
// from. A change to any of them changes the answer, which is what makes
// this list worth keeping accurate.
//
// deal.fx_rate_to_base is deliberately absent — it is null on every open
// deal, which is why the view stopped reading it.
var openPipelineDependencies = []string{
	"deal.amount_minor", "deal.currency", "deal.company_id",
	"deal.status", "deal.archived_at",
	"fx_rate.from_currency", "fx_rate.to_currency", "fx_rate.rate", "fx_rate.rate_date",
	"setting.installation.base_currency",
}

// openPipeline is what the view reports for one company: the converted
// total, how many open deals there are, and how many of them actually reached
// the total. The third is what tells a complete figure from a short one — SUM
// ignores a null summand without saying so.
type openPipeline struct {
	minorBase   *int64
	dealCount   int
	pricedCount int
	// noValue and noRate split the deals the total leaves out by cause, and
	// noRateCurrencies lists the currencies that have no rate.
	noValue, noRate  int
	noRateCurrencies *string
	// withheld is set for a reader whose deal amount or currency is masked.
	withheld bool
}

// withholdsDealDetail is true for a reader whose deal amount or currency is
// masked on any row. The counts and the currency list are read over every open
// deal of the company, so they would name what the mask hides. Such a reader
// gets the partial flag and nothing more.
func withholdsDealDetail(ctx context.Context) (bool, error) {
	for _, field := range []string{"amount_minor", "currency"} {
		masked, err := auth.MasksAnyRowOf(ctx, "deal", field)
		if err != nil || masked {
			return true, err
		}
	}
	return false, nil
}

// openPipelineRollup reads the company_open_pipeline_rollup view
// (0065) for one company, inside the SAME workspace transaction the
// caller (GetCompany) already opened. The view is
// security_invoker=true (0065's own comment): it runs with the CALLING
// role's privileges, so it can read no more than a direct SELECT on deal
// could from here.
//
// What bounds the SUM is the key, not the transaction. Since core 0217
// (ADR-0091) there is no tenant-isolation policy behind either the view or
// deal, so the GUC the transaction binds constrains this statement only
// through predicates a statement writes for itself — and this one writes
// none beyond company_id. That is sufficient here and it is worth
// saying why rather than leaving it to look like an omission: every deal
// the view sums reaches this company through its own foreign key, so
// the id IS the scope, and A107/ADR-0061 gives the installation one
// company for that id to belong to.
//
// No tenant term appears here because none exists to appear: the tables this
// read touches carry no tenant column, and a join on one would only re-derive
// the bound the foreign key above already carries. The view reaches setting
// and fx_rate as well as deal now, and neither is tenant-scoped either — the
// migration says what that does and does not disclose.
//
// No row (a company with no open deals at all) is the honest
// "nothing to sum" case: a zero openPipeline, never an error. The two counts
// are what separate the three states a caller must tell apart — no deals at
// all, every deal priced, and some priced while others could not be. Without
// pricedCount the third is indistinguishable from the second, and a total
// covering half the pipeline would be reported as the pipeline.
func openPipelineRollup(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, asOf time.Time) (roll openPipeline, err error) {
	err = tx.QueryRow(ctx,
		`SELECT r.open_pipeline_minor_base, r.open_deal_count, r.priced_deal_count,
		        g.no_value_deal_count, g.no_rate_deal_count, g.no_rate_currencies
		 FROM company_open_pipeline_rollup(`+occurredday.TodaySQL("$2::timestamptz")+`) r
		 JOIN company_open_pipeline_gaps(`+occurredday.TodaySQL("$2::timestamptz")+`) g USING (company_id)
		 WHERE company_id = $1`,
		companyID, asOf).Scan(&roll.minorBase, &roll.dealCount, &roll.pricedCount, &roll.noValue, &roll.noRate, &roll.noRateCurrencies)
	if err == nil {
		withheld, maskErr := withholdsDealDetail(ctx)
		if maskErr != nil {
			return openPipeline{}, maskErr
		}
		if withheld {
			roll.noValue, roll.noRate, roll.noRateCurrencies = 0, 0, nil
			roll.minorBase, roll.withheld = nil, true
		}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		// Scan never ran, so roll is still its zero value — return it, not
		// literal zeroes: the honest "nothing to sum" case above, not a
		// swallowed error.
		return roll, nil
	}
	if err != nil {
		return openPipeline{}, err
	}
	return roll, nil
}

// openPipelineCoverage says which open deals the total leaves out. Absent when
// the company has no open deal.
func openPipelineCoverage(open openPipeline) *crmcontracts.ReportRowCoverage {
	if open.dealCount == 0 {
		return nil
	}
	currencies := []string{}
	if open.noRateCurrencies != nil && *open.noRateCurrencies != "" {
		currencies = strings.Split(*open.noRateCurrencies, ",")
	}
	return &crmcontracts.ReportRowCoverage{
		Aggregates: []string{"open_pipeline"}, Partial: open.pricedCount < open.dealCount,
		NoValueDeals: open.noValue, NoRateDeals: open.noRate, MissingCurrencies: currencies,
	}
}

// companyComputedFields assembles the 5 display rows RD-T08 names.
// It takes the view's two output columns (rule T8: no dead returns —
// openDealCount now has a real consumer, the three-way branch below).
//
// open_pipeline is a genuine three-way state, not a single floor:
//   - openDealCount == 0 (no view row: a company with no open
//     deals at all) is the honest "nothing to sum" case: computable:true,
//     value_minor:0 — a real zero, not a missing one.
//   - The aggregate is non-NULL: computable:true, value_minor:sum. When some
//     open deals did not reach it the sum is partial, and coverage says how
//     many were left out and why.
//   - The aggregate is NULL although some deals were priced: the sum does not
//     fit a bigint. computable:false with the partial reason.
//   - No deal reached it (aggregate NULL, open deals exist): not one of them
//     could be priced. It floors to computable:false, reason:"awaiting_fx",
//     with no value_minor on the wire. formula_sql stays populated either way:
//     the formula exists, only a rate for these currencies does not.
func companyComputedFields(open openPipeline) []crmcontracts.ComputedField {
	weightedReason := servedByHierarchyRollupReason
	customerAgeReason := notYetBuiltReason
	nrrReason := notYetBuiltReason
	marginReason := notYetBuiltReason

	openPipelineRow := crmcontracts.ComputedField{
		Key:          "open_pipeline",
		Label:        "Open pipeline",
		Kind:         crmcontracts.ComputedFieldKindCurrencyMinor,
		FormulaSql:   openPipelineFormulaSQL,
		Dependencies: openPipelineDependencies,
	}
	switch {
	case open.dealCount == 0:
		zero := int64(0)
		openPipelineRow.Computable = true
		openPipelineRow.ValueMinor = &zero
	case open.withheld:
		reason := amountWithheldReason
		openPipelineRow.Computable = false
		openPipelineRow.Reason = &reason
	case open.minorBase != nil:
		value := *open.minorBase
		openPipelineRow.Computable = true
		openPipelineRow.ValueMinor = &value
	case open.pricedCount > 0:
		reason := partialPipelineReason
		openPipelineRow.Computable = false
		openPipelineRow.Reason = &reason
	default:
		reason := awaitingFXReason
		openPipelineRow.Computable = false
		openPipelineRow.Reason = &reason
	}

	openPipelineRow.Coverage = openPipelineCoverage(open)

	return []crmcontracts.ComputedField{
		openPipelineRow,
		{
			Key:          "weighted_pipeline",
			Label:        "Weighted pipeline",
			Kind:         crmcontracts.ComputedFieldKindCurrencyMinor,
			FormulaSql:   "",
			Dependencies: []string{},
			Computable:   false,
			Reason:       &weightedReason,
		},
		{
			Key:          "customer_age",
			Label:        "Customer age",
			Kind:         crmcontracts.ComputedFieldKindDurationMonths,
			FormulaSql:   "",
			Dependencies: []string{},
			Computable:   false,
			Reason:       &customerAgeReason,
		},
		{
			Key:          "net_revenue_retention",
			Label:        "Net revenue retention",
			Kind:         crmcontracts.ComputedFieldKindPercent,
			FormulaSql:   "",
			Dependencies: []string{},
			Computable:   false,
			Reason:       &nrrReason,
		},
		{
			Key:          "blended_gross_margin",
			Label:        "Blended gross margin",
			Kind:         crmcontracts.ComputedFieldKindPercent,
			FormulaSql:   "",
			Dependencies: []string{},
			Computable:   false,
			Reason:       &marginReason,
		},
	}
}
