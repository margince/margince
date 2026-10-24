// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// Capture stamps once whether a sending system sent a received mail, from the
// sender's own address, so every reader asks the stored answer.
func TestCaptureMarksMailASendingSystemSent(t *testing.T) {
	ctx, db := bootstrapInternalMailWorkspace(t, "acme.com")
	ws, _ := principal.WorkspaceID(ctx)
	seat := seedSeat(ctx, t, db, "rep@acme.com", "active")
	for _, test := range []struct {
		name, from, direction string
		want                  bool
	}{
		{"a file-share notice", "drive-shares-dm-noreply@google.com", connector.DirectionInbound, true},
		{"a document comment reply address", "d+" + strings.Repeat("aorgB9", 34) + "@docs.google.com", connector.DirectionInbound, true},
		{"a delivery report", "mailer-daemon@googlemail.com", connector.DirectionInbound, true},
		{"a human's mail", "pat@client.io", connector.DirectionInbound, false},
		{"a human whose name holds a marker", "anna.notify.weber@client.io", connector.DirectionInbound, false},
		{"our mail to a no-reply address", "noreply@client.io", connector.DirectionOutbound, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			rec := mailRecord("machine-sender-"+test.name, test.from, test.from, "rep@acme.com")
			rec.Counterparty.Direction = test.direction
			fields, ok := rec.Fields.(capture.ActivityFields)
			if !ok {
				t.Fatal("a mail record carries ActivityFields")
			}
			fields.Direction = test.direction
			rec.Fields = fields
			ref, err := capture.NewSink(db).Upsert(capturedAs(ctx, ws, seat, rec))
			if err != nil {
				t.Fatal(err)
			}
			var machine bool
			if err := db.Tx(ctx, func(tx pgx.Tx) error {
				return tx.QueryRow(ctx, `SELECT sender_is_machine FROM activity WHERE id=$1`, ref.ID).Scan(&machine)
			}); err != nil {
				t.Fatal(err)
			}
			if machine != test.want {
				t.Fatalf("sender_is_machine=%v, want %v", machine, test.want)
			}
		})
	}
}
