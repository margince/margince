// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package accountdraft

import (
	"github.com/margince/margince/backend/internal/compose/draftcore"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// Grounding names every record this input was folded from: what a kept draft
// must re-prove its reader still sees. The dossier is the company's own.
func (in Input) Grounding() ([]datasource.EntityRef, error) {
	named := []draftcore.Ground{
		{Type: datasource.EntityCompany, ID: in.CompanyID},
		{Type: datasource.EntityContact, ID: in.Recipient.ID},
	}
	if in.Deal != nil {
		named = append(named, draftcore.Ground{Type: datasource.EntityDeal, ID: in.Deal.ID})
	}
	if in.Project != nil {
		named = append(named, draftcore.Ground{Type: datasource.EntityProject, ID: in.Project.ID})
	}
	if in.Commitment != nil {
		named = append(named, draftcore.Ground{Type: datasource.EntityActivity, ID: in.Commitment.ID})
	}
	for _, act := range in.Recent {
		named = append(named, draftcore.Ground{Type: datasource.EntityActivity, ID: act.ID})
	}
	return draftcore.Grounding(named)
}
