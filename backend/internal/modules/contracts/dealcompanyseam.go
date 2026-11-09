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
// the contracts filed against it do not name. It is ensureLinksShareCompany
// seen from the deal's side: that check refuses to file company A's contract
// against company B's deal, and this one refuses to move the deal there later.
//
// Archived contracts count, because a contract read by id is filtered by its
// anchor's archived_at, not its own, so an archived one still reads through the
// deal.
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
	// The caller may write this deal and so see every contract on it, but a
	// role without contract read is told how many, not what they are called.
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
	subject, verb, them := "1 contract on this deal", "belongs", "it"
	if e.Count != 1 {
		subject, verb, them = strconv.Itoa(e.Count)+" contracts on this deal", "belong", "them"
	}
	if len(e.Named) > 0 {
		names := strings.Join(e.Named, ", ")
		if more := e.Count - len(e.Named); more > 0 {
			names += ", and " + strconv.Itoa(more) + " more"
		}
		subject += " (" + names + ")"
	}
	return subject + " " + verb + " to another company. Detach " + them + ", or move " + them +
		" to a deal of the new company, before changing the company."
}

// FieldFault names company_id, the field the caller sent, so the transport
// answers 422 with a field error the form can show.
func (e *DealContractsCrossCompanyError) FieldFault() (field, code, message string) {
	return "company_id", "deal_contracts_cross_company", e.Error()
}
