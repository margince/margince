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
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
)

// relationshipFieldImage is the edge as its own fields: every column
// UpdateRelationship can move, plus the kind that says what those fields mean.
//
// Only a PATCH renders the wide image. A create keeps the two keys it always
// recorded, because widening it would widen something else: field ownership is
// decided from the latest audit image holding a key, so a create claiming the
// dates and the primary-employer flag makes its author their owner, and an
// agent's later patch of a flag nobody typed would stage for a human decision it
// never used to need. What a create records is its own question, asked of every
// record type at once, and it is not this one's to answer.
func relationshipFieldImage(rel relationshipRow) map[string]any {
	return map[string]any{
		relationshipKindField: rel.Kind,
		relationshipRoleField: rel.Role,
		"is_current_primary":  rel.IsCurrentPrimary,
		// Both are `date` columns, and a date has two renderings that are not
		// interchangeable here: `to_jsonb(row)` gives "2024-05-06" and a Go
		// time.Time marshals as "2024-05-06T00:00:00Z". The undo path asks whether
		// a field has moved by comparing this image against the live row as JSON,
		// so the timestamp spelling reads as moved the instant it is written — and
		// undo refuses a change nobody has touched, blaming a supersession that
		// never happened. imageTime reads both spellings, because images written
		// the old way are already in every deployed database.
		"started_at": edgeImageDate(rel.StartedAt),
		"ended_at":   edgeImageDate(rel.EndedAt),
	}
}

// edgeImageDate renders a link's date the way Postgres renders the `date`
// column it came from, keeping absence as absence.
//
//craft:ignore naked-any a date is either its own text or a JSON null, and this feeds a map[string]any audit image that must carry both
func edgeImageDate(v *time.Time) any {
	if v == nil {
		return nil
	}
	return v.Format(time.DateOnly)
}

// relationshipIdentityImage is the edge named and typed, which is what a create
// and an archive are about: neither moved a field, and both are answerable by
// which edge they were.
func relationshipIdentityImage(rel relationshipRow) map[string]any {
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
	return emitRelationshipChangeWithEvidence(ctx, tx, action, before, rel, nil)
}

// emitRelationshipChangeWithEvidence is the same write shape carrying context
// ABOUT the mutation. Evidence is never a field image — the reversal path names
// the entry it put back there, which is what lets a reader pair the two lines
// instead of counting an undo as a fresh change.
func emitRelationshipChangeWithEvidence(ctx context.Context, tx pgx.Tx, action string,
	before map[string]any, rel relationshipRow, evidence map[string]any,
) error {
	// Through the shared resolver, which answers with an error rather than a
	// nil dereference: this used to read the endpoint pointer the kind implied
	// straight off the row, and a kind whose shape did not hold would panic
	// here instead of refusing.
	anchorObject, anchorID, err := anchorIDOf(rel)
	if err != nil {
		return err
	}
	after := relationshipIdentityImage(rel)
	if !storekit.AbsentImage(before) {
		before, after = storekit.ChangedColumns(before, relationshipFieldImage(rel))
	}
	auditID, err := storekit.AuditWithEvidence(ctx, tx, action, "relationship", rel.ID, before, after, evidence)
	if err != nil {
		return err
	}
	changedFields := map[string]any{
		"delta": map[string]any{"relationship": map[string]any{"id": rel.ID, "kind": rel.Kind, "action": action}},
	}
	return storekit.EmitEvent(ctx, tx, auditID, anchorID, relationshipUpdatedPayload(anchorObject, changedFields))
}

// relationshipUpdatedPayload builds the anchor's .updated event for a
// relationship mutation — the same changed_fields delta wrapped in
// whichever of the three anchors' published OPEN envelopes this edge
// points at. All three (deal.updated, person.updated,
// organization.updated) are OPEN envelopes with an identical
// changed_fields shape, so the only real work here is picking the right
// generated struct for the anchor.
//
//nolint:ireturn // dispatches to one of PublicEventDeal/Project/Person/OrganizationUpdated by anchorObject; tested directly via the interface in person_organization_payload_test.go
func relationshipUpdatedPayload(anchorObject string, changedFields map[string]any) events.Payload {
	switch anchorObject {
	case anchorDeal:
		return crmcontracts.PublicEventDealUpdated{ChangedFields: changedFields}
	case projectObjectName:
		return crmcontracts.PublicEventProjectUpdated{ChangedFields: changedFields}
	case anchorPerson:
		return crmcontracts.PublicEventPersonUpdated{ChangedFields: changedFields}
	default: // organization
		return crmcontracts.PublicEventOrganizationUpdated{ChangedFields: changedFields}
	}
}
