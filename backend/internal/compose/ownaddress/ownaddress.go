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
)

// refileLimit is the most messages one claim moves. A contact with more is left
// as it is, so the claim request stays short.
const refileLimit = 500

// NewStore is the owner-identity store with the claim hook bound. Every door
// that lets a member declare an address builds its store here.
func NewStore(db *database.DB, store *contacts.Store) *capture.OwnerIdentityStore {
	return capture.NewOwnerIdentityStore(db).WithAddressClaimed(retire(store, slog.Default()))
}

// retire withdraws the contact capture made for the claimed address, when the
// claimant minted it and holds it alone, and refiles its mail. Every other
// contact on the address is left as it is.
func retire(store *contacts.Store, log *slog.Logger) capture.AddressClaimHook {
	return func(ctx context.Context, tx pgx.Tx, claim capture.ClaimedAddress) error {
		if claim.HeldByAnotherSeat {
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
			retired, reason, err := retireOne(ctx, tx, store, holder)
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
func retireOne(ctx context.Context, tx pgx.Tx, store *contacts.Store, holder contacts.CaptureOnlyHolder) (retired bool, reason string, err error) {
	sp, err := tx.Begin(ctx)
	if err != nil {
		return false, "", fmt.Errorf("ownaddress: opening a savepoint: %w", err)
	}
	reason, err = withdraw(ctx, sp, store, holder)
	if err != nil || reason != "" {
		return false, reason, errors.Join(err, sp.Rollback(ctx))
	}
	return true, "", sp.Commit(ctx)
}

// withdraw is the work inside the savepoint. It answers why the contact was
// left. An empty reason means it archived the contact, ended the notice duties
// it owed and refiled its mail.
func withdraw(ctx context.Context, tx pgx.Tx, store *contacts.Store, holder contacts.CaptureOnlyHolder) (string, error) {
	archived, err := store.RetractOwnAddressContactTx(ctx, tx, holder.ContactID, holder.OwnerID)
	if err != nil || !archived {
		return "not the member's alone", err
	}
	if _, err := consent.SettleWhenCaptureWithdrewTx(ctx, tx, holder.ContactID.UUID, time.Now().UTC()); err != nil {
		return "", err
	}
	mail, err := activities.RefileOwnAddressMailTx(ctx, tx, holder.ContactID, holder.OwnerID, refileLimit)
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
