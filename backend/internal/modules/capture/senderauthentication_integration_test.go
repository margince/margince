// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestCapturedInboundKeepsReceivingServerAuthentication(t *testing.T) {
	ctx, db := bootstrapInternalMailWorkspace(t, "acme.com")
	ws, _ := principal.WorkspaceID(ctx)
	seat := seedSeat(ctx, t, db, "rep@acme.com", "active")
	for _, test := range []struct {
		name, provider, server, domain string
		want                           bool
	}{
		{"aligned Gmail sender", "gmail", "mx.google.com", "client.io", true},
		{"no result", "gmail", "", "", false},
		{"different domain", "gmail", "mx.google.com", "elsewhere.example", false},
		{"different receiving server", "gmail", "mx.elsewhere.example", "client.io", false},
		{"unverified transport", "imap", "mx.google.com", "client.io", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			rec := mailRecord("sender-auth-"+test.name, "pat@client.io", "pat@client.io", "rep@acme.com")
			rec.CapturedBy = "connector:" + test.provider
			rec.SenderAuthentication = connector.SenderAuthentication{Server: test.server, DMARCPassDomain: test.domain}
			ref, err := capture.NewSink(db).Upsert(capturedAs(ctx, ws, seat, rec))
			if err != nil {
				t.Fatal(err)
			}
			var authenticated bool
			if err := db.Tx(ctx, func(tx pgx.Tx) error {
				return tx.QueryRow(ctx, `SELECT inbound_sender_authenticated FROM activity WHERE id=$1`, ref.ID).Scan(&authenticated)
			}); err != nil {
				t.Fatal(err)
			}
			if authenticated != test.want {
				t.Fatalf("sender authenticated=%v, want %v", authenticated, test.want)
			}
		})
	}
}
