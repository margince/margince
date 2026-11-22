// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// ClaimedAddress is an address a member has just declared as their own.
type ClaimedAddress struct {
	User    ids.UUID
	Address string
	// HeldByAnotherSeat is true when a colleague also claims the address, signs
	// in with it, or has proven it by connecting its mailbox. Such an address
	// is not one member's alone, so nothing may be withdrawn on one claim.
	HeldByAnotherSeat bool
}

// AddressClaimHook withdraws what capture made from an address before its
// owner claimed it. It runs in the claim's own transaction, so a refusal rolls
// the claim back with it. compose injects it, because the contact and its mail
// belong to sibling modules.
type AddressClaimHook func(ctx context.Context, tx pgx.Tx, claim ClaimedAddress) error

// WithAddressClaimed returns a store that calls hook after every claim of an
// address, including one put back from a withdrawal.
func (s *OwnerIdentityStore) WithAddressClaimed(hook AddressClaimHook) *OwnerIdentityStore {
	out := *s
	out.addressClaimed = hook
	return &out
}

// addressClaimedTx tells the hook about a claim the member typed. A machine
// discovery (delivered_to) never reaches it. A seat that never typed the
// address has not told us its contact is their own.
func (s *OwnerIdentityStore) addressClaimedTx(ctx context.Context, tx pgx.Tx, identity OwnerIdentity) error {
	if s.addressClaimed == nil || identity.Kind != IdentityKindAddress || identity.Source == IdentitySourceDeliveredTo {
		return nil
	}
	held, err := claimHeldElsewhereTx(ctx, tx, identity.UserID, identity.Value)
	if err != nil {
		return err
	}
	return s.addressClaimed(ctx, tx, ClaimedAddress{User: identity.UserID, Address: identity.Value, HeldByAnotherSeat: held})
}

// claimHeldElsewhereTx reports whether a colleague has a claim on the address.
// That is a declared identity or a connected mailbox, or a login they sign in with.
func claimHeldElsewhereTx(ctx context.Context, tx pgx.Tx, user ids.UUID, address string) (bool, error) {
	held, err := addressHeldByAnotherSeatTx(ctx, tx, user, address)
	if err != nil || held {
		return held, err
	}
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM app_user u
		                WHERE u.id <> $1 AND lower(btrim(u.email)) = $2 AND `+workforce.LiveMemberSQL("u")+`)`,
		user, address).Scan(&held); err != nil {
		return false, fmt.Errorf("capture: reading whether a colleague signs in with a claimed address: %w", err)
	}
	return held, nil
}
