// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The two events a lead promotion emits, built here so promote.go holds the
// transaction and this file holds the wire shapes.

import (
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// promotedContactPayload builds the contact-side event a lead promotion
// emits — its own verb (contact.created) on a fresh contact, or a
// contact.updated changed_fields note carrying the fields the merge ACTUALLY
// applied when the promotion instead merged into an existing contact
// (merged=true, PO-F-1). changed_fields is the real merge delta (mergeFields),
// so it reports a filled title and omits converted_from_lead_id when that was
// already set — not a fixed map that could misstate the change. A merge that
// applied nothing returns nil (no contact.updated to emit). The two shapes are
// different published events, not variants of one, so the return type is the
// shared events.Payload seam rather than a single struct.
//
//nolint:ireturn // dispatches to PublicEventContactCreated vs Updated by the merged condition; tested directly via the interface in contact_company_payload_test.go
func promotedContactPayload(contact crmcontracts.Contact, merged bool, mergeFields map[string]any) events.Payload {
	if merged {
		if len(mergeFields) == 0 {
			return nil
		}
		return crmcontracts.PublicEventContactUpdated{ChangedFields: mergeFields}
	}
	return crmcontracts.PublicEventContactCreated{FullName: contact.FullName}
}

// leadPromotedPayload builds the lead-side event a promotion emits —
// its own verb (events.md §5.5), never a lead.updated. evidenceActivityID
// is nil for a human_qualify with no linked activity; the wire field is
// then omitted rather than marshaled as null.
func leadPromotedPayload(
	contactID ids.ContactID, outcome, trigger string,
	evidenceActivityID *ids.ActivityID, carried []ids.UUID,
) crmcontracts.PublicEventLeadPromoted {
	p := crmcontracts.PublicEventLeadPromoted{
		PromotedContactId: openapi_types.UUID(contactID.UUID),
		DedupeOutcome:     outcome,
		Trigger:           trigger,
	}
	if evidenceActivityID != nil {
		p.EvidenceRef = uuidPtr(&evidenceActivityID.UUID)
	}
	// Omitted when the promotion carried nothing, rather than sent as an empty
	// array: a lead with no timeline and a build that does not report one are
	// different facts, and a consumer that treats [] as "nothing to do" would
	// read the second as the first.
	if len(carried) > 0 {
		ids := make([]openapi_types.UUID, 0, len(carried))
		for _, activity := range carried {
			ids = append(ids, openapi_types.UUID(activity))
		}
		p.CarriedActivityIds = &ids
	}
	return p
}
