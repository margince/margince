// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

// The headers-first question gives Upsert's own answer. A backfill asks
// DropBeforeStore before downloading a message in full, so a verdict that
// differed from Upsert's would either drop customer mail or download colleague
// mail for nothing.

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestDropBeforeStoreAgreesWithUpsert(t *testing.T) {
	ctx, db := bootstrapInternalMailWorkspace(t, "acme.com")
	sink := capture.NewSink(db)

	internal := mailRecord("prestore-internal", "boss@acme.com", "boss@acme.com", "rep@acme.com")
	internal.Raw = nil
	drop, err := sink.DropBeforeStore(ctx, internal)
	if err != nil || !drop {
		t.Fatalf("all-internal headers: drop=%v err=%v, want dropped", drop, err)
	}
	// The drop leaves the breadcrumb Upsert's drop leaves, and nothing else.
	if reasons := breadcrumbReasons(ctx, t, db.Pool(), "prestore-internal"); len(reasons) != 1 || reasons[0] != "internal_only" {
		t.Errorf("ledger reasons = %v, want exactly one internal_only", reasons)
	}
	if activities, _, raws, _ := countsFor(ctx, t, db.Pool(), "prestore-internal"); activities != 0 || raws != 0 {
		t.Errorf("the pre-store question stored rows: activity=%d raw_capture=%d", activities, raws)
	}

	external := mailRecord("prestore-external", "buyer@customer.example", "buyer@customer.example", "rep@acme.com")
	external.Raw = nil
	if drop, err := sink.DropBeforeStore(ctx, external); err != nil || drop {
		t.Fatalf("customer headers: drop=%v err=%v, want kept for the full download", drop, err)
	}
	if activities, _, _, _ := countsFor(ctx, t, db.Pool(), "prestore-external"); activities != 0 {
		t.Errorf("the pre-store question stored the kept message; only Upsert may")
	}
}

// Mail the seat sent between their own addresses may carry a forward to self,
// which only its body shows. Its headers alone never drop current mail; the
// full download decides. Unsent mail, and history, are dropped as before.
func TestDropBeforeStoreLeavesTheSeatsOwnSentMailToItsBody(t *testing.T) {
	ctx, db := bootstrapInternalMailWorkspace(t, "acme.com")
	ws, _ := principal.WorkspaceID(ctx)
	seat := seedSeat(ctx, t, db, "rep@acme.com", "active")
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO capture_connection (user_id, provider, status, credential_ref, account_label)
			VALUES ($1, 'imap', 'connected', 'vault:test', 'rep@acme.com')`, seat)
		return err
	}); err != nil {
		t.Fatalf("connecting the seat's mailbox: %v", err)
	}
	now := time.Date(2026, 6, 5, 8, 0, 0, 0, time.UTC)
	sink := capture.NewSink(db).WithClock(func() time.Time { return now })
	own := func(sourceID string, sent bool, daysAgo int) connector.NormalizedRecord {
		headers := []byte("From: rep@acme.com\r\nTo: rep@acme.com\r\nSubject: note\r\nMessage-ID: <" + sourceID + ">\r\n\r\n")
		parsed, err := mailmap.Parse(headers, "rep@acme.com")
		if err != nil {
			t.Fatalf("parsing the headers: %v", err)
		}
		rec := parsed.AttestSentByOwner(sent).ToRecord("imap", nil)
		rec.ProviderReceivedAt = now.AddDate(0, 0, -daysAgo)
		return rec
	}

	if drop, err := sink.DropBeforeStore(capturedAs(ctx, ws, seat, own("prestore-own-sent@acme.com", true, 1))); err != nil || drop {
		t.Fatalf("the seat's own sent mail: drop=%v err=%v, want kept for the full download", drop, err)
	}
	if reasons := breadcrumbReasons(ctx, t, db.Pool(), "prestore-own-sent@acme.com"); len(reasons) != 0 {
		t.Errorf("ledger reasons = %v, want none for mail the headers did not judge", reasons)
	}
	if drop, err := sink.DropBeforeStore(capturedAs(ctx, ws, seat, own("prestore-own-unsent@acme.com", false, 1))); err != nil || !drop {
		t.Fatalf("own mail nobody filed as sent: drop=%v err=%v, want dropped", drop, err)
	}
	if drop, err := sink.DropBeforeStore(capturedAs(ctx, ws, seat, own("prestore-own-history@acme.com", true, 400))); err != nil || !drop {
		t.Fatalf("own sent mail from history: drop=%v err=%v, want dropped on its headers", drop, err)
	}
}
