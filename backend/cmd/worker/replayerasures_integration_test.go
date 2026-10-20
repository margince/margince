// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/config/configtest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const restoredAddress = "geloescht@betroffene.example"

// The restore the replay exists for. The database went back to a point before
// an erasure, so the contact's rows came back and the suppression list lost
// the entry. Only the journal in the object store still names the address.
func TestReplayErasuresTakesAgainWhatARestoreBroughtBack(t *testing.T) {
	e := integration.Setup(t)
	eraser := journalledEraser(t, e)
	eraseAndExport(t, e, eraser, seedContactWithAddress(t, e, restoredAddress))

	rollBackSuppressionList(t)
	restored := seedContactWithAddress(t, e, restoredAddress)

	out, err := replayErasures(t, e)
	if err != nil {
		t.Fatalf("replay-erasures: %v", err)
	}
	if !strings.Contains(out, "Re-erased 1 contact(s)") {
		t.Fatalf("the command reported %q, want the one restored contact", out)
	}
	if live := countRows(t, e, `SELECT count(*) FROM contact_email WHERE contact_id = $1`, restored); live != 0 {
		t.Fatalf("the restored contact still holds %d address(es)", live)
	}
}

// A contact known only by a chat account carries no address. The replay
// matches the account itself, as the erasure hashed it.
func TestReplayErasuresTakesAgainAContactKnownOnlyByAChatAccount(t *testing.T) {
	e := integration.Setup(t)
	eraser := journalledEraser(t, e)
	eraseAndExport(t, e, eraser, seedContactWithChatAccount(t, e))

	rollBackSuppressionList(t)
	restored := seedContactWithChatAccount(t, e)

	out, err := replayErasures(t, e)
	if err != nil {
		t.Fatalf("replay-erasures: %v", err)
	}
	if !strings.Contains(out, "Re-erased 1 contact(s)") {
		t.Fatalf("the command reported %q, want the chat-only contact taken again", out)
	}
	if live := countRows(t, e, `SELECT count(*) FROM contact_channel_identity WHERE contact_id = $1`, restored); live != 0 {
		t.Fatalf("the restored contact still holds %d chat account(s)", live)
	}
}

// The journal ends at its last export. The command says when, so the operator
// knows which erasures to take again by hand.
func TestReplayErasuresSaysWhichErasuresItCannotCover(t *testing.T) {
	e := integration.Setup(t)
	eraser := journalledEraser(t, e)
	eraseAndExport(t, e, eraser, seedContactWithAddress(t, e, restoredAddress))
	manifest, err := eraser.ReadJournalManifest(e.Admin())
	if err != nil {
		t.Fatalf("reading the manifest: %v", err)
	}

	out, err := replayErasures(t, e)
	if err != nil {
		t.Fatalf("replay-erasures: %v", err)
	}
	if manifest.Entries != 1 || !strings.Contains(out, "in 1 entries") {
		t.Fatalf("the command reported %q from a manifest of %d entries, want the one", out, manifest.Entries)
	}
	if !strings.Contains(out, "Anything erased after "+manifest.NewestErasure.Format(time.RFC3339)+" is NOT covered") {
		t.Fatalf("the command reported %q, want the uncovered window named", out)
	}
}

// An empty journal is most often a store pointed at the wrong place. The
// operator has to say it is right before the replay reports a clean pass.
func TestReplayErasuresRefusesAnEmptyJournalUnlessTold(t *testing.T) {
	e := integration.Setup(t)
	configureObjectStore(t)

	if _, err := replayErasures(t, e); !errors.Is(err, compose.ErrReplayJournalEmpty) {
		t.Fatalf("err = %v, want an empty journal refused", err)
	}
	out, err := replayErasures(t, e, "--empty-journal-ok")
	if err != nil {
		t.Fatalf("replay-erasures --empty-journal-ok: %v", err)
	}
	if !strings.Contains(out, "holds no entry") {
		t.Fatalf("the command reported %q, want the empty journal named", out)
	}
}

// Without the object store the journal cannot be read. A pass over the
// rolled-back list alone would report a clean run it cannot vouch for.
func TestReplayErasuresRefusesToRunWithoutTheObjectStore(t *testing.T) {
	e := integration.Setup(t)
	t.Setenv(blobstore.EnvEndpoint, "")
	t.Setenv(blobstore.EnvPath, "")

	if _, err := replayErasures(t, e, "--empty-journal-ok"); !errors.Is(err, compose.ErrReplayNeedsObjectStore) {
		t.Fatalf("err = %v, want the replay refused for want of the journal", err)
	}
}

// replayErasures runs the command through the worker's own dispatch.
func replayErasures(t *testing.T, e *integration.Env, flags ...string) (string, error) {
	t.Helper()
	vault := newTestVault(t, e)
	settings, err := configtest.Load(t, "", config.Environ(), nil, blobstore.Settings()...)
	if err != nil {
		t.Fatalf("loading the blobstore settings: %v", err)
	}
	var out bytes.Buffer
	handled, err := runDatabaseSubcommand(t.Context(), e.Pool,
		func() (keyvault.Vault, error) { return vault, nil }, settings,
		append([]string{"replay-erasures"}, flags...), &out)
	if !handled {
		t.Fatal("the worker does not know the replay-erasures command")
	}
	return out.String(), err
}

// journalledEraser points the command at a filesystem object store and returns
// the eraser the api would build over the same store.
func journalledEraser(t *testing.T, e *integration.Env) *privacy.Eraser {
	t.Helper()
	store, err := blobstore.NewFilesystem(configureObjectStore(t))
	if err != nil {
		t.Fatalf("opening the object store: %v", err)
	}
	return compose.ComposedEraser(e.Pool, store, nil)
}

func eraseAndExport(t *testing.T, e *integration.Env, eraser *privacy.Eraser, contact ids.UUID) {
	t.Helper()
	if err := eraser.EraseContact(e.Admin(), contact, "Art. 17 request"); err != nil {
		t.Fatalf("erasing the contact: %v", err)
	}
	if _, err := eraser.ExportSuppressions(e.Admin()); err != nil {
		t.Fatalf("exporting the journal: %v", err)
	}
}

func configureObjectStore(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(blobstore.EnvEndpoint, "")
	t.Setenv(blobstore.EnvPath, dir)
	return dir
}

func newTestVault(t *testing.T, e *integration.Env) keyvault.Vault {
	t.Helper()
	rootKey := make([]byte, 32)
	if _, err := rand.Read(rootKey); err != nil {
		t.Fatalf("making a root key: %v", err)
	}
	vault, err := keyvault.New(keyvault.Config{RootKey: rootKey, Pool: e.Pool})
	if err != nil {
		t.Fatalf("building the key vault: %v", err)
	}
	return vault
}

// rollBackSuppressionList stands in for the restore: the list goes back to a
// point before the erasure.
func rollBackSuppressionList(t *testing.T) {
	t.Helper()
	if _, err := integration.OwnerConn(t).Exec(context.Background(), `DELETE FROM erasure_suppression`); err != nil {
		t.Fatalf("rolling the suppression list back: %v", err)
	}
}

func seedContactWithAddress(t *testing.T, e *integration.Env, email string) ids.UUID {
	t.Helper()
	contact := e.SeedContact(t, "Die Betroffene", nil)
	if _, err := integration.OwnerConn(t).Exec(context.Background(), `
		INSERT INTO contact_email (id, contact_id, email, email_type, is_primary, source, captured_by)
		VALUES ($1, $2, $3, 'work', true, 'manual', 'human:restore')`,
		ids.NewV7(), contact, email); err != nil {
		t.Fatalf("seeding the address: %v", err)
	}
	return contact
}

func seedContactWithChatAccount(t *testing.T, e *integration.Env) ids.UUID {
	t.Helper()
	contact := e.SeedContact(t, "Nur im Chat", nil)
	e.WsExec(t, `
		INSERT INTO contact_channel_identity (contact_id, provider, channel_user_id, username, source, captured_by)
		VALUES ($1, 'telegram', '4711', 'nurimchat', 'telegram', 'connector:telegram')`,
		contact)
	return contact
}

func countRows(t *testing.T, e *integration.Env, query string, contact ids.UUID) int {
	t.Helper()
	var n int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), query, contact).Scan(&n)
	}); err != nil {
		t.Fatalf("counting rows: %v", err)
	}
	return n
}
