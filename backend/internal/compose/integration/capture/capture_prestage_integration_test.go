// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A new message's files are stored before its capture transaction takes the
// authorization lock, and a guess the transaction overrules leaves no bytes.

import (
	"context"
	"io"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// lockWatchingBlob asks, during every put, whether any session holds the
// authorization lock a capture transaction takes, and counts puts and deletes.
type lockWatchingBlob struct {
	blobstore.Store
	e *integration.SearchEnv

	mu                    sync.Mutex
	puts, deletes, locked int
	failure               error
}

func (b *lockWatchingBlob) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	var held int
	err := database.WithWorkspaceTx(b.e.Admin(), b.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT count(*) FROM pg_locks
			 WHERE locktype = 'advisory' AND granted
			   AND objid::bigint = hashtext('margince:authz')::bigint & 4294967295`).Scan(&held)
	})
	b.mu.Lock()
	b.puts++
	b.locked += held
	if err != nil {
		b.failure = err
	}
	b.mu.Unlock()
	return b.Store.Put(ctx, key, r, size, contentType)
}

func (b *lockWatchingBlob) Delete(ctx context.Context, key string) error {
	b.mu.Lock()
	b.deletes++
	b.mu.Unlock()
	return b.Store.Delete(ctx, key)
}

func TestAMessagesFilesAreStoredBeforeItsTransactionTakesTheAuthorizationLock(t *testing.T) {
	blob := &lockWatchingBlob{Store: blobstore.NewMemory()}
	env := newCaptureEnvWithBlob(t, captureNow, blob)
	blob.e = env.e
	env.sync(t, emailWithFile("her@globex.example", captureOwner, "prestage-a@globex.example", "", "contract.pdf", signedContract))

	if blob.failure != nil {
		t.Fatal(blob.failure)
	}
	if blob.puts != 1 {
		t.Fatalf("put %d objects for one file, want 1", blob.puts)
	}
	if blob.locked != 0 {
		t.Fatal("the file was uploaded while a session held the authorization lock")
	}
	if keys := fileKeys(t, env.e, "prestage-a@globex.example"); len(keys) != 1 || keys[0] == "" {
		t.Fatalf("the message names stored files %v, want the one uploaded", keys)
	}
}

func TestFilesStoredForAMessageTheCaptureDropsAreDeleted(t *testing.T) {
	blob := &lockWatchingBlob{Store: blobstore.NewMemory()}
	env := newCaptureEnvWithBlob(t, captureNow, blob)
	blob.e = env.e
	owner := humanWithScopes(env.e, env.e.Rep1, []principal.Scope{principal.ScopeRead, principal.ScopeWrite})
	// An exclusion is a gate the short read does not ask, so the files are
	// stored and the transaction then drops the message.
	if _, err := capture.NewExclusionStore(env.e.DB()).Add(owner, capture.ExclusionScopeUser, capture.ExclusionKindAddress, "her@globex.example", ids.Nil); err != nil {
		t.Fatal(err)
	}
	env.sync(t, emailWithFile("her@globex.example", captureOwner, "prestage-b@globex.example", "", "contract.pdf", signedContract))

	if n := countRows(t, env.e, `SELECT count(*) FROM activity WHERE source_id = 'prestage-b@globex.example'`); n != 0 {
		t.Fatalf("%d activity rows for an excluded message, want 0; the fixture proves nothing", n)
	}
	if blob.puts != 1 || blob.deletes != 1 {
		t.Fatalf("put %d and deleted %d objects for a dropped message, want 1 and 1", blob.puts, blob.deletes)
	}
	if n := countRows(t, env.e, `SELECT count(*) FROM stored_object_intent`); n != 0 {
		t.Fatalf("%d provisional objects remain after the drop, want 0", n)
	}
}

func TestFilesStoredForACaptureItsAuthorityRefusesAreDeleted(t *testing.T) {
	e := integration.SetupSearch(t)
	seedShippedRoles(t, e)
	holdOnlyRole(t, e, e.Rep1, "rep")
	blob := &lockWatchingBlob{Store: blobstore.NewMemory(), e: e}
	first, second := "refused-1@globex.example", "refused-2@globex.example"
	page := &skippingPage{&mailPageConnector{
		nextToken: "next",
		raws: [][]byte{
			emailWithFile("her@globex.example", captureOwner, first, "", "a.pdf", signedContract),
			emailWithFile("her@globex.example", captureOwner, second, "", "b.pdf", []byte("%PDF-1.4 another")),
		},
	}}
	// The role goes between the two messages: the second one's files are
	// stored ahead of its transaction, which then refuses the capture.
	moved := false
	page.afterMessage = func() {
		if !moved {
			moved = true
			holdOnlyRole(t, e, e.Rep1, "read_only")
		}
	}
	registry := compose.NewCaptureRegistry(e.Pool, newTestKeyvault(t, e), compose.CaptureConfig{Blob: blob})
	registry.Register(page)
	actor := humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead})
	if _, err := registry.Connect(actor, "gmail", connector.Auth("refresh")); err != nil {
		t.Fatal(err)
	}
	run, err := registry.StartBackfill(actor, capture.ProviderMailbox("gmail"), ids.From[ids.UserKind](e.Rep1), 6, connector.BackfillEstimate{Messages: 2}, enqueueNothing)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := registry.RunBackfillStep(principal.WithWorkspaceID(context.Background(), e.WS), run.ID); err == nil {
		t.Fatal("the page captured under a role that may not capture")
	}
	if n := countRows(t, e, `SELECT count(*) FROM activity WHERE source_id = '`+second+`'`); n != 0 {
		t.Fatalf("%d activity rows for the refused message, want 0", n)
	}
	if blob.puts != 2 || blob.deletes != 1 {
		t.Fatalf("put %d and deleted %d objects, want 2 and 1: the refused message's file must go", blob.puts, blob.deletes)
	}
}
