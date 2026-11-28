// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package keyvault_test

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The vault a role boots is pinned to the installation's stored-key id. Its
// first read inside an open transaction must not wait for a second connection.
// The pool holds one, so a vault that resolved the id lazily would wait forever.
func TestAPinnedVaultReadsInsideAnOpenTransactionOnAOneConnectionPool(t *testing.T) {
	pool := singleConnPool(t)
	key := rootKey(t)
	company := ids.From[ids.WorkspaceKind](ids.NewV7())
	if err := storekit.RecordInstallationTx(t.Context(), pool, company.UUID, clocktest.Now(t)); err != nil {
		t.Fatal(err)
	}
	sealing, err := keyvault.New(keyvault.Config{RootKey: key, Pool: pool})
	if err != nil {
		t.Fatal(err)
	}
	ref, err := sealing.Put(t.Context(), company, []byte("sealed-before"))
	if err != nil {
		t.Fatal(err)
	}
	vault, _, err := keyvault.FromEnv(t.Context(), pool, config.Static(map[string]string{
		keyvault.EnvRootKey: base64.StdEncoding.EncodeToString(key),
	}))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(context.Background()); err != nil {
			t.Errorf("rolling back: %v", err)
		}
	}()
	got, err := vault.GetOn(ctx, tx, ids.From[ids.WorkspaceKind](ids.NewV7()), ref)
	if err != nil || string(got) != "sealed-before" {
		t.Fatalf("a vault read inside an open transaction: %q, %v", got, err)
	}
}

// A credential sealed under an archived company's id stays out of reach of a
// live caller, as it was before the id was pinned. An erasure pass can still
// destroy its ciphertext, because deleting needs no read access.
func TestAnArchivedCompanysCredentialIsUnreadableButStillErasable(t *testing.T) {
	pool := singleConnPool(t)
	key := rootKey(t)
	live := ids.From[ids.WorkspaceKind](ids.NewV7())
	archived := ids.From[ids.WorkspaceKind](ids.NewV7())
	if err := storekit.RecordInstallationTx(t.Context(), pool, live.UUID, clocktest.Now(t)); err != nil {
		t.Fatal(err)
	}
	sealing, err := keyvault.New(keyvault.Config{RootKey: key, Pool: pool})
	if err != nil {
		t.Fatal(err)
	}
	predecessorRef, err := sealing.Put(t.Context(), archived, []byte("their mailbox"))
	if err != nil {
		t.Fatal(err)
	}
	vault, _, err := keyvault.FromEnv(t.Context(), pool, config.Static(map[string]string{
		keyvault.EnvRootKey: base64.StdEncoding.EncodeToString(key),
	}))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := sealing.Get(t.Context(), live, predecessorRef); !errors.Is(err, keyvault.ErrNotFound) {
		t.Fatalf("the unpinned vault, which is develop's behaviour for a live caller, read it: %v", err)
	}
	if _, err := vault.Get(t.Context(), live, predecessorRef); !errors.Is(err, keyvault.ErrNotFound) {
		t.Fatalf("a live caller read an archived company's credential: %v", err)
	}
	if err := vault.Delete(t.Context(), live, predecessorRef); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM vault_secret WHERE ref = $1`, string(predecessorRef)).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatalf("the archived company's ciphertext survived its erasure: %d rows", left)
	}
}
