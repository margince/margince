// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"fmt"
	"time"
)

// DateWindowSQL accepts a validated, quoted column expression and binds its bounds.
// Annual dates compare month and day so the stored year cannot suppress a renewal.
func DateWindowSQL(column string, from, to time.Time, recurring bool, arg func(any) int) string {
	format := time.DateOnly
	if recurring {
		column = "to_char(" + column + ",'MMDD')"
		format = "0102"
	}
	lower, upper := from.Format(format), to.Format(format)
	first, last := arg(lower), arg(upper)
	if recurring && lower >= upper {
		return fmt.Sprintf("(%s >= $%d OR %s <= $%d)", column, first, column, last)
	}
	return fmt.Sprintf("%s BETWEEN $%d AND $%d", column, first, last)
}
