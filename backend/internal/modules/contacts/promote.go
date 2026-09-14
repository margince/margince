// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// PromoteTrigger is the genuine-engagement vocabulary (features/01
// §6.4): the closed set of events that justify graduating a lead —
// typed, so a misspelled trigger is unrepresentable past the seam.
type PromoteTrigger string

const (
	TriggerInboundReply  PromoteTrigger = "inbound_reply"
	TriggerMeetingBooked PromoteTrigger = "meeting_booked"
	TriggerMeetingHeld   PromoteTrigger = "meeting_held"
	TriggerHumanQualify  PromoteTrigger = "human_qualify"
)

// ParsePromoteTrigger is the store-side membership check; the transport
// enum is the first line, this is the seam's own guard (an MCP or
// internal caller doesn't pass through the HTTP validator).
func ParsePromoteTrigger(raw string) (PromoteTrigger, error) {
	switch tr := PromoteTrigger(raw); tr {
	case TriggerInboundReply, TriggerMeetingBooked, TriggerMeetingHeld, TriggerHumanQualify:
		return tr, nil
	}
	return "", &values.ParseError{
		Field: "trigger", Code: "invalid_promote_trigger",
		Message: "trigger is one of inbound_reply, meeting_booked, meeting_held, human_qualify",
	}
}

// PromoteLeadInput carries the genuine-engagement trigger and the
// evidence pointer the audit row records.
type PromoteLeadInput struct {
	Trigger            string
	EvidenceActivityID *ids.ActivityID
	EvidenceNote       *string
	// Deal, when set, opens a deal in the same transaction (qualify-to-deal).
	Deal *QualifyDealInput
}

// AlreadyPromotedError maps to 409: promotion happened once; the pointer
// to its outcome lives on the lead row.
type AlreadyPromotedError struct{ ContactID ids.ContactID }

func (e *AlreadyPromotedError) Error() string { return "lead is already promoted" }

// PromoteNeedsIdentityError maps to 422: a lead nothing names cannot become a
// contact worth having.
type PromoteNeedsIdentityError struct{}

// Error says what is missing rather than which field is absent. A full_name
// that is present and empty is refused here, and telling that caller the lead
// "has no full_name" contradicts the record they can see.
func (e *PromoteNeedsIdentityError) Error() string {
	return "lead has no name and no email to be named by; enrich it before promoting"
}

// MessageFault names the condition and no field: the remedy is EITHER of two
// inputs on the lead record, and `lead` is the record itself, not a request
// field a caller can set. Naming one of the pair would be wrong half the time,
// and naming the record would be wrong always.
func (e *PromoteNeedsIdentityError) MessageFault() (code, message string) {
	return "identity_required", e.Error()
}

// PromoteLead graduates a lead into the clean core (features/01 §6.4,
// ADR-0008): if the lead's email matches a live contact it MERGES into
// that contact — never a duplicate — else it creates one, carrying the
// lead's provenance, owner and identity. The lead is marked
// status=promoted, stamped with the outcome pointer, and archived off the
// lead list, all in one transaction with ONE audit row (action=promote on
// the lead, recording trigger + evidence + the resulting contact) and the
// first-class lead.promoted event alongside the contact.* it caused.
func (s *Store) PromoteLead(ctx context.Context, id ids.LeadID, in PromoteLeadInput) (crmcontracts.Contact, bool, error) {
	out, err := s.QualifyLead(ctx, id, in)
	return out.Contact, out.Merged, err
}

// QualifyLead is PromoteLead with the whole outcome: the contact, whether it
// merged, and the deal opened alongside when the call asked for one.
func (s *Store) QualifyLead(ctx context.Context, id ids.LeadID, in PromoteLeadInput) (PromoteOutcome, error) {
	// Promotion mutates the lead AND writes the contact side, so it needs
	// both grants — a rep who may work leads but not create contacts
	// cannot mint contacts through this door.
	if err := auth.Require(ctx, "lead", principal.ActionUpdate); err != nil {
		return PromoteOutcome{}, err
	}
	if err := auth.Require(ctx, "contact", principal.ActionCreate); err != nil {
		return PromoteOutcome{}, err
	}
	if _, err := ParsePromoteTrigger(in.Trigger); err != nil {
		return PromoteOutcome{}, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return PromoteOutcome{}, err
	}
	active, err := s.activeColumns(ctx, "contact")
	if err != nil {
		return PromoteOutcome{}, err
	}

	var out PromoteOutcome
	err = s.tx(ctx, func(tx pgx.Tx) error {
		// The lead lock comes BEFORE the promotability read: two
		// concurrent promotes of one lead must serialize here, so the
		// loser re-reads status=promoted and answers 409 instead of
		// minting a second contact. IncludeArchived keeps the re-promote
		// 409-with-pointer diagnostic reachable.
		if _, err := storekit.LockRow(ctx, tx, "lead", id.UUID, storekit.IncludeArchived); err != nil {
			return err
		}
		lead, err := promotableLead(ctx, tx, id, in)
		if err != nil {
			return err
		}

		contactID, mergeFields, err := s.promoteTarget(ctx, tx, lead, by, &out.Merged)
		if err != nil {
			return err
		}
		if err := carryLeadConsent(ctx, tx, id, contactID, by); err != nil {
			return fmt.Errorf("carry lead consent: %w", err)
		}
		// And the lead's STOPS, which live in consent's table rather than ours
		// — see stopcarry.go. A lead who asked us to stop and was then promoted
		// would otherwise arrive as a contact carrying no stop at all, which is
		// the same silent resumption the contact merge produced.
		if err := s.carryStopsTx(ctx, tx,
			commsauthz.LeadStopSubject(id),
			commsauthz.ContactStopSubject(contactID)); err != nil {
			return fmt.Errorf("carry the lead's stops: %w", err)
		}
		carried, err := carryLeadActivities(ctx, tx, id, contactID)
		if err != nil {
			return err
		}

		out.DealID, err = s.openQualifiedDeal(ctx, tx, id, lead, contactID, in.Deal)
		if err != nil {
			return err
		}
		out.Contact, err = finalizeLeadPromotion(ctx, tx, id, in, lead, contactID, out.Merged, mergeFields, active, out.DealID, carried)
		return err
	})
	return out, err
}

// carryLeadConsent carries the promoted lead's consent onto the contact it
// became (data-model §7: subject re-pointed, proof preserved), inside the same
// transaction — contacts's sanctioned cross-aggregate SQL ownership. The rules
// are consentcarry.go's; promotion is the carry that does NOT re-home the
// proof rows, because the lead-scoped events are the evidence that the consent
// predates the promotion.
func carryLeadConsent(ctx context.Context, tx pgx.Tx, leadID ids.LeadID, contactID ids.ContactID, by string) error {
	return carryConsent(ctx, tx, consentCarryLeadPromotion, leadID.UUID, contactID.UUID, by)
}

// carryLeadActivities moves the lead's timeline onto the contact it became
// (LEADS-FORM-5 step 3: "the lead's history, provenance, and activities carry
// over with nothing orphaned").
//
// The link row is CONVERTED, not merely repointed: activity_link_shape admits
// exactly one target per row, so a row that kept its lead_id while gaining a
// contact_id violates the CHECK. entity_type and both id columns move together.
//
// The conflict arm is the case where the activity was ALREADY linked to that
// contact — a lead whose reply was captured against a contact we already knew,
// which is exactly the merge path. uq_activity_link would reject the duplicate,
// so the row is dropped instead of converted; the contact keeps the link it had.
// It ANSWERS what it moved. The activity ids ride the lead.promoted event so a
// consumer can act on the tasks this promotion carried rather than on every
// task the contact happens to hold — a distinction that does not exist for a
// freshly created contact and is the whole question for a merge.
//
// The re-pointed rows only. An activity the survivor already carried has its
// lead link deleted above rather than moved: the promotion did not bring it,
// and naming it would hand a consumer work that was already there.
func carryLeadActivities(
	ctx context.Context, tx pgx.Tx, leadID ids.LeadID, contactID ids.ContactID,
) ([]ids.UUID, error) {
	if _, err := tx.Exec(ctx, `
		DELETE FROM activity_link a
		WHERE a.lead_id = $1 AND EXISTS (
		  SELECT 1 FROM activity_link b
		  WHERE b.activity_id = a.activity_id
		    AND b.entity_type = 'contact' AND b.contact_id = $2)`,
		leadID, contactID); err != nil {
		return nil, fmt.Errorf("drop already-linked lead activities: %w", err)
	}
	rows, err := tx.Query(ctx, `
		UPDATE activity_link
		SET entity_type = 'contact', contact_id = $2, lead_id = NULL
		WHERE lead_id = $1
		RETURNING activity_id`,
		leadID, contactID)
	if err != nil {
		return nil, fmt.Errorf("carry lead activities: %w", err)
	}
	defer rows.Close()
	carried, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("carry lead activities: %w", err)
	}
	return carried, nil
}

// finalizeLeadPromotion retires the lead and lands the write shape for the
// whole promotion: the status flip, the ONE audit row (action=promote,
// recording trigger + evidence + the resulting contact), and the paired
// lead.promoted + contact.* events — all inside the caller's transaction,
// still under the lead row lock taken by PromoteLead.
func finalizeLeadPromotion(ctx context.Context, tx pgx.Tx, id ids.LeadID, in PromoteLeadInput, lead crmcontracts.Lead, contactID ids.ContactID, merged bool, mergeFields map[string]any, active []fieldcatalog.Column, dealID *ids.UUID, carried []ids.UUID) (crmcontracts.Contact, error) {
	now := time.Now().UTC()
	setBy, err := statusSetByFor(ctx)
	if err != nil {
		return crmcontracts.Contact{}, err
	}
	tag, err := tx.Exec(ctx,
		`UPDATE lead SET status = 'promoted', status_set_by = $4, promoted_contact_id = $2, promoted_at = $3, archived_at = $3,
		        qualified_deal_id = $5, `+firstResponseSet+`
		 WHERE id = $1 AND archived_at IS NULL`,
		id, contactID, now, setBy, dealID)
	if err != nil {
		return crmcontracts.Contact{}, fmt.Errorf("mark lead promoted: %w", err)
	}
	if tag.RowsAffected() != 1 {
		// Under the row lock only this transaction can retire the
		// lead; a zero-row update means the guards above are broken.
		// Failing loudly keeps the phantom contact and its events out.
		return crmcontracts.Contact{}, apperrors.ErrConflict
	}

	outcome := "created"
	if merged {
		outcome = "merged"
	}
	after := map[string]any{
		leadStatusColumn: "promoted", fieldKeyPromotedContact: contactID,
		"trigger": in.Trigger, "dedupe_outcome": outcome,
	}
	if in.EvidenceActivityID != nil {
		after["evidence_activity_id"] = *in.EvidenceActivityID
	}
	if in.EvidenceNote != nil {
		after["evidence_note"] = *in.EvidenceNote
	}
	if dealID != nil {
		after["qualified_deal_id"] = *dealID
	}
	auditID, err := storekit.Audit(ctx, tx, "promote", "lead", id.UUID,
		map[string]any{leadStatusColumn: lead.Status}, after)
	if err != nil {
		return crmcontracts.Contact{}, fmt.Errorf("audit lead promote: %w", err)
	}

	contact, err := readContact(ctx, tx, contactID, storekit.LiveOnly, active)
	if err != nil {
		return crmcontracts.Contact{}, fmt.Errorf("read promoted contact: %w", err)
	}

	// lead.promoted is the first-class verb (events.md §5.5) — the
	// moment the context graph adds the node; never a lead.updated.
	if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, leadPromotedPayload(contactID, outcome, in.Trigger, in.EvidenceActivityID, carried)); err != nil {
		return crmcontracts.Contact{}, fmt.Errorf("emit lead.promoted: %w", err)
	}
	// A fill-only merge that changed nothing has no contact.updated to emit —
	// a changed_fields note with no fields would be a false claim, so skip it
	// (lead.promoted above still records the promotion). A create always emits
	// contact.created.
	if contactPayload := promotedContactPayload(contact, merged, mergeFields); contactPayload != nil {
		if err := storekit.EmitEvent(ctx, tx, auditID, contactID.UUID, contactPayload); err != nil {
			return crmcontracts.Contact{}, fmt.Errorf("emit %s: %w", contactPayload.EventType(), err)
		}
	}
	return contact, nil
}

// promotableLead loads the lead and enforces every promotion guard:
// visibility, the once-only rule, live status, minimal identity, and
// in-scope evidence. Archived leads resolve here so a re-promote answers
// 409 with the outcome pointer instead of a misleading 404; a
// disqualified (archived, unpromoted) lead stays 404 like any archived
// row.
func promotableLead(ctx context.Context, tx pgx.Tx, id ids.LeadID, in PromoteLeadInput) (crmcontracts.Lead, error) {
	if err := auth.EnsureWritable(ctx, tx, "lead", id.UUID); err != nil {
		return crmcontracts.Lead{}, err
	}
	// An internal read that builds the promoted contact; its result is not
	// returned to the wire as a lead, so it carries no custom columns (nil).
	lead, err := readLead(ctx, tx, id, storekit.IncludeArchived, nil)
	if err != nil {
		return crmcontracts.Lead{}, fmt.Errorf("read lead before promote: %w", err)
	}
	if lead.Status == crmcontracts.LeadStatusPromoted {
		e := &AlreadyPromotedError{}
		if lead.PromotedContactId != nil {
			e.ContactID = ids.From[ids.ContactKind](ids.UUID(*lead.PromotedContactId))
		}
		return crmcontracts.Lead{}, e
	}
	if lead.ArchivedAt != nil {
		return crmcontracts.Lead{}, apperrors.ErrNotFound
	}
	// Read through the same derivation the ladder matches on, not through a
	// nil check: a full_name that is present and empty passes `!= nil` and
	// names nobody, so such a lead promotes into a contact with no name at all.
	if leadIdentityName(lead) == "" {
		return crmcontracts.Lead{}, &PromoteNeedsIdentityError{}
	}
	if in.EvidenceActivityID != nil {
		// The evidence must be a real, in-scope activity — a promotion
		// justified by a record the promoter cannot see is not evidence.
		if err := auth.EnsureActivityVisible(ctx, tx, in.EvidenceActivityID.UUID); err != nil {
			return crmcontracts.Lead{}, err
		}
	}
	return lead, nil
}

// promoteTarget resolves where the lead lands: the §1.3 dedupe path — a
// live contact already holding the lead's email is merged into, anything
// else creates. Returns the contact id, sets *merged, and (on the merge path)
// the fields the merge actually applied so the contact.updated event reports
// the true delta (nil on the create path).
// It runs the full PO-F-1 ladder rather than the single email probe it used
// to: a lead whose address nobody holds may still name a contact already in the
// workspace under a spelling of the same name, and promoting it silently
// minted the twin. The exact-email answer is unchanged; a near-match still
// creates (DEDUPE_FUZZY_AUTOMERGE is pinned never) and now leaves the pair on
// the review queue instead of nothing at all.
func (s *Store) promoteTarget(ctx context.Context, tx pgx.Tx, lead crmcontracts.Lead, by string, merged *bool) (ids.ContactID, map[string]any, error) {
	candidate, err := s.leadContactCandidate(ctx, tx, lead)
	if err != nil {
		return ids.ContactID{}, nil, err
	}
	// The contact is created under the SAME name the ladder matched on, so a
	// lead that resolved as a new contact is stored as the candidate that was
	// compared, not as a second reading of the lead.
	name := candidate.FullName
	match, err := DedupeContact(ctx, tx, candidate)
	if err != nil {
		return ids.ContactID{}, nil, err
	}
	if match.Decision == DecisionExactCollision {
		// Merging CHANGES the matched contact — the lead's fields land on it —
		// and returns it, so the probe asks for write authority and the refusal
		// still discloses nothing: a match the promoter cannot change answers a
		// bare conflict, not the record, exactly as one they cannot see does.
		writable, verr := auth.WritableBy(ctx, tx, "contact", match.ContactID.UUID)
		if verr != nil {
			return ids.ContactID{}, nil, verr
		}
		if !writable {
			return ids.ContactID{}, nil, apperrors.ErrConflict
		}
		*merged = true
		mergeFields, merr := s.mergeLeadIntoContact(ctx, tx, lead, match.ContactID)
		return match.ContactID, mergeFields, merr
	}

	leadID := ids.UUID(lead.Id)
	var leadEmails []ContactEmailInput
	if lead.Email != nil {
		leadEmails = []ContactEmailInput{{
			Email: string(*lead.Email), EmailType: emailTypeWork, IsPrimary: true, Position: 1,
		}}
	}
	id, err := createContact(ctx, tx, match, ContactSpec{
		FullName: name,
		// A promoted lead was acquired however the LEAD was, and a lead
		// carries no acquisition record yet. Claiming one here would invent
		// the fact; unknown_legacy says plainly that nobody has established
		// it, which is what the contact's file should show until a lead
		// carries its own.
		Acquisition:         Acquisition{Kind: AcquiredUnknownLegacy},
		Title:               lead.Title,
		OwnerID:             ownerFromUUID(uuidPtrToIDs(lead.OwnerId)),
		ConvertedFromLeadID: &leadID,
		Emails:              leadEmails,
		Source:              lead.Source,
		CapturedBy:          by,
	})
	if err != nil {
		return ids.ContactID{}, nil, err
	}
	if err := match.recordIfReview(ctx, tx, id, name, lead.Source, by); err != nil {
		return ids.ContactID{}, nil, err
	}
	return id, nil, nil
}

// mergeLeadIntoContact is the non-lossy merge half: the contact gains the
// origin pointer and any identity the lead has that the contact lacks
// (fill-only — a promotion never overwrites human-curated contact data).
// It returns the fields the merge actually applied (the patch's after map),
// so contact.updated.changed_fields reports the real delta — not a fixed
// converted_from_lead_id that lies when the field was already set, and never
// omitting a title it just filled. A no-op merge returns a nil map.
func (s *Store) mergeLeadIntoContact(ctx context.Context, tx pgx.Tx, lead crmcontracts.Lead, contactID ids.ContactID) (map[string]any, error) {
	// BEFORE the contact row lock. Recording a stop takes consent's lock on the
	// contact and then reads the contact row; taking the row first here and
	// consent's lock later (in carryStopsTx, once the promotion knows this is
	// the surviving contact) inverts the order and deadlocks. See
	// StopCarrier.LockStopsTx.
	if err := lockStopsOrSkip(ctx, tx, s.stopCarrier,
		commsauthz.ContactStopSubject(contactID)); err != nil {
		return nil, err
	}
	lock, err := storekit.LockRow(ctx, tx, "contact", contactID.UUID, storekit.LiveOnly)
	if err != nil {
		return nil, fmt.Errorf("lock merge-target contact: %w", err)
	}
	// A fill-only decision read, never the wire — core columns suffice.
	current, err := readContact(ctx, tx, contactID, storekit.LiveOnly, nil)
	if err != nil {
		return nil, fmt.Errorf("read merge-target contact: %w", err)
	}
	p := storekit.NewPatch()
	if current.ConvertedFromLeadId == nil {
		p.Set("converted_from_lead_id", nil, ids.UUID(lead.Id))
	}
	if current.Title == nil && lead.Title != nil {
		p.Set("title", nil, *lead.Title)
	}
	if p.Empty() {
		// A no-op fill-only merge: no columns changed. Return an empty (not
		// nil) map so the caller reads "no delta" via len == 0 and skips the
		// contact.updated event, without a nil-nil return.
		return map[string]any{}, nil
	}
	if err := p.ApplyLocked(ctx, tx, lock); err != nil {
		return nil, err
	}
	return p.After(), nil
}

// uuidPtrToIDs converts the contract's optional UUID back to the kernel
// type for SQL args.
func uuidPtrToIDs(u *openapi_types.UUID) *ids.UUID {
	if u == nil {
		return nil
	}
	converted := ids.UUID(*u)
	return &converted
}
