// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"math"
	"math/big"

	"github.com/jackc/pgx/v5/pgtype"
)

// outOfRangeColumn names, on a report or analytics row, the columns whose
// number was withheld. Prefixed so it cannot collide with a caller's alias.
const outOfRangeColumn = "_out_of_range"

// beyondExactRange reports whether v, a row value bound for JSON, is a number
// past ±reportingExactInteger. A browser and a model parse every JSON number
// as a float64, which turns such a number into another one. So a total that
// large is withheld rather than sent wrong. A column bound holds a stored
// value; a sum or average is computed, and only this check holds it.
//
//craft:ignore naked-any a row value spans every SQL type a report selects; the switch narrows each
func beyondExactRange(v any) bool {
	switch n := v.(type) {
	case int64:
		return n > reportingExactInteger || n < -reportingExactInteger
	case float64:
		return math.Abs(n) > float64(reportingExactInteger)
	case pgtype.Numeric:
		return numericBeyondExactRange(n)
	default:
		return false
	}
}

// numericBeyondExactRange compares the whole part of n, so an average with a
// fraction is judged by the integer a reader would round it to.
func numericBeyondExactRange(n pgtype.Numeric) bool {
	if !n.Valid || n.NaN || n.InfinityModifier != pgtype.Finite || n.Int == nil {
		return false
	}
	whole := new(big.Int).Abs(n.Int)
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(math.Abs(float64(n.Exp)))), nil)
	if n.Exp >= 0 {
		whole.Mul(whole, scale)
	} else {
		whole.Quo(whole, scale)
	}
	return whole.Cmp(big.NewInt(reportingExactInteger)) > 0
}

// withholdBeyondExactRange sets every out-of-range value of row to nil and
// names those columns under outOfRangeColumn, so the rest of the row is still
// answered. A row with none carries no such key.
//
//craft:ignore naked-any a report row is a JSON object whose values span every SQL type
func withholdBeyondExactRange(row map[string]any, columns []string) {
	var withheld []string
	for _, column := range columns {
		if beyondExactRange(row[column]) {
			row[column] = nil
			withheld = append(withheld, column)
		}
	}
	if len(withheld) > 0 {
		row[outOfRangeColumn] = withheld
	}
}
