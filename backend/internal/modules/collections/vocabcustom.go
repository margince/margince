// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

import (
	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// customFieldTypes maps the closed catalog types onto the predicate engine's
// own. A new catalog type arrives with its own entry here, and
// TestEveryCustomFieldTypeIsFilterable fails until it has one.
var customFieldTypes = map[string]storekit.FieldType{
	fieldcatalog.TypeText:        storekit.FieldText,
	fieldcatalog.TypeNumber:      storekit.FieldNumber,
	fieldcatalog.TypeDate:        storekit.FieldDate,
	fieldcatalog.TypeCurrency:    storekit.FieldCurrency,
	fieldcatalog.TypeMultiselect: storekit.FieldMultiselect,
	fieldcatalog.TypePicklist:    storekit.FieldPicklist,
	fieldcatalog.TypeBoolean:     storekit.FieldBoolean,
	fieldcatalog.TypeRecord:      storekit.FieldText,
}

// customField types one custom column for the predicate engine, and answers
// false for a catalog type this engine has no operators for. The column is
// left out rather than refused. This mapping runs over every column of the
// object, so a refusal would fail the whole resolution for the record type.
// A predicate that names a left-out field is still refused by CompilePredicate
// with CodeFilterFieldNotAllowed, so nothing matches the wrong rows.
func customField(column fieldcatalog.Column) (storekit.Field, bool) {
	fieldType, ok := customFieldTypes[column.Type]
	if !ok {
		return storekit.Field{}, false
	}
	return storekit.Field{
		Expr: `t.` + pgx.Identifier{column.Name}.Sanitize(),
		Type: fieldType,
		// Straight from the catalogue, which owns them for the same reason it owns
		// labels: they are per-workspace admin state. Empty for every non-picklist
		// type, which is what the column itself reports.
		Options:  column.Options,
		Currency: column.Currency,
	}, true
}
