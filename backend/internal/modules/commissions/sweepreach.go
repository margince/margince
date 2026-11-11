// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commissions

import "github.com/margince/margince/backend/pkg/extension/reach"

// SweepReach declares the partner company an entry credits, so a company merge
// moves what the retired partner earned onto the survivor.
func SweepReach() []reach.Table {
	return []reach.Table{{
		Name:  "commission_entry",
		Links: []reach.Link{{Column: filterPartnerCompanyID, Record: reach.Company}},
	}}
}
