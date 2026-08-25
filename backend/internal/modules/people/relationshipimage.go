// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// What an edge mutation records about the edge, and the write shape that
// carries it.
//
// The image is the row's own field values and nothing else: an edge carries
// role, dates and the primary-employer flag, and a patch that moves any of them
// is only answerable later if the row says what they held. The anchor the edge
// annotates is addressed by the event, not by the image — operation context
// belongs in evidence, because field history projects an image key as a change
// to a field of that name (storekit.AuditWithEvidence).

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/platform/database/storekit"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// relationshipImage is the edge as its own fields: every column
// UpdateRelationship can move, plus the kind that says what those fields mean.
//
// Only a PATCH renders the wide image. A create keeps the two keys it always
// recorded, because widening it would widen something else: field ownership is
// decided from the latest audit image holding a key, so a create claiming the
// dates and the primary-employer flag makes its author their owner, and an
// agent's later patch of a flag nobody typed would stage for a human decision it
// never used to need. What a create records is its own question, asked of every
// record type at once, and it is not this one's to answer.
// relationshipAnchorDeal names the deal anchor, so the switch below and any
// later reader share one spelling of it.
const relationshipAnchorDeal = "deal"

func relationshipImage(rel relationshipRow) map[string]any {
	return map[string]any{
		relationshipKindField: rel.Kind,
		relationshipRoleField: rel.Role,
		"is_current_primary":  rel.IsCurrentPrimary,
		"started_at":          rel.StartedAt,
		"ended_at":            rel.EndedAt,
	}
}

// relationshipCreateImage is what a create and an archive record: the edge named
// and typed, which is what those verbs are about.
func relationshipCreateImage(rel relationshipRow) map[string]any {
	return map[string]any{
		relationshipKindField: rel.Kind,
		relationshipRoleField: rel.Role,
	}
}

// emitRelationshipChange lands the write shape on the edge's anchor:
// audit on the relationship row, event on the anchor entity (an
// employment change IS a person change to every consumer).
//
// before is the edge as it stood before the write, and a patch is the only
// verb that has one — a create had no prior row, and an archive moves
// archived_at, which none of these fields describes. Given one, the pair is
// narrowed to what actually moved, so a role edit does not present the kind
// and the dates as changes nobody made.
func emitRelationshipChange(ctx context.Context, tx pgx.Tx, action string, before map[string]any, rel relationshipRow) error {
	anchorObject, _ := relationshipAnchor(rel.Kind)
	var anchorID ids.UUID
	switch anchorObject {
	case entityPerson:
		anchorID = rel.PersonID.UUID
	case relationshipAnchorDeal:
		anchorID = rel.DealID.UUID
	case projectObjectName:
		anchorID = rel.ProjectID.UUID
	default:
		anchorID = rel.OrganizationID.UUID
	}
	after := relationshipCreateImage(rel)
	if !storekit.AbsentImage(before) {
		before, after = storekit.ChangedColumns(before, relationshipImage(rel))
	}
	auditID, err := storekit.Audit(ctx, tx, action, "relationship", rel.ID, before, after)
	if err != nil {
		return err
	}
	changedFields := map[string]any{
		"delta": map[string]any{"relationship": map[string]any{"id": rel.ID, "kind": rel.Kind, "action": action}},
	}
	return storekit.EmitEvent(ctx, tx, auditID, anchorID, relationshipUpdatedPayload(anchorObject, changedFields))
}
