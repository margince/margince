// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import "fmt"

// dealLinkRows is the tail of a query, everything after SELECT. It covers the
// links that join the deal named by dealID to another live deal. Links to a
// company or contact are left out. The other end is then a deal. Every seat
// holding the deal read right may open it, so no filter or breakdown names a
// record the caller could not open.
func dealLinkRows(dealID string) string {
	return fmt.Sprintf(`FROM record_link rl
		  JOIN deal linked ON linked.archived_at IS NULL
		   AND linked.id = CASE WHEN rl.from_id = %[1]s THEN rl.to_id ELSE rl.from_id END
		 WHERE rl.from_type = 'deal' AND rl.to_type = 'deal'
		   AND (rl.from_id = %[1]s OR rl.to_id = %[1]s)`, dealID)
}

// DealLinkFilter is "the deal named by dealID has a link of the type bound at
// typePlaceholder to another live deal". dealID is a column expression and
// typePlaceholder a bound parameter such as `$3`, never a request value.
func DealLinkFilter(dealID, typePlaceholder string) string {
	return fmt.Sprintf(`EXISTS (SELECT 1 %s AND rl.link_type = %s)`, dealLinkRows(dealID), typePlaceholder)
}

// DealLinkTypesExpr is the sorted combination of link types the deal holds with
// other live deals, or `none`. One deal falls in one group, so a breakdown by it
// adds up to the whole population.
func DealLinkTypesExpr(dealID string) string {
	return fmt.Sprintf(`COALESCE((SELECT string_agg(DISTINCT rl.link_type, ', ' ORDER BY rl.link_type) %s), 'none')`,
		dealLinkRows(dealID))
}
