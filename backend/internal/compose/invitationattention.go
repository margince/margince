// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// invitationAttention raises the host's notice in the transaction that parks
// an invitation. The episode is the dedupe key. A disconnect that parks a
// whole queue, and a held row that joins it later, tell the host once.
type invitationAttention struct{ pool *pgxpool.Pool }

var invitationAttentionCopy = map[string]struct{ subject, body string }{
	"calendar_disconnected": {
		"Your calendar refused Margince, so your invitations are paused",
		"Reconnect your calendar with permission to add events. The paused invitations then go out on their own.",
	},
	"delivery_failed": {
		"An invitation could not be delivered",
		"Open the meeting to retry the invitation. The time stays reserved while you decide.",
	},
}

func (a invitationAttention) InvitationAttentionTx(ctx context.Context, tx pgx.Tx, host ids.UserID, activity, episode ids.UUID, reason string) error {
	words := invitationAttentionCopy[reason]
	_, err := notices.NewStore(InstallationDB(a.pool)).CreateTx(ctx, tx, notices.NewNotice{
		Recipient: host, Kind: notices.KindMeetingInvitationAttention,
		Subject: words.subject, Body: words.body,
		DedupeKey: notices.KindMeetingInvitationAttention + ":" + episode.String(),
		Target:    notices.Target{Type: entityActivity, ID: activity},
	})
	return err
}
