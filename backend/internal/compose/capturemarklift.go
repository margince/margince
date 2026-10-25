// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Taking the vendor mark off once the workspace corresponds with the contact.
//
// The sender check marks a pitching sender only while nobody here has answered.
// Two callers end that mark with one transaction body. The capture sink runs it
// when later mail from a known contact lands.
// The verdict worker's pass runs it over every marked contact with mail. So an
// answer sent from the governed send path, or lost to a fault, is seen next tick.

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

// markLiftPage bounds one read of marked contacts. The pass pages until a short
// page, because a marked contact the workspace never answered stays listed.
const markLiftPage = 200

// liftVendorMarkTx clears the contact's automatic mark when the workspace
// corresponds with the address. A mark a human set is never read here.
func liftVendorMarkTx(
	ctx context.Context, tx pgx.Tx, store *contacts.Store, contactID ids.ContactID, email string,
) error {
	marked, err := store.AutomaticallyMarkedTx(ctx, tx, contactID)
	if err != nil || !marked {
		return err
	}
	corresponds, err := capture.WorkspaceCorrespondsTx(ctx, tx, email)
	if err != nil || !corresponds {
		return err
	}
	_, err = store.LiftAutomaticMarkTx(ctx, tx, contactID)
	return err
}

// LiftVendorMarksWorkspace clears every automatic mark whose contact the
// workspace now corresponds with, one transaction per contact. A contact that
// fails is skipped and counted, so a failing one never hides the ones behind it.
func (e *CounterpartyVerdictEngine) LiftVendorMarksWorkspace(ctx context.Context) error {
	return e.inWorkspace(ctx, func(wsCtx context.Context, _ ids.UUID) error {
		var failed []error
		var after ids.UUID
		for {
			var page []capture.MarkedContact
			if err := database.WithWorkspaceTx(wsCtx, e.pool, func(tx pgx.Tx) error {
				var err error
				page, err = capture.MarkedContactsWithMailTx(wsCtx, tx, contacts.CaptureVerdictActor, after, markLiftPage)
				return err
			}); err != nil {
				return fmt.Errorf("verdict: reading the marked contacts with mail: %w", err)
			}
			for _, c := range page {
				if err := database.WithWorkspaceTx(wsCtx, e.pool, func(tx pgx.Tx) error {
					return liftVendorMarkTx(wsCtx, tx, e.contacts, ids.From[ids.ContactKind](c.ContactID), c.Email)
				}); err != nil {
					failed = append(failed, fmt.Errorf("verdict: clearing the mark on contact %s: %w", c.ContactID, err))
				}
			}
			if len(page) < markLiftPage {
				break
			}
			after = page[len(page)-1].ContactID
		}
		if len(failed) > 0 {
			return fmt.Errorf("verdict: %d marks not cleared: %w", len(failed), errors.Join(failed...))
		}
		return nil
	})
}
