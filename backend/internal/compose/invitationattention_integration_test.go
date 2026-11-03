// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A disconnect parks a queue and a held row joins it later. The host is told
// once per episode, and a new episode tells them again.
func TestAHostIsToldOncePerParkedEpisode(t *testing.T) {
	e := integration.Setup(t)
	host := ids.From[ids.UserKind](e.Rep1)
	attention := invitationAttention{pool: e.Pool}
	meeting := e.SeedContact(t, "Episode subject", &e.Rep1)
	first, second := ids.NewV7(), ids.NewV7()
	for _, episode := range []ids.UUID{first, first, second} {
		if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
			return attention.InvitationAttentionTx(e.Admin(), tx, host, meeting, episode, "calendar_disconnected")
		}); err != nil {
			t.Fatal(err)
		}
	}
	if got := e.WsCount(t, `SELECT count(*) FROM notice WHERE recipient_user_id=$1 AND kind=$2`, e.Rep1, notices.KindMeetingInvitationAttention); got != 2 {
		t.Errorf("the host holds %d notices for two episodes, want 2", got)
	}
}
