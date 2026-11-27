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
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

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
	ctx := context.Background()
	company, db, _ := seedCompanyDB(t)
	blob := blobstore.NewMemory()
	got, ref := storeOriginal(t, blob, company)
	read, err := originalstore.Read(originalstore.Bind(ctx, db, blob), ref)
	if err != nil || !bytes.Equal(read, got) {
		t.Fatalf("an original stored under the company id was lost: %q, %v", read, err)
	}
}

// An original under an archived company's id was refused while the database
// bound the live company, and it still is.
func TestAnArchivedCompanysOriginalIsStillRefused(t *testing.T) {
	ctx := context.Background()
	_, db, _ := seedCompanyDB(t)
	blob := blobstore.NewMemory()
	_, ref := storeOriginal(t, blob, ids.NewV7())
	if _, err := originalstore.Read(originalstore.Bind(ctx, db, blob), ref); err == nil {
		t.Fatal("an original under another company's id was read")
	}
}

// The id is read when the store is bound. A read inside an open transaction on
// a one-connection pool then needs no second connection.
func TestAnOriginalReadInsideAnOpenTransactionTakesNoSecondConnection(t *testing.T) {
	company, db, pool := seedCompanyDB(t)
	blob := blobstore.NewMemory()
	payload, ref := storeOriginal(t, blob, company)
	ctx := originalstore.Bind(context.Background(), db, blob)

	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	tx, err := pool.Begin(waitCtx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(context.Background()); err != nil {
			t.Errorf("rolling back: %v", err)
		}
	}()
	got, err := originalstore.Read(waitCtx, ref)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("a read inside an open transaction: %q, %v", got, err)
	}
}

func seedCompanyDB(t *testing.T) (ids.UUID, *database.DB, *pgxpool.Pool) {
	t.Helper()
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
	cfg, err := pgxpool.ParseConfig(appDSN)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 1
	pool, err := testdb.OwnPoolFromConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	db := database.Bind(pool, func(context.Context) (ids.WorkspaceID, error) {
		return ids.From[ids.WorkspaceKind](company), nil
	})
	return company, db, pool
}

// storeOriginal puts a payload into blob under company's raw-original prefix, as
// the store wrote it before the id moved, and answers the reference to it.
func storeOriginal(t *testing.T, blob blobstore.Store, company ids.UUID) ([]byte, originalstore.Reference) {
	t.Helper()
	payload := []byte(`{"message":"stored before"}`)
	sum := sha256.Sum256(payload)
	key := blobstore.WorkspaceKey(ids.From[ids.WorkspaceKind](company), "raw-original", ids.NewV7().String())
	if err := blob.Put(context.Background(), key, bytes.NewReader(payload), int64(len(payload)), "application/json"); err != nil {
		t.Fatal(err)
	}
	return payload, originalstore.Reference{Key: key, SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(payload))}
}
