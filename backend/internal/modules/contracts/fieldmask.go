// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contracts

// The contract read mask, the sibling of the deal's and the project's and for
// the same reason: a projection carrying a reference to ANOTHER record must
// withhold the one its reader could not open, or the record becomes an
// existence oracle over rows that reader's own reads would refuse.
//
// A contract is admitted by its deal OR its company (visibility.go), and
// that disjunction is about ADMISSION — it says nothing about disclosure. A
// contract admitted through its deal still hands back a company_id whose
// company may be capture-private to the colleague who captured it, and a
// project_id whose delivery keeps its own own/team row scope.
//
// The deal reference is masked on the same footing. It is the anchor the
// admission arm tested, so a contract admitted through its COMPANY was
// never asked anything about the deal it names.
//
// Why this closes a hole the deal mask left open: `deal_project_same_company`
// forces a deal's project and its company to name one company, so a caller
// who reads the project and not the company recovered, in one hop through the
// contract, the id the deal read had just withheld.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The masked_fields names for the three references. They are the wire members'
// own names, which is what a client reads the mask against.
const (
	filterCompanyID = "company_id"
	filterDealID         = "deal_id"
	filterProjectID      = "project_id"
)

// maskedReference is one reference of one row: where to read it, what to call
// it when it is withheld, and how to withhold it.
//
// Held as data rather than as three copies of the same if-statement, because
// the three differ only in which table answers for them — and a fourth
// reference added to the projection is then one line here instead of a fourth
// place to forget.
type maskedReference struct {
	table string
	field string
	ref   func(*crmcontracts.Contract) **openapi_types.UUID
}

var maskedReferences = []maskedReference{
	{
		table: companyTable, field: filterCompanyID,
		ref: func(c *crmcontracts.Contract) **openapi_types.UUID { return &c.CompanyId },
	},
	{
		table: dealTable, field: filterDealID,
		ref: func(c *crmcontracts.Contract) **openapi_types.UUID { return &c.DealId },
	},
	{
		table: projectTable, field: filterProjectID,
		ref: func(c *crmcontracts.Contract) **openapi_types.UUID { return &c.ProjectId },
	},
}

// readContractForCaller is readContract with the reader's own sight applied to
// what the row POINTS AT.
//
// The two are separate deliberately. `readContract` answers what the row is,
// and the write paths take their pre-image, their anchor and their audit images
// from it — a masked pre-image would authorize a patch against a withheld
// company. This is the one that leaves the store, and every outward answer
// comes through it.
func readContractForCaller(
	ctx context.Context, tx pgx.Tx, id ids.ContractID, asOf time.Time,
) (crmcontracts.Contract, error) {
	out, err := readContract(ctx, tx, id, asOf)
	if err != nil {
		return crmcontracts.Contract{}, err
	}
	return maskContractForCaller(ctx, tx, out)
}

// maskContractForCaller applies the read mask to ONE row about to leave the
// store.
func maskContractForCaller(
	ctx context.Context, tx pgx.Tx, c crmcontracts.Contract,
) (crmcontracts.Contract, error) {
	one := []crmcontracts.Contract{c}
	if err := maskContracts(ctx, tx, one); err != nil {
		return crmcontracts.Contract{}, err
	}
	return one[0], nil
}

// maskContracts withholds, per row, the references naming a record this reader
// could not open. ONE statement per referenced table for the whole page, never
// a probe per row.
func maskContracts(ctx context.Context, tx pgx.Tx, page []crmcontracts.Contract) error {
	withheld := make([][]string, len(page))
	for _, reference := range maskedReferences {
		named := make([]ids.UUID, 0, len(page))
		for i := range page {
			if id := *reference.ref(&page[i]); id != nil {
				named = append(named, ids.UUID(*id))
			}
		}
		// VisibleSubset answers an empty list without a round trip, so a page
		// naming none of a table pays nothing for it. It checks the object
		// grant as well as the row scope: a seat holding no `project.read`
		// learns no project id from a contract either.
		visible, err := auth.VisibleSubset(ctx, tx, reference.table, named)
		if err != nil {
			return err
		}
		for i := range page {
			held := reference.ref(&page[i])
			if *held == nil || visible[ids.UUID(**held)] {
				continue
			}
			*held = nil
			withheld[i] = append(withheld[i], reference.field)
		}
	}
	for i := range page {
		if len(withheld[i]) > 0 {
			page[i].MaskedFields = &withheld[i]
		}
	}
	return nil
}
