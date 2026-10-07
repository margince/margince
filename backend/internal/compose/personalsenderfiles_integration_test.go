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
	"fmt"
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
	rec.Raw = []byte("From: " + rec.Counterparty.Email + "\r\n" +
		"Content-Type: multipart/mixed; boundary=\"b1\"\r\n\r\n" +
		"--b1\r\nContent-Type: text/plain\r\n\r\nAttached, as promised.\r\n" +
		"--b1\r\nContent-Type: application/pdf\r\nContent-Transfer-Encoding: base64\r\n\r\n" +
		encoded + "\r\n--b1--\r\n")
	return rec
}

// fromAddress is the record sent from address. Each case uses its own,
// because a verdict, override or contact one case seeds outlives it in the
// shared database and would decide the next case's sender.
func fromAddress(rec connector.NormalizedRecord, address string) connector.NormalizedRecord {
	rec.Counterparty.Email = address
	return rec
}

// judgeSenderPersonal settles this seat's verdict on address as a personal
// correspondent, against the activity the first message created.
func judgeSenderPersonal(ctx context.Context, t *testing.T, db *database.DB, firstSourceID, address string) {
	t.Helper()
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			INSERT INTO capture_pending_counterparty (email, activity_id, owner_id, status, kind, resolved_at)
			SELECT $3, a.id, $2, 'noise', 'personal', now()
			  FROM activity a WHERE a.source_id = $1`, firstSourceID, captureSeatID, address)
		if err == nil && tag.RowsAffected() != 1 {
			err = fmt.Errorf("the first message %s was not captured, so no verdict was seeded", firstSourceID)
		}
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

	address := "personal-" + tag + "@example.com"
	if _, err := sink.Upsert(ctx, fromAddress(mailRecord("msg-first-"+tag), address)); err != nil {
		t.Fatalf("capturing the first message: %v", err)
	}
	judgeSenderPersonal(ctx, t, db, "msg-first-"+tag, address)

	rec := withAttachedOriginal(withFiles(fromAddress(mailRecord("msg-personal-"+tag), address), onePDF()))
	if _, err := sink.Upsert(ctx, rec); err != nil {
		t.Fatalf("capturing the personal sender's message: %v", err)
	}

	requireNamedWithoutBytes(t, filesFor(ctx, t, db, "msg-personal-"+tag))
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
	address := "override-" + tag + "@example.com"

	if _, err := sink.Upsert(ctx, fromAddress(mailRecord("msg-first-"+tag), address)); err != nil {
		t.Fatalf("capturing the first message: %v", err)
	}
	judgeSenderPersonal(ctx, t, db, "msg-first-"+tag, address)
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO capture_sender_override (user_id, address, decision, overruled_kind)
			VALUES ($1, $2, 'business', 'personal')`, captureSeatID, address)
		return err
	}); err != nil {
		t.Fatalf("overriding the verdict: %v", err)
	}

	rec := withAttachedOriginal(withFiles(fromAddress(mailRecord("msg-readmitted-"+tag), address), onePDF()))
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

// A personal verdict on an address a correspondence contact holds is the
// forgery noiseMailScope already refuses: the contact's mail keeps its files.
func TestACorrespondenceContactsFilesSurviveAPersonalVerdict(t *testing.T) {
	ctx, db, tag := captureWorkspace(t)
	blob := blobstore.NewMemory()
	sink := capture.NewSink(db).WithFileKeeper(fileKeeper(db.Pool(), blob))
	address := "contact-" + tag + "@example.com"
	from := func(rec connector.NormalizedRecord) connector.NormalizedRecord { return fromAddress(rec, address) }

	if _, err := sink.Upsert(ctx, from(mailRecord("msg-first-"+tag))); err != nil {
		t.Fatalf("capturing the first message: %v", err)
	}
	judgeSenderPersonal(ctx, t, db, "msg-first-"+tag, address)
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		var contact string
		if err := tx.QueryRow(ctx, `
			INSERT INTO contact (full_name, source, captured_by, visibility)
			VALUES ('Her', 'imap', 'connector:imap', 'workspace') RETURNING id`).Scan(&contact); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO contact_email (contact_id, email, source, captured_by)
			VALUES ($1, $2, 'imap', 'connector:imap')`, contact, address)
		return err
	}); err != nil {
		t.Fatalf("seeding the correspondence contact: %v", err)
	}

	rec := withAttachedOriginal(withFiles(from(mailRecord("msg-contact-"+tag)), onePDF()))
	if _, err := sink.Upsert(ctx, rec); err != nil {
		t.Fatalf("capturing the contact's message: %v", err)
	}
	if files := filesFor(ctx, t, db, "msg-contact-"+tag); len(files) != 1 {
		t.Errorf("stored %d files for a correspondence contact, want 1", len(files))
	}
	encoded := base64.StdEncoding.EncodeToString(onePDF().Body)
	if raw := storedOriginal(ctx, t, db, "msg-contact-"+tag); !bytes.Contains(raw, []byte(encoded)) {
		t.Error("the stored original lost a correspondence contact's attachment")
	}
}

// A file the parser's bounds refused is in no part to splice out, so a
// personal sender's original keeps only its headers.
func TestAPersonalSendersRefusedFileLeavesNoBytesInTheOriginal(t *testing.T) {
	ctx, db, tag := captureWorkspace(t)
	sink := capture.NewSink(db).WithFileKeeper(fileKeeper(db.Pool(), blobstore.NewMemory()))

	address := "dropped-" + tag + "@example.com"
	if _, err := sink.Upsert(ctx, fromAddress(mailRecord("msg-first-"+tag), address)); err != nil {
		t.Fatalf("capturing the first message: %v", err)
	}
	judgeSenderPersonal(ctx, t, db, "msg-first-"+tag, address)

	rec := withAttachedOriginal(withFiles(fromAddress(mailRecord("msg-dropped-"+tag), address), onePDF()))
	rec.Parts = nil
	rec.PartDrops = []connector.PartDrop{{Count: 1, Reason: "too large"}}
	if _, err := sink.Upsert(ctx, rec); err != nil {
		t.Fatalf("capturing the message: %v", err)
	}
	encoded := base64.StdEncoding.EncodeToString(onePDF().Body)
	raw := storedOriginal(ctx, t, db, "msg-dropped-"+tag)
	if bytes.Contains(raw, []byte(encoded)) {
		t.Error("the refused file's bytes are still in the stored original")
	}
	if !bytes.HasPrefix(raw, []byte("From: "+address+"\r\n")) {
		t.Errorf("the stored original lost its headers: %q", raw)
	}
}

// requireNamedWithoutBytes holds a private message's file list: one row for the
// one file it carried, with its name, type and size, and no stored object.
func requireNamedWithoutBytes(t *testing.T, files []capturedFile) {
	t.Helper()
	if len(files) != 1 {
		t.Fatalf("a private message left %d attachment row(s), want 1 naming its file", len(files))
	}
	f := files[0]
	if !f.withheld || f.storageKey != "" {
		t.Errorf("the row has withheld=%v, storage_key=%q; want a withheld row with no object", f.withheld, f.storageKey)
	}
	if f.filename != onePDF().Filename || f.byteSize != int64(len(onePDF().Body)) ||
		f.contentType == nil || *f.contentType != onePDF().ContentType {
		t.Errorf("the row names %q (%v), %d bytes; want %q (%s), %d bytes",
			f.filename, f.contentType, f.byteSize, onePDF().Filename, onePDF().ContentType, len(onePDF().Body))
	}
	if f.company != nil {
		t.Errorf("a withheld file rolled up to account %s; a file nobody can open has no place in a library", *f.company)
	}
}
