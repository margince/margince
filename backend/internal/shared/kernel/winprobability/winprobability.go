// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package winprobability spells a deal's effective win probability in SQL:
// its own where set, else its stage's. In the kernel because the deals module
// and the saved-list vocabulary both filter on it and may not import each
// other.
package winprobability

import "fmt"

// SQL is the deal's effective win probability over a deal alias. A scalar
// subquery, so a caller needs no join.
func SQL(deal string) string {
	return fmt.Sprintf("COALESCE(%[1]s.win_probability_override, "+
		"(SELECT wp_stage.win_probability FROM stage wp_stage WHERE wp_stage.id = %[1]s.stage_id))", deal)
}
