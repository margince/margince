// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
)

// gateObject maps an extension kind to its declared object; an undeclared one is no record, so even the system principal is refused.
func gateObject(kind string) (string, error) {
	if !recordkinds.IsExtensionName(kind) {
		return kind, nil
	}
	declared, ok := recordkinds.Lookup(kind)
	if !ok {
		return "", fmt.Errorf("no extension declares the record kind %q: %w", kind, apperrors.ErrPermissionDenied)
	}
	return declared.Object, nil
}

// ReadableKind is an extension kind the caller can read: a unit still declares
// it and the caller holds its read right. A removed kind and a kind the caller
// may not read answer alike, so no reader can probe which kinds exist.
func ReadableKind(ctx context.Context, kind string) (recordkinds.Declared, bool) {
	declared, ok := recordkinds.Lookup(kind)
	if !ok || !Allows(ctx, kind, principal.ActionRead) {
		return recordkinds.Declared{}, false
	}
	return declared, true
}

// ExtensionScopeClause renders the caller's row scope over a declared kind's
// table aliased alias, as ScopeClauseFor does over a deal. Empty means every
// row: the kind declares no owner column, or the caller reads every row. It
// answers which rows only; each reader takes the kind's read right first.
func ExtensionScopeClause(ctx context.Context, kind, alias string, arg func(any) int) (string, error) {
	declared, ok := recordkinds.Lookup(kind)
	if !ok {
		return "", fmt.Errorf("No installed extension declares the record kind %q: %w", kind, apperrors.ErrNotFound)
	}
	if declared.OwnerColumn == "" {
		return "", nil
	}
	p, err := rbacActor(ctx)
	if err != nil {
		return "", err
	}
	if Unbounded(p) {
		return "", nil
	}
	column := pgx.Identifier{declared.OwnerColumn}.Sanitize()
	return ownerColumnPredicate(p, arg, unownedIsShared, column)(alias), nil
}

// ExtensionTableScopeClause is ExtensionScopeClause for a unit table named
// directly. A table that holds no declared kind has no owner core knows of.
// Its clause is empty, as for a kind without an owner column.
func ExtensionTableScopeClause(ctx context.Context, table, alias string, arg func(any) int) (string, error) {
	declared, ok := recordkinds.LookupTable(table)
	if !ok {
		return "", nil
	}
	return ExtensionScopeClause(ctx, declared.Name, alias, arg)
}
