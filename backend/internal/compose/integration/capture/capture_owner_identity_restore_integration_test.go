// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// The way back from withdrawing one of a seat's own addresses, through the real
// store. It covers what comes back, who may bring it back and what refuses it.
// It also covers how long the withdrawn address is kept at all.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func declareAndWithdraw(seat context.Context, t *testing.T, store *capturemod.OwnerIdentityStore, value string) capturemod.OwnerIdentity {
	t.Helper()
	identity, err := store.Add(seat, capturemod.IdentityKindAddress, value)
	if err != nil {
		t.Fatalf("declaring %s: %v", value, err)
	}
	if err := store.Remove(seat, identity.ID); err != nil {
		t.Fatalf("withdrawing %s: %v", value, err)
	}
	return identity
}

// expireWithdrawals ages every withdrawal past its undo, which is the one fact
// the purge reads.
func expireWithdrawals(t *testing.T, env captureEnv) {
	t.Helper()
	err := database.WithWorkspaceTx(env.e.Admin(), env.e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			UPDATE capture_owner_identity_withdrawn SET withdrawn_at = now() - interval '11 minutes'`)
		return err
	})
	if err != nil {
		t.Fatalf("ageing the withdrawals: %v", err)
	}
}

func TestARestoredIdentityComesBackAsItWas(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	store := capturemod.NewOwnerIdentityStore(e.DB())
	seat := seatContext(e, e.Rep1)
	original := declareAndWithdraw(seat, t, store, ownerAlias)

	restored, err := store.RestoreRemoval(seat, original.ID)
	if err != nil {
		t.Fatalf("restoring the alias: %v", err)
	}
	if restored.ID != original.ID || !restored.CreatedAt.Equal(original.CreatedAt) || restored.Source != original.Source {
		t.Errorf("restored %+v, want the withdrawn row as it was: %+v", restored, original)
	}
	if n := countRows(t, e, `SELECT count(*) FROM capture_owner_identity_withdrawn`); n != 0 {
		t.Errorf("%d withdrawn row(s) kept after the restore, want 0: the address is back where it belongs", n)
	}
	// It does what declaring does: mail among the seat's own addresses is
	// dropped again.
	env.sync(t, email(ownerAlias, "Lars Private", captureOwner, "restored1@private.example", ""))
	if n := countRows(t, e, `SELECT count(*) FROM activity WHERE kind = 'email'`); n != 0 {
		t.Errorf("%d activity row(s) from the restored alias, want 0", n)
	}
}

func TestOnlyTheSeatThatWithdrewCanRestore(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	store := capturemod.NewOwnerIdentityStore(e.DB())
	original := declareAndWithdraw(seatContext(e, e.Rep1), t, store, ownerAlias)

	if _, err := store.RestoreRemoval(seatContext(e, e.Rep3), original.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a colleague restoring Rep1's withdrawal → %v, want not-found", err)
	}
}

func TestARestoreOfAnAddressDeclaredAgainConflicts(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	store := capturemod.NewOwnerIdentityStore(e.DB())
	seat := seatContext(e, e.Rep1)
	original := declareAndWithdraw(seat, t, store, ownerAlias)
	if _, err := store.Add(seat, capturemod.IdentityKindAddress, ownerAlias); err != nil {
		t.Fatalf("declaring the alias again: %v", err)
	}

	_, err := store.RestoreRemoval(seat, original.ID)
	if !errors.Is(err, capturemod.ErrIdentityDeclaredAgain) || !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("restoring over a fresh declaration → %v, want ErrIdentityDeclaredAgain (409)", err)
	}
}

func TestARestoreAfterALaterWithdrawalHasMovedOn(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	store := capturemod.NewOwnerIdentityStore(e.DB())
	seat := seatContext(e, e.Rep1)
	first := declareAndWithdraw(seat, t, store, ownerAlias)
	second := declareAndWithdraw(seat, t, store, ownerAlias)

	if _, err := store.RestoreRemoval(seat, first.ID); !errors.Is(err, capturemod.ErrRemovalMovedOn) {
		t.Errorf("restoring the older withdrawal → %v, want ErrRemovalMovedOn", err)
	}
	if _, err := store.RestoreRemoval(seat, second.ID); err != nil {
		t.Errorf("restoring the newest withdrawal → %v, want it back", err)
	}
}

func TestAWithdrawalPastItsUndoIsPurged(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	store := capturemod.NewOwnerIdentityStore(e.DB())
	seat := seatContext(e, e.Rep1)
	original := declareAndWithdraw(seat, t, store, ownerAlias)
	expireWithdrawals(t, env)

	// The next withdrawal is what purges the expired one.
	declareAndWithdraw(seat, t, store, "other@private.example")
	if n := countRows(t, e, `SELECT count(*) FROM capture_owner_identity_withdrawn WHERE value = '`+ownerAlias+`'`); n != 0 {
		t.Errorf("%d expired withdrawal(s) of a private address kept, want 0", n)
	}
	if _, err := store.RestoreRemoval(seat, original.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("restoring past the undo → %v, want not-found", err)
	}
}

func TestAWithdrawnLearnedAliasIsNotLearnedAgain(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	store := capturemod.NewOwnerIdentityStore(e.DB())
	seat := seatContext(e, e.Rep1)
	learned := learnAlias(t, env, store)
	if err := store.Remove(seat, learned); err != nil {
		t.Fatalf("withdrawing the learned alias: %v", err)
	}
	// Past the undo window too: the refusal must not live in the side table.
	expireWithdrawals(t, env)
	declareAndWithdraw(seat, t, store, "other@private.example")

	env.sync(t,
		deliveredMail(forwardingAlias, "", "buyer@customer.example", "learn-3@customer.example"),
		deliveredMail(forwardingAlias, "", "other@customer.example", "learn-4@customer.example"))
	if got := ownAddresses(t, env, e.Rep1)[forwardingAlias]; got != "" {
		t.Errorf("the withdrawn alias was learned again with source %q: the seat said it is not theirs", got)
	}
}

// learnAlias has two deliveries teach the seat the forwarding alias, and
// answers the identity it became.
func learnAlias(t *testing.T, env captureEnv, store *capturemod.OwnerIdentityStore) ids.UUID {
	t.Helper()
	env.sync(t,
		deliveredMail(forwardingAlias, "", "buyer@customer.example", "learn-1@customer.example"),
		deliveredMail(forwardingAlias, "", "other@customer.example", "learn-2@customer.example"))
	listed, err := store.List(seatContext(env.e, env.e.Rep1))
	if err != nil {
		t.Fatalf("listing the seat's addresses: %v", err)
	}
	for _, identity := range listed {
		if identity.Value == forwardingAlias {
			return identity.ID
		}
	}
	t.Fatalf("the alias was never learned, so its withdrawal proves nothing (%v)", listed)
	return ids.Nil
}

const refusedAliases = `SELECT count(*) FROM capture_alias_refused WHERE value = '` + forwardingAlias + `'`

func TestUndoingTheWithdrawalOfALearnedAliasLiftsItsRefusal(t *testing.T) {
	env := newCaptureEnv(t)
	store := capturemod.NewOwnerIdentityStore(env.e.DB())
	seat := seatContext(env.e, env.e.Rep1)
	learned := learnAlias(t, env, store)
	if err := store.Remove(seat, learned); err != nil {
		t.Fatalf("withdrawing the learned alias: %v", err)
	}
	if _, err := store.RestoreRemoval(seat, learned); err != nil {
		t.Fatalf("undoing the withdrawal: %v", err)
	}
	if n := countRows(t, env.e, refusedAliases); n != 0 {
		t.Errorf("%d refusal(s) left after the undo, want 0: the seat took the withdrawal back", n)
	}
}

// A typed address restored after its own removal says nothing about the
// learned alias the seat refused earlier, so the refusal stays.
func TestRestoringATypedAddressKeepsALearnedAliasRefused(t *testing.T) {
	env := newCaptureEnv(t)
	store := capturemod.NewOwnerIdentityStore(env.e.DB())
	seat := seatContext(env.e, env.e.Rep1)
	if err := store.Remove(seat, learnAlias(t, env, store)); err != nil {
		t.Fatalf("withdrawing the learned alias: %v", err)
	}
	typed := declareAndWithdraw(seat, t, store, forwardingAlias)
	if _, err := store.RestoreRemoval(seat, typed.ID); err != nil {
		t.Fatalf("restoring the typed address: %v", err)
	}
	if n := countRows(t, env.e, refusedAliases); n != 1 {
		t.Errorf("%d refusal(s) after restoring a typed address, want the learned alias still refused", n)
	}
}
