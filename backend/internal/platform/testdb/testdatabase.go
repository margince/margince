// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package testdb

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/internal/shared/clockskew"
)

// TemplateEnv names the migrated database every test's own database is copied
// from. The lane exports it. Nothing may connect to that database: the server
// refuses a copy while a session is open on it.
const TemplateEnv = "MARGINCE_TEST_TEMPLATE"

// InPlaceEnv set to 1 makes New and For answer the lane's database itself and
// keep it. The bench targets seed a corpus there and read it after the run.
const InPlaceEnv = "MARGINCE_TEST_IN_PLACE"

// Owner and app DSNs of the lane's package database. A test's own database
// takes their credentials and host, and only the database name changes.
const (
	ownerDSNEnv = "MARGINCE_TEST_DSN"
	appDSNEnv   = "MARGINCE_TEST_APP_DSN"
)

// maxIdentifierBytes is Postgres's stock identifier limit. The server
// truncates a longer name and would then act on a different database.
const maxIdentifierBytes = 63

// copyAttempts bounds the retries while a lane migrates the template, which is
// the only time a session is open on it.
const copyAttempts = 5

// Database is one test's own copy of the migrated template.
type Database struct {
	Name     string
	OwnerDSN string
	AppDSN   string
}

var (
	databasesMu sync.Mutex
	// current maps a test's name to the database For answers for it.
	current = map[string]Database{}
	// seeded holds the copies New has handed to a fixture.
	seeded = map[string]bool{}
	// live holds every copy this process has made and not yet dropped.
	live = map[string]bool{}

	// forMu makes a look-up and its create one step, so two goroutines of one
	// test never make two databases.
	forMu  sync.Mutex
	copies atomic.Int64

	adminOnce sync.Once
	admin     *pgxpool.Pool
	adminErr  error

	verifyOnce sync.Once
	verifyErr  error
)

// New answers a database for a fixture to seed, which then becomes the one
// For answers for t and its subtests. It is the copy For already made for t
// if no fixture took it yet, so a pool opened before the fixture shares it.
// Otherwise it is a fresh copy, as a second fixture in one test expects.
func New(t testing.TB) Database {
	t.Helper()
	return newDatabase(t, inPlaceRequested())
}

// newDatabase is New with the mode passed in, so a test can drive the in-place
// mode without setting the environment every other test reads.
func newDatabase(t testing.TB, inPlace bool) Database {
	t.Helper()
	forMu.Lock()
	defer forMu.Unlock()
	databasesMu.Lock()
	db, ok := current[t.Name()]
	ok = ok && !seeded[db.Name]
	databasesMu.Unlock()
	if !ok {
		db = create(t, inPlace)
	}
	databasesMu.Lock()
	seeded[db.Name] = true
	databasesMu.Unlock()
	return db
}

// For answers the database of t or of the nearest parent test that has one,
// for a helper that reads what a fixture seeded. A test with none gets one.
func For(t testing.TB) Database {
	t.Helper()
	forMu.Lock()
	defer forMu.Unlock()
	if db, ok := lookup(t.Name()); ok {
		return db
	}
	return create(t, inPlaceRequested())
}

func inPlaceRequested() bool { return os.Getenv(InPlaceEnv) == "1" }

// create copies the template for t and drops the copy when t ends, after
// every pool opened on it is closed.
func create(t testing.TB, inPlace bool) Database {
	t.Helper()
	if inPlace {
		return laneDatabase(t)
	}
	db, err := copyTemplate(t.Context(), os.Getenv(TemplateEnv))
	if err != nil {
		t.Fatalf("testdb: %v", err)
	}
	t.Cleanup(func() {
		if err := release(context.Background(), t.Name(), db); err != nil {
			t.Errorf("testdb: %v", err)
		}
	})
	databasesMu.Lock()
	current[t.Name()] = db
	databasesMu.Unlock()
	return db
}

// laneDatabase answers the lane's database itself, brought to head, and keeps
// it after the test.
func laneDatabase(t testing.TB) Database {
	t.Helper()
	db := Database{OwnerDSN: os.Getenv(ownerDSNEnv), AppDSN: os.Getenv(appDSNEnv)}
	name, err := databaseName(db.OwnerDSN)
	if err != nil {
		t.Fatalf("testdb: %v", err)
	}
	db.Name = name
	owner, err := pgx.Connect(t.Context(), db.OwnerDSN)
	if err != nil {
		t.Fatalf("testdb: connecting to %s: %v", name, err)
	}
	if err := errors.Join(EnsureSchema(t.Context(), owner), owner.Close(t.Context())); err != nil {
		t.Fatalf("testdb: %v", err)
	}
	databasesMu.Lock()
	current[t.Name()] = db
	databasesMu.Unlock()
	return db
}

// lookup walks from a subtest's name up to its top-level test.
func lookup(name string) (Database, bool) {
	databasesMu.Lock()
	defer databasesMu.Unlock()
	for {
		if db, ok := current[name]; ok {
			return db, true
		}
		parent := strings.LastIndex(name, "/")
		if parent < 0 {
			return Database{}, false
		}
		name = name[:parent]
	}
}

// isCopy reports whether name is a test database this process copied and has not dropped.
func isCopy(name string) bool {
	databasesMu.Lock()
	defer databasesMu.Unlock()
	return live[name]
}

// copyTemplate makes the next copy of template and proves it ready.
func copyTemplate(ctx context.Context, template string) (Database, error) {
	ownerDSN, appDSN := os.Getenv(ownerDSNEnv), os.Getenv(appDSNEnv)
	if template == "" || ownerDSN == "" || appDSN == "" {
		return Database{}, fmt.Errorf("%s, %s and %s must all be set; run the test through `make test-it` (integration tests fail loudly, they never skip)", TemplateEnv, ownerDSNEnv, appDSNEnv)
	}
	base, err := databaseName(ownerDSN)
	if err != nil {
		return Database{}, err
	}
	name := base + "_" + strconv.Itoa(os.Getpid()) + "_" + strconv.FormatInt(copies.Add(1), 10)
	if len(name) > maxIdentifierBytes {
		return Database{}, fmt.Errorf("the test database name %q is over %d bytes; give the package database %q a shorter name", name, maxIdentifierBytes, base)
	}
	db := Database{Name: name}
	if db.OwnerDSN, err = withDatabase(ownerDSN, name); err != nil {
		return Database{}, err
	}
	if db.AppDSN, err = withDatabase(appDSN, name); err != nil {
		return Database{}, err
	}
	if err := copyWithRetry(ctx, ownerDSN, name, template); err != nil {
		return Database{}, err
	}
	databasesMu.Lock()
	live[name] = true
	databasesMu.Unlock()
	if err := prepareCopy(ctx, db); err != nil {
		return Database{}, errors.Join(err, release(ctx, "", db))
	}
	return db, nil
}

func copyWithRetry(ctx context.Context, ownerDSN, name, template string) error {
	pool, err := adminPool(ctx, ownerDSN)
	if err != nil {
		return err
	}
	for attempt := 1; ; attempt++ {
		err := pool.AcquireFunc(ctx, func(conn *pgxpool.Conn) error {
			return dbmigrate.CopyDatabase(ctx, conn.Conn(), name, template)
		})
		var pgErr *pgconn.PgError
		if err == nil || attempt == copyAttempts || !errors.As(err, &pgErr) || pgErr.Code != "55006" {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt) * 200 * time.Millisecond):
		}
	}
}

// adminPool is one connection to the maintenance database, where CREATE and
// DROP DATABASE run. It lives as long as the process.
func adminPool(ctx context.Context, ownerDSN string) (*pgxpool.Pool, error) {
	adminOnce.Do(func() {
		maintenance, err := withDatabase(ownerDSN, "postgres")
		if err != nil {
			adminErr = err
			return
		}
		admin, adminErr = OwnPoolWith(ctx, maintenance, func(cfg *pgxpool.Config) { cfg.MaxConns = 1 })
	})
	return admin, adminErr
}

// prepareCopy proves the first copy of the process is this binary's head and
// empty. Every later copy comes from the same template. The drift lane's
// clock shadow is per database, so each copy gets its own.
func prepareCopy(ctx context.Context, db Database) error {
	verifyOnce.Do(func() { verifyErr = withOwner(ctx, db, verifyCopy) })
	if verifyErr != nil {
		return verifyErr
	}
	skew, err := clocktest.Skew()
	if err != nil || skew.Applier != clockskew.Database {
		return err
	}
	return withOwner(ctx, db, installClockShadow)
}

func withOwner(ctx context.Context, db Database, work func(context.Context, *pgx.Conn) error) (err error) {
	owner, err := pgx.Connect(ctx, db.OwnerDSN)
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", db.Name, err)
	}
	defer func() { err = errors.Join(err, owner.Close(ctx)) }()
	return work(ctx, owner)
}

func verifyCopy(ctx context.Context, owner *pgx.Conn) error {
	var name string
	var columns int
	if err := owner.QueryRow(ctx, preflightSQL, ledgerColumns).Scan(&name, &columns); err != nil {
		return fmt.Errorf("probing the first test database for the template's ledger: %w", err)
	}
	reason, err := headAndEmpty(ctx, owner, columns)
	if err != nil {
		return err
	}
	if reason != "" {
		return fmt.Errorf("the template %s, copied into %s, is not ready: %s", os.Getenv(TemplateEnv), name, reason)
	}
	return nil
}

// release forgets db, closes every shared pool on it and drops it.
func release(ctx context.Context, test string, db Database) error {
	databasesMu.Lock()
	if test != "" && current[test] == db {
		delete(current, test)
	}
	delete(live, db.Name)
	delete(seeded, db.Name)
	databasesMu.Unlock()
	closePoolsOn(db.Name)
	if err := awaitDisconnect(ctx, db.Name); err != nil {
		return err
	}
	if _, err := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{db.Name}.Sanitize()+" WITH (FORCE)"); err != nil {
		return fmt.Errorf("dropping %s, which stays on the test cluster: %w", db.Name, err)
	}
	return nil
}

// disconnectWait bounds how long release waits for closed sessions to end.
// A session still open after it is ended by the drop itself.
const (
	disconnectWait = time.Second
	disconnectPoll = 2 * time.Millisecond
)

// awaitDisconnect waits until no session is open on name. A closed client's
// backend exits shortly after, and a forced `DROP DATABASE` sleeps 100 ms each
// time it still finds one.
func awaitDisconnect(ctx context.Context, name string) error {
	deadline := time.After(disconnectWait)
	poll := time.NewTicker(disconnectPoll)
	defer poll.Stop()
	for {
		var open bool
		if err := admin.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE datname = $1)`, name).Scan(&open); err != nil {
			return fmt.Errorf("checking for sessions on %s: %w", name, err)
		}
		if !open {
			return nil
		}
		select {
		case <-deadline:
			return nil
		case <-poll.C:
		}
	}
}

// databaseName is the database a URL-form DSN names.
func databaseName(dsn string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil || u.Scheme == "" {
		return "", fmt.Errorf("the test DSN %s is not a postgres:// URL", redactDSN(dsn))
	}
	return strings.TrimPrefix(u.Path, "/"), nil
}

// withDatabase is dsn with its database replaced and everything else kept.
func withDatabase(dsn, name string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil || u.Scheme == "" {
		return "", fmt.Errorf("the test DSN %s is not a postgres:// URL", redactDSN(dsn))
	}
	u.Path = "/" + name
	return u.String(), nil
}
