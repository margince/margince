// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "github.com/margince/margince/backend/internal/modules/deals"

// Won sales are read by win-loss and by the company rollup. Both draw the deals
// from closedDealWhere, place them by wonSalesAnchorColumn and value them with
// wonSalesValueSQL, so the two give one period the same total.

// wonSalesAnchorColumn is the deal column that places a closed deal in a period:
// the moment it was won or lost.
const wonSalesAnchorColumn = "closed_at"

// wonSalesAnchor is the anchor over one deal alias.
func wonSalesAnchor(alias string) string { return alias + "." + wonSalesAnchorColumn }

// closedDealWhere is the live deals that were won or lost.
func closedDealWhere(alias string) string {
	return alias + ".archived_at IS NULL AND " + alias + ".status IN ('won','lost')"
}

// wonSalesValueSQL is one closed deal's money in the base currency. That is the
// base amount frozen when it was won, or its own amount in the base currency.
func wonSalesValueSQL(asOfSQL, baseSQL, alias string) string {
	return deals.BaseValueSQL(asOfSQL, baseSQL, alias)
}
