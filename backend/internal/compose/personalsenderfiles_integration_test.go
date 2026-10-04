// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Mail from a sender this seat's verdict judged a personal correspondent keeps
// no attachment bytes: not in the object store, and not in the stored original.

import (
	"bytes"
	"context"
	"encoding/base64"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// withAttachedOriginal gives the record a stored original that carries the
// part's bytes base64-encoded, the way a provider's MIME does.
func withAttachedOriginal(rec connector.NormalizedRecord) connector.NormalizedRecord {
	encoded := base64.StdEncoding.EncodeToString(rec.Parts[0].Body)
	rec.Raw = []byte("From: her@example.com\r\n" +
		"Content-Type: multipart/mixed; boundary=\"b1\"\r\n\r\n" +
		"--b1\r\nContent-Type: text/plain\r\n\r\nAttached, as promised.\r\n" +
		"--b1\r\nContent-Type: application/pdf\r\nContent-Transfer-Encoding: base64\r\n\r\n" +
		encoded + "\r\n--b1--\r\n")
	return rec
}

// judgeSenderPersonal settles this seat's verdict on her@example.com as a
// personal correspondent, against the activity the first message created.
func judgeSenderPersonal(ctx context.Context, t *testing.T, db *database.DB, firstSourceID string) {
	t.Helper()
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO capture_pending_counterparty (email, activity_id, owner_id, status, kind, resolved_at)
			SELECT 'her@example.com', a.id, $2, 'noise', 'personal', now()
			  FROM activity a WHERE a.source_id = $1`, firstSourceID, captureSeatID)
		return err
	}); err != nil {
		t.Fatalf("judging the sender personal: %v", err)
	}
}

// storedOriginal reads back what raw_capture kept for one message.
func storedOriginal(ctx context.Context, t *testing.T, db *database.DB, sourceID string) []byte {
	t.Helper()
	var payload []byte
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT payload FROM raw_capture WHERE source_id = $1`, sourceID).Scan(&payload)
	}); err != nil {
		t.Fatalf("reading the stored original: %v", err)
	}
	raw, err := capture.DecodeStoredOriginal(payload)
	if err != nil {
		t.Fatalf("decoding the stored original: %v", err)
	}
	return raw
}

func TestAPersonalSendersFilesAreKeptNowhere(t *testing.T) {
	ctx, db, tag := captureWorkspace(t)
	blob := blobstore.NewMemory()
	sink := capture.NewSink(db).WithFileKeeper(fileKeeper(db.Pool(), blob))

	if _, err := sink.Upsert(ctx, mailRecord("msg-first-"+tag)); err != nil {
		t.Fatalf("capturing the first message: %v", err)
	}
	judgeSenderPersonal(ctx, t, db, "msg-first-"+tag)

	rec := withAttachedOriginal(withFiles(mailRecord("msg-personal-"+tag), onePDF()))
	if _, err := sink.Upsert(ctx, rec); err != nil {
		t.Fatalf("capturing the personal sender's message: %v", err)
	}

	if files := filesFor(ctx, t, db, "msg-personal-"+tag); len(files) != 0 {
		t.Errorf("a personal sender's message stored %d file(s) in the object store", len(files))
	}
	encoded := base64.StdEncoding.EncodeToString(onePDF().Body)
	if raw := storedOriginal(ctx, t, db, "msg-personal-"+tag); bytes.Contains(raw, []byte(encoded)) {
		t.Error("the stored original still carries the attachment's bytes")
	}
	if got := withheldVerdict(ctx, t, db, "msg-personal-"+tag); got != "personal_sender" {
		t.Errorf("breadcrumb verdict = %q, want personal_sender — the trail must say which lane decided", got)
	}
	if !activityExists(ctx, t, db, "msg-personal-"+tag) {
		t.Fatal("the message was lost along with its files")
	}
}

// A `business` override readmits the sender, so their files are stored as any
// other correspondent's are.
func TestABusinessOverrideKeepsAPersonalSendersFiles(t *testing.T) {
	ctx, db, tag := captureWorkspace(t)
	blob := blobstore.NewMemory()
	sink := capture.NewSink(db).WithFileKeeper(fileKeeper(db.Pool(), blob))

	if _, err := sink.Upsert(ctx, mailRecord("msg-first-"+tag)); err != nil {
		t.Fatalf("capturing the first message: %v", err)
	}
	judgeSenderPersonal(ctx, t, db, "msg-first-"+tag)
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO capture_sender_override (user_id, address, decision, overruled_kind)
			VALUES ($1, 'her@example.com', 'business', 'personal')`, captureSeatID)
		return err
	}); err != nil {
		t.Fatalf("overriding the verdict: %v", err)
	}

	rec := withAttachedOriginal(withFiles(mailRecord("msg-readmitted-"+tag), onePDF()))
	if _, err := sink.Upsert(ctx, rec); err != nil {
		t.Fatalf("capturing the readmitted sender's message: %v", err)
	}
	if files := filesFor(ctx, t, db, "msg-readmitted-"+tag); len(files) != 1 {
		t.Errorf("stored %d files for a sender overridden to business, want 1", len(files))
	}
	encoded := base64.StdEncoding.EncodeToString(onePDF().Body)
	if raw := storedOriginal(ctx, t, db, "msg-readmitted-"+tag); !bytes.Contains(raw, []byte(encoded)) {
		t.Error("the stored original lost the bytes of a business sender's attachment")
	}
}
