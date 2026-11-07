// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"testing"

	capturemod "github.com/margince/margince/backend/internal/modules/capture"
)

// Mail the seat sent from an alias arrives inbound and becomes the seat's own
// outbound mail inside the capture transaction. Its recipients are then the
// seat's to admit. So admission after the commit must follow what the
// transaction stored, not the record the connector handed over.
func TestAdmissionRunsForMailTheSeatSentFromAnAlias(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	declareIdentity(t, e, e.Rep1, capturemod.IdentityKindAddress, ownerAlias)

	env.syncSent(t, map[string]bool{"alias-admit1@private.example": true},
		emailCC(ownerAlias, "Lars Private", "alice@acme.example", "bob@acme.example", "alias-admit1@private.example"),
	)

	if n := countRows(t, e, `SELECT count(*) FROM capture_party_source WHERE sent_by_owner`); n != 1 {
		t.Fatalf("%d party source(s) stored as sent by the seat, want 1: the transaction reads alias mail as the seat's own", n)
	}
	if n := countRows(t, e, `SELECT count(*) FROM capture_party_source WHERE admission_pending`); n != 0 {
		t.Errorf("%d party source(s) still pending after capture, want 0: admission skipped mail the seat sent", n)
	}
}

// Mail the seat only received names no one its proof can admit, so capture
// leaves nothing pending and runs no admission after the commit.
func TestReceivedMailLeavesNoAdmissionPending(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e

	env.syncSent(t, map[string]bool{},
		emailCC("alice@acme.example", "Alice", captureOwner, "bob@acme.example", "received-admit1@acme.example"),
	)

	if n := countRows(t, e, `SELECT count(*) FROM capture_party_source WHERE NOT sent_by_owner AND NOT admission_pending`); n != 1 {
		t.Fatalf("%d received party source(s) with nothing pending, want 1", n)
	}
}
