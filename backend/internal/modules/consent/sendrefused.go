// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The refusal a sender reads: which review it opened, what they may do, and
// which recipients stopped the send and why.

import (
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// SendRefusedError is a refusal that left a review behind.
//
// It wraps the original refusal rather than replacing it. Every reader that
// knows apperrors.ErrConsentNotGranted keeps working: the 409 mapping, the
// job-fault table, the tests. It adds a reference to the record of what was
// refused and for whom.
type SendRefusedError struct {
	ReviewID ids.UUID
	Cause    error
	// Actions is what this caller may do about the refusal. It is decided
	// where the refusal is recorded, because the principal is known there.
	// Empty for an agent: both moves are human-only.
	Actions []string
	// Overrulable names the contact and category of each vouch the overrule
	// action needs. Empty unless Actions offers it.
	Overrulable []OverrulableRecipient
	// Refusals is each recipient that stopped the send, with the engine's
	// reason. The sender learns which address stopped and why. The caller
	// named every one of these addresses.
	Refusals []RefusedRecipient
}

func (e *SendRefusedError) Error() string {
	return e.Cause.Error() + " (review " + e.ReviewID.String() + ")"
}

// Unwrap keeps the refusal's own identity reachable, which is what lets the
// existing 409 mapping and every errors.Is on it go on working.
func (e *SendRefusedError) Unwrap() error { return e.Cause }

// SendRefusedError has no FieldFault, though every other typed refusal in this
// module carries one.
//
// A field fault answers 422 and names an input the caller should change. Here
// the input was fine and the engine refused on consent grounds, which is a 409
// every client recognises. The transport prefers a declared fault over a
// wrapped sentinel, so a FieldFault would turn this 409 into a 422.
//
// The reference travels in the message instead, where a human and the MCP
// surface read it.

// BlockingRefusals is the refused recipients whose answer actually stopped the
// send. A deny the rollout mode only records did not stop it, so a sender told
// about it would hunt for a cause that changed nothing.
func BlockingRefusals(set commsauthz.DecisionSet) []RefusedRecipient {
	var out []RefusedRecipient
	for _, d := range set.Denied() {
		if d.WouldRefuse(d.Mode) {
			out = append(out, refusedRecipientOf(d))
		}
	}
	return out
}

// FaultReference implements apperrors.ReferencedFault: the review this refusal
// opened, what this caller may do, and what an overrule must cover, as fields.
// It adds no FieldFault: the refusal stays the 409 every client recognises,
// not a 422 about the caller's input.
func (e *SendRefusedError) FaultReference() map[string]any {
	if e.ReviewID.IsZero() {
		return nil
	}
	reference := map[string]any{
		fieldReviewID: e.ReviewID.String(),
		// Both moves are human-only; an agent gets an empty list and the
		// reference, which a human reading its transcript can pick up.
		"available_actions": e.Actions,
	}
	if len(e.Overrulable) > 0 {
		overrulable := make([]map[string]string, 0, len(e.Overrulable))
		for _, o := range e.Overrulable {
			overrulable = append(overrulable, map[string]string{"contact_id": o.ContactID.String(), "category": o.Category})
		}
		reference["overrulable"] = overrulable
	}
	if len(e.Refusals) > 0 {
		reference["refusals"] = refusalReference(e.Refusals)
	}
	return reference
}

// refusalReference is what a sender may read of each refused recipient. That
// is the address, the reason, the category and whether a reason may lift it. The subject id stays off the wire, because the sender may not
// be able to read that record.
func refusalReference(refusals []RefusedRecipient) []map[string]any {
	out := make([]map[string]any, 0, len(refusals))
	for _, r := range refusals {
		entry := map[string]any{
			"address": r.Address, "reason_code": r.ReasonCode,
			"can_be_overruled": r.CanBeOverruled,
		}
		if r.Category != "" {
			entry["category"] = r.Category
		}
		out = append(out, entry)
	}
	return out
}

// Unreferenced answers the refusal without the reference, which is what decides
// the status. It is the wrapped cause rather than this value, or classification
// would come straight back here.
func (e *SendRefusedError) Unreferenced() error { return e.Cause }
