// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contracts

// The account read: every agreement on one company.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// ListContractsInput selects one account's agreements.
type ListContractsInput struct {
	CompanyID ids.CompanyID
	// Status filters to one asserted status. Nil means every status, which
	// includes the superseded predecessors that make a renewal chain readable.
	Status *string
	// UnderContractOnly filters to the DERIVED reading rather than the status
	// column (CONTRACT-FORM-1). The two are different questions and a caller
	// asking for one must not silently get the other.
	UnderContractOnly bool
	Cursor            *string
	Limit             *int
}

// ListCompanyContracts reads one account's agreements, newest term first.
func (s *Store) ListCompanyContracts(ctx context.Context, in ListContractsInput) (crmcontracts.ContractListResponse, error) {
	if err := auth.Require(ctx, contractObject, principal.ActionRead); err != nil {
		return crmcontracts.ContractListResponse{}, err
	}
	active, err := s.catalogColumns(ctx)
	if err != nil {
		return crmcontracts.ContractListResponse{}, err
	}

	var out crmcontracts.ContractListResponse
	err = s.tx(ctx, func(tx pgx.Tx) error {
		// Naming the account is a read of it: a caller who cannot see the
		// company does not learn how many agreements it holds.
		if err := auth.EnsureLinkTarget(ctx, tx, companyTable, in.CompanyID.UUID); err != nil {
			return err
		}
		var err error
		out, err = listContractsTx(ctx, tx, in, s.today(), active)
		return err
	})
	return out, err
}

func listContractsTx(ctx context.Context, tx pgx.Tx, in ListContractsInput, asOf time.Time, active []fieldcatalog.Column) (crmcontracts.ContractListResponse, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	companyPos := arg(in.CompanyID)
	asOfPos := arg(asOf)

	where := []string{storekit.SQLf("company_id = $%d", companyPos), "archived_at IS NULL"}

	scope, err := VisibleClause(ctx, "", arg)
	if err != nil {
		return crmcontracts.ContractListResponse{}, err
	}
	if scope != "" {
		where = append(where, scope)
	}
	if in.Status != nil {
		where = append(where, storekit.SQLf("status = $%d", arg(*in.Status)))
	}
	if in.UnderContractOnly {
		where = append(where, underContractSQL(asOfPos))
	}
	if in.Cursor != nil && *in.Cursor != "" {
		cursor, err := storekit.DecodeCursor(*in.Cursor)
		if err != nil {
			return crmcontracts.ContractListResponse{}, err
		}
		where = append(where, continueAfterTerm(cursor, arg))
	}

	limit := storekit.ClampLimit(in.Limit)
	rows, err := tx.Query(ctx, storekit.SQLf(
		`SELECT %s%s, %s FROM contract WHERE %s ORDER BY `+termOrder+` LIMIT $%d`,
		contractColumns, storekit.SelectSuffix(active), underContractSQL(asOfPos),
		strings.Join(where, " AND "), arg(limit+1)), args...)
	if err != nil {
		return crmcontracts.ContractListResponse{}, fmt.Errorf("list contracts: %w", err)
	}
	defer rows.Close()

	contracts := make([]crmcontracts.Contract, 0, limit)
	for rows.Next() {
		c, err := scanContract(rows, active)
		if err != nil {
			return crmcontracts.ContractListResponse{}, fmt.Errorf("scan contract: %w", err)
		}
		contracts = append(contracts, c)
	}
	if err := rows.Err(); err != nil {
		return crmcontracts.ContractListResponse{}, fmt.Errorf("read contract page: %w", err)
	}
	if err := maskContracts(ctx, tx, contracts); err != nil {
		return crmcontracts.ContractListResponse{}, err
	}

	page := crmcontracts.PageInfo{}
	// One row beyond the page proves another page exists without a second
	// count query, which would answer a different question under concurrency.
	if len(contracts) > limit {
		contracts = contracts[:limit]
		last := contracts[len(contracts)-1]
		next, err := termCursor(last)
		if err != nil {
			return crmcontracts.ContractListResponse{}, err
		}
		page.NextCursor = &next
		page.HasMore = true
	}
	return crmcontracts.ContractListResponse{Data: contracts, Page: page}, nil
}

// cursorTime reads a row's creation instant for the keyset token. The
// generated type carries it as optional; a persisted row always has one.
func cursorTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

// ListProjectContractsTx reads the agreements attached to one project, newest
// first, inside a caller-opened transaction — the project page's contracts
// section. The project itself is the caller's to see before its paper is:
// naming it is a read of it, the same rule the account list keeps for the
// company.
func (s *Store) ListProjectContractsTx(ctx context.Context, tx pgx.Tx, projectID ids.ProjectID, limit *int) (crmcontracts.ContractListResponse, error) {
	if err := auth.Require(ctx, contractObject, principal.ActionRead); err != nil {
		return crmcontracts.ContractListResponse{}, err
	}
	if err := auth.EnsureLinkTarget(ctx, tx, projectTable, projectID.UUID); err != nil {
		return crmcontracts.ContractListResponse{}, err
	}
	// NO catalog fetch here, deliberately. This runs INSIDE a transaction the
	// caller opened — the 360 assembly holds one across every section — and the
	// catalog reader opens a second transaction of its own to answer. On a pool
	// with one connection, or a busy one, that waits for a connection this
	// caller is itself holding, which is a deadlock rather than a slow page.
	//
	// The cost is that the project page's contract rows carry no custom values.
	// That is the right trade for a compact section listing title, value and
	// dates: the full record is one click away and reads them through the
	// handler path, which fetches the catalog before it opens anything.
	var active []fieldcatalog.Column
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	asOfPos := arg(s.today())
	where := []string{storekit.SQLf("project_id = $%d", arg(projectID)), "archived_at IS NULL"}
	scope, err := VisibleClause(ctx, "", arg)
	if err != nil {
		return crmcontracts.ContractListResponse{}, err
	}
	if scope != "" {
		where = append(where, scope)
	}
	lim := storekit.ClampLimit(limit)
	rows, err := tx.Query(ctx, storekit.SQLf(
		`SELECT %s%s, %s FROM contract WHERE %s ORDER BY `+termOrder+` LIMIT $%d`,
		contractColumns, storekit.SelectSuffix(active), underContractSQL(asOfPos),
		strings.Join(where, " AND "), arg(lim+1)), args...)
	if err != nil {
		return crmcontracts.ContractListResponse{}, fmt.Errorf("list project contracts: %w", err)
	}
	defer rows.Close()
	contracts := make([]crmcontracts.Contract, 0, lim)
	for rows.Next() {
		c, err := scanContract(rows, active)
		if err != nil {
			return crmcontracts.ContractListResponse{}, fmt.Errorf("scan contract: %w", err)
		}
		contracts = append(contracts, c)
	}
	if err := rows.Err(); err != nil {
		return crmcontracts.ContractListResponse{}, fmt.Errorf("read contract page: %w", err)
	}
	if err := maskContracts(ctx, tx, contracts); err != nil {
		return crmcontracts.ContractListResponse{}, err
	}
	page := crmcontracts.PageInfo{}
	if len(contracts) > lim {
		contracts = contracts[:lim]
		page.HasMore = true
	}
	return crmcontracts.ContractListResponse{Data: contracts, Page: page}, nil
}

// termOrder is how an account's agreements are ordered, in both list queries.
//
// BY TERM, not by when the row was written. A reader looking at a contract list
// is asking which agreement is CURRENT, and ordering by creation answered a
// different question: an older agreement imported today sorted ahead of a newer
// one created last week, so the page presented the wrong agreement as current.
//
// NULLS LAST because an imported agreement often carries no start date, and a
// term we know no start for is not the newest one — it is the one we know least
// about. Floating it to the top would misinform exactly the reader this
// ordering is for, and on an account whose contracts were all imported, an
// arbitrary undated row would present itself as current.
//
// created_at is the tiebreak among terms beginning on the same day, and the
// only ordering left among undated ones. id breaks the remaining tie so the
// order is total — a keyset cursor over a non-total order repeats or skips rows
// at a page boundary.
//
// A constant both queries reference, rather than the clause written out twice:
// they are one order, and the index (contract_account_ix) is built to serve
// this exact clause, null placement included. Two copies could drift from each
// other and from the index, and a page that no longer matches its index is
// served by a sort node over the whole account without saying so.
const termOrder = "starts_on DESC NULLS LAST, created_at DESC, id DESC"

// continueAfterTerm renders the keyset predicate that resumes a page after the
// row a cursor names, matching termOrder including where it puts NULLs.
//
// THE NULL HANDLING IS THE PART THAT GOES WRONG. Under `DESC NULLS LAST` every
// dated term precedes every undated one, so the continuation asks a different
// question on each side of that boundary:
//
//   - after a DATED row, the rest of the page is the terms ordering below it
//     AND every undated term, because all of those come later;
//   - after an UNDATED row, only undated terms remain — admitting a dated one
//     would serve a row the reader already passed.
//
// A single tuple comparison cannot say that: `(starts_on, created_at, id) <
// (…)` is NULL-valued whenever either side is NULL, so it admits nothing at all
// once the page crosses into the undated tail, and the list ends early while
// rows remain.
func continueAfterTerm(cursor storekit.Cursor, arg func(any) int) string {
	if cursor.SortKey == nil {
		// The cursor names an undated term: the tail is undated terms only.
		return storekit.SQLf("starts_on IS NULL AND (created_at, id) < ($%d, $%d)",
			arg(cursor.CreatedAt), arg(cursor.ID))
	}
	// Each branch registers ONLY the arguments its own clause names. An
	// argument passed and never referenced has no type Postgres can infer from
	// anywhere, and it answers 42P18 for a parameter that is simply unused —
	// a confusing failure to meet at a page boundary, and one that reads as a
	// cast problem rather than an arity one.
	//
	// ::date because the token carries the start date as TEXT, and a row
	// comparison gives Postgres nothing to infer it from.
	return storekit.SQLf(
		"((starts_on IS NOT NULL AND (starts_on, created_at, id) < ($%d::date, $%d, $%d)) OR starts_on IS NULL)",
		arg(*cursor.SortKey), arg(cursor.CreatedAt), arg(cursor.ID))
}

// termCursor mints the token that continues a page after one contract,
// carrying the term the ordering is by.
//
// SortKey holds the start date, and nil is not "unset" here — it is the row's
// own NULL, which continueAfterTerm reads as "the page is into the undated
// tail". SortField and SortDesc record which ordering the token was minted
// under, so a cursor from a differently ordered list is legible rather than
// silently applied to this one.
func termCursor(c crmcontracts.Contract) (string, error) {
	cur := storekit.Cursor{
		CreatedAt: cursorTime(c.CreatedAt),
		ID:        ids.UUID(c.Id),
		SortField: "starts_on",
		SortDesc:  true,
	}
	if c.StartsOn != nil {
		key := c.StartsOn.String()
		cur.SortKey = &key
	}
	return storekit.EncodeOpaque(cur)
}
