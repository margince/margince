// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The RD-T08 formula-field display rows GetOrganization surfaces
// (RD-AC-6/RD-AC-7/RD-AC-N-1): one DB-computed row (open_pipeline, fed
// by the 0065 security_invoker view), plus four honest floor rows. The
// visibility gate is a pure in-memory permission check — the STATE-4
// absent-key case — never a database round trip.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// notYetBuiltReason floors a computed field with no backend data model
// at all. servedByHierarchyRollupReason is the honest alternative for
// weighted_pipeline: poc-v1, unlike the poc-1 reference this ports,
// already serves that figure — GET /organizations/{id}/hierarchy-rollup
// (arc 1b) — so "not_yet_built" would misstate the truth; the row still
// floors computable=false because it is not a DB-GENERATED artifact
// (RD-AC-6's own bar), it just isn't UNBUILT.
const (
	notYetBuiltReason             = "not_yet_built"
	servedByHierarchyRollupReason = "served_by_hierarchy_rollup"
	// awaitingFXReason floors open_pipeline when the view row EXISTS
	// (open deals reference this organization) but its aggregate is
	// itself NULL: not one of those deals could be converted, because
	// the installation holds no rate on or before today for any
	// currency they are held in. Distinct from the genuine zero of an
	// organization with no open deals at all, and — since the view
	// converts rather than reading a column that is null until close —
	// now the rare state its name always implied.
	awaitingFXReason = "awaiting_fx"
	// partialPipelineReason floors open_pipeline when SOME open deals reached
	// the total and others could not be priced. The sum is real but short, and
	// a short figure shown as a total is worse than none: the reader has no way
	// to see what is missing from it, and the number sits on the page looking
	// like the whole pipeline.
	partialPipelineReason  = "partial_pipeline"
	openPipelineFormulaSQL = "organization_open_pipeline_rollup: SUM over deal WHERE status = 'open' AND organization_id = <this org> AND archived_at IS NULL, each amount converted to the installation base currency at the latest fx_rate on or before today"
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
	"deal.amount_minor", "deal.currency", "deal.organization_id",
	"deal.status", "deal.archived_at",
	"fx_rate.from_currency", "fx_rate.to_currency", "fx_rate.rate", "fx_rate.rate_date",
	"setting.installation.base_currency",
}

// openPipeline is what the view reports for one organization: the converted
// total, how many open deals there are, and how many of them actually reached
// the total. The third is what tells a complete figure from a short one — SUM
// ignores a null summand without saying so.
type openPipeline struct {
	minorBase   *int64
	dealCount   int
	pricedCount int
}

// computedFieldsVisible answers the STATE-4 gate: does the acting
// principal's merged role policy grant computed_field:read? poc-1
// re-loaded role permissions from the database on every call
// (RollupStore.ComputedFieldsVisible); poc-v1's principal already
// carries its merged Permissions, resolved once at authentication
// (B-EP03.1), so this is a pure in-memory check — no query. The system
// principal (workspace provisioning, no role of its own) is trusted by
// construction, mirroring auth.Require's own carve-out; a request with
// no actor bound at all fails closed.
func computedFieldsVisible(ctx context.Context) bool {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return false
	}
	if actor.Type == principal.PrincipalSystem {
		return true
	}
	return actor.Permissions.Allows("computed_field", principal.ActionRead)
}

// openPipelineRollup reads the organization_open_pipeline_rollup view
// (0065) for one organization, inside the SAME workspace transaction the
// caller (GetOrganization) already opened. The view is
// security_invoker=true (0065's own comment): it runs with the CALLING
// role's privileges, so it can read no more than a direct SELECT on deal
// could from here.
//
// What bounds the SUM is the key, not the transaction. Since core 0217
// (ADR-0091) there is no tenant-isolation policy behind either the view or
// deal, so the GUC the transaction binds constrains this statement only
// through predicates a statement writes for itself — and this one writes
// none beyond organization_id. That is sufficient here and it is worth
// saying why rather than leaving it to look like an omission: every deal
// the view sums reaches this organization through its own foreign key, so
// the id IS the scope, and A107/ADR-0061 gives the installation one
// organization for that id to belong to.
//
// No tenant term appears here because none exists to appear: the tables this
// read touches carry no tenant column, and a join on one would only re-derive
// the bound the foreign key above already carries. The view reaches setting
// and fx_rate as well as deal now, and neither is tenant-scoped either — the
// migration says what that does and does not disclose.
//
// No row (an organization with no open deals at all) is the honest
// "nothing to sum" case: a zero openPipeline, never an error. The two counts
// are what separate the three states a caller must tell apart — no deals at
// all, every deal priced, and some priced while others could not be. Without
// pricedCount the third is indistinguishable from the second, and a total
// covering half the pipeline would be reported as the pipeline.
func openPipelineRollup(ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID) (roll openPipeline, err error) {
	err = tx.QueryRow(ctx,
		`SELECT open_pipeline_minor_base, open_deal_count, priced_deal_count
		 FROM organization_open_pipeline_rollup WHERE organization_id = $1`,
		orgID).Scan(&roll.minorBase, &roll.dealCount, &roll.pricedCount)
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

// organizationComputedFields assembles the 5 display rows RD-T08 names.
// It takes the view's two output columns (rule T8: no dead returns —
// openDealCount now has a real consumer, the three-way branch below).
//
// open_pipeline is a genuine three-way state, not a single floor:
//   - openDealCount == 0 (no view row: an organization with no open
//     deals at all) is the honest "nothing to sum" case: computable:true,
//     value_minor:0 — a real zero, not a missing one.
//   - Every open deal reached the total (priced == open) and the aggregate is
//     non-NULL: computable:true, value_minor:sum.
//   - Some deals reached it and others did not: the sum is SHORT, and a short
//     figure presented as a total is worse than no figure — a reader cannot
//     see what is missing from it. It floors to computable:false with the
//     partial reason.
//   - No deal reached it (aggregate NULL, open deals exist): not one of them
//     could be priced. It floors to computable:false, reason:"awaiting_fx",
//     with no value_minor on the wire. formula_sql stays populated either way:
//     the formula exists, only a rate for these currencies does not.
func organizationComputedFields(open openPipeline) []crmcontracts.ComputedField {
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
	case open.minorBase != nil && open.pricedCount == open.dealCount:
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
