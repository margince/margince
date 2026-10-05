// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A thread judged personal after its first messages arrived: once the undo
// window closes, those messages' files are withheld as if the verdict had been
// there at capture — the object deleted, the row kept by name, the bytes cut
// out of the stored original.

import (
	"bytes"
	"context"
	"encoding/base64"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// captureBeforeTheVerdict lands a message with a file on a thread nobody has
// judged yet, so the file is stored, and answers the object's key.
func captureBeforeTheVerdict(ctx context.Context, t *testing.T, db *database.DB, blob blobstore.Store, sourceID, thread, address string) string {
	t.Helper()
	sink := capture.NewSink(db).WithFileKeeper(fileKeeper(db.Pool(), blob))
	rec := withAttachedOriginal(withFiles(fromAddress(mailRecord(sourceID), address), onePDF()))
	rec.ThreadKey = thread
	if _, err := sink.Upsert(ctx, rec); err != nil {
		t.Fatalf("capturing before the verdict: %v", err)
	}
	files := filesFor(ctx, t, db, sourceID)
	if len(files) != 1 || files[0].storageKey == "" {
		t.Fatalf("the fixture stored %+v, want one file with an object", files)
	}
	return files[0].storageKey
}

// judgeThreadPersonal records the seat's own personal verdict on the thread,
// resolved `ago` back, and moves the message's capture back with it, so the
// window is measured as it would be in a mailbox that has lived with the
// verdict that long.
func judgeThreadPersonal(ctx context.Context, t *testing.T, db *database.DB, sourceID, thread, seen, ago string) {
	t.Helper()
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO capture_thread_verdict (thread_key, user_id, status, kind, seen_addresses, resolved_at)
			VALUES ($1, $2, 'held_by_owner', 'personal', ARRAY[$3], now() - $4::interval)`,
			thread, captureSeatID, seen, ago); err != nil {
			return err
		}
		_, err := tx.Exec(ctx,
			`UPDATE activity SET created_at = now() - $2::interval WHERE source_id = $1`, sourceID, ago)
		return err
	}); err != nil {
		t.Fatalf("judging the thread personal: %v", err)
	}
}

func TestAPersonalThreadsEarlierFilesAreWithheldOnceItsWindowCloses(t *testing.T) {
	ctx, db, tag := captureWorkspace(t)
	blob := blobstore.NewMemory()
	address := "thread-" + tag + "@example.com"
	source, thread := "msg-early-"+tag, "thread-"+tag
	key := captureBeforeTheVerdict(ctx, t, db, blob, source, thread, address)
	judgeThreadPersonal(ctx, t, db, source, thread, address, "8 days")

	if _, err := privateThreadStripperFor(db.Pool(), blob).StripWorkspace(ctx, capture.DefaultPersonalPurgeWindows()); err != nil {
		t.Fatalf("StripWorkspace: %v", err)
	}

	requireNamedWithoutBytes(t, filesFor(ctx, t, db, source))
	requireQueuedForDeletion(ctx, t, db, key)
	encoded := base64.StdEncoding.EncodeToString(onePDF().Body)
	if raw := storedOriginal(ctx, t, db, source); bytes.Contains(raw, []byte(encoded)) {
		t.Error("the stored original still carries the file's bytes")
	}
}

// Inside the window nothing is touched: the verdict can still be overruled, and
// a withheld file cannot come back.
func TestAPersonalThreadsFilesStayWhileItsWindowIsOpen(t *testing.T) {
	ctx, db, tag := captureWorkspace(t)
	blob := blobstore.NewMemory()
	address := "thread-" + tag + "@example.com"
	source, thread := "msg-recent-"+tag, "thread-"+tag
	captureBeforeTheVerdict(ctx, t, db, blob, source, thread, address)
	judgeThreadPersonal(ctx, t, db, source, thread, address, "1 day")

	if _, err := privateThreadStripperFor(db.Pool(), blob).StripWorkspace(ctx, capture.DefaultPersonalPurgeWindows()); err != nil {
		t.Fatalf("StripWorkspace: %v", err)
	}
	if files := filesFor(ctx, t, db, source); len(files) != 1 || files[0].withheld {
		t.Fatalf("files = %+v, want the file still stored inside the window", files)
	}
}

// A message from an address the verdict never saw is not the conversation it
// judged, so its file stays.
func TestAPersonalThreadVerdictDoesNotReachASenderItNeverSaw(t *testing.T) {
	ctx, db, tag := captureWorkspace(t)
	blob := blobstore.NewMemory()
	address := "thread-" + tag + "@example.com"
	source, thread := "msg-stranger-"+tag, "thread-"+tag
	captureBeforeTheVerdict(ctx, t, db, blob, source, thread, address)
	judgeThreadPersonal(ctx, t, db, source, thread, "somebody-else-"+tag+"@example.com", "8 days")

	if _, err := privateThreadStripperFor(db.Pool(), blob).StripWorkspace(ctx, capture.DefaultPersonalPurgeWindows()); err != nil {
		t.Fatalf("StripWorkspace: %v", err)
	}
	if files := filesFor(ctx, t, db, source); len(files) != 1 || files[0].withheld {
		t.Fatalf("files = %+v, want the file kept for a sender the verdict never saw", files)
	}
}

// An owner who shares the thread after all has overruled the verdict, and its
// files stay however long ago the verdict was.
func TestAnOverruledPersonalThreadKeepsItsFiles(t *testing.T) {
	ctx, db, tag := captureWorkspace(t)
	blob := blobstore.NewMemory()
	address := "thread-" + tag + "@example.com"
	source, thread := "msg-shared-"+tag, "thread-"+tag
	captureBeforeTheVerdict(ctx, t, db, blob, source, thread, address)
	judgeThreadPersonal(ctx, t, db, source, thread, address, "40 days")
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE capture_thread_verdict SET status = 'shared_by_owner' WHERE thread_key = $1`, thread)
		return err
	}); err != nil {
		t.Fatalf("sharing the thread: %v", err)
	}

	if _, err := privateThreadStripperFor(db.Pool(), blob).StripWorkspace(ctx, capture.DefaultPersonalPurgeWindows()); err != nil {
		t.Fatalf("StripWorkspace: %v", err)
	}
	if files := filesFor(ctx, t, db, source); len(files) != 1 || files[0].withheld {
		t.Fatalf("files = %+v, want the file kept on a thread its owner shared", files)
	}
}

// requireQueuedForDeletion holds that the object is left to the stored-object
// reaper: declared provisional, and named by no row, which is the pair the
// reaper deletes on. Deleting it inside the strip's transaction would leave a
// row naming a missing object whenever the transaction then failed.
func requireQueuedForDeletion(ctx context.Context, t *testing.T, db *database.DB, key string) {
	t.Helper()
	var queued, named int
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM stored_object_intent WHERE storage_key = $1`, key).Scan(&queued); err != nil {
			return err
		}
		return tx.QueryRow(ctx,
			`SELECT count(*) FROM attachment WHERE storage_key = $1`, key).Scan(&named)
	}); err != nil {
		t.Fatalf("reading the deletion queue: %v", err)
	}
	if queued != 1 || named != 0 {
		t.Errorf("object %s: queued=%d, named by %d row(s); want queued for the reaper and named by none", key, queued, named)
	}
}

// Archiving hides an attachment and keeps its object, so an archived file on a
// personal thread is withheld like any other, and stays archived.
func TestAnArchivedFileOnAPersonalThreadIsWithheldToo(t *testing.T) {
	ctx, db, tag := captureWorkspace(t)
	blob := blobstore.NewMemory()
	address := "thread-" + tag + "@example.com"
	source, thread := "msg-archived-"+tag, "thread-"+tag
	key := captureBeforeTheVerdict(ctx, t, db, blob, source, thread, address)
	judgeThreadPersonal(ctx, t, db, source, thread, address, "8 days")
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE attachment SET archived_at = now() WHERE storage_key = $1`, key)
		return err
	}); err != nil {
		t.Fatalf("archiving the file: %v", err)
	}

	if _, err := privateThreadStripperFor(db.Pool(), blob).StripWorkspace(ctx, capture.DefaultPersonalPurgeWindows()); err != nil {
		t.Fatalf("StripWorkspace: %v", err)
	}
	requireNamedWithoutBytes(t, filesFor(ctx, t, db, source))
	requireQueuedForDeletion(ctx, t, db, key)
}

// The scan's answer goes stale the moment its transaction ends. Asked again
// inside the strip's own transaction, a thread shared back in between is no
// longer due.
func TestTheRecheckSeesAThreadSharedAfterTheScan(t *testing.T) {
	ctx, db, tag := captureWorkspace(t)
	blob := blobstore.NewMemory()
	address := "thread-" + tag + "@example.com"
	source, thread := "msg-recheck-"+tag, "thread-"+tag
	captureBeforeTheVerdict(ctx, t, db, blob, source, thread, address)
	judgeThreadPersonal(ctx, t, db, source, thread, address, "8 days")
	strip := principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem, ID: "system:private-thread-strip"})

	ask := func() bool {
		t.Helper()
		var due bool
		if err := db.Tx(strip, func(tx pgx.Tx) error {
			var id ids.UUID
			if err := tx.QueryRow(strip, `SELECT id FROM activity WHERE source_id = $1`, source).Scan(&id); err != nil {
				return err
			}
			var err error
			due, err = capture.PrivateThreadFilesStillDueTx(strip, tx, capture.DefaultPersonalPurgeWindows(), statutoryFloor(), id)
			return err
		}); err != nil {
			t.Fatalf("rechecking: %v", err)
		}
		return due
	}
	if !ask() {
		t.Fatal("a message past its window was not due — the recheck refuses everything")
	}
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE capture_thread_verdict SET status = 'shared_by_owner' WHERE thread_key = $1`, thread)
		return err
	}); err != nil {
		t.Fatalf("sharing the thread: %v", err)
	}
	if ask() {
		t.Fatal("a thread shared back after the scan was still due")
	}
}

// A file whose object is already gone is withheld all the same: there is
// nothing left to cut out by its bytes, so the original keeps only its
// headers, and the message behind it in the queue is not held up.
func TestAFileWhoseObjectIsGoneIsStillWithheld(t *testing.T) {
	ctx, db, tag := captureWorkspace(t)
	blob := blobstore.NewMemory()
	address := "thread-" + tag + "@example.com"
	source, thread := "msg-gone-"+tag, "thread-"+tag
	key := captureBeforeTheVerdict(ctx, t, db, blob, source, thread, address)
	judgeThreadPersonal(ctx, t, db, source, thread, address, "8 days")
	if err := blob.Delete(ctx, key); err != nil {
		t.Fatalf("losing the object: %v", err)
	}

	if _, err := privateThreadStripperFor(db.Pool(), blob).StripWorkspace(ctx, capture.DefaultPersonalPurgeWindows()); err != nil {
		t.Fatalf("StripWorkspace: %v", err)
	}
	requireNamedWithoutBytes(t, filesFor(ctx, t, db, source))
	encoded := base64.StdEncoding.EncodeToString(onePDF().Body)
	if raw := storedOriginal(ctx, t, db, source); bytes.Contains(raw, []byte(encoded)) {
		t.Error("the stored original still carries the file's bytes")
	}
}
