// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/originalstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type incomingOriginal struct {
	pid int
	id  ids.UUID
	ref *originalstore.Reference
	err error
}

func TestErasureSeesTheDeliveryWhoseSubjectLockItWaitedFor(t *testing.T) {
	for _, size := range []int{1, originalstore.InlineLimit} {
		t.Run(map[bool]string{true: "external", false: "inline"}[size == originalstore.InlineLimit], func(t *testing.T) {
			e := integration.Setup(t)
			email := "incoming-original@example.test"
			contact := seedContactWithAddress(t, e, "Incoming subject", email)
			blob := blobstore.NewMemory()
			ctx := originalstore.Bind(e.Admin(), e.DB(), blob)
			release := make(chan struct{})
			finish := sync.OnceFunc(func() { close(release) })
			defer finish()
			ready, stored := make(chan incomingOriginal, 1), make(chan error, 1)
			go func() {
				stored <- e.DB().Tx(ctx, func(tx pgx.Tx) error {
					if err := storekit.LockSubjectKeys(ctx, tx, nil, []string{email}); err != nil {
						ready <- incomingOriginal{err: err}
						return err
					}
					payload, err := json.Marshal(map[string]string{"email": email, "body": strings.Repeat("x", size)})
					if err != nil {
						ready <- incomingOriginal{err: err}
						return err
					}
					id, err := capture.InsertRawCaptureTx(ctx, tx, capture.RawRecord{SourceSystem: "fixture", SourceID: "incoming", Payload: payload})
					if err != nil {
						ready <- incomingOriginal{err: err}
						return err
					}
					var ref *originalstore.Reference
					if err := tx.QueryRow(ctx, `SELECT payload_object FROM raw_capture WHERE id=@id`, pgx.NamedArgs{"id": id}).Scan(&ref); err != nil {
						ready <- incomingOriginal{err: err}
						return err
					}
					ready <- incomingOriginal{pid: backendPID(t, tx), id: id, ref: ref}
					<-release
					return nil
				})
			}()
			incoming := <-ready
			if incoming.err != nil {
				t.Fatal(incoming.err)
			}
			erased := make(chan error, 1)
			go func() {
				erased <- privacy.NewEraser(e.DB()).WithBlobstore(blob).EraseContact(ctx, contact, "subject request")
			}()
			if err := e.DB().Tx(ctx, func(tx pgx.Tx) error { waitForBackendBlockedBy(t, tx, incoming.pid, erased); return nil }); err != nil {
				t.Fatal(err)
			}
			finish()
			if err := <-stored; err != nil {
				t.Fatal(err)
			}
			if err := <-erased; err != nil {
				t.Fatal(err)
			}
			if n := e.WsCount(t, `SELECT count(*) FROM raw_capture WHERE id=$1`, incoming.id); n != 0 {
				t.Fatal("erasure missed the delivery admitted before it obtained the subject key")
			}
			if incoming.ref != nil {
				if _, _, err := blob.Get(ctx, incoming.ref.Key); !errors.Is(err, blobstore.ErrNotFound) {
					t.Fatal("erasure retained incoming evidence", err)
				}
			}
		})
	}
}
