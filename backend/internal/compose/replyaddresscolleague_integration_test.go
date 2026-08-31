// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// An automation composes replies with no human present to name the addressee,
// so the adapter has to refuse the one addressee the store cannot recognise as
// ours: a colleague on the workspace's own domain who holds no seat.

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedMeetingWith writes one captured meeting whose only party besides the
// connected owner is the address given.
func seedMeetingWith(t *testing.T, e *integration.Env, address string) ids.UUID {
	t.Helper()
	activity := integration.SeedIDRow(t, integration.OwnerConn(t), `
		INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'meeting', 'Daily stand-up', now(), 'test', 'human:seed')`)
	e.WsExec(t, `
		INSERT INTO activity_participant (id, activity_id, role, user_id, address)
		VALUES ($1, $2, 'from', $3, 'rep@ourcompany.test')`, ids.NewV7(), activity, e.Rep1)
	e.WsExec(t, `
		INSERT INTO activity_participant (id, activity_id, role, address)
		VALUES ($1, $2, 'attendee', $3)`, ids.NewV7(), activity, address)
	return activity
}

func TestAReplyIsNeverAddressedToAColleagueOnTheOwnDomain(t *testing.T) {
	e := integration.Setup(t)
	e.WsExec(t, `INSERT INTO workspace_email_domain (domain, source, verified) VALUES ('ourcompany.test', 'admin', true)`)
	comms := newCommsAdapter(e.Pool, nil, SendPath{})

	_, err := comms.ReplyAddress(e.Admin(), seedMeetingWith(t, e, "colleague@ourcompany.test"))
	var refusal *activities.NoReplyAddressError
	if !errors.As(err, &refusal) || !refusal.Colleague {
		t.Fatalf("ReplyAddress → %v, want the colleague refusal", err)
	}

	// The admit case: the same meeting with a guest is answered to the guest.
	got, err := comms.ReplyAddress(e.Admin(), seedMeetingWith(t, e, "dana@client.example"))
	if err != nil || got != "dana@client.example" {
		t.Fatalf("ReplyAddress → (%q, %v), want the guest", got, err)
	}
}
