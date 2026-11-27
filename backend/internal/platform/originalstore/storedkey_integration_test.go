// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package originalstore_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/originalstore"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// An original stored under the company id before the id moved into
// installation_state is still found, by a caller whose context binds no
// workspace.
func TestAnOriginalStoredUnderTheCompanyIdIsStillFound(t *testing.T) {
	ownerDSN, appDSN := os.Getenv("MARGINCE_TEST_DSN"), os.Getenv("MARGINCE_TEST_APP_DSN")
	if ownerDSN == "" || appDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN / MARGINCE_TEST_APP_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing owner connection: %v", err)
		}
	})
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if err := testdb.Reset(ctx, owner); err != nil {
		t.Fatal(err)
	}
	company := ids.NewV7()
	if _, err := owner.Exec(ctx, `INSERT INTO workspace (id) VALUES ($1)`, company); err != nil {
		t.Fatal(err)
	}
	if err := storekit.RecordInstallationTx(ctx, owner, company, clocktest.Now(t)); err != nil {
		t.Fatal(err)
	}
	pool, err := testdb.OwnPool(ctx, appDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	payload := []byte(`{"message":"stored before"}`)
	sum := sha256.Sum256(payload)
	blob := blobstore.NewMemory()
	oldKey := blobstore.WorkspaceKey(ids.From[ids.WorkspaceKind](company), "raw-original", ids.NewV7().String())
	if err := blob.Put(ctx, oldKey, bytes.NewReader(payload), int64(len(payload)), "application/json"); err != nil {
		t.Fatal(err)
	}
	db := database.Bind(pool, func(context.Context) (ids.WorkspaceID, error) {
		return ids.From[ids.WorkspaceKind](company), nil
	})

	got, err := originalstore.Read(originalstore.Bind(ctx, db, blob),
		originalstore.Reference{Key: oldKey, SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(payload))})
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("an original stored under the company id was lost: %q, %v", got, err)
	}
}
