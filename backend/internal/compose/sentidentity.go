// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// conversationFollowsIdentity re-keys a sent message's timeline row. A message
// that roots its conversation names it, so a provider-stamped identity renames
// the conversation. Every other row keyed on the old name moves too, so a hold
// carried to a reply on a new topic stays on it. A reply in an older conversation
// has no rows under its own identity, so the move finds nothing.
type conversationFollowsIdentity struct{ timeline *activities.Store }

func sentIdentity(db *database.DB) conversationFollowsIdentity {
	return conversationFollowsIdentity{timeline: activities.NewStore(db)}
}

func (r conversationFollowsIdentity) ReconcileMessageIdentityTx(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, previous, stamped string) error {
	if err := r.timeline.ReconcileMessageIdentityTx(ctx, tx, activityID, previous, stamped); err != nil {
		return err
	}
	return mergeThreadsTx(ctx, tx, previous, stamped)
}
