// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A received message whose capture recorded no sender participant still shows
// who sent it: on a received message the counterparty is the sender.
func TestAReceivedMessageAlwaysNamesItsSender(t *testing.T) {
	e := setupLoad(t)
	id := seedEmailRequest(t, e, "Please send the report", "commitment", OwedVerdictAsksUs)
	// An older capture dropped a sender address the seat held, leaving no row.
	e.exec(t, `DELETE FROM activity_participant WHERE activity_id = $1 AND role = 'from'`, id)

	email, err := storeKnowing(e).GetEmailPresentation(e.as(), ids.From[ids.ActivityKind](id), nil)
	if err != nil {
		t.Fatalf("reading the message: %v", err)
	}
	if len(email.From) != 1 || email.From[0].Address != requestCounterparty {
		t.Fatalf("From reads %+v, want the sender %s", email.From, requestCounterparty)
	}
}
