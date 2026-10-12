// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contactdraft

import (
	"github.com/margince/margince/backend/internal/compose/draftcore"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// Grounding names every record this input was folded from: what a kept draft
// must re-prove its reader still sees. The caller names the recipient's type,
// because a lead's draft folds into this same input.
func (in Input) Grounding(recipient datasource.EntityType) ([]datasource.EntityRef, error) {
	named := []draftcore.Ground{{Type: recipient, ID: in.Recipient.ID}}
	if in.Deal != nil {
		named = append(named, draftcore.Ground{Type: datasource.EntityDeal, ID: in.Deal.ID})
	}
	if in.Project != nil {
		named = append(named, draftcore.Ground{Type: datasource.EntityProject, ID: in.Project.ID})
	}
	for _, act := range in.Recent {
		named = append(named, draftcore.Ground{Type: datasource.EntityActivity, ID: act.ID})
	}
	for _, claim := range in.Claims {
		named = append(named, draftcore.Ground{Type: datasource.EntityActivity, ID: claim.SourceID})
	}
	if in.Meeting != nil {
		named = append(named, draftcore.Ground{Type: datasource.EntityActivity, ID: in.Meeting.ActivityID})
	}
	return draftcore.Grounding(named)
}
