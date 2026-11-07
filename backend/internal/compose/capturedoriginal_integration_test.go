// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/partslim"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// mimeWithFile is one message whose second part is the file, base64 folded at
// 76 columns as providers send it.
func mimeWithFile(file []byte) []byte {
	encoded := base64.StdEncoding.EncodeToString(file)
	var folded strings.Builder
	for at := 0; at < len(encoded); at += 76 {
		folded.WriteString(encoded[at:min(at+76, len(encoded))] + "\r\n")
	}
	return []byte("From: her@example.com\r\nMIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=\"b1\"\r\n\r\n" +
		"--b1\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nAttached, as promised.\r\n" +
		"--b1\r\nContent-Type: application/pdf\r\nContent-Disposition: attachment; filename=\"contract.pdf\"\r\n" +
		"Content-Transfer-Encoding: base64\r\n\r\n" + folded.String() + "--b1--\r\n")
}

func largePDF() []byte {
	body := make([]byte, 16<<10)
	copy(body, "%PDF-1.4\n")
	for i := len("%PDF-1.4\n"); i < len(body); i++ {
		body[i] = byte(i*37%251 + 3)
	}
	return body
}

type slimmedOriginal struct {
	payload []byte
	outcome *string
}

func slimmedOriginalOf(ctx context.Context, t *testing.T, db *database.DB, activity ids.UUID) slimmedOriginal {
	t.Helper()
	var out slimmedOriginal
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT rc.payload, rc.parts_slim_outcome
			  FROM activity a JOIN raw_capture rc ON rc.id = a.raw_capture_id
			 WHERE a.id = $1`, activity).Scan(&out.payload, &out.outcome)
	}); err != nil {
		t.Fatalf("read the activity's stored original: %v", err)
	}
	return out
}

// A new message's file is stored once, in the object store. Its original names
// the object rather than carrying the bytes, and gives the provider's exact
// message back when the object is read.
func TestANewMessagesOriginalNamesItsStoredFileInsteadOfCarryingIt(t *testing.T) {
	ctx, db, tag := captureWorkspace(t)
	blob := blobstore.NewMemory()
	sink := capture.NewSink(db).WithFileKeeper(fileKeeper(db.Pool(), blob))
	file := largePDF()
	raw := mimeWithFile(file)
	rec := withFiles(mailRecord("msg-slim-"+tag), connector.Part{
		Ordinal: 1, Filename: "contract.pdf", ContentType: "application/pdf", Body: file,
	})
	rec.Raw = raw
	for range 2 {
		if _, err := sink.Upsert(ctx, rec); err != nil {
			t.Fatalf("capture: %v", err)
		}
	}

	stored := slimmedOriginalOf(ctx, t, db, activityOf(ctx, t, db, connector.EmailSourceSystem+":msg-slim-"+tag))
	if stored.outcome == nil || *stored.outcome != "slimmed" {
		t.Fatalf("the original is stamped %v, want slimmed so the part sweep passes it by", stored.outcome)
	}
	original, err := capture.DecodeStoredOriginal(stored.payload)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(original, []byte(base64.StdEncoding.EncodeToString(file)[:76])) {
		t.Fatal("the stored original still carries the file's bytes")
	}
	restored, err := partslim.RestoreStoredParts(original, func(ref partslim.PartRef) ([]byte, error) {
		body, _, err := blob.Get(ctx, ref.StorageKey)
		if err != nil {
			return nil, err
		}
		read, err := io.ReadAll(body)
		if closeErr := body.Close(); closeErr != nil {
			return nil, closeErr
		}
		return read, err
	})
	if err != nil {
		t.Fatalf("restoring the original from its object: %v", err)
	}
	if !bytes.Equal(restored, raw) {
		t.Fatal("the restored original is not the message the provider sent")
	}
}

// An original whose file cannot be located in it, here one with no MIME parts,
// is stored whole. The part sweep meets it as before.
func TestAnOriginalNothingCanComeOutOfIsStoredWhole(t *testing.T) {
	ctx, db, tag := captureWorkspace(t)
	sink := capture.NewSink(db).WithFileKeeper(fileKeeper(db.Pool(), blobstore.NewMemory()))
	rec := withFiles(mailRecord("msg-whole-"+tag), onePDF())
	if _, err := sink.Upsert(ctx, rec); err != nil {
		t.Fatalf("capture: %v", err)
	}
	stored := slimmedOriginalOf(ctx, t, db, activityOf(ctx, t, db, connector.EmailSourceSystem+":msg-whole-"+tag))
	if stored.outcome != nil {
		t.Fatalf("the original is stamped %q, want it unstamped for the sweep", *stored.outcome)
	}
	original, err := capture.DecodeStoredOriginal(stored.payload)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original, rec.Raw) {
		t.Fatalf("stored %q, want the provider's bytes unchanged", original)
	}
}

// A file whose attachment row was already there names an older object, which
// only the part sweep can prove. The original keeps that file's bytes rather
// than naming an object this capture staged and no row points at.
func TestAFileRecordedBeforeKeepsItsBytesInTheOriginal(t *testing.T) {
	ctx, db, tag := captureWorkspace(t)
	sourceID := "msg-conflict-" + tag
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		var earlier ids.UUID
		if err := tx.QueryRow(ctx, `INSERT INTO activity (kind, occurred_at, source, captured_by)
			VALUES ('email', now(), 'manual', 'human:test') RETURNING id`).Scan(&earlier); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO attachment (entity_type, entity_id, activity_id, filename, byte_size,
			storage_key, checksum, source, captured_by, category, external_source_id, external_part_id)
			VALUES ('activity', $1, $1, 'contract.pdf', 1, 'an/older/object', 'sum', 'email', 'human:test',
			        'email_attachment', $2, 'part:1')`, earlier, connector.EmailSourceSystem+":"+sourceID)
		return err
	}); err != nil {
		t.Fatalf("seeding the earlier attachment row: %v", err)
	}
	sink := capture.NewSink(db).WithFileKeeper(fileKeeper(db.Pool(), blobstore.NewMemory()))
	file := largePDF()
	rec := withFiles(mailRecord(sourceID), connector.Part{
		Ordinal: 1, Filename: "contract.pdf", ContentType: "application/pdf", Body: file,
	})
	rec.Raw = mimeWithFile(file)
	ref, err := sink.Upsert(ctx, rec)
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	stored := slimmedOriginalOf(ctx, t, db, ref.ID)
	if stored.outcome != nil {
		t.Fatalf("the original is stamped %q, want it whole for the sweep", *stored.outcome)
	}
	original, err := capture.DecodeStoredOriginal(stored.payload)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original, rec.Raw) {
		t.Fatal("the original lost a file whose row names another object")
	}
}
