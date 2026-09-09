// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The sweep removes a stored part's octets and leaves alone everything it
// cannot prove.
//
// Driven through the real store against a real database and a real in-memory
// object store, because the defect this guards is a SEAM: a sweep that slimmed
// a row whose bytes were never staged would destroy the only copy there is, and
// no unit test over the filter can prove the join finds the right attachment
// rows for the right message.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/partslim"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// slimPDF is an attachment worth removing: large enough that the stanza costs
// less than the octets, and varied enough that its encoding cannot appear twice
// by accident.
func slimPDF(seed byte) []byte {
	body := make([]byte, 8192)
	copy(body, "%PDF-1.4\n")
	for i := len("%PDF-1.4\n"); i < len(body); i++ {
		body[i] = byte(int(seed)*13 + i*37%251)
	}
	return body
}

func slimDigest(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// slimWrap76 folds base64 the way every provider in the corpus does.
func slimWrap76(body []byte) string {
	b64 := base64.StdEncoding.EncodeToString(body)
	var out strings.Builder
	for at := 0; at < len(b64); at += 76 {
		if at > 0 {
			out.WriteString("\r\n")
		}
		end := at + 76
		if end > len(b64) {
			end = len(b64)
		}
		out.WriteString(b64[at:end])
	}
	return out.String()
}

// slimMessage is one RFC822 message carrying a visible body and one base64
// attachment.
func slimMessage(pdf []byte) []byte {
	return []byte("MIME-Version: 1.0\r\n" +
		"Subject: Quarterly figures\r\n" +
		"Content-Type: multipart/mixed; boundary=\"b1\"\r\n" +
		"\r\n" +
		"--b1\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		"the numbers themselves\r\n" +
		"--b1\r\n" +
		"Content-Type: application/pdf\r\n" +
		"Content-Disposition: attachment; filename=\"figures.pdf\"\r\n" +
		"Content-Transfer-Encoding: base64\r\n" +
		"\r\n" +
		slimWrap76(pdf) + "\r\n" +
		"--b1--\r\n")
}

// seedSlimOriginal writes one provider original the way the sink does: the
// payload is a JSON string, which is rawCapturePayload's spelling for text that
// is valid UTF-8 and NUL-free.
func seedSlimOriginal(ctx context.Context, tx pgx.Tx, sourceID string, raw []byte) error {
	payload, err := json.Marshal(string(raw))
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO raw_capture (source_system, source_id, payload)
		VALUES ('email', $1, $2)`, sourceID, payload)
	return err
}

// seedSlimAttachment writes the row the sweep's join finds. entity_type is the
// person arm so the fixture needs no activity: the sweep reads neither column.
func seedSlimAttachment(
	ctx context.Context, tx pgx.Tx, sourceID, key string, size int64, sum string,
) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO attachment (entity_type, entity_id, filename, content_type,
		                        byte_size, storage_key, checksum, source, captured_by,
		                        category, external_source_id, external_part_id)
		VALUES ('person', gen_random_uuid(), 'figures.pdf', 'application/pdf',
		        $1, $2, $3, 'email', 'connector:test', 'email_attachment',
		        'email:' || $4, 'part:1')`, size, key, sum, sourceID)
	return err
}

// payloadOf reads one original back as text.
func payloadOf(t *testing.T, e *Env, sourceID string) string {
	t.Helper()
	var payload string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT payload #>> '{}' FROM raw_capture WHERE source_id = $1`, sourceID).Scan(&payload)
	}); err != nil {
		t.Fatalf("reading back %s: %v", sourceID, err)
	}
	return payload
}

func TestTheSweepSlimsOnlyWhatItCanProve(t *testing.T) {
	e := Setup(t)
	ctx := context.Background()
	blob := blobstore.NewMemory()

	const provable, unstaged = "provable-msg", "unstaged-msg"
	const key = "ws/attachment/provable"
	provablePDF, unstagedPDF := slimPDF(1), slimPDF(2)

	if err := blob.Put(ctx, key, bytes.NewReader(provablePDF),
		int64(len(provablePDF)), "application/pdf"); err != nil {
		t.Fatalf("staging the provable bytes: %v", err)
	}

	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if err := seedSlimOriginal(ctx, tx, provable, slimMessage(provablePDF)); err != nil {
			return err
		}
		if err := seedSlimAttachment(ctx, tx, provable, key,
			int64(len(provablePDF)), slimDigest(provablePDF)); err != nil {
			return err
		}
		// The unstaged message: an original with a part, and NO attachment row,
		// which is the posture of every message captured before an object store
		// was wired. Its bytes are the only copy in existence.
		return seedSlimOriginal(ctx, tx, unstaged, slimMessage(unstagedPDF))
	}); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	store := capture.NewPartSlimStore(e.DB(), blob)
	got, err := store.SlimBatch(ctx, 10)
	if err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	if got.Considered < 2 {
		t.Fatalf("considered %d rows, want at least the 2 seeded", got.Considered)
	}
	if got.Slimmed != 1 {
		t.Fatalf("slimmed %d rows, want exactly 1", got.Slimmed)
	}
	if got.Freed <= 0 {
		t.Errorf("freed %d bytes, want more than zero", got.Freed)
	}

	slimmed := payloadOf(t, e, provable)
	if !strings.Contains(slimmed, partslim.PartStoredHeader+": part:1") {
		t.Errorf("the provable original carries no stanza")
	}
	if strings.Contains(slimmed, slimWrap76(provablePDF)) {
		t.Errorf("the provable original kept its encoded body")
	}
	if !strings.Contains(slimmed, "the numbers themselves") {
		t.Errorf("the visible body was collateral damage")
	}

	// THE ASSERTION THIS FILE EXISTS FOR. A message whose bytes were never
	// staged holds the only copy of them. If this ever fails, the sweep has
	// become capable of destroying an attachment that exists nowhere else.
	if kept := payloadOf(t, e, unstaged); !strings.Contains(kept, slimWrap76(unstagedPDF)) {
		t.Fatalf("the unstaged original LOST its only copy of its attachment")
	}
}

// A second pass does nothing, because the first stamped every row it read.
// Without the stamp the sweep re-reads, re-fetches and re-encodes the whole
// table every cadence forever.
func TestTheSweepConsidersEachOriginalOnce(t *testing.T) {
	e := Setup(t)
	ctx := context.Background()
	blob := blobstore.NewMemory()
	pdf := slimPDF(3)
	const sourceID, key = "once-msg", "ws/attachment/once"

	if err := blob.Put(ctx, key, bytes.NewReader(pdf), int64(len(pdf)), "application/pdf"); err != nil {
		t.Fatalf("staging: %v", err)
	}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if err := seedSlimOriginal(ctx, tx, sourceID, slimMessage(pdf)); err != nil {
			return err
		}
		return seedSlimAttachment(ctx, tx, sourceID, key, int64(len(pdf)), slimDigest(pdf))
	}); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	store := capture.NewPartSlimStore(e.DB(), blob)
	first, err := store.SlimBatch(ctx, 50)
	if err != nil {
		t.Fatalf("first pass: %v", err)
	}
	if first.Slimmed != 1 {
		t.Fatalf("first pass slimmed %d, want 1", first.Slimmed)
	}
	second, err := store.SlimBatch(ctx, 50)
	if err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if second.Considered != 0 || second.Slimmed != 0 {
		t.Errorf("second pass considered %d and slimmed %d, want 0 and 0",
			second.Considered, second.Slimmed)
	}
}

// With no object store the sweep does nothing at all — it does not even read.
//
// The trap it must not fall into is STAMPING: a pass that marked every row
// considered while proving nothing would consume the whole backlog, and an
// object store wired later would find no work left. So this asserts the rows
// come back byte-identical AND that none was stamped, which is the difference
// between a no-op and a silent, permanent loss of the backlog.
func TestTheSweepRemovesNothingWithoutAnObjectStore(t *testing.T) {
	e := Setup(t)
	ctx := context.Background()
	pdf := slimPDF(4)
	const sourceID = "nostore-msg"
	original := slimMessage(pdf)

	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if err := seedSlimOriginal(ctx, tx, sourceID, original); err != nil {
			return err
		}
		// The attachment row exists and names a key, which is exactly the trap:
		// a sweep trusting the row rather than the store would strip here.
		return seedSlimAttachment(ctx, tx, sourceID, "ws/attachment/nostore",
			int64(len(pdf)), slimDigest(pdf))
	}); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	store := capture.NewPartSlimStore(e.DB(), nil)
	got, err := store.SlimBatch(ctx, 50)
	if err != nil {
		t.Fatalf("sweeping with no store: %v", err)
	}
	if got.Slimmed != 0 {
		t.Fatalf("slimmed %d rows with no object store, want 0", got.Slimmed)
	}
	if got.Considered != 0 {
		t.Errorf("considered %d rows with no object store; want 0 — a stamp here "+
			"consumes the backlog a later-wired store would have worked", got.Considered)
	}
	if kept := payloadOf(t, e, sourceID); kept != string(original) {
		t.Errorf("an original was rewritten on a deployment with no object store")
	}
	var stamped int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT count(*) FROM raw_capture WHERE parts_slimmed_at IS NOT NULL`).Scan(&stamped)
	}); err != nil {
		t.Fatalf("counting the stamps: %v", err)
	}
	if stamped != 0 {
		t.Errorf("%d rows were stamped with no object store to prove anything against", stamped)
	}
}

// The round trip through the database, not just through memory: what the sweep
// wrote must restore to the bytes the provider sent. jsonb is the part of this
// that a unit test cannot exercise — the payload goes in as a JSON string and
// comes back through #>> '{}'.
func TestASlimmedOriginalRestoresToWhatTheProviderSent(t *testing.T) {
	e := Setup(t)
	ctx := context.Background()
	blob := blobstore.NewMemory()
	pdf := slimPDF(5)
	const sourceID, key = "restore-msg", "ws/attachment/restore"
	original := slimMessage(pdf)

	if err := blob.Put(ctx, key, bytes.NewReader(pdf), int64(len(pdf)), "application/pdf"); err != nil {
		t.Fatalf("staging: %v", err)
	}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if err := seedSlimOriginal(ctx, tx, sourceID, original); err != nil {
			return err
		}
		return seedSlimAttachment(ctx, tx, sourceID, key, int64(len(pdf)), slimDigest(pdf))
	}); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	store := capture.NewPartSlimStore(e.DB(), blob)
	swept, err := store.SlimBatch(ctx, 50)
	if err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	// Asserted before the round trip, because a restore of an UNSLIMMED payload
	// returns it unchanged — so without this the comparison below passes
	// whether or not the sweep did anything at all.
	if swept.Slimmed != 1 {
		t.Fatalf("slimmed %d rows, want 1; the round trip below would pass vacuously", swept.Slimmed)
	}
	if !partslim.IsSlimmed([]byte(payloadOf(t, e, sourceID))) {
		t.Fatal("the stored payload carries no stanza, so there is nothing to restore")
	}

	restored, err2 := partslim.RestoreStoredParts([]byte(payloadOf(t, e, sourceID)),
		func(ref partslim.PartRef) ([]byte, error) {
			body, _, err := blob.Get(ctx, ref.StorageKey)
			if err != nil {
				return nil, err
			}
			defer func() { _ = body.Close() }()
			return io.ReadAll(io.LimitReader(body, ref.Bytes+1))
		})
	if err2 != nil {
		t.Fatalf("restoring: %v", err2)
	}
	if !bytes.Equal(restored, original) {
		t.Errorf("the restored original is not what the provider sent: %d bytes vs %d",
			len(restored), len(original))
	}
}

// The export hands the subject the MESSAGE, not the reference to it.
//
// After the sweep the column holds a stanza naming an object. Art. 15 owes the
// message, so the assembly restores before disclosing — and when the object is
// gone it withholds that one payload while still LISTING the row, because the
// fact that an original is held is itself owed.
func TestTheExportRestoresASlimmedOriginal(t *testing.T) {
	e := Setup(t)
	ctx := context.Background()
	blob := blobstore.NewMemory()
	pdf := slimPDF(6)
	const sourceID, key = "sar-msg", "ws/attachment/sar"
	const subject = "restore@slim.test"

	if err := blob.Put(ctx, key, bytes.NewReader(pdf), int64(len(pdf)), "application/pdf"); err != nil {
		t.Fatalf("staging: %v", err)
	}

	// The original has to name the subject for the export's raw_capture arm to
	// match it, and carry an open activity for its content to be disclosable.
	original := bytes.Replace(slimMessage(pdf),
		[]byte("Subject: Quarterly figures"),
		[]byte("To: "+subject+"\r\nSubject: Quarterly figures"), 1)

	var person string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx,
			`INSERT INTO person (full_name, source, captured_by)
			 VALUES ('Restore Subject', 'manual', 'human:seed') RETURNING id`).Scan(&person); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO person_email (person_id, email, source, captured_by)
			 VALUES ($1, $2, 'manual', 'human:seed')`, person, subject); err != nil {
			return err
		}
		if err := seedSlimOriginal(ctx, tx, sourceID, original); err != nil {
			return err
		}
		if err := seedSlimAttachment(ctx, tx, sourceID, key,
			int64(len(pdf)), slimDigest(pdf)); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO activity (kind, subject, occurred_at, source, captured_by,
			                      source_system, source_id, audience)
			VALUES ('email', 'Quarterly figures', now(), 'capture', 'connector:test',
			        'email', $1, 'workspace')`, sourceID)
		return err
	}); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	swept, err := capture.NewPartSlimStore(e.DB(), blob).SlimBatch(ctx, 50)
	if err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	if swept.Slimmed != 1 {
		t.Fatalf("slimmed %d rows, want 1; the assertions below would pass vacuously", swept.Slimmed)
	}

	payloads, err := compose.ExportedRawCapturePayloadsForTest(
		e.Admin(), e.DB(), blob, personUUID(t, person))
	if err != nil {
		t.Fatalf("assembling the export: %v", err)
	}
	if len(payloads) != 1 {
		t.Fatalf("the export listed %d raw originals, want 1", len(payloads))
	}
	if strings.Contains(payloads[0], partslim.PartStoredHeader) {
		t.Errorf("the export disclosed the stanza rather than the message")
	}
	if !strings.Contains(payloads[0], slimWrap76(pdf)) {
		t.Errorf("the export did not restore the attachment's octets")
	}

	// The same export with the object GONE. The row must still be listed --
	// Art. 15 owes the fact that an original is held -- and its payload
	// withheld rather than handed over as a reference the subject cannot use.
	if err := blob.Delete(ctx, key); err != nil {
		t.Fatalf("removing the object: %v", err)
	}
	withheld, err := compose.ExportedRawCapturePayloadsForTest(
		e.Admin(), e.DB(), blob, personUUID(t, person))
	if err != nil {
		t.Fatalf("assembling the export with the object gone: %v", err)
	}
	if len(withheld) != 1 {
		t.Fatalf("the export listed %d raw originals with the object gone, want 1", len(withheld))
	}
	if withheld[0] != "" {
		t.Errorf("an unrestorable original was disclosed anyway: %.120s", withheld[0])
	}
}

// personUUID parses the id the seeding scanned as text.
func personUUID(t *testing.T, id string) ids.UUID {
	t.Helper()
	parsed, err := ids.Parse(id)
	if err != nil {
		t.Fatalf("parsing the seeded person id %q: %v", id, err)
	}
	return parsed
}
