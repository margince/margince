// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The edge from the lead drafter to the activities store.
//
// leaddraft may not import activities — a module never imports a sibling, and
// the drafter sits in compose precisely so this edge is injected rather than
// taken. The seam is also the narrowing: leaddraft asks one question, "what has
// been said to this lead", and cannot ask the activities list anything else.

import (
	"context"

	"github.com/margince/margince/backend/internal/compose/persondraft"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// leadLinkType is what an activity_link row calls a lead. Named here rather
// than reusing captureCollisionTarget, which holds the same characters for a
// different question — which entity type capture stages a collision against —
// and would tie this read to a decision about capture.
const leadLinkType = "lead"

// leadCorrespondence answers one lead's timeline through the activities store's
// own gate, so a draft can only stand on messages this caller could open.
type leadCorrespondence struct{ store *activities.Store }

// ForLead reads what has been said to this lead, newest first.
//
// Bounded to what the draft actually reads. persondraft.FoldRecent keeps the
// newest few and the newest inbound message's opening; asking for a page far
// larger than that would carry a lead's whole history across the seam for the
// drafter to throw away.
func (c leadCorrespondence) ForLead(ctx context.Context, id ids.LeadID) ([]crmcontracts.Activity, error) {
	entityType := leadLinkType
	entityID := ids.UUID(id.UUID)
	limit := persondraft.DraftInputActivities
	rows, _, err := c.store.ListActivities(ctx, activities.ListActivitiesInput{
		EntityType: &entityType,
		EntityID:   &entityID,
		Limit:      &limit,
	})
	if err != nil {
		return nil, err
	}
	return rows, nil
}
