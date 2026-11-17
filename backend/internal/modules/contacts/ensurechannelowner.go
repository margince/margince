// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import "github.com/margince/margince/backend/internal/shared/kernel/ids"

// owner is who owns a contact this ensure creates: nobody for a workspace bot,
// the member for a chat app that is theirs.
func (in EnsureChannelCounterpartyInput) owner() *ids.UserID {
	if in.OwnerID.IsZero() {
		return nil
	}
	return new(ids.From[ids.UserKind](in.OwnerID))
}
