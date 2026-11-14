// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type contactIdentity struct {
	reference crmcontracts.RecordReference
	email     *string
	logoKey   *string
}

func (s *Store) ContactLabels(ctx context.Context, want []ids.UUID) (map[ids.UUID]string, error) {
	return s.labelsOf(ctx, contactEntity, want)
}

func (s *Store) CompanyLabels(ctx context.Context, want []ids.UUID) (map[ids.UUID]string, error) {
	return s.labelsOf(ctx, companyEntity, want)
}

func (s *Store) LeadLabels(ctx context.Context, want []ids.UUID) (map[ids.UUID]string, error) {
	return s.labelsOf(ctx, leadEntity, want)
}

func (s *Store) labelsOf(ctx context.Context, object string, want []ids.UUID) (map[ids.UUID]string, error) {
	query, args, err := identityQuery(ctx, object, want, storekit.LiveOnly, false)
	if err != nil {
		return nil, err
	}
	labels := map[ids.UUID]string{}
	if len(want) == 0 {
		return labels, nil
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		labels, err = storekit.LabelsByID(ctx, tx, query, args...)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("contacts: reading %s labels: %w", object, err)
	}
	return labels, nil
}

func (s *Store) recordIdentities(ctx context.Context, object string, want []ids.UUID, archived storekit.ArchivedFilter) ([]contactIdentity, error) {
	query, args, err := identityQuery(ctx, object, want, archived, true)
	if err != nil {
		return nil, err
	}
	found := []contactIdentity{}
	if len(want) == 0 {
		return found, nil
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		found, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (contactIdentity, error) {
			var identity contactIdentity
			var id ids.UUID
			err := row.Scan(&id, &identity.reference.Name, &identity.reference.Archived, &identity.email, &identity.logoKey)
			identity.reference.Id = openapi_types.UUID(id)
			return identity, err
		})
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("contacts: reading %s identities: %w", object, err)
	}
	return found, nil
}

// Legacy labels keep their two-column projection; references also need archive and identity fields.
func identityQuery(ctx context.Context, object string, want []ids.UUID, archived storekit.ArchivedFilter, reference bool) (string, []any, error) {
	if err := auth.Require(ctx, object, principal.ActionRead); err != nil {
		return "", nil, err
	}
	if len(want) == 0 {
		return "", nil, nil
	}
	table, name, email, logo := "", "r.full_name", "NULL::text", "NULL::text"
	switch object {
	case contactEntity:
		table = "contact"
	case companyEntity:
		table, name, logo = "company", "r.display_name", "r.logo_object_key"
	case leadEntity:
		table, email = "lead", "r.email"
	default:
		return "", nil, apperrors.ErrInvalidArgument
	}
	columns := "r.id, coalesce(" + name + ", '')"
	if reference {
		columns = "r.id, " + name + ", r.archived_at IS NOT NULL, " + email + ", " + logo
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idsPos := arg(want)
	scope, err := auth.ScopeClauseFor(ctx, object, "r", arg)
	if err != nil {
		return "", nil, err
	}
	if scope == "" {
		scope = sqlAlwaysVisible
	}
	live := ""
	if archived == storekit.LiveOnly {
		live = " AND r.archived_at IS NULL"
	}
	query := fmt.Sprintf(`SELECT %s FROM %s r WHERE r.id = ANY($%d) AND (%s)%s`, columns, table, idsPos, scope, live)
	return query, args, nil
}
