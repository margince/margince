// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// conversationFollowsIdentity re-keys a sent message's timeline row. When the
// message roots its conversation, a provider-stamped identity renames it, so
// every other row keyed on the old name moves too: a hold carried to a reply on
// a new topic must stay on the conversation.
type conversationFollowsIdentity struct{ timeline *activities.Store }

func sentIdentity(db *database.DB) conversationFollowsIdentity {
	return conversationFollowsIdentity{timeline: activities.NewStore(db)}
}

func (r conversationFollowsIdentity) ReconcileMessageIdentityTx(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, previous, stamped string) error {
	if err := r.timeline.ReconcileMessageIdentityTx(ctx, tx, activityID, previous, stamped); err != nil {
		return err
	}
	current, err := storekit.LockActivityThreadTx(ctx, tx, activityID.UUID)
	if err != nil || current != stamped || previous == stamped {
		return err
	}
	return mergeThreadsTx(ctx, tx, previous, stamped)
}
