// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The deal's field masks, applied where the row meets the wire. A rep reads
// every deal in the workspace; the amount of one that is not theirs to change
// is withheld — null, and named in masked_fields so the reader can tell it
// from an amount nobody entered.

import (
	"context"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/fieldmask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
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
const maskObject = fieldmask.Deal

// dealWithholders is how each field a mask may name is withheld on the wire.
// WHICH fields those are, and what each drags with it, is the fieldmask
// package's — every surface that meets a masked deal reads that one table, and
// this is the half only the deal can do: nulling the struct.
//
// The keys are checked against the catalog by
// TestEveryMaskableDealFieldHasSomethingThatWithholdsIt, so a field added
// there and forgotten here fails rather than going quietly unenforced.
//
//nolint:goconst // wire field names against column names, each its own vocabulary
var dealWithholders = map[string]func(*crmcontracts.Deal){
	"amount_minor":          func(d *crmcontracts.Deal) { d.AmountMinor = nil },
	"expected_arr_minor":    func(d *crmcontracts.Deal) { d.ExpectedArrMinor = nil },
	"currency":              func(d *crmcontracts.Deal) { d.Currency = nil },
	filterCompanyID:         func(d *crmcontracts.Deal) { d.CompanyId = nil },
	filterProjectID:         func(d *crmcontracts.Deal) { d.ProjectId = nil },
	filterPartnerCompanyID:  func(d *crmcontracts.Deal) { d.PartnerCompanyId = nil },
	partnerAttributionField: func(d *crmcontracts.Deal) { d.PartnerAttribution = nil },
}

// withheldFields is the ordered set of columns withheld from ONE row. Ordered
// because masked_fields goes on the wire, and a client diffing two reads of
// the same deal should not see the list reshuffle under it.
type withheldFields []string

func (w *withheldFields) add(field string) {
	if !slices.Contains(*w, field) {
		*w = append(*w, field)
	}
}

// applyTo withholds every named field from the row and records the names on
// it.
//
// The names are expanded through the catalog first, so both sources of a
// withholding land on the same answer: a role mask on the amount and a partner
// this reader cannot open each drag a field with them, and the drag belongs to
// the field rather than to whichever pass noticed it. A name with no withhold
// func is dropped rather than reported — naming a field in masked_fields while
// still sending its value is a worse answer than either half alone.
func (w withheldFields) applyTo(d *crmcontracts.Deal) {
	withheld := fieldmask.Withheld(maskObject, w)
	named := make([]string, 0, len(withheld))
	for _, field := range withheld {
		withhold, known := dealWithholders[field]
		if !known {
			continue
		}
		withhold(d)
		named = append(named, field)
	}
	if len(named) > 0 {
		d.MaskedFields = &named
	}
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
// Both end the same way — the field goes out null and masked_fields names it —
// so both are collected per row and applied once.
func maskDeals(ctx context.Context, tx pgx.Tx, deals []crmcontracts.Deal) error {
	withheld := make([]withheldFields, len(deals))
	// ONE statement answers which rows of the page the caller could change, and
	// both consumers read it: the wire flag a client draws its edit affordances
	// from, and the masks conditioned on write authority. Asking twice would be
	// two round trips for one question, and two answers that can disagree.
	writable, err := auth.StampWritable(ctx, tx, dealTable, deals,
		func(d crmcontracts.Deal) ids.UUID { return ids.UUID(d.Id) },
		func(d *crmcontracts.Deal, may bool) { d.Writable = &may })
	if err != nil {
		return err
	}
	if err := roleMaskedFields(ctx, deals, withheld, writable); err != nil {
		return err
	}
	if err := unreadableReferences(ctx, tx, deals, withheld); err != nil {
		return err
	}
	for i := range deals {
		withheld[i].applyTo(&deals[i])
	}
	return nil
}

// roleMaskedFields collects the columns the caller's role withholds on each
// row. One statement answers which rows of the page the caller could change;
// the masks conditioned on write authority lift on those.
func roleMaskedFields(ctx context.Context, deals []crmcontracts.Deal, withheld []withheldFields,
	writable map[ids.UUID]bool,
) error {
	p, err := storekit.Actor(ctx)
	if err != nil {
		return err
	}
	// Cheap exit for the common case — no mask on deals at all.
	if len(auth.MaskedFields(p, maskObject, false)) == 0 {
		return nil
	}
	for i := range deals {
		for _, field := range auth.MaskedFields(p, maskObject, writable[ids.UUID(deals[i].Id)]) {
			withheld[i].add(field)
		}
	}
	return nil
}

// unreadableReferences withholds a deal's links to records the caller could
// not open. Every seat of the workspace reads every deal — a deal is customer
// identity — but the records it POINTS AT are not: a company can be
// capture-private to the colleague who captured it, and a project keeps its own
// own/team row scope. Handing the id back regardless would make the deal an
// existence oracle over rows the reader's own company and project reads
// would refuse.
//
// The write path has enforced exactly this rule all along: applyDealLinkPatches
// gates all three references with auth.EnsureLinkTarget before setting them.
// The system already agrees you may not NAME a company you cannot see;
// this is the half that never asked when handing one back.
//
// ONE statement per referenced table for the whole page, never a probe per row.
func unreadableReferences(ctx context.Context, tx pgx.Tx, deals []crmcontracts.Deal, withheld []withheldFields) error {
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
		return err
	}
	visibleProjects, err := auth.VisibleSubset(ctx, tx, "project", projectIDs)
	if err != nil {
		return err
	}
	for i := range deals {
		d := deals[i]
		if d.CompanyId != nil && !visibleCompanies[ids.UUID(*d.CompanyId)] {
			withheld[i].add(filterCompanyID)
		}
		if d.PartnerCompanyId != nil && !visibleCompanies[ids.UUID(*d.PartnerCompanyId)] {
			withheld[i].add(filterPartnerCompanyID)
		}
		if d.ProjectId != nil && !visibleProjects[ids.UUID(*d.ProjectId)] {
			withheld[i].add(filterProjectID)
		}
	}
	return nil
}

// refuseMaskedSort refuses a sort over a column the caller's role masks on
// any row: ordering by a value is reading it, and a page ordered by amounts
// the caller may not see would disclose them through the order.
func refuseMaskedSort(ctx context.Context, sort *string) error {
	if sort == nil || *sort == "" {
		return nil
	}
	field := strings.TrimPrefix(strings.TrimSpace(*sort), "-")
	if _, withholdable := dealWithholders[field]; !withholdable {
		return nil
	}
	masked, err := auth.MasksAnyRowOf(ctx, "deal", field)
	if err != nil {
		return err
	}
	if masked {
		return &values.ParseError{
			Field: "sort", Code: "field_masked",
			Message: "sort by " + field + " is not available: your role does not read it on every deal",
		}
	}
	return nil
}
