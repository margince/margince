// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// archivingStore stores the bytes, then archives the deal they are for: the
// race in which the record goes while its upload is still writing.
type archivingStore struct {
	blobstore.Store
	archive func()
}

func (s archivingStore) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	if err := s.Store.Put(ctx, key, r, size, contentType); err != nil {
		return err
	}
	s.archive()
	return nil
}

// An attachment's key stops being provisional in the transaction that gives it
// a row. The stored-object sweep then never spends a pass on a settled file. When
// that transaction fails the key stays provisional, and the sweep finds its
// bytes as the orphan they are.
func TestAnAttachmentClearsItsIntentOnlyWithItsRow(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	pipeline, stage, _ := integration.DealFixture(t, e)
	deal := e.SeedDeal(t, "Fleet renewal", pipeline, stage, &e.Rep1)
	intents := attachmentIntents(e)
	upload := func(store *activities.Store) error {
		_, err := store.UploadAttachment(ctx, activities.AttachmentInput{
			EntityType: "deal", EntityID: deal, Filename: "terms.txt", ContentType: "text/plain",
			Content: strings.NewReader("The renewal terms."),
		})
		return err
	}

	if err := upload(activities.NewStore(e.DB()).WithBlobstore(blobstore.NewMemory())); err != nil {
		t.Fatalf("uploading an attachment: %v", err)
	}
	if got := intents(t); got != 0 {
		t.Errorf("%d attachment intents remain after the upload committed, want 0", got)
	}

	racing := archivingStore{Store: blobstore.NewMemory(), archive: func() {
		e.WsExec(t, `UPDATE deal SET archived_at = now() WHERE id = $1`, deal)
	}}
	if err := upload(activities.NewStore(e.DB()).WithBlobstore(racing)); err == nil {
		t.Fatal("an upload whose deal was archived while it wrote was accepted")
	}
	if got := intents(t); got != 1 {
		t.Errorf("%d attachment intents remain after the upload was refused, want 1 for its stored bytes", got)
	}
}

// A captured file clears its key on the capture's own transaction. A capture
// that rolls back leaves the key provisional; one that commits settles it.
func TestACapturedFileClearsItsIntentOnlyWithItsRow(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	intents := attachmentIntents(e)
	store := activities.NewStore(e.DB()).WithBlobstore(blobstore.NewMemory())
	contact := e.SeedContact(t, "Ines Duarte", &e.Rep1)
	subject := "Re: renewal terms"
	logged, _, err := store.LogActivity(ctx, activities.LogActivityInput{
		Kind: "email", Subject: &subject, Direction: new("inbound"),
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("logging the mail: %v", err)
	}
	activityID := ids.From[ids.ActivityKind](ids.UUID(logged.Id))
	staged, err := store.StageCapturedFiles(ctx, []activities.CapturedFile{{
		PartID: "1", Filename: "terms.pdf", Body: []byte("%PDF-1.7 terms"), ContentType: "application/pdf",
	}})
	if err != nil || len(staged) != 1 {
		t.Fatalf("staging the captured file: %d staged, %v", len(staged), err)
	}
	captureFails := errors.New("the capture failed after recording its files")
	record := func(after error) error {
		return database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			if err := store.RecordCapturedFiles(ctx, tx, activityID, activities.CapturedFileSource{
				System: "imap", MessageID: "renewal-" + activityID.String(),
				CapturedBy: "connector:imap", Category: "email_attachment",
			}, staged); err != nil {
				return err
			}
			return after
		})
	}

	if err := record(captureFails); !errors.Is(err, captureFails) {
		t.Fatalf("a failing capture returned %v, want its own failure", err)
	}
	if got := intents(t); got != 1 {
		t.Errorf("%d attachment intents remain after the capture rolled back, want 1 for its stored bytes", got)
	}
	if err := record(nil); err != nil {
		t.Fatalf("recording the captured file: %v", err)
	}
	if got := intents(t); got != 0 {
		t.Errorf("%d attachment intents remain after the capture committed, want 0", got)
	}
}

func attachmentIntents(e *integration.Env) func(*testing.T) int {
	return func(t *testing.T) int {
		t.Helper()
		return e.WsCount(t, `SELECT count(*) FROM stored_object_intent WHERE kind = 'attachment'`)
	}
}
