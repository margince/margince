// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package customfields

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// ListInput narrows the catalog read: Object is required (the admin
// field table is always per-object); Status of active/retired selects
// one lifecycle state, empty returns both — this admin view deliberately
// does NOT default-exclude retired rows (CUSTOM-FIELDS-WIRE-1), because
// a retired field's slug stays reserved and the admin table is the one
// surface that still shows it.
type ListInput struct {
	Object string
	Status *string
	Cursor *string
	Limit  *int
	// Sort is the caller's ordering, in the shared Sort component's spelling.
	// Nil or the default spelling means the house order.
	Sort *string
}

// catalogSortFields is what this list may be ordered by: the columns it
// publishes and a reader can therefore see.
//
// A column a reader can see is one they can order by — a header that cannot be
// clicked is a dead control — so the vocabulary is derived from what the table
// shows rather than picked. The ones left out are left out for a reason: `id`
// and `version` are not columns anybody sorts a field table by, `options` is
// json, and `column_name` is the physical column behind the field, which the
// admin table shows as provenance rather than as an axis.
//
// No custom columns: the catalog IS the custom-column registry, and a
// `cf_` sort over it would be asking the registry to order itself by one of
// the things it registers.
var catalogSortFields = map[string]storekit.SortField{
	"label":       storekit.Column(fieldcatalog.TypeText),
	"slug":        storekit.Column(fieldcatalog.TypeText),
	"type":        storekit.Column(fieldcatalog.TypeText),
	fieldStatus:   storekit.Column(fieldcatalog.TypeText),
	"object":      storekit.Column(fieldcatalog.TypeText),
	"currency":    storekit.Column(fieldcatalog.TypeText),
	"created_at":  storekit.Column(storekit.KindTimestamp),
	"updated_at":  storekit.Column(storekit.KindTimestamp),
	"archived_at": storekit.Column(storekit.KindTimestamp),
}

// predicate renders the WHERE clause and its arguments after validating
// the closed object/status vocabularies — split from List to keep each
// half inside the lint's complexity budget.
func (in ListInput) predicate(ctx context.Context) (string, *storekit.ListSort, []any, error) {
	if !allowedObjects[in.Object] {
		return "", nil, nil, &ValidationError{Errors: []FieldError{{Field: fieldObject, Code: codeUnsupportedObject}}}
	}
	status := ""
	if in.Status != nil {
		status = *in.Status
	}
	if status != "" && status != statusActive && status != statusRetired {
		return "", nil, nil, &ValidationError{Errors: []FieldError{{Field: fieldStatus, Code: codeUnsupportedStatus}}}
	}

	args := []any{}
	arg := func(v any) int { args = append(args, v); return len(args) }
	// The sort FIRST, because it may bind parameters of its own and every
	// clause below binds through the same counter.
	sorted, err := storekit.ParseListSort(ctx, in.Sort, catalogSortFields, arg)
	if err != nil {
		return "", nil, nil, err
	}
	where := storekit.SQLf("object = $%d", arg(in.Object))
	if status != "" {
		where += storekit.SQLf(" AND status = $%d", arg(status))
	}
	if in.Cursor != nil && *in.Cursor != "" {
		// Through the sort's own keyset, which refuses a token minted under a
		// different ordering rather than resuming on an axis this page is not
		// ordered by.
		keyset, err := sorted.KeysetClause(*in.Cursor, arg)
		if err != nil {
			return "", nil, nil, err
		}
		where += " AND " + keyset
	}
	return where, sorted, args, nil
}

// List reads the catalog page for one object, in the caller's order or the
// contract's default -created_at,id. Installation-wide admin config: the object
// read grant is the whole authority, and there is no row scope to compose
// (the pipeline precedent).
func (s *Service) List(ctx context.Context, in ListInput) ([]crmcontracts.CustomField, storekit.Page, error) {
	if err := auth.Require(ctx, rbacObject, principal.ActionRead); err != nil {
		return nil, storekit.Page{}, err
	}
	where, sorted, args, err := in.predicate(ctx)
	if err != nil {
		return nil, storekit.Page{}, err
	}
	limit := storekit.ClampLimit(in.Limit)

	fields := []crmcontracts.CustomField{}
	var cursorKeys []*string
	var page storekit.Page
	err = database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT `+catalogColumns+sorted.CursorKeySuffix()+` FROM custom_field WHERE `+where+
				sorted.OrderBy()+storekit.SQLf(` LIMIT %d`, limit+1),
			args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			f, key, err := scanCustomFieldPage(rows, sorted)
			if err != nil {
				return err
			}
			fields = append(fields, f)
			cursorKeys = append(cursorKeys, key)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(fields) > limit {
			fields = fields[:limit]
			last := fields[len(fields)-1]
			// The sort's own token, so the next page can only continue THIS
			// ordering: a cursor minted under one sort and replayed under
			// another is refused rather than resumed on the wrong axis.
			next, err := sorted.EncodePageCursor(cursorKeys[limit-1], last.CreatedAt, ids.UUID(last.Id))
			if err != nil {
				return err
			}
			page = storekit.Page{HasMore: true, NextCursor: next}
		}
		return nil
	})
	if err != nil {
		return nil, storekit.Page{}, err
	}
	return fields, page, nil
}
