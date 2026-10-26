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
	"sync"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// markLiftPage is how many marked contacts one tick examines per workspace.
// The next tick continues after the last one, so a marked contact that never
// gets an answer cannot hide the ones behind it.
const markLiftPage = 200

// markLiftCursor remembers, per workspace, where the next tick continues. It
// lives in memory because losing it only restarts the scan from the first
// contact. The engine is copied by value, so the engine holds it by pointer.
type markLiftCursor struct {
	mu    sync.Mutex
	after map[ids.UUID]ids.UUID
}

func (c *markLiftCursor) get(ws ids.UUID) ids.UUID {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.after[ws]
}

func (c *markLiftCursor) set(ws, after ids.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.after == nil {
		c.after = map[ids.UUID]ids.UUID{}
	}
	c.after[ws] = after
}

// liftVendorMarkTx clears the contact's automatic mark when the workspace has
// written to the address since the contact was made. A mark a human set is never
// read here.
//
// It locks the contact first and decides under that lock. A caller's earlier
// read of the contact and its address may be stale by now.
func liftVendorMarkTx(
	ctx context.Context, tx pgx.Tx, store *contacts.Store, contactID ids.ContactID, email string,
) error {
	madeAt, marked, err := store.LockAutomaticMarkTx(ctx, tx, contactID, email)
	if err != nil || !marked {
		return err
	}
	corresponds, err := capture.CorrespondsUnderContactTx(ctx, tx, contactID.UUID, email, madeAt)
	if err != nil || !corresponds {
		return err
	}
	_, err = store.LiftAutomaticMarkTx(ctx, tx, contactID)
	return err
}

// LiftVendorMarksWorkspace examines the next page of marked contacts and clears
// each automatic mark whose contact the workspace now writes to, one transaction
// per contact. A contact that fails is skipped and counted.
func (e *CounterpartyVerdictEngine) LiftVendorMarksWorkspace(ctx context.Context) error {
	return e.inWorkspace(ctx, func(wsCtx context.Context, ws ids.UUID) error {
		var page []capture.MarkedContact
		if err := database.WithWorkspaceTx(wsCtx, e.pool, func(tx pgx.Tx) error {
			var err error
			page, err = capture.MarkedContactsWithMailTx(wsCtx, tx, contacts.CaptureVerdictActor,
				e.markLift.get(ws), markLiftPage)
			return err
		}); err != nil {
			return fmt.Errorf("verdict: reading the marked contacts with mail: %w", err)
		}
		var failed []error
		for _, c := range page {
			if err := database.WithWorkspaceTx(wsCtx, e.pool, func(tx pgx.Tx) error {
				return liftVendorMarkTx(wsCtx, tx, e.contacts, ids.From[ids.ContactKind](c.ContactID), c.Email)
			}); err != nil {
				failed = append(failed, fmt.Errorf("verdict: clearing the mark on contact %s: %w", c.ContactID, err))
			}
		}
		var next ids.UUID
		if len(page) == markLiftPage {
			next = page[len(page)-1].ContactID
		}
		e.markLift.set(ws, next)
		if len(failed) > 0 {
			return fmt.Errorf("verdict: %d marks not cleared: %w", len(failed), errors.Join(failed...))
		}
		return nil
	})
}
