// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func updaterCtx(update bool) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:imap",
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"activity": {Read: true, Update: update}},
			RowScope: principal.RowScopeAll,
		},
	})
}

// The withholding writer refuses a caller without update, and a store with no
// object store refuses to read bytes it cannot reach; neither touches a row.
func TestWithholdingStoredFilesRefusesBeforeItTouchesARow(t *testing.T) {
	store := &Store{}
	if _, err := store.StoredFilesOfMessageTx(updaterCtx(false), nil, ids.NewV7()); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("read without update: %v, want ErrPermissionDenied", err)
	}
	if _, err := store.StoredFilesOfMessageTx(updaterCtx(true), nil, ids.NewV7()); !errors.Is(err, ErrBlobstoreUnconfigured) {
		t.Errorf("read with no object store: %v, want ErrBlobstoreUnconfigured", err)
	}
	if err := store.WithholdStoredFilesTx(updaterCtx(false), nil, []StoredMessageFile{{ID: ids.NewV7(), Key: "k"}}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("withhold without update: %v, want ErrPermissionDenied", err)
	}
}

// An object already gone reads as no bytes rather than an error, so one lost
// object cannot stop the strip; an object still there reads whole.
func TestReadingAGoneObjectAnswersNoBytes(t *testing.T) {
	blob := blobstore.NewMemory()
	store := (&Store{}).WithBlobstore(blob)
	ctx := context.Background()
	if body, err := store.readObject(ctx, "ws/attachment/gone"); err != nil || body != nil {
		t.Errorf("a gone object: %q, %v; want no bytes and no error", body, err)
	}
	if err := blob.Put(ctx, "ws/attachment/here", bytes.NewReader([]byte("%PDF")), 4, "application/pdf"); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	if body, err := store.readObject(ctx, "ws/attachment/here"); err != nil || string(body) != "%PDF" {
		t.Errorf("a present object: %q, %v; want its bytes", body, err)
	}
}
