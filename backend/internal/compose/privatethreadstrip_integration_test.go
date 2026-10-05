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
	if rc, _, err := blob.Get(ctx, key); err == nil {
		if cerr := rc.Close(); cerr != nil {
			t.Errorf("closing: %v", cerr)
		}
		t.Error("the file's object is still in the store")
	}
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
