// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package recordexport

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/fieldmask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// MaskedRowSelects renders the per-column SELECT list with the caller's field
// masks applied in the statement itself. An always-masked column goes out NULL
// on every row. A write-authority-conditioned one goes out NULL only on the rows
// the caller could not change, the decision the deal list makes per row.
// The raw export and the filter preview share this one spelling of it.
func MaskedRowSelects(ctx context.Context, tx pgx.Tx, table string, columns []string, idList []ids.UUID) ([]string, []any, error) {
	args := []any{idList}
	p, err := storekit.Actor(ctx)
	if err != nil {
		return nil, nil, err
	}
	alwaysMasked, conditioned := maskSets(p, table, columns)
	// Only a conditioned mask naming a REAL column earns the writable-set bind.
	// A stale mask row naming no column would register a parameter no CASE
	// references, which Postgres refuses outright.
	rendered := false
	for _, col := range columns {
		if conditioned[col] {
			rendered = true
			break
		}
	}
	writablePos := 0
	if rendered {
		writable, err := auth.WritableSubset(ctx, tx, table, idList)
		if err != nil {
			return nil, nil, err
		}
		writableIDs := make([]ids.UUID, 0, len(writable))
		for id, ok := range writable {
			if ok {
				writableIDs = append(writableIDs, id)
			}
		}
		args = append(args, writableIDs)
		writablePos = len(args)
	}
	selects := make([]string, len(columns))
	for i, col := range columns {
		source := "t." + col
		if derived, ok := DerivedColumnExpr(table, col); ok {
			source = derived
		}
		switch {
		case alwaysMasked[col]:
			selects[i] = fmt.Sprintf("NULL AS %s", col)
		case conditioned[col]:
			selects[i] = fmt.Sprintf("CASE WHEN t.id = ANY($%d) THEN %s END AS %s", writablePos, source, col)
		default:
			selects[i] = source + " AS " + col
		}
	}
	return selects, args, nil
}

// withholdDerivedColumns extends both mask sets to the stored columns computed
// from a withheld field. A deal's base amount and rate print the masked money
// back out. The stricter mask on any input decides the column.
func withholdDerivedColumns(table string, columns []string, alwaysMasked, conditioned map[string]bool) {
	for _, col := range columns {
		inputs, derived := fieldmask.DerivedInputs(table, col)
		if !derived {
			continue
		}
		for _, input := range inputs {
			switch {
			case alwaysMasked[input]:
				alwaysMasked[col] = true
			case conditioned[input]:
				conditioned[col] = true
			}
		}
	}
}

// maskSets are the columns the caller's role masks always withhold and the ones
// it withholds outside its write authority, derived columns included.
func maskSets(p principal.Principal, table string, columns []string) (alwaysMasked, conditioned map[string]bool) {
	alwaysMasked, conditioned = map[string]bool{}, map[string]bool{}
	for _, f := range auth.MaskedFields(p, table, true) {
		alwaysMasked[f] = true
	}
	for _, f := range auth.MaskedFields(p, table, false) {
		if !alwaysMasked[f] {
			conditioned[f] = true
		}
	}
	withholdDerivedColumns(table, columns, alwaysMasked, conditioned)
	return alwaysMasked, conditioned
}
