// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contracts

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
)

// namedInRefusal and nameRunes bound the names the refusal carries, so the
// whole sentence fits the transport's fault-text ceiling (httperr.MaxFaultText)
// and is never cut mid-name.
const (
	namedInRefusal = 3
	nameRunes      = 32
)

// EnsureDealContractsShareCompany refuses to point a deal at a company that
// the contracts filed against it do not name. ensureLinksShareCompany refuses
// the same pairing when a contract is filed; this refuses it on a deal move.
//
// Archived contracts count. A read by id filters on the anchor's archived_at,
// not the contract's, so an archived contract still reads through the deal.
func EnsureDealContractsShareCompany(ctx context.Context, tx pgx.Tx, dealID ids.DealID, companyID ids.CompanyID) error {
	rows, err := tx.Query(ctx,
		`SELECT COALESCE(contract_number, title)
		   FROM contract
		  WHERE deal_id = $1 AND company_id <> $2
		  ORDER BY created_at, id`,
		dealID, companyID)
	if err != nil {
		return fmt.Errorf("read the deal's contracts: %w", err)
	}
	blocking, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return fmt.Errorf("read the deal's contracts: %w", err)
	}
	if len(blocking) == 0 {
		return nil
	}
	refusal := &DealContractsCrossCompanyError{Count: len(blocking)}
	// The caller may write this deal, so every contract on it is in their
	// row scope. A role without contract read still gets only the count.
	if auth.ReadGranted(ctx, contractObject) {
		for _, name := range blocking[:min(len(blocking), namedInRefusal)] {
			refusal.Named = append(refusal.Named, textcut.RunesMarked(name, nameRunes, "…"))
		}
	}
	return refusal
}

// DealContractsCrossCompanyError maps to 422 on company_id: the deal's
// contracts name another company, so moving the deal would show them to the
// new company's readers.
type DealContractsCrossCompanyError struct {
	// Count is how many contracts block the move.
	Count int
	// Named is what the first of them are called (contract number, else
	// title), empty when the caller may not read contracts.
	Named []string
}

func (e *DealContractsCrossCompanyError) Error() string {
	subject, verb, them, each := "1 contract on this deal", "belongs", "it", "it"
	if e.Count != 1 {
		subject, verb, them, each = strconv.Itoa(e.Count)+" contracts on this deal", "belong", "them", "each"
	}
	if len(e.Named) > 0 {
		names := strings.Join(e.Named, ", ")
		if more := e.Count - len(e.Named); more > 0 {
			names += ", and " + strconv.Itoa(more) + " more"
		}
		subject += " (" + names + ")"
	}
	// No edit changes a contract's company, so only a deal of that company
	// can take it.
	return subject + " " + verb + " to another company. Detach " + them + ", or file " + each +
		" against a deal of its own company, before changing the company."
}

// FieldFault names company_id, the field the caller sent, so the transport
// answers 422 with a field error the form can show.
func (e *DealContractsCrossCompanyError) FieldFault() (field, code, message string) {
	return "company_id", "deal_contracts_cross_company", e.Error()
}
