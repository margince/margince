// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const restoredAddress = "geloescht@betroffene.example"

// The restore the replay exists for: the database went back to a point before
// an erasure, so the contact's rows came back and the suppression list lost
// the entry. Only the journal in the object store still names the address.
func TestReplayErasuresTakesAgainWhatARestoreBroughtBack(t *testing.T) {
	e := integration.Setup(t)
	journalDir := configureReplayEnvironment(t)

	// Before the backup: the contact was erased and the journal exported, both
	// through the eraser the api uses.
	journal, err := blobstore.NewFilesystem(journalDir)
	if err != nil {
		t.Fatalf("opening the journal store: %v", err)
	}
	eraser := compose.ComposedEraser(e.Pool, journal, nil)
	erased := seedContactWithAddress(t, e, "Die Betroffene", restoredAddress)
	if err := eraser.EraseContact(e.Admin(), erased, "Art. 17 request"); err != nil {
		t.Fatalf("erasing the contact: %v", err)
	}
	if _, err := eraser.ExportSuppressions(e.Admin()); err != nil {
		t.Fatalf("exporting the journal: %v", err)
	}

	// The restore: the list rolled back and the contact's rows are back.
	if _, err := integration.OwnerConn(t).Exec(context.Background(), `DELETE FROM erasure_suppression`); err != nil {
		t.Fatalf("rolling the suppression list back: %v", err)
	}
	restored := seedContactWithAddress(t, e, "Die Betroffene", restoredAddress)

	var out bytes.Buffer
	handled, err := runDatabaseSubcommand(t.Context(), e.Pool, []string{"replay-erasures"}, &out)
	if !handled {
		t.Fatal("the worker does not know the replay-erasures command")
	}
	if err != nil {
		t.Fatalf("replay-erasures: %v", err)
	}
	if !strings.Contains(out.String(), "Re-erased 1 contact(s)") {
		t.Fatalf("the command reported %q, want the one restored contact", out.String())
	}
	if live := addressCount(t, e, restored); live != 0 {
		t.Fatalf("the restored contact still holds %d address(es)", live)
	}
}

// Without the object store the journal cannot be read, and a pass over the
// rolled-back list alone would report a clean run it cannot vouch for.
func TestReplayErasuresRefusesToRunWithoutTheObjectStore(t *testing.T) {
	e := integration.Setup(t)
	t.Setenv(blobstore.EnvEndpoint, "")
	t.Setenv(blobstore.EnvPath, "")
	t.Setenv(keyvault.EnvRootKey, "")

	_, err := runDatabaseSubcommand(t.Context(), e.Pool, []string{"replay-erasures"}, &bytes.Buffer{})
	if !errors.Is(err, compose.ErrReplayNeedsObjectStore) {
		t.Fatalf("err = %v, want the replay refused for want of the journal", err)
	}
}

// configureReplayEnvironment gives the command the object store and key vault
// an installation's roles read, and returns where the store keeps its objects.
func configureReplayEnvironment(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	rootKey := make([]byte, 32)
	if _, err := rand.Read(rootKey); err != nil {
		t.Fatalf("making a root key: %v", err)
	}
	t.Setenv(blobstore.EnvEndpoint, "")
	t.Setenv(blobstore.EnvPath, dir)
	t.Setenv(keyvault.EnvRootKey, base64.StdEncoding.EncodeToString(rootKey))
	return dir
}

func seedContactWithAddress(t *testing.T, e *integration.Env, name, email string) ids.UUID {
	t.Helper()
	contact := e.SeedContact(t, name, nil)
	if _, err := integration.OwnerConn(t).Exec(context.Background(), `
		INSERT INTO contact_email (id, contact_id, email, email_type, is_primary, source, captured_by)
		VALUES ($1, $2, $3, 'work', true, 'manual', 'human:restore')`,
		ids.NewV7(), contact, email); err != nil {
		t.Fatalf("seeding the address: %v", err)
	}
	return contact
}

func addressCount(t *testing.T, e *integration.Env, contact ids.UUID) int {
	t.Helper()
	var n int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM contact_email WHERE contact_id = $1`, contact).Scan(&n)
	}); err != nil {
		t.Fatalf("counting addresses: %v", err)
	}
	return n
}
