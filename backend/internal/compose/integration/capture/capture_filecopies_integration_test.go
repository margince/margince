// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// One object per distinct file: identical bytes on several attachment rows are
// stored once, and an erasure takes the object only with its last row.

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

var signedContract = []byte("%PDF-1.4 the signed contract")

// putCountingBlob counts the objects put into it.
type putCountingBlob struct {
	blobstore.Store
	puts int
}

func (b *putCountingBlob) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	b.puts++
	return b.Store.Put(ctx, key, r, size, contentType)
}

// fileKeys answers the storage keys of the files a message landed with.
func fileKeys(t *testing.T, e *integration.SearchEnv, msgID string) []string {
	t.Helper()
	var keys []string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(),
			`SELECT storage_key FROM attachment WHERE activity_id = $1 ORDER BY external_part_id`, activityIDOf(t, e, msgID))
		if err != nil {
			return err
		}
		keys, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	}); err != nil {
		t.Fatalf("reading the files of %s: %v", msgID, err)
	}
	return keys
}

// captureSeat is the mailbox seat's own authority to create what it captures.
func captureSeat(e *integration.SearchEnv) context.Context {
	return humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead})
}

func eraseMessage(t *testing.T, e *integration.SearchEnv, blob blobstore.Store, msgID string) error {
	t.Helper()
	return eraseActivity(e, blob, activityIDOf(t, e, msgID))
}

// eraseActivity runs the activity content erasure, safe to call off the test
// goroutine.
func eraseActivity(e *integration.SearchEnv, blob blobstore.Store, activity ids.UUID) error {
	eraser := privacy.NewEraser(e.DB()).WithBlobstore(blob).WithRawCapturePurger(compose.RawCapturePurgerFor(e.DB()))
	return database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return eraser.PurgeContentDerivedFromNoiseTx(e.Admin(), tx, []ids.UUID{activity})
	})
}

func TestIdenticalFilesInSeveralMessagesAreStoredAsOneObject(t *testing.T) {
	blob := &putCountingBlob{Store: blobstore.NewMemory()}
	env := newCaptureEnvWithBlob(t, captureNow, blob)
	env.sync(t,
		emailWithFiles("her@globex.example", captureOwner, "copies-a@globex.example", "", "contract.pdf", signedContract, signedContract),
		emailWithFile("him@globex.example", captureOwner, "copies-b@globex.example", "", "contract.pdf", signedContract))

	keys := append(fileKeys(t, env.e, "copies-a@globex.example"), fileKeys(t, env.e, "copies-b@globex.example")...)
	if len(keys) != 3 {
		t.Fatalf("stored %d attachment rows, want 3", len(keys))
	}
	for _, key := range keys {
		if key == "" || key != keys[0] {
			t.Fatalf("rows name keys %v; identical files share one object", keys)
		}
	}
	if blob.puts != 1 {
		t.Errorf("put %d objects for one distinct file, want 1", blob.puts)
	}
}

func TestAnErasureKeepsAnObjectAnotherRowStillNamesAndTakesItWithTheLastRow(t *testing.T) {
	blob := blobstore.NewMemory()
	env := newCaptureEnvWithBlob(t, captureNow, blob)
	env.sync(t,
		emailWithFile("her@globex.example", captureOwner, "erase-a@globex.example", "", "contract.pdf", signedContract),
		emailWithFile("him@globex.example", captureOwner, "erase-b@globex.example", "", "contract.pdf", signedContract))
	key := fileKeys(t, env.e, "erase-a@globex.example")[0]

	if err := eraseMessage(t, env.e, blob, "erase-a@globex.example"); err != nil {
		t.Fatalf("erasing the first message: %v", err)
	}
	if _, _, err := blob.Get(context.Background(), key); err != nil {
		t.Fatalf("erasing one of two rows deleted the object the other row still names: %v", err)
	}
	if err := eraseMessage(t, env.e, blob, "erase-b@globex.example"); err != nil {
		t.Fatalf("erasing the second message: %v", err)
	}
	if _, _, err := blob.Get(context.Background(), key); !errors.Is(err, blobstore.ErrNotFound) {
		t.Fatalf("erasing the last row left its object: err = %v, want ErrNotFound", err)
	}
}

func TestAnErasureWaitingOnACaptureThatReusesItsObjectKeepsTheObject(t *testing.T) {
	blob := blobstore.NewMemory()
	env := newCaptureEnvWithBlob(t, captureNow, blob)
	e := env.e
	env.sync(t,
		emailWithFile("her@globex.example", captureOwner, "race-a@globex.example", "", "contract.pdf", signedContract),
		emailWithFile("him@globex.example", captureOwner, "race-b@globex.example", "", "other.pdf", []byte("%PDF-1.4 another document")))
	key := fileKeys(t, e, "race-a@globex.example")[0]
	store := activities.NewStore(compose.InstallationDB(e.Pool)).WithBlobstore(blob)
	staged, err := store.StageCapturedFiles(captureSeat(e), []activities.CapturedFile{{PartID: "part:9", Filename: "contract.pdf", Body: signedContract}})
	if err != nil || len(staged) != 1 {
		t.Fatalf("staging the copy: %d staged, %v", len(staged), err)
	}
	first, second := activityIDOf(t, e, "race-a@globex.example"), ids.From[ids.ActivityKind](activityIDOf(t, e, "race-b@globex.example"))
	erased := make(chan error, 1)
	// The copy holds the first row `FOR SHARE` until it commits. The erasure
	// waits on that lock, then must still see the copy's new row.
	err = database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if err := store.RecordCapturedFiles(captureSeat(e), tx, second, activities.CapturedFileSource{
			System: connector.EmailSourceSystem, MessageID: "race-b@globex.example", CapturedBy: "connector:gmail", Category: "email_attachment",
		}, staged); err != nil {
			return err
		}
		go func() { erased <- eraseActivity(e, blob, first) }()
		waitForALockWait(t, e)
		return nil
	})
	if err != nil {
		t.Fatalf("recording the reused copy: %v", err)
	}
	if err := <-erased; err != nil {
		t.Fatalf("erasing the first message: %v", err)
	}
	if _, _, err := blob.Get(context.Background(), key); err != nil {
		t.Fatalf("the erasure deleted an object a copy committed while it waited still names: %v", err)
	}
}

func TestAReusedCopyWhoseRowWentAwayBeforeItsOwnRowIsRefusedForARetry(t *testing.T) {
	blob := blobstore.NewMemory()
	env := newCaptureEnvWithBlob(t, captureNow, blob)
	e := env.e
	env.sync(t,
		emailWithFile("her@globex.example", captureOwner, "gone-a@globex.example", "", "contract.pdf", signedContract),
		emailWithFile("him@globex.example", captureOwner, "gone-b@globex.example", "", "other.pdf", []byte("%PDF-1.4 another document")))
	store := activities.NewStore(compose.InstallationDB(e.Pool)).WithBlobstore(blob)
	staged, err := store.StageCapturedFiles(captureSeat(e), []activities.CapturedFile{{PartID: "part:9", Filename: "contract.pdf", Body: signedContract}})
	if err != nil || len(staged) != 1 {
		t.Fatalf("staging the copy: %d staged, %v", len(staged), err)
	}
	first, second := activityIDOf(t, e, "gone-a@globex.example"), ids.From[ids.ActivityKind](activityIDOf(t, e, "gone-b@globex.example"))
	err = database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `DELETE FROM attachment WHERE activity_id = $1`, first); err != nil {
			return err
		}
		return store.RecordCapturedFiles(captureSeat(e), tx, second, activities.CapturedFileSource{
			System: connector.EmailSourceSystem, MessageID: "gone-b@globex.example", CapturedBy: "connector:gmail", Category: "email_attachment",
		}, staged)
	})
	if !errors.Is(err, activities.ErrReusedFileGone) {
		t.Fatalf("recording a row for an object no row holds any more: err = %v, want ErrReusedFileGone", err)
	}
}

func TestAFileWhoseOnlyCopyWasWithheldIsStoredAfresh(t *testing.T) {
	blob := blobstore.NewMemory()
	env := newCaptureEnvWithBlob(t, captureNow, blob)
	e := env.e
	env.sync(t, emailWithFile("her@globex.example", captureOwner, "withheld-a@globex.example", "", "contract.pdf", signedContract))
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE attachment SET storage_key = '', checksum = NULL, bytes_withheld = true
		                                          WHERE activity_id = $1`, activityIDOf(t, e, "withheld-a@globex.example"))
		return err
	}); err != nil {
		t.Fatalf("withholding the first copy: %v", err)
	}
	env.sync(t, emailWithFile("him@globex.example", captureOwner, "withheld-b@globex.example", "", "contract.pdf", signedContract))
	keys := fileKeys(t, e, "withheld-b@globex.example")
	if len(keys) != 1 || keys[0] == "" {
		t.Fatalf("the second message stored %v", keys)
	}
	if _, _, err := blob.Get(context.Background(), keys[0]); err != nil {
		t.Fatalf("the second message names an object that is not there: %v", err)
	}
}
