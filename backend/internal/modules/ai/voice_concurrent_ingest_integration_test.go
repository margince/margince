// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package ai

// Concurrency is the subject here, so this runs against a real Postgres: a
// deadlock is something only the lock manager can produce, and no fake
// transaction has one. Onboarding drops several files at once and every ingest
// lands on the SAME voice_profile row, which is the contention this proves the
// store survives.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// concurrentIngestClock is the store's injected now, so nothing here reads the
// wall clock.
var concurrentIngestClock = time.Date(2026, 9, 6, 8, 0, 0, 0, time.UTC)

// concurrentIngestCount is how many sources are added at once. Onboarding's
// file drop is the shape being modelled; more than a couple of writers is what
// makes a lock cycle reachable at all.
const concurrentIngestCount = 8

// pgSerializationDeadlock is PostgreSQL's deadlock_detected. Named here so the
// failure message can say what actually happened rather than printing a code.
const pgSerializationDeadlock = "40P01"

type concurrentIngestEnv struct {
	owner *pgx.Conn
	pool  *pgxpool.Pool
}

func setupConcurrentIngest(t *testing.T) *concurrentIngestEnv {
	t.Helper()
	ownerDSN := os.Getenv("MARGINCE_TEST_DSN")
	appDSN := os.Getenv("MARGINCE_TEST_APP_DSN")
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
	pool, err := testdb.Pool(ctx, appDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testdb.AssertPoolsQuiesced(t) })
	return &concurrentIngestEnv{owner: owner, pool: pool}
}

// seedBuiltProfile mints a workspace holding one human's ALREADY BUILT voice
// profile. Built is the load-bearing part: markProfileStale returns early on a
// profile_version of 0, so a never-built profile would leave the shared row
// untouched and the test would pass without exercising the contention it
// exists to prove.
func (e *concurrentIngestEnv) seedBuiltProfile(t *testing.T) (context.Context, ids.UUID) {
	t.Helper()
	ctx := context.Background()
	workspace := ids.NewV7()
	if _, err := e.owner.Exec(ctx, `INSERT INTO workspace (id) VALUES ($1)`, workspace); err != nil {
		t.Fatal(err)
	}
	var owner ids.UUID
	if err := e.owner.QueryRow(ctx, `
		INSERT INTO app_user (email, display_name)
		VALUES ($1, 'Corpus Owner') RETURNING id`,
		"owner-"+ids.NewV7().String()+"@example.test").Scan(&owner); err != nil {
		t.Fatal(err)
	}
	var profile ids.UUID
	if err := e.owner.QueryRow(ctx, `
		INSERT INTO voice_profile (owner_id, scope, status, profile_version, source, captured_by)
		VALUES ($1, 'user', 'ready', 3, 'ui', $2) RETURNING id`,
		owner, "human:"+owner.String()).Scan(&profile); err != nil {
		t.Fatal(err)
	}
	actor := principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + owner.String(), UserID: owner,
		SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"}, RowScope: principal.RowScopeTeam,
			Objects: map[string]principal.ObjectGrant{
				"voice_profile": {Read: true, Update: true},
			},
		},
	}
	callCtx := principal.WithCorrelationID(
		principal.WithActor(principal.WithWorkspaceID(ctx, workspace), actor),
		ids.NewV7())
	return callCtx, profile
}

func (e *concurrentIngestEnv) storeIn(ws ids.UUID) *VoiceStore {
	s := NewVoiceStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](ws)))
	s.now = func() time.Time { return concurrentIngestClock }
	return s
}

// Adding several sources at once must add all of them. Each writer owns a
// DISTINCT source_ref, so the per-source advisory lock never serializes them,
// and the profile row is the only thing they all touch.
//
// This is the onboarding file drop as a test, and it fails without the
// profile-first lock: removing that one call from ingestPreparedSource aborts
// five to seven of the eight with SQLSTATE 40P01 and leaves one or two sources
// in the manifest — measured over eight runs of the mutation, every one of
// which failed here.
func TestConcurrentIngestOfDistinctSourcesAllSucceed(t *testing.T) {
	e := setupConcurrentIngest(t)
	ctx, profile := e.seedBuiltProfile(t)
	store := e.storeIn(workspaceOf(ctx, t))

	errs := make([]error, concurrentIngestCount)
	// The barrier releases the writers together. It synchronizes their START,
	// not their arrival at any particular lock — nothing here can force that,
	// since each transaction opens inside IngestSource. Overlap is what the
	// cycle needs and what releasing them together buys; the deadlock is
	// structural rather than a narrow window, which is why the mutation above
	// fails every time rather than occasionally.
	var ready, done sync.WaitGroup
	start := make(chan struct{})
	ready.Add(concurrentIngestCount)
	done.Add(concurrentIngestCount)
	for i := range concurrentIngestCount {
		go func() {
			defer done.Done()
			ready.Done()
			<-start
			_, _, _, err := store.IngestSource(ctx, profile, IngestSourceInput{
				Kind:        voiceSourceKindDocument,
				SourceLabel: fmt.Sprintf("sample-%d.txt", i),
				SourceRef:   fmt.Sprintf("voice:upload:concurrent-%d", i),
				Format:      corpusWireFormatText,
				Content:     fmt.Sprintf("Sample %d. I write plainly and I keep my sentences short.", i),
			})
			errs[i] = err
		}()
	}
	ready.Wait()
	close(start)
	done.Wait()

	for i, err := range errs {
		if err == nil {
			continue
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgSerializationDeadlock {
			t.Errorf("ingest %d deadlocked (%s): concurrent ingests of distinct sources must not contend on the profile row", i, pgErr.Code)
			continue
		}
		t.Errorf("ingest %d: %v", i, err)
	}

	var stored int
	if err := database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT count(*) FROM voice_corpus_source
			WHERE voice_profile_id = $1 AND archived_at IS NULL`, profile).Scan(&stored)
	}); err != nil {
		t.Fatalf("counting the manifest: %v", err)
	}
	if stored != concurrentIngestCount {
		t.Errorf("manifest holds %d sources, want %d", stored, concurrentIngestCount)
	}
}

// The staleness mark still fires, and exactly once per ingest that changes
// anything: a corpus that moved under a built profile must invalidate it, or
// the reader is served a voice that no longer describes its sources. Guarding
// this is what stops the deadlock fix from being "stop writing the row".
func TestConcurrentIngestMarksTheBuiltProfileStale(t *testing.T) {
	e := setupConcurrentIngest(t)
	ctx, profile := e.seedBuiltProfile(t)
	store := e.storeIn(workspaceOf(ctx, t))

	if _, _, _, err := store.IngestSource(ctx, profile, IngestSourceInput{
		Kind:        voiceSourceKindDocument,
		SourceLabel: "first.txt",
		SourceRef:   "voice:upload:stale-1",
		Format:      corpusWireFormatText,
		Content:     "I write plainly and I keep my sentences short.",
	}); err != nil {
		t.Fatalf("first ingest: %v", err)
	}

	status, version := e.readProfile(ctx, t, profile)
	if status != voiceProfileStatusStale {
		t.Errorf("profile status is %q, want %q — a corpus change must invalidate the built voice", status, voiceProfileStatusStale)
	}

	// A second ingest onto an already-stale profile must not bump the version
	// again: the row says "stale" once, and a second bump would present version
	// skew to a reader holding the first.
	if _, _, _, err := store.IngestSource(ctx, profile, IngestSourceInput{
		Kind:        voiceSourceKindDocument,
		SourceLabel: "second.txt",
		SourceRef:   "voice:upload:stale-2",
		Format:      corpusWireFormatText,
		Content:     "A second sample, in the same voice as the first.",
	}); err != nil {
		t.Fatalf("second ingest: %v", err)
	}
	_, after := e.readProfile(ctx, t, profile)
	if after != version {
		t.Errorf("profile version moved from %d to %d on an already-stale profile; the mark must be idempotent", version, after)
	}
}

func (e *concurrentIngestEnv) readProfile(ctx context.Context, t *testing.T, profile ids.UUID) (status string, version int64) {
	t.Helper()
	if err := database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT status, version FROM voice_profile WHERE id = $1`, profile).Scan(&status, &version)
	}); err != nil {
		t.Fatalf("reading the profile: %v", err)
	}
	return status, version
}
