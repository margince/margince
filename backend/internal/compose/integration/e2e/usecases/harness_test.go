// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package usecases

// The harness every scenario boots, and the seat vocabulary they share.
//
// One app per scenario, never shared: apptest.SetupApp* RESETS the package
// database, so a second setup inside a running scenario would delete the
// fixture the first one seeded. That is also why nothing here calls
// t.Parallel.

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The seats every scenario needs. Two humans, because the criteria that matter
// most are about the difference between them: an account you own reads
// differently from a colleague's, and "is_you" is only meaningful when someone
// else exists to be not-you.
const (
	repEmail       = "rep@usecases.test"
	repName        = "Sam Rep"
	colleagueEmail = "colleague@usecases.test"
	colleagueName  = "Alex Colleague"
)

// scopesRead is what a read-only scenario mints its passport with. Named
// rather than spelled at each call site: a scenario that quietly asks for write
// when it is testing a read is a test proving less than it claims.
var scopesRead = []string{"read"}

// scopesReadWrite is what a scenario that WRITES mints. Case 1 is the only
// journey whose point is the write path.
var scopesReadWrite = []string{"read", "write"}

// scenario is one use case's world: the running app, the assistant's client,
// and the two seats.
type scenario struct {
	*apptest.AppEnv
	// MCP is the assistant. Every tool call in a scenario goes through it.
	MCP *apptest.MCPClient
	// Rep is the human the assistant acts for — the passport's granting seat.
	Rep ids.UUID
	// Colleague is somebody else, so a scenario can seed an account the rep
	// does not own and assert the product says so.
	Colleague ids.UUID
}

// boot brings up a composed app with /mcp mounted and a passport ready.
//
// It takes no extra compose options on purpose. A scenario that needs one —
// case 3's blob store, a job runner for the transcript reader — adds its own
// variant beside this rather than widening a signature nothing else passes.
//
// The connector options are not optional: without WithMCPConnector the handler
// is nil and the /mcp route is never registered, so every call 404s and the
// scenario reads it as a broken test rather than a missing option.
func boot(t *testing.T, scopes []string) *scenario {
	t.Helper()
	e := apptest.SetupAppWithOriginOptions(t, func(origin string) []compose.Option {
		return []compose.Option{
			compose.WithMCPConnector(),
			compose.WithMCPResource(origin + "/mcp"),
		}
	})
	apptest.BootstrapWorkspaceSession(t, e, "Use Cases", repEmail, repName)

	return finishBoot(t, e, scopes)
}

// finishBoot names the seats and mints the assistant's credential, for both
// boot variants.
func finishBoot(t *testing.T, e *apptest.AppEnv, scopes []string) *scenario {
	t.Helper()
	s := &scenario{AppEnv: e}
	s.Rep = seatIDFor(t, e, repEmail)
	s.Colleague = seedColleague(t, e)
	s.MCP = apptest.NewMCPClient(e, apptest.MCPBearerToken(t, e, "use-case assistant", scopes...))
	return s
}

// bootWithTranscriptReading is boot plus the reading lane, for the one scenario
// whose criterion is that a landed transcript gets read.
//
// An INSERT-ONLY runner: the api role never calls a model in-request — it
// inserts the job and answers — so a job inserter is the whole dependency and
// no model lane is wired anywhere. What this proves is that the reading is
// QUEUED, which is the half that silently did not happen for as long as asking
// was the only way in. Whether the model then reads good commitments out of it
// is the weekly lane's question.
func bootWithTranscriptReading(t *testing.T) *scenario {
	t.Helper()
	appDSN := os.Getenv("MARGINCE_TEST_APP_DSN")
	if appDSN == "" {
		t.Fatal("MARGINCE_TEST_APP_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	// A separate pool from the one the harness opens: the inserter needs SOME
	// pool reaching the same Postgres, not the same object.
	wirePool, err := testdb.OwnPool(context.Background(), appDSN)
	if err != nil {
		t.Fatalf("opening the insert-only wiring pool: %v", err)
	}
	t.Cleanup(wirePool.Close)
	inserter, err := jobs.NewInserter(wirePool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("jobs.NewInserter: %v", err)
	}

	e := apptest.SetupAppWithOriginOptions(t, func(origin string) []compose.Option {
		return []compose.Option{
			compose.WithMCPConnector(),
			compose.WithMCPResource(origin + "/mcp"),
			compose.WithTranscriptRead(inserter),
		}
	})
	// The landing enqueues in the same transaction as the run record, so the
	// job schema has to exist before the FIRST write, not merely before a
	// worker.
	integration.ApplyRiverSchema(t)
	apptest.BootstrapWorkspaceSession(t, e, "Use Cases", repEmail, repName)
	return finishBoot(t, e, scopesReadWrite)
}

// bootWithImports is boot plus an in-memory blob store, for the one scenario
// that imports a file.
//
// The store is not optional decoration: profiling an import STORES the bytes,
// and both the dry run and the commit reopen the stored source — a composition
// without one refuses the profile outright. In memory rather than MinIO,
// because what is under test is the import's arithmetic and not object storage.
func bootWithImports(t *testing.T) *scenario {
	t.Helper()
	e := apptest.SetupAppWithOriginOptions(t, func(origin string) []compose.Option {
		return []compose.Option{
			compose.WithMCPConnector(),
			compose.WithMCPResource(origin + "/mcp"),
			compose.WithBlobstore(blobstore.NewMemory()),
		}
	})
	apptest.BootstrapWorkspaceSession(t, e, "Use Cases", repEmail, repName)
	return finishBoot(t, e, scopesReadWrite)
}

// queryRow runs one scalar query inside the workspace.
//
//craft:ignore naked-any the destination is pgx Scan's own — the caller supplies the concrete type
func queryRow(t *testing.T, s *scenario, sql string, out any, args ...any) error {
	t.Helper()
	return apptest.InWorkspace(s.AppEnv, t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), sql, args...).Scan(out)
	})
}

// seatIDFor reads a seat's id by email. The bootstrap creates the admin seat
// through the real installation path, so its id is discovered rather than
// chosen.
func seatIDFor(t *testing.T, e *apptest.AppEnv, email string) ids.UUID {
	t.Helper()
	var id ids.UUID
	err := apptest.InWorkspace(e, t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT id FROM app_user WHERE lower(email) = lower($1)`, email).Scan(&id)
	})
	if err != nil {
		t.Fatalf("reading the seat for %s: %v", email, err)
	}
	return id
}

// seedColleague adds a second human so ownership has two sides.
//
// Inserted rather than invited: an invitation flow is its own subsystem with
// its own suite, and a scenario about who owns an account should not fail
// because email delivery changed.
func seedColleague(t *testing.T, e *apptest.AppEnv) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	err := apptest.InWorkspace(e, t, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`INSERT INTO app_user (id, email, display_name, status)
			 VALUES ($1, $2, $3, 'active')`,
			id, colleagueEmail, colleagueName)
		return err
	})
	if err != nil {
		t.Fatalf("seeding the colleague seat: %v", err)
	}
	return id
}

// seed runs one statement inside the workspace, for a fixture that has nothing
// to return.
func (s *scenario) seed(t *testing.T, sql string, args ...any) {
	t.Helper()
	err := apptest.InWorkspace(s.AppEnv, t, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), sql, args...)
		return err
	})
	if err != nil {
		t.Fatalf("seeding: %v\n%s", err, sql)
	}
}

// seedID runs one INSERT whose id is generated here and returned, so a fixture
// can link the rows it just made.
func (s *scenario) seedID(t *testing.T, sql string, args ...any) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	s.seed(t, sql, append([]any{id}, args...)...)
	return id
}

// recordName reads a record's display name out of its fields, for a failure
// message a person can act on.
//
// A record on this surface carries its fields as a raw JSON document rather
// than as named members, so there is no Title to read. A failure naming
// "Rheinufer AG" is worth the decode; one naming a uuid is not.
func recordName(t *testing.T, fields json.RawMessage) string {
	t.Helper()
	var named struct {
		DisplayName string `json:"display_name"`
		FullName    string `json:"full_name"`
	}
	if err := json.Unmarshal(fields, &named); err != nil {
		return "(unnamed record)"
	}
	if named.DisplayName != "" {
		return named.DisplayName
	}
	if named.FullName != "" {
		return named.FullName
	}
	return "(unnamed record)"
}

// defaultOpenStage names the installation's default pipeline and an OPEN stage
// in it.
//
// Scenarios read these rather than seeding their own. The bootstrap creates a
// pipeline with its stages, so an inserted second one collides on
// pipeline_name_unique — and an inserted second DEFAULT would be a state the
// product itself cannot produce, which is not a state worth testing against.
func (s *scenario) defaultOpenStage(t *testing.T) (pipeline, stage ids.UUID) {
	t.Helper()
	err := apptest.InWorkspace(s.AppEnv, t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT p.id, st.id
			  FROM pipeline p
			  JOIN stage st ON st.pipeline_id = p.id AND st.semantic = 'open'
			 WHERE p.is_default
			 ORDER BY st.position
			 LIMIT 1`).Scan(&pipeline, &stage)
	})
	if err != nil {
		t.Fatalf("reading the installation's default pipeline: %v", err)
	}
	return pipeline, stage
}
