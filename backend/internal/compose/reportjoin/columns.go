// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reportjoin

import (
	"context"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
	"github.com/margince/margince/backend/pkg/extension"
)

func ResolvedPopulation(unit extension.Name, population string) string {
	if strings.Contains(population, "-") {
		return population
	}
	return extension.RecordKindName(unit, population)
}

func SortedCombination(expr string) string {
	return "(SELECT string_agg(v, ',' ORDER BY v) FROM unnest(" + expr + ") v)"
}

func MultiselectColumns(ctx context.Context, tx pgx.Tx, object datasource.EntityType) ([]fieldcatalog.Column, error) {
	cols, err := customfields.ActiveColumnsIn(ctx, tx, string(object))
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(cols, func(c fieldcatalog.Column) bool { return c.Type != fieldcatalog.TypeMultiselect }), nil
}

func ReportableColumns(cols []fieldcatalog.Column) []fieldcatalog.ReportableColumn {
	out := make([]fieldcatalog.ReportableColumn, 0, len(cols))
	for _, c := range cols {
		out = append(out, fieldcatalog.ReportableColumn{Column: c, Label: c.Name})
	}
	return out
}

// companyFieldExpr reads one company column for the deal's company. The
// subselect on the company's primary key matches at most one row, so the grain
// stays one row per deal. A deal with no company reads NULL.
func CompanyFieldExpr(column string) string {
	return "(SELECT c." + pgx.Identifier{column}.Sanitize() + " FROM " + "company" +
		" c WHERE c.id = " + "t.company_id" + ")"
}
