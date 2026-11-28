// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var cachedProbe = settings.Define[string](
	"installation.cache_probe_cached", "installation_settings", "update", "none", nil,
).AsCacheable()

var uncachedProbe = settings.Define[bool](
	"installation.cache_probe_uncached", "installation_settings", "update", false, nil,
)

type cacheClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *cacheClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *cacheClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// A pooled read is remembered for the cache lifetime and a write through the
// store clears it at once.
func TestAPooledSettingReadIsCachedAndItsWriterClearsIt(t *testing.T) {
	e := Setup(t)
	clock := &cacheClock{now: time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)}
	settings.EnableCacheAt(e.Pool, settings.CacheLifetime, clock.Now)
	store := settings.New(e.Pool, settings.NewRegistry(cachedProbe))
	ctx := e.As(e.AdminUser, nil, principal.Permissions{
		Objects:  map[string]principal.ObjectGrant{"installation_settings": {Read: true, Update: true}},
		RowScope: principal.RowScopeAll,
	})
	ctx, meter := database.WithStatementMeter(ctx)
	read := func() (string, int64) {
		t.Helper()
		before := meter.Cost().Statements
		zone, err := settings.Get(ctx, store, cachedProbe)
		if err != nil {
			t.Fatal(err)
		}
		return zone, meter.Cost().Statements - before
	}
	if err := settings.Set(ctx, store, cachedProbe, "Europe/Lisbon"); err != nil {
		t.Fatal(err)
	}
	clock.advance(settings.CacheLifetime)
	if zone, sent := read(); zone != "Europe/Lisbon" || sent == 0 {
		t.Fatalf("first read = %q with %d statements, want Europe/Lisbon from the database", zone, sent)
	}
	if zone, sent := read(); zone != "Europe/Lisbon" || sent != 0 {
		t.Fatalf("second read = %q with %d statements, want the remembered value and none", zone, sent)
	}
	if err := settings.Set(ctx, store, cachedProbe, "Pacific/Auckland"); err != nil {
		t.Fatal(err)
	}
	if zone, _ := read(); zone != "Pacific/Auckland" {
		t.Fatalf("after a write the read is %q, want Pacific/Auckland", zone)
	}
	clock.advance(settings.CacheLifetime)
	if _, err := e.Pool.Exec(context.Background(),
		`UPDATE setting SET value = '"Asia/Tokyo"' WHERE key = 'installation.cache_probe_cached'`); err != nil {
		t.Fatal(err)
	}
	if zone, sent := read(); zone != "Asia/Tokyo" || sent == 0 {
		t.Fatalf("a read after the lifetime = %q with %d statements, want Asia/Tokyo from the database", zone, sent)
	}
}

func TestASettingThatIsNotDeclaredCacheableIsAlwaysReadFromTheDatabase(t *testing.T) {
	e := Setup(t)
	clock := &cacheClock{now: time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)}
	settings.EnableCacheAt(e.Pool, settings.CacheLifetime, clock.Now)
	store := settings.New(e.Pool, settings.NewRegistry(uncachedProbe))
	ctx := e.As(e.AdminUser, nil, principal.Permissions{
		Objects:  map[string]principal.ObjectGrant{"installation_settings": {Read: true, Update: true}},
		RowScope: principal.RowScopeAll,
	})
	ctx, meter := database.WithStatementMeter(ctx)
	key := uncachedProbe.Key()
	t.Cleanup(func() { e.WsExec(t, `DELETE FROM setting WHERE key = $1`, key) })
	for i, want := range []string{"true", "false"} {
		if _, err := e.Pool.Exec(context.Background(),
			`INSERT INTO setting (key, value) VALUES ($1, $2::jsonb)
			 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, key, want); err != nil {
			t.Fatal(err)
		}
		before := meter.Cost().Statements
		raw, err := store.Raw(ctx, key)
		if err != nil || !json.Valid(raw) || string(raw) != want || meter.Cost().Statements == before {
			t.Fatalf("read %d = %s (%v), want %s straight from the database", i, raw, err, want)
		}
	}
}

// A context with no workspace is refused whether the value is cached or not.
func TestACachedSettingReadWithoutAWorkspaceIsRefusedLikeAMiss(t *testing.T) {
	e := Setup(t)
	clock := &cacheClock{now: time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)}
	settings.EnableCacheAt(e.Pool, settings.CacheLifetime, clock.Now)
	store := settings.New(e.Pool, settings.NewRegistry(cachedProbe))
	granted := principal.Permissions{
		Objects:  map[string]principal.ObjectGrant{"installation_settings": {Read: true, Update: true}},
		RowScope: principal.RowScopeAll,
	}
	ctx := e.As(e.AdminUser, nil, granted)
	if err := settings.Set(ctx, store, cachedProbe, "x"); err != nil {
		t.Fatal(err)
	}
	clock.advance(settings.CacheLifetime)
	bare := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.AdminUser.String(), UserID: e.AdminUser, Permissions: granted,
	})
	_, miss := settings.Get(bare, store, cachedProbe)
	if !errors.Is(miss, database.ErrNoWorkspace) {
		t.Fatalf("a miss without a workspace = %v, want ErrNoWorkspace", miss)
	}
	if _, err := settings.Get(ctx, store, cachedProbe); err != nil {
		t.Fatal(err)
	}
	if _, hit := settings.Get(bare, store, cachedProbe); !errors.Is(hit, database.ErrNoWorkspace) {
		t.Fatalf("a hit without a workspace = %v, want ErrNoWorkspace", hit)
	}
}

// A write transaction open past the lifetime, with a reader in between, still
// leaves the next read after the commit seeing the new value.
func TestAReaderDuringALongWriteTransactionDoesNotKeepTheOldValue(t *testing.T) {
	e := Setup(t)
	clock := &cacheClock{now: time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)}
	settings.EnableCacheAt(e.Pool, settings.CacheLifetime, clock.Now)
	store := settings.New(e.Pool, settings.NewRegistry(cachedProbe))
	ctx := e.As(e.AdminUser, nil, principal.Permissions{
		Objects:  map[string]principal.ObjectGrant{"installation_settings": {Read: true, Update: true}},
		RowScope: principal.RowScopeAll,
	})
	if err := settings.Set(ctx, store, cachedProbe, "old"); err != nil {
		t.Fatal(err)
	}
	if err := database.WithTx(ctx, e.Pool, func(tx pgx.Tx) error {
		if err := settings.SetTx(ctx, store, tx, cachedProbe, "new"); err != nil {
			return err
		}
		clock.advance(time.Minute)
		for range 2 {
			if seen, err := settings.Get(ctx, store, cachedProbe); err != nil || seen != "old" {
				t.Fatalf("a reader before the commit saw %q (%v), want the committed old", seen, err)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if seen, err := settings.Get(ctx, store, cachedProbe); err != nil || seen != "new" {
		t.Fatalf("the read after the commit = %q (%v), want new", seen, err)
	}
}
