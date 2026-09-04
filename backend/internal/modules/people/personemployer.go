// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Who a contact works for, as every person read carries it: the current
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
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// attachPersonEmployers stamps each person's employer onto a whole page in one
// statement — the contact list's company column and the record read share it,
// because a reader asking "who is this and where do they work" is asking one
// question wherever they ask it.
//
// CURRENT PRIMARY employment only, through CurrentPrimaryEmploymentSQL rather
// than the flag alone — a list that trusted the flag would go on naming the
// company somebody's last day has already passed at. The match is at most one
// row per person (uq_rel_current_primary_employer), so the join cannot duplicate
// a page the way an unconstrained edge join would.
//
// It carries BOTH gates and returns nothing rather than failing when either
// refuses. The edge gate, because who works where is a fact about the PAIR that
// the grant on the person does not cover; the organization gate and row scope,
// because the name is that record's to disclose. Refusal omits the field and
// keeps the page: a person list is not a question about employers, so a caller
// who may read people and not edges still gets their people — the contract says
// absent never means "works nowhere", which is what stops a reader taking the
// omission for an answer.
func attachPersonEmployers(ctx context.Context, tx pgx.Tx, idx map[openapi_types.UUID]*crmcontracts.Person, personIDs []ids.UUID) error {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	people := arg(personIDs)

	edgeBound, err := auth.EdgeReadScope(ctx, "rel", arg)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil
	}
	if err != nil {
		return err
	}
	if edgeBound == "" {
		edgeBound = scopeAllRows
	}
	if err := auth.Require(ctx, organizationEntity, principal.ActionRead); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return nil
		}
		return err
	}
	orgScope, err := auth.ScopeClauseFor(ctx, organizationEntity, "org", arg)
	if err != nil {
		return err
	}
	if orgScope == "" {
		orgScope = scopeAllRows
	}

	rows, err := tx.Query(ctx, storekit.SQLf(`SELECT rel.person_id, org.id, org.display_name
		 FROM relationship rel
		 JOIN organization org ON org.id = rel.organization_id
		 WHERE rel.person_id = ANY($%d)
		   AND rel.kind = 'employment'
		   AND `+CurrentPrimaryEmploymentSQL("rel")+`
		   AND rel.archived_at IS NULL
		   AND `+edgeBound+`
		   AND org.archived_at IS NULL
		   AND `+orgScope, people), args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var personID, orgID ids.UUID
		var name string
		if err := rows.Scan(&personID, &orgID, &name); err != nil {
			return err
		}
		idx[openapi_types.UUID(personID)].Employer = &crmcontracts.PersonEmployer{
			OrganizationId:   openapi_types.UUID(orgID),
			OrganizationName: name,
		}
	}
	return rows.Err()
}
