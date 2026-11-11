// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// invitationAttention raises the host's notice in the transaction that parks
// an invitation. The episode is the dedupe key. A disconnect that parks a
// whole queue, and a held row that joins it later, tell the host once.
type invitationAttention struct{ pool *pgxpool.Pool }

var invitationAttentionCopy = map[string]struct{ subject, body string }{
	string(crmcontracts.MeetingInvitationAttentionReasonCalendarDisconnected): {
		systemtext.Literal("invitation.calendar_disconnected.subject"),
		systemtext.Literal("invitation.calendar_disconnected.body"),
	},
	string(crmcontracts.MeetingInvitationAttentionReasonDeliveryFailed): {
		systemtext.Literal("invitation.delivery_failed.subject"),
		systemtext.Literal("invitation.delivery_failed.body"),
	},
}

func (a invitationAttention) InvitationAttentionTx(ctx context.Context, tx pgx.Tx, host ids.UserID, activity, episode ids.UUID, reason string) error {
	words := invitationAttentionCopy[reason]
	_, err := notices.NewStore(InstallationDB(a.pool)).CreateTx(ctx, tx, notices.NewNotice{
		Recipient: host, Kind: notices.KindMeetingInvitationAttention,
		Subject: words.subject, Body: words.body,
		SystemText: &systemtext.Message{Version: 1, Subject: systemtext.Sentence("invitation." + reason + ".subject"), Body: systemtext.Sentence("invitation." + reason + ".body")},
		DedupeKey:  notices.KindMeetingInvitationAttention + ":" + episode.String(),
		Target:     notices.Target{Type: entityActivity, ID: activity},
	})
	return err
}
