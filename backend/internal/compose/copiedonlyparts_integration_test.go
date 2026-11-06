// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A mailbox only copied on a message keeps its files as names with no bytes.
// A seat in To arriving later gives those rows their bytes.
//
// Two captures of one message, which is how the pair happens. A copied mailbox
// syncs first, and the addressed seat's mailbox connects after. Files are
// staged for a new activity only, so the join is where they can still land.
func TestACopiedMailboxWithholdsUntilAnAddressedSeatArrives(t *testing.T) {
	ctx, db, tag := captureWorkspace(t)
	blob := blobstore.NewMemory()
	sink := capture.NewSink(db).WithFileKeeper(fileKeeper(db.Pool(), blob))

	const copied = "copied@fixture.example"
	const addressed = "addressed@fixture.example"
	declareOwnedAddress(ctx, t, db, captureSeatID, copied)

	source := "msg-copied-" + tag
	// withAttachedOriginal, so the stored original really carries the part's
	// base64. Without it the assertion below passes whether withholding works
	// or not, because mailRecord's Raw holds no attachment at all.
	rec := withAttachedOriginal(withFiles(mailRecord(source), onePDF()))
	fields := rec.Fields.(capture.ActivityFields)
	fields.Envelope = &capture.MailEnvelope{
		DirectRecipients: []string{addressed},
		OpenRecipients:   []string{addressed, copied},
	}
	rec.Fields = fields
	if _, err := sink.Upsert(ctx, rec); err != nil {
		t.Fatalf("the copied mailbox's capture: %v", err)
	}

	files := filesFor(ctx, t, db, source)
	if len(files) != 1 {
		t.Fatalf("stored %d rows, want 1 naming the file", len(files))
	}
	if !files[0].withheld {
		t.Error("the row keeps bytes, want a name with none: no seat here was in To")
	}
	if files[0].storageKey != "" {
		t.Errorf("storage_key = %q, want empty so no object is pointed at", files[0].storageKey)
	}
	encoded := base64.StdEncoding.EncodeToString(onePDF().Body)
	if stored := storedOriginal(ctx, t, db, source); bytes.Contains(stored, []byte(encoded)) {
		t.Error("the stored original still carries the file's bytes")
	}

	// The addressed seat's mailbox connects, and the same message arrives again.
	addressedSeat := ids.NewV7()
	seedCapturingSeat(ctx, t, db, addressedSeat)
	declareOwnedAddress(ctx, t, db, addressedSeat, addressed)
	if _, err := sink.Upsert(asConnectorFor(ctx, "connector:imap", addressedSeat), rec); err != nil {
		t.Fatalf("the addressed seat's capture: %v", err)
	}

	files = filesFor(ctx, t, db, source)
	if len(files) != 1 {
		t.Fatalf("stored %d rows after the second arrival, want the same 1", len(files))
	}
	if files[0].withheld {
		t.Error("still withheld, want the bytes an addressed seat is entitled to")
	}
	if files[0].storageKey == "" {
		t.Fatal("no storage_key, so the adopted row points at nothing")
	}
	body, _, err := blob.Get(ctx, files[0].storageKey)
	if err != nil {
		t.Fatalf("the adopted row points at bytes the object store does not have: %v", err)
	}
	defer func() {
		if err := body.Close(); err != nil {
			t.Errorf("closing the adopted object: %v", err)
		}
	}()
	// The file's own bytes, not merely an object at that key. A key pointing at
	// something else would satisfy a presence check and still have lost the file.
	got, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("reading the adopted object: %v", err)
	}
	if !bytes.Equal(got, onePDF().Body) {
		t.Errorf("adopted %d bytes, want the part's own %d", len(got), len(onePDF().Body))
	}
}

// declareOwnedAddress says this installation owns an address, which is what
// makes a seat in To one of ours.
func declareOwnedAddress(ctx context.Context, t *testing.T, db *database.DB, seat ids.UUID, value string) {
	t.Helper()
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO capture_owner_identity (user_id, kind, value, source, created_by)
			VALUES ($1, 'address', $2, 'user', 'human:test')
			ON CONFLICT (user_id, kind, value) DO NOTHING`, seat, value)
		return err
	}); err != nil {
		t.Fatalf("declaring %s as ours: %v", value, err)
	}
}

// seedCapturingSeat writes the app_user a capture principal names, which the
// audit trail needs as a real row.
func seedCapturingSeat(ctx context.Context, t *testing.T, db *database.DB, seat ids.UUID) {
	t.Helper()
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO app_user (id, email, display_name)
			VALUES ($1, $2, 'Addressed Seat')
			ON CONFLICT (id) DO NOTHING`, seat, "seat-"+seat.String()+"@fixture.example")
		return err
	}); err != nil {
		t.Fatalf("seeding the addressed seat: %v", err)
	}
}
