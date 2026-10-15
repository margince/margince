// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contracts

// What the deals module has to ask this one before it moves a deal to another
// company. The port is declared there and filled by the composition root; this
// is the answer, because `contract` is this module's table.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// namedInRefusal bounds how many agreements the refusal spells out. A message
// is an instruction, and a list of fifty is neither readable nor survivable at
// the transport's fault-text ceiling, which would cut it mid-name.
const namedInRefusal = 5

// EnsureDealContractsShareCompany refuses to re-point a deal at a company the
// agreements already filed against it do not name.
//
// This is ensureLinksShareCompany's rule reached from the other end. That one
// refuses to file company A's agreement against company B's deal. Moving the
// deal afterwards would reach the same pairing.
//
// The result would be a cross-tenant read. VisibleClause judges a deal-anchored
// contract by its deal alone, so agreements on deals a caller cannot see stay
// hidden. Once the deal names B, everyone who can see it reads A's agreement.
//
// Archived agreements count. A contract read by id is not filtered by its own
// archived_at, only by its anchor's. An archived one can still be detached, so
// naming it leaves the caller something to do.
//
// The blocking agreements are named, because a refusal a rep cannot act on is
// the objection to refusing at all. The caller can see this deal, so every
// agreement anchored to it is already theirs to read.
func EnsureDealContractsShareCompany(ctx context.Context, tx pgx.Tx, dealID ids.DealID, companyID ids.CompanyID) error {
	rows, err := tx.Query(ctx,
		`SELECT COALESCE(contract_number, title)
		   FROM contract
		  WHERE deal_id = $1 AND company_id <> $2
		  ORDER BY created_at, id
		  LIMIT $3`,
		dealID, companyID, namedInRefusal+1)
	if err != nil {
		return fmt.Errorf("read the deal's agreements: %w", err)
	}
	blocking, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return fmt.Errorf("read the deal's agreements: %w", err)
	}
	if len(blocking) == 0 {
		return nil
	}
	if len(blocking) > namedInRefusal {
		return &DealContractsCrossCompanyError{Named: blocking[:namedInRefusal], AndOthers: true}
	}
	return &DealContractsCrossCompanyError{Named: blocking}
}

// DealContractsCrossCompanyError maps to 422: the deal names a company its own
// agreements do not, so moving it would publish them to that company's readers.
//
// It names `company_id`, the deal's wire field, because that is the field the
// caller sent. It lives here, as NotAPartnerError lives with the partner, since
// only this module may read `contract` rows.
type DealContractsCrossCompanyError struct {
	// Named is what the blocking agreements are called: their contract
	// number, or their title where they carry no number.
	Named []string
	// AndOthers says the list was cut at namedInRefusal, so a caller who
	// detaches everything named still knows more is waiting.
	AndOthers bool
}

func (e *DealContractsCrossCompanyError) Error() string {
	named := strings.Join(e.Named, ", ")
	if e.AndOthers {
		named += ", and others"
	}
	// "do not" rather than "still name the old one". A deal may have named
	// nobody when the agreement was filed.
	return "a deal and its agreements must name the same company, and these do not: " + named +
		"; detach them from this deal before moving it"
}

// FieldFault names the field the caller sent, so the refusal classifies as a
// 422 against company_id rather than an opaque failure.
func (e *DealContractsCrossCompanyError) FieldFault() (field, code, message string) {
	return "company_id", "deal_contracts_cross_company", e.Error()
}
