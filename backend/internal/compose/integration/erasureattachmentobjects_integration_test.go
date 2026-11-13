// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// failingCopyScrub fails the erasure at a step after its attachment step.
type failingCopyScrub struct{ privacy.RawCapturePurger }

func (failingCopyScrub) ScrubDeliveryCopiesTx(context.Context, pgx.Tx, string, []ids.UUID) error {
	return errors.New("the erasure failed after its attachment step")
}

// uploadSubjectFile stores a file on the contact through the upload writer
// and answers its object key.
func uploadSubjectFile(t *testing.T, e *Env, blob blobstore.Store, contact ids.UUID) (ids.UUID, string) {
	t.Helper()
	att, err := e.Activities.WithBlobstore(blob).UploadAttachment(e.Admin(), activities.AttachmentInput{
		EntityType: "contact", EntityID: contact, Filename: "passport.pdf", Content: bytes.NewReader([]byte("subject bytes")),
	})
	if err != nil {
		t.Fatalf("UploadAttachment: %v", err)
	}
	return ids.UUID(att.Id), blobstore.WorkspaceKey(ids.From[ids.WorkspaceKind](e.WS), "attachment", att.Id.String())
}

// An erasure that fails after its attachment step rolls the queue entry back
// with the row delete. The row comes back with its bytes, and nothing is
// queued for the reap to delete.
func TestAFailedErasureKeepsTheAttachmentObjectItsRowNames(t *testing.T) {
	e := Setup(t)
	blob := blobstore.NewMemory()
	contact := e.SeedContact(t, "Failed erasure", &e.Rep1)
	file, key := uploadSubjectFile(t, e, blob, contact)

	eraser := compose.ComposedEraser(e.Pool, blob, nil).
		WithRawCapturePurger(failingCopyScrub{compose.RawCapturePurgerFor(e.DB())})
	if err := eraser.EraseContact(e.Admin(), contact, "subject request"); err == nil {
		t.Fatal("the failure after the attachment step was hidden")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM attachment WHERE id = $1 AND storage_key = $2`, file, key); got != 1 {
		t.Fatal("the rollback did not bring the attachment row back")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM stored_object_intent WHERE storage_key = $1`, key); got != 0 {
		t.Fatalf("a rolled-back erasure left %d deletion entries for an object its row still names", got)
	}
	if _, _, err := blob.Get(e.Admin(), key); err != nil {
		t.Fatalf("the row came back pointing at deleted bytes: %v", err)
	}
}
