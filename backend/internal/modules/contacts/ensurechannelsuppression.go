// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// channelSubjectSuppressed asks every key the subject can have been erased by.
//
// The channel identities are asked always, and the address only when the record
// carries one. An erasure keyed on an address would otherwise be undone by the
// subject's next direct message. That message names them by an account the
// suppression list has never heard of. Deletion has to stick on every key the
// record can be recognised by, not only the one this path started with.
func (s *Store) channelSubjectSuppressed(ctx context.Context, tx pgx.Tx, in EnsureChannelCounterpartyInput) (bool, error) {
	for _, key := range channelKeysOf(in.Identity) {
		suppressed, err := storekit.ChannelIdentitySuppressed(ctx, tx, key.Provider, key.ChannelUserID)
		if err != nil || suppressed {
			return suppressed, err
		}
	}
	if in.CorroboratingEmail == "" {
		return false, nil
	}
	return storekit.EmailSuppressed(ctx, tx, in.CorroboratingEmail)
}

// channelKeysOf names the identity and its aliases as the suppression list keys them.
func channelKeysOf(ci connector.ChannelIdentity) []storekit.ChannelIdentityKey {
	var keys []storekit.ChannelIdentityKey
	for _, each := range ci.All() {
		keys = append(keys, storekit.ChannelIdentityKey{Provider: each.Provider, ChannelUserID: storekit.ChannelSuppressionAccount(each.AccountScope, each.ChannelUserID)})
	}
	return keys
}
