// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The deal's field masks, applied where the row meets the wire. A rep reads
// every deal in the workspace; the amount of one that is not theirs to change
// is withheld — null, and named in masked_fields so the reader can tell it
// from an amount nobody entered.

import (
	"context"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// maskObject is the RBAC object a deal's masks are configured under — a
// different vocabulary from dealTable, which happens to spell it the same way:
// the object is what an administrator writes in field_mask, the table is what
// Postgres calls the relation, and renaming one would not rename the other.
//
// The database holds this package to it: maskable_field carries this name
// against every key below, and field_mask references that pair, so a mask
// naming something nothing here withholds is refused where it is written
// rather than going quietly inert where it is read.
const maskObject = "deal"

// dealMaskableFields are the columns a mask may name on a deal, and how each
// is withheld. Withholding is a deliberate act per field, not a reflective one
// over the struct, and the set is therefore finite — which is why it can be
// offered as a catalog: migrations/testdata/maskable_fields.txt is these keys
// under maskObject, and the database will not store a mask outside it.
//
// The keys are WIRE field names as a configured mask spells them, which is a
// different vocabulary from the column constants they happen to coincide with:
// renaming a column would not rename what an installation's stored mask says.
//
//nolint:goconst // wire field names against column names, each its own vocabulary
var dealMaskableFields = map[string]func(*crmcontracts.Deal){
	// The money group — which mask pulls in which other field — lives in
	// auth.maskGroups now; each func here withholds only its own column.
	"amount_minor":       func(d *crmcontracts.Deal) { d.AmountMinor = nil },
	"expected_arr_minor": func(d *crmcontracts.Deal) { d.ExpectedArrMinor = nil },
	"currency":           func(d *crmcontracts.Deal) { d.Currency = nil },
	// The three references. They are withheld by the same mechanism as a role
	// mask because the reader needs the same thing from them: a null they can
	// tell from an empty field. Which rows they are withheld ON is a different
	// question, answered per row by unreadableReferences.
	filterCompanyID: func(d *crmcontracts.Deal) { d.CompanyId = nil },
	filterProjectID: func(d *crmcontracts.Deal) { d.ProjectId = nil },
	// The attribution describes the partner it travels with, so a withheld
	// partner takes it along: "sourced" beside a null partner would disclose
	// that SOME partner brought the deal to a reader who may not know which.
	filterPartnerCompanyID: func(d *crmcontracts.Deal) { d.PartnerCompanyId, d.PartnerAttribution = nil, nil },
}

// finishDealPage is what every page of deals goes through before it leaves the
// store, the one-row page of a single read included: the read masks, and the
// newest email each card dates. One spelling, because the list, the
// transaction-scoped list and the single read each used to compose the passes
// by hand, and a pass added to one of them was a fact the other two stopped
// stating.
func finishDealPage(ctx context.Context, tx pgx.Tx, page []crmcontracts.Deal) error {
	if err := maskDeals(ctx, tx, page); err != nil {
		return err
	}
	return attachLastEmail(ctx, tx, page)
}

// finishDealForCaller is finishDealPage over ONE row about to leave the store
// — a single read, or the echo of a write, which is a read too.
func finishDealForCaller(ctx context.Context, tx pgx.Tx, d crmcontracts.Deal) (crmcontracts.Deal, error) {
	one := []crmcontracts.Deal{d}
	if err := finishDealPage(ctx, tx, one); err != nil {
		return crmcontracts.Deal{}, err
	}
	return one[0], nil
}

// maskDeals withholds, per row, what this reader may not have: the columns
// their ROLE masks, and the references naming a record they could not open.
// auth.ApplyFieldMasks is the shared pass every masked record type goes
// through; this spends its own round trips only on what is deal-specific —
// write authority, and which references the page's rows point at.
func maskDeals(ctx context.Context, tx pgx.Tx, deals []crmcontracts.Deal) error {
	dealID := func(d crmcontracts.Deal) ids.UUID { return ids.UUID(d.Id) }
	// ONE statement answers which rows of the page the caller could change, and
	// both consumers read it: the wire flag a client draws its edit affordances
	// from, and the masks conditioned on write authority. Asking twice would be
	// two round trips for one question, and two answers that can disagree.
	writable, err := auth.StampWritable(ctx, tx, dealTable, deals, dealID,
		func(d *crmcontracts.Deal, may bool) { d.Writable = &may })
	if err != nil {
		return err
	}
	extra, err := unreadableReferences(ctx, tx, deals)
	if err != nil {
		return err
	}
	return auth.ApplyFieldMasks(ctx, tx, maskObject, deals, dealID, dealMaskableFields,
		func(d *crmcontracts.Deal, names []string) { d.MaskedFields = &names },
		extra, writable)
}

// unreadableReferences answers, for each row of the page, which of a deal's
// links to another record this caller could not open — the extra names
// auth.ApplyFieldMasks withholds alongside the role's own masks. Every seat of
// the workspace reads every deal — a deal is customer identity — but the
// records it POINTS AT are not: a company can be capture-private to the
// colleague who captured it, and a project keeps its own own/team row scope.
// Handing the id back regardless would make the deal an existence oracle over
// rows the reader's own company and project reads would refuse.
//
// The write path has enforced exactly this rule all along: applyDealLinkPatches
// gates all three references with auth.EnsureLinkTarget before setting them.
// The system already agrees you may not NAME a company you cannot see;
// this is the half that never asked when handing one back.
//
// ONE statement per referenced table for the whole page, never a probe per row.
func unreadableReferences(ctx context.Context, tx pgx.Tx, deals []crmcontracts.Deal) (func(int) []string, error) {
	companyIDs := make([]ids.UUID, 0, 2*len(deals))
	projectIDs := make([]ids.UUID, 0, len(deals))
	for _, d := range deals {
		// partner_company_id points at the same table as company_id, so one
		// company query answers both arms.
		for _, ref := range []*openapi_types.UUID{d.CompanyId, d.PartnerCompanyId} {
			if ref != nil {
				companyIDs = append(companyIDs, ids.UUID(*ref))
			}
		}
		if d.ProjectId != nil {
			projectIDs = append(projectIDs, ids.UUID(*d.ProjectId))
		}
	}
	// VisibleSubset answers an empty list without a round trip, so a page that
	// names no company or no project pays for neither.
	visibleCompanies, err := auth.VisibleSubset(ctx, tx, "company", companyIDs)
	if err != nil {
		return nil, err
	}
	visibleProjects, err := auth.VisibleSubset(ctx, tx, "project", projectIDs)
	if err != nil {
		return nil, err
	}
	return func(i int) []string {
		d := deals[i]
		var extra []string
		if d.CompanyId != nil && !visibleCompanies[ids.UUID(*d.CompanyId)] {
			extra = append(extra, filterCompanyID)
		}
		if d.PartnerCompanyId != nil && !visibleCompanies[ids.UUID(*d.PartnerCompanyId)] {
			extra = append(extra, filterPartnerCompanyID)
		}
		if d.ProjectId != nil && !visibleProjects[ids.UUID(*d.ProjectId)] {
			extra = append(extra, filterProjectID)
		}
		return extra
	}, nil
}

// refuseMaskedSort refuses a sort over a column the caller's role masks on any
// row, through the refusal every list offering a maskable order shares. The
// deal's own contribution is which of its columns a mask can name.
func refuseMaskedSort(ctx context.Context, sort *string) error {
	return auth.RefuseMaskedSort(ctx, maskObject, sort, func(field string) bool {
		_, maskable := dealMaskableFields[field]
		return maskable
	})
}
