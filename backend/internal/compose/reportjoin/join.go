// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package reportjoin resolves opt-in deal record pickers to extension dimensions.
package reportjoin

import (
	"context"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
	"github.com/margince/margince/backend/pkg/extension"
)

const (
	reportExtensionAlias = "ext_report"
	extSchemaPrefix      = `"ext".`
)

// Join carries a report dimension and the target record access checks.
type Join struct {
	Kind, SQL, Dimension string
	object, reference    string
}

// Scope excludes inaccessible targets while preserving empty pickers.
func (j Join) Scope(ctx context.Context, arg func(any) int) (string, error) {
	scope, err := auth.ExtensionScopeClause(ctx, j.Kind, reportExtensionAlias, arg)
	if err != nil {
		return "", err
	}
	live := reportExtensionAlias + ".id IS NOT NULL"
	if scope != "" {
		live += " AND " + scope
	}
	return "(" + j.reference + " IS NULL OR (" + live + "))", nil
}

// Masks applies the target kind field exclusions to joined rows.
func (j Join) Masks(ctx context.Context, arg func(any) int) ([]string, error) {
	clauses, err := auth.MaskExclusionClauses(ctx, j.object, reportExtensionAlias, arg)
	if err != nil {
		return nil, err
	}
	for i, clause := range clauses {
		clauses[i] = "(" + j.reference + " IS NULL OR (" + clause + "))"
	}
	return clauses, nil
}

// Resolve requires both the picker and its target field to opt into reporting.
func Resolve(ctx context.Context, pool *pgxpool.Pool, unit extension.Name, by string) (*Join, bool, error) {
	picker, field, _ := strings.Cut(by, ".")
	var columns []fieldcatalog.ReportableColumn
	err := database.WithTx(ctx, pool, func(tx pgx.Tx) error {
		var err error
		columns, err = customfields.ReportableColumnsIn(ctx, tx, "deal")
		return err
	})
	if err != nil {
		return nil, false, err
	}
	for _, column := range columns {
		if column.Name != picker || column.Type != fieldcatalog.TypeRecord || len(column.Options) != 1 {
			continue
		}
		kind, ok := recordkinds.Lookup(column.Options[0])
		if !ok || kind.Unit != unit {
			return nil, false, nil
		}
		reportable := slices.ContainsFunc(kind.Fields, func(f extension.RecordKindField) bool { return f.Name == field && f.Reportable })
		if !reportable {
			return nil, false, nil
		}
		if err := auth.Require(ctx, kind.Name, principal.ActionRead); err != nil {
			return nil, false, err
		}
		reference := "t." + pgx.Identifier{column.Name}.Sanitize()
		return &Join{
			Kind: kind.Name, object: kind.Object, reference: reference,
			SQL:       "LEFT JOIN " + extSchemaPrefix + pgx.Identifier{kind.Table}.Sanitize() + " " + reportExtensionAlias + " ON " + reportExtensionAlias + ".id = CASE WHEN pg_input_is_valid(" + reference + ", 'uuid') THEN " + reference + "::uuid END AND " + reportExtensionAlias + ".archived_at IS NULL",
			Dimension: reportExtensionAlias + "." + pgx.Identifier{field}.Sanitize() + "::text",
		}, true, nil
	}
	return nil, false, nil
}
