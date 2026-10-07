// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Who a contact works for, as every contact read carries it: the current
// primary employment edge resolved to the account it names.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// attachContactEmployers stamps each contact's employer onto a whole page in one
// statement — the contact list's company column and the record read share it,
// because a reader asking "who is this and where do they work" is asking one
// question wherever they ask it.
//
// CURRENT PRIMARY employment only, through employment.CurrentPrimarySQL rather
// than the flag alone — a list that trusted the flag would go on naming the
// company somebody's last day has already passed at. The match is at most one
// row per contact (uq_rel_current_primary_employer), so the join cannot duplicate
// a page the way an unconstrained edge join would.
//
// It carries BOTH gates and returns nothing rather than failing when either
// refuses. The edge gate, because who works where is a fact about the PAIR that
// the grant on the contact does not cover; the company gate and row scope,
// because the name is that record's to disclose. Refusal omits the field and
// keeps the page: a contact list is not a question about employers, so a caller
// who may read contacts and not edges still gets their contacts — the contract says
// absent never means "works nowhere", which is what stops a reader taking the
// omission for an answer.
// currentEmployerFrom is the FROM and WHERE of "which company this contact works
// for, as this caller may see it", with the contact binding left to the caller.
//
// The row that PRINTS the employer and the expression that ORDERS BY it read
// this, so a reader sees the company the list was arranged by rather than a
// second reading of the same question.
//
// It takes its own gates rather than being handed them — the employment EDGE
// scope, the company object grant and its row scope, each of which can
// hide an employer independently — because a caller that had to remember to
// pass three could forget one, and the one forgotten would be the one nobody
// notices until a reader sees an account they hold no grant for.
//
// A false is not a refusal: a caller who may not traverse edges, or may not
// read companies, still gets their contacts. A contact list is not a question
// about employers, and the contract says an absent employer never means "works
// nowhere".
func currentEmployerFrom(
	ctx context.Context, contactBinding string, arg func(any) int,
) (from string, visible bool, err error) {
	scope, visible, err := employerScope(ctx, "rel", arg)
	if err != nil || !visible {
		return "", visible, err
	}
	return `
		 FROM relationship rel
		 JOIN company company ON company.id = rel.company_id
		 WHERE ` + contactBinding + `
		   AND rel.kind = 'employment'
		   AND ` + employment.CurrentPrimarySQL("rel") + `
		   AND rel.archived_at IS NULL
		   AND ` + scope, true, nil
}

// employerScope is the clause under which an employment edge aliased relAlias,
// joined to its company as `company`, is this caller's to see. Shared by every
// read that names an employer, so the three gates are asked in one place.
func employerScope(ctx context.Context, relAlias string, arg func(any) int) (clause string, visible bool, err error) {
	edgeBound, err := auth.EdgeReadScope(ctx, relAlias, arg)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if edgeBound == "" {
		edgeBound = scopeAllRows
	}
	if err := auth.Require(ctx, companyEntity, principal.ActionRead); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return "", false, nil
		}
		return "", false, err
	}
	companyScope, err := auth.ScopeClauseFor(ctx, companyEntity, "company", arg)
	if err != nil {
		return "", false, err
	}
	if companyScope == "" {
		companyScope = scopeAllRows
	}
	return edgeBound + ` AND company.archived_at IS NULL AND ` + companyScope, true, nil
}

func attachContactEmployers(ctx context.Context, tx pgx.Tx, idx map[openapi_types.UUID]*crmcontracts.Contact, contactIDs []ids.UUID) error {
	employers, err := readContactEmployers(ctx, tx, contactIDs)
	if err != nil {
		return err
	}
	for contactID, employer := range employers {
		idx[openapi_types.UUID(contactID)].Employer = &employer
	}
	return nil
}

// CurrentEmployers answers where each of these contacts works today, as this
// caller may see it: a contact whose employer is withheld from them is absent
// from the map. The edge scope in employerScope covers both ends of the
// employment, so a contact the caller may not see is withheld with it. A caller
// without read on contacts or companies is refused with
// apperrors.ErrPermissionDenied.
func (s *Store) CurrentEmployers(ctx context.Context, contactIDs []ids.UUID) (map[ids.UUID]crmcontracts.ContactEmployer, error) {
	for _, object := range []string{contactEntity, companyEntity} {
		if err := auth.Require(ctx, object, principal.ActionRead); err != nil {
			return nil, err
		}
	}
	var out map[ids.UUID]crmcontracts.ContactEmployer
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = readContactEmployers(ctx, tx, contactIDs)
		return err
	})
	return out, err
}

// readContactEmployers is the one read behind every employer a contact
// surface names, under currentEmployerFrom's gates.
func readContactEmployers(ctx context.Context, tx pgx.Tx, contactIDs []ids.UUID) (map[ids.UUID]crmcontracts.ContactEmployer, error) {
	out := make(map[ids.UUID]crmcontracts.ContactEmployer, len(contactIDs))
	if len(contactIDs) == 0 {
		return out, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	contacts := arg(contactIDs)

	from, visible, err := currentEmployerFrom(ctx, storekit.SQLf("rel.contact_id = ANY($%d)", contacts), arg)
	if err != nil || !visible {
		return out, err
	}

	rows, err := tx.Query(ctx, `SELECT rel.contact_id, company.id, company.display_name`+from, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var contactID, companyID ids.UUID
		var name string
		if err := rows.Scan(&contactID, &companyID, &name); err != nil {
			return nil, err
		}
		out[contactID] = crmcontracts.ContactEmployer{CompanyId: openapi_types.UUID(companyID), CompanyName: name}
	}
	return out, rows.Err()
}
