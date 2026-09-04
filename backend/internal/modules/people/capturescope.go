// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// When a captured contact stops being the mailbox owner's and becomes the
// workspace's.
//
// Capture mints a person from a message nothing has judged yet, so the record
// starts owner-scoped: a mailbox with a year of history names a lawyer, a
// doctor and a school, and one email is not a reason to publish any of them.
// Something judging the sender a business counterparty is the decision, and the
// verdict path expresses it by ensuring the same address without asking for
// owner scope.
//
// Distinct from promote.go beside it, which is the LEAD promotion surface: that
// one converts a lead into a person, this one widens who may read a person who
// already exists.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// promoteIfWorkspaceScoped moves an owner-scoped person to the workspace when a
// workspace-scoped ensure reaches them.
//
// One direction only. A record already the workspace's is never narrowed back
// by a later owner-scoped ensure: the sink runs on every message from that
// sender, so narrowing here would un-publish a contact somebody promoted the
// next time they wrote.
//
// The guard is on the ROW's visibility rather than on what the caller believes:
// the UPDATE matches nothing when the row is already workspace, so a second
// ensure over the same person writes nothing whatever it thought it was doing.
// fieldVisibility names the column in an audit image, so the word a reader
// greps for is the word the table uses.
const fieldVisibility = "visibility"

func promoteIfWorkspaceScoped(ctx context.Context, tx pgx.Tx, id ids.PersonID, ownerScoped bool) error {
	if ownerScoped {
		// Nothing to do, and saying so here saves a statement per captured
		// message from the sink — which is the caller that runs on every one.
		// It is not what makes this safe: the UPDATE below writes owner to
		// workspace and has no spelling that goes the other way, so an
		// owner-scoped caller reaching it would still narrow nothing.
		return nil
	}
	// The visibility pin is the concurrency guard, checked through RowsAffected:
	// the statement matches only a row still owner-scoped, so a second pass over
	// the same person moves nothing and reports zero rather than writing again.
	tag, err := tx.Exec(ctx, `
		UPDATE person SET visibility = $2
		 WHERE id = $1 AND visibility = $3 AND archived_at IS NULL`,
		id, visibilityWorkspace, visibilityOwner)
	if err != nil {
		return fmt.Errorf("people: promoting a judged counterparty to the workspace: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Already the workspace's, or archived. Both are answers, not faults,
		// and neither is a change to record: an audit row about a write that
		// moved nothing puts a lie in the compliance trail.
		return nil
	}
	// A contact stops being one person's and becomes everybody's, which is the
	// most disclosure-relevant write this module makes and the one that was
	// leaving no trace. "Which contacts were published, when, and on whose
	// authority" is answered from audit_log or it is not answered at all.
	//
	// The before-image is what the guard above already proved: the row was
	// owner-scoped, or the UPDATE would have matched nothing.
	auditID, err := storekit.Audit(ctx, tx, "update", entityPerson, id.UUID,
		map[string]any{fieldVisibility: visibilityOwner},
		map[string]any{fieldVisibility: visibilityWorkspace})
	if err != nil {
		return fmt.Errorf("people: recording a captured contact's promotion: %w", err)
	}
	return storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventPersonUpdated{
		ChangedFields: map[string]any{fieldVisibility: visibilityWorkspace},
	})
}
