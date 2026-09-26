// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// When a captured contact stops being the mailbox owner's and becomes the
// workspace's.
//
// Capture mints a contact from a message nothing has judged yet, so the record
// starts owner-scoped: a mailbox with a year of history names a lawyer, a
// doctor and a school, and one email is not a reason to publish any of them.
// Something judging the sender a business counterparty is the decision, and the
// verdict path expresses it by ensuring the same address without asking for
// owner scope.
//
// Distinct from promote.go beside it, which is the LEAD promotion surface: that
// one converts a lead into a contact, this one widens who may read a contact who
// already exists.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// promoteIfWorkspaceScoped moves an owner-scoped contact to the workspace when a
// workspace-scoped ensure reaches them — a verdict or a human accept judging
// the sender a business counterparty.
//
// One direction only THROUGH THIS DOOR. A record already the workspace's is
// never narrowed back by a later owner-scoped ensure: the sink runs on every
// message from that sender, so narrowing here would un-publish a contact
// somebody promoted the next time they wrote. The other direction is a
// different decision, taken where a verdict has judged the whole
// correspondence private — see RetractCaptureOnlyContactTx.
//
// A judgment ends awaiting_verdict and outbound_no_answer. It does not end a
// hold, an advisor verdict, a human's choice or a legacy row with no recorded
// reason: a later judgment about the address answers none of those.
//
// The guard is on the ROW's visibility rather than on what the caller believes:
// the UPDATE matches nothing when the row is already workspace, so a second
// ensure over the same contact writes nothing whatever it thought it was doing.
func promoteIfWorkspaceScoped(ctx context.Context, tx pgx.Tx, id ids.ContactID, ownerScoped bool) error {
	if ownerScoped {
		// Nothing to do, and saying so here saves a statement per captured
		// message from the sink — which is the caller that runs on every one.
		return nil
	}
	_, err := widenTx(ctx, tx, id, widening{lifts: []NarrowingReason{NarrowedAwaitingVerdict, NarrowedOutboundNoAnswer}})
	return err
}

// PromoteOnReplyTx moves an owner-scoped contact to the workspace because the
// address has answered mail its owner sent, and reports whether it moved.
//
// Only outbound_no_answer ends this way. Every other reason, and a row that
// records none, stays its owner's: the column names what is safe to lift, so
// a narrowing added later refuses until somebody decides otherwise.
//
// owner pins the row to the mailbox the reply arrived in. Capture privacy is
// the importing seat's, and a reply one seat captured is not authority to
// publish a contact another seat is keeping.
func (s *Store) PromoteOnReplyTx(ctx context.Context, tx pgx.Tx, id ids.ContactID, owner ids.UUID) (bool, error) {
	if err := auth.Require(ctx, entityContact, principal.ActionUpdate); err != nil {
		return false, err
	}
	return widenTx(ctx, tx, id, widening{lifts: []NarrowingReason{NarrowedOutboundNoAnswer}, owner: &owner})
}

// fieldVisibility names the column in an audit image, so the word a reader
// greps for is the word the table uses.
const fieldVisibility = "visibility"

// widening says which narrowings one widening may end.
type widening struct {
	// lifts names the reasons it ends. A NULL reason is never among them.
	lifts []NarrowingReason
	// liftsAny ends every narrowing. Only the owner's own publish sets it: the
	// reasons protect the owner's choice from machines, not from the owner.
	liftsAny bool
	// owner, when set, matches only a row that seat owns.
	owner *ids.UUID
}

// widenTx moves one owner-scoped contact to the workspace, if w may end its
// narrowing, and reports whether it moved.
//
// The reason is cleared in the same statement: it describes a narrowed row, and
// the CHECK contact_narrowing_reason_only_when_owner refuses one left behind.
//
// The pins are the concurrency guard, read through RowsAffected: a second pass
// over the same contact matches nothing and writes nothing again.
func widenTx(ctx context.Context, tx pgx.Tx, id ids.ContactID, w widening) (bool, error) {
	lifts := make([]string, 0, len(w.lifts))
	for _, reason := range w.lifts {
		lifts = append(lifts, string(reason))
	}
	tag, err := tx.Exec(ctx, `
		UPDATE contact SET visibility = $2, narrowing_reason = NULL
		 WHERE id = $1 AND visibility = $3 AND archived_at IS NULL
		   AND (narrowing_reason = ANY($4::text[]) OR $5)
		   AND ($6::uuid IS NULL OR owner_id = $6)`,
		id, visibilityWorkspace, visibilityOwner, lifts, w.liftsAny, w.owner)
	if err != nil {
		return false, fmt.Errorf("contacts: moving a contact to the workspace: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Already there, archived, or kept for a reason this widening does not
		// end. None is a change to record.
		return false, nil
	}
	return true, recordVisibilityShift(ctx, tx, id,
		map[string]any{fieldVisibility: visibilityOwner},
		map[string]any{fieldVisibility: visibilityWorkspace})
}

// narrowTx moves one workspace contact to its owner alone, recording why.
//
// Pinned to `workspace` for the reason widenTx is pinned to `owner`: a row
// already narrowed keeps the reason that narrowed it first.
func narrowTx(ctx context.Context, tx pgx.Tx, id ids.ContactID, reason NarrowingReason) error {
	tag, err := tx.Exec(ctx, `
		UPDATE contact SET visibility = $2, narrowing_reason = $4
		 WHERE id = $1 AND visibility = $3 AND archived_at IS NULL`,
		id, visibilityOwner, visibilityWorkspace, reason)
	if err != nil {
		return fmt.Errorf("contacts: moving a contact to its owner alone: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	return recordVisibilityShift(ctx, tx, id,
		map[string]any{fieldVisibility: visibilityWorkspace},
		map[string]any{fieldVisibility: visibilityOwner, fieldNarrowingReason: reason})
}

// recordVisibilityShift lands the write shape for a change of who may read a
// contact — the most disclosure-relevant write this module makes. "Which
// contacts were published or withdrawn, when, and on whose authority" is
// answered from audit_log or it is not answered at all.
func recordVisibilityShift(ctx context.Context, tx pgx.Tx, id ids.ContactID, before, after map[string]any) error {
	auditID, err := storekit.Audit(ctx, tx, "update", entityContact, id.UUID, before, after)
	if err != nil {
		return fmt.Errorf("contacts: recording a contact's change of visibility: %w", err)
	}
	return storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventContactUpdated{
		ChangedFields: after,
	})
}
