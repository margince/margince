// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Publishing a contact once the address answers the owner who wrote to it.
//
// Two callers share one transaction body. The capture sink runs it the moment
// a reply lands, so the contact opens with the reply; the verdict worker's pass
// runs it over every contact still owed it, so a promotion the live hook lost
// to a fault is made on the next tick rather than waiting for another message.
// Both re-read the evidence inside the transaction that publishes, so neither
// can publish on a belief the other formed.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// answeredSweepContacts bounds one workspace's reconciling pass. A contact is
// only ever listed while it is owed a promotion, so the rest follow next tick.
const answeredSweepContacts = 200

// publishAnsweredTx publishes the contact when capture.OwnerWasAnsweredTx
// holds for its owner and this address, and contacts.PromoteOnReplyTx finds it
// narrowed only for want of an answer. The ledger's withholding is cleared with
// it: the mail-side readers ask the ledger, not the contact.
func publishAnsweredTx(
	ctx context.Context, tx pgx.Tx, store *contacts.Store, contactID ids.ContactID, owner ids.UUID, email string,
) error {
	answered, err := capture.OwnerWasAnsweredTx(ctx, tx, owner, email)
	if err != nil || !answered {
		return err
	}
	moved, err := store.PromoteOnReplyTx(ctx, tx, contactID, owner)
	if err != nil || !moved {
		return err
	}
	// Kept to its owner, the contact took no lead when the mail was captured.
	// Published by the reply, it takes one now.
	if err := store.PromoteHeldLeadsOnReplyTx(ctx, tx, contactID); err != nil {
		return err
	}
	return capture.ClearWithheldFromWorkspaceTx(ctx, tx, email)
}

// PublishAnsweredContactsWorkspace publishes every contact in the workspace
// whose owner has been answered, one transaction per contact.
func (e *CounterpartyVerdictEngine) PublishAnsweredContactsWorkspace(ctx context.Context) error {
	return e.inWorkspace(ctx, func(wsCtx context.Context, _ ids.UUID) error {
		var owed []capture.AnsweredContact
		if err := database.WithWorkspaceTx(wsCtx, e.pool, func(tx pgx.Tx) error {
			var err error
			owed, err = capture.AnsweredContactsTx(wsCtx, tx,
				string(contacts.NarrowedOutboundNoAnswer), answeredSweepContacts)
			return err
		}); err != nil {
			return fmt.Errorf("verdict: reading the contacts owed publication on an answer: %w", err)
		}
		// One contact that fails must not starve the ones behind it on every tick,
		// so the pass carries on and reports every failure together.
		var failed []error
		for _, c := range owed {
			if err := database.WithWorkspaceTx(wsCtx, e.pool, func(tx pgx.Tx) error {
				return publishAnsweredTx(wsCtx, tx, e.contacts,
					ids.From[ids.ContactKind](c.ContactID), c.OwnerID, c.Email)
			}); err != nil {
				failed = append(failed, fmt.Errorf("verdict: publishing answered contact %s: %w", c.ContactID, err))
			}
		}
		return errors.Join(failed...)
	})
}
