// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package ownaddress archives the contact capture minted for an address its
// owner then claims as their own. It refiles that contact's mail as the
// member's own correspondence. Contacts and activities are sibling modules, so
// the two halves meet here, in the claim's own transaction.
package ownaddress

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// RefileLimit is the most messages one claim moves. A contact with more is left
// as it is, so the claim request stays short.
const RefileLimit = 500

// Restore puts one archived contact back, in the caller's transaction.
type Restore func(ctx context.Context, tx pgx.Tx, id ids.ContactID) error

// NewStore is the owner-identity store with the claim hooks bound. The claim
// archives and refiles, and its withdrawal puts both back through restore.
func NewStore(db *database.DB, store *contacts.Store, restore Restore) *capture.OwnerIdentityStore {
	log := slog.Default()
	return capture.NewOwnerIdentityStore(db).
		WithAddressClaimed(Retire(store, log)).
		WithAddressWithdrawn(release(store, restore, log))
}

// release puts back what the claim's retire did. A contact whose address has
// been taken since stays archived, and its mail stays as it is. A half undo
// would link mail to an archived record.
func release(store *contacts.Store, restore Restore, log *slog.Logger) capture.AddressWithdrawnHook {
	return func(ctx context.Context, tx pgx.Tx, claim, _ ids.UUID) error {
		contact, found, err := store.ContactArchivedByClaimTx(ctx, tx, claim)
		if err != nil || !found {
			return err
		}
		sp, err := tx.Begin(ctx)
		if err != nil {
			return fmt.Errorf("ownaddress: opening a savepoint: %w", err)
		}
		if err := restore(ctx, sp, contact); err != nil {
			log.InfoContext(ctx, "own address withdrawn: left the archived contact as it is",
				"contact", contact.String(), "error", err.Error())
			return sp.Rollback(ctx)
		}
		if _, err := activities.RestoreOwnAddressMailTx(ctx, sp, claim, RefileLimit); err != nil {
			return errors.Join(err, sp.Rollback(ctx))
		}
		return sp.Commit(ctx)
	}
}

// Retire is the claim hook. It withdraws the contact capture made for the
// claimed address when the claimant minted it and holds it alone, and refiles
// its mail. Every other contact on the address is left as it is.
func Retire(store *contacts.Store, log *slog.Logger) capture.AddressClaimHook {
	return func(ctx context.Context, tx pgx.Tx, claim capture.ClaimedAddress) error {
		if !claim.Proven || claim.HeldByAnotherSeat {
			return nil
		}
		holders, err := store.CaptureOnlyHoldersOfAddressTx(ctx, tx, claim.Address)
		if err != nil {
			return err
		}
		for _, holder := range holders {
			if holder.OwnerID != claim.User {
				log.InfoContext(ctx, "own address claimed: left the contact a colleague owns",
					"contact", holder.ContactID.String())
				continue
			}
			retired, reason, err := retireOne(ctx, tx, store, holder, claim.ID)
			if err != nil {
				return err
			}
			if !retired {
				log.InfoContext(ctx, "own address claimed: left a contact that is not the member's alone",
					"contact", holder.ContactID.String(), "reason", reason)
			}
		}
		return nil
	}
}

// retireOne archives one contact and refiles its mail inside a savepoint. A
// contact that proves not to be the member's alone rolls the savepoint back,
// so nothing of it is archived or moved.
func retireOne(ctx context.Context, tx pgx.Tx, store *contacts.Store, holder contacts.CaptureOnlyHolder, claim ids.UUID) (retired bool, reason string, err error) {
	sp, err := tx.Begin(ctx)
	if err != nil {
		return false, "", fmt.Errorf("ownaddress: opening a savepoint: %w", err)
	}
	reason, err = withdraw(ctx, sp, store, holder, claim)
	if err != nil || reason != "" {
		return false, reason, errors.Join(err, sp.Rollback(ctx))
	}
	return true, "", sp.Commit(ctx)
}

// withdraw is the work inside the savepoint. It answers why the contact was
// left. An empty reason means it archived the contact, ended the notice duties
// it owed and refiled its mail.
func withdraw(ctx context.Context, tx pgx.Tx, store *contacts.Store, holder contacts.CaptureOnlyHolder, claim ids.UUID) (string, error) {
	archived, err := store.RetractOwnAddressContactTx(ctx, tx, holder.ContactID, holder.OwnerID, claim)
	if err != nil || !archived {
		return "not the member's alone", err
	}
	if _, err := consent.SettleWhenCaptureWithdrewTx(ctx, tx, holder.ContactID.UUID, time.Now().UTC()); err != nil {
		return "", err
	}
	mail, err := activities.RefileOwnAddressMailTx(ctx, tx, holder.ContactID, holder.OwnerID, claim, RefileLimit)
	switch {
	case err != nil:
		return "", err
	case mail.Foreign:
		return "other mail", nil
	case mail.TooMany:
		return "too much mail", nil
	}
	return "", nil
}
