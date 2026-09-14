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
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// promoteIfWorkspaceScoped moves an owner-scoped contact to the workspace when a
// workspace-scoped ensure reaches them.
//
// One direction only THROUGH THIS DOOR. A record already the workspace's is
// never narrowed back by a later owner-scoped ensure: the sink runs on every
// message from that sender, so narrowing here would un-publish a contact
// somebody promoted the next time they wrote. The other direction is a
// different decision, taken where a verdict has judged the whole
// correspondence private — see RetractCaptureOnlyContactTx.
//
// The guard is on the ROW's visibility rather than on what the caller believes:
// the UPDATE matches nothing when the row is already workspace, so a second
// ensure over the same contact writes nothing whatever it thought it was doing.
// fieldVisibility names the column in an audit image, so the word a reader
// greps for is the word the table uses.
const fieldVisibility = "visibility"

func promoteIfWorkspaceScoped(ctx context.Context, tx pgx.Tx, id ids.ContactID, ownerScoped bool) error {
	if ownerScoped {
		// Nothing to do, and saying so here saves a statement per captured
		// message from the sink — which is the caller that runs on every one.
		// It is not what makes this safe: shiftVisibilityTx is pinned to the
		// direction its caller names, so an owner-scoped caller reaching it
		// would still narrow nothing.
		return nil
	}
	return shiftVisibilityTx(ctx, tx, id, visibilityOwner, visibilityWorkspace)
}

// shiftVisibilityTx moves one contact between visibilities and lands the write
// shape for it, pinned to the direction the caller names.
//
// One spelling, two callers going opposite ways: the promotion above, and the
// narrowing a personal verdict performs before it archives. They are the same
// write — a disclosure-relevant change of who may read a contact — and two
// copies of it would drift until one of them stopped auditing.
//
// The `from` pin is the concurrency guard, checked through RowsAffected: the
// statement matches only a row still on the visibility the caller believed, so
// a second pass over the same contact moves nothing and reports false rather
// than writing again. It is also what makes the direction safe — passing the
// arguments the other way round matches no row rather than reversing a
// decision somebody else made.
//
// A no-op is not reported, because neither caller has anything to do with the
// difference: the promotion runs on every captured message from a sender and
// the withdrawal runs after its own eligibility check, so "already there" is
// the ordinary case for both and not news to either.
func shiftVisibilityTx(ctx context.Context, tx pgx.Tx, id ids.ContactID, from, to string) error {
	tag, err := tx.Exec(ctx, `
		UPDATE contact SET visibility = $2
		 WHERE id = $1 AND visibility = $3 AND archived_at IS NULL`,
		id, to, from)
	if err != nil {
		return fmt.Errorf("contacts: moving a contact to %s visibility: %w", to, err)
	}
	if tag.RowsAffected() == 0 {
		// Already there, or archived. Both are answers, not faults, and neither
		// is a change to record: an audit row about a write that moved nothing
		// puts a lie in the compliance trail.
		return nil
	}
	// A contact stops being one contact's and becomes everybody's, or stops
	// being everybody's and becomes one contact's again. That is the most
	// disclosure-relevant write this module makes and the one that was leaving
	// no trace. "Which contacts were published or withdrawn, when, and on whose
	// authority" is answered from audit_log or it is not answered at all.
	//
	// The before-image is what the guard above already proved: the row was on
	// `from`, or the UPDATE would have matched nothing.
	auditID, err := storekit.Audit(ctx, tx, "update", entityContact, id.UUID,
		map[string]any{fieldVisibility: from},
		map[string]any{fieldVisibility: to})
	if err != nil {
		return fmt.Errorf("contacts: recording a contact's change of visibility: %w", err)
	}
	return storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventContactUpdated{
		ChangedFields: map[string]any{fieldVisibility: to},
	})
}
