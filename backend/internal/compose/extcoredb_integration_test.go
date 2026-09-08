// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The extension core port against a real database.
//
// Four of its claims are about what a TRANSACTION does, and a unit test cannot
// reach any of them: the port's own suite runs on a fake runtime whose Tx
// merely calls the callback, so it models neither transaction ownership nor
// rollback and the atomicity claim is proven by construction and by nothing
// else. The overlay refusal reads a real row. The attribution is merged into
// audit_log.evidence by storekit, which no fake runs. And the whole point of
// holding the caller's transaction rather than taking a connection is invisible
// until the pool has one connection in it.
//
// Driven through callRuntime.Tx — the served entry point — rather than by
// building an extensionCore here. A fixture that assembled the port itself
// would be describing a wiring nothing ships, and the wiring is half of what
// these claims are about.

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
	"github.com/margince/margince/backend/pkg/extension/crm"
)

// The unit whose attribution every audit row below must carry.
const (
	coreUnit    = "notes"
	coreVersion = "1.0.0"
	coreVia     = "tool/file_note"
)

// coreBudget bounds a filing. A port that reaches for a second connection
// waits on the one its own transaction holds, and with one connection in the
// pool that wait never ends — so the deadline is the failure mode, and without
// it the regression arrives as a package timeout naming nothing.
const coreBudget = 20 * time.Second

// coreEnv is one workspace, one pool of ONE connection, and a subject to file
// against.
type coreEnv struct {
	owner   *pgx.Conn
	pool    *pgxpool.Pool
	ws      ids.UUID
	subject ids.UUID
	// user is the human the invocation acts as: a core write resolves its
	// actor from the context, and captured_by is stamped from it.
	user ids.UUID
}

func setupCore(t *testing.T) coreEnv {
	t.Helper()
	ctx := context.Background()
	ownerDSN, appDSN := os.Getenv("MARGINCE_TEST_DSN"), os.Getenv("MARGINCE_TEST_APP_DSN")
	if ownerDSN == "" || appDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN / MARGINCE_TEST_APP_DSN not set — run `make db-up` " +
			"(integration tests fail loudly, they never skip)")
	}
	owner, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing the owner connection: %v", err)
		}
	})
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if err := testdb.Reset(ctx, owner); err != nil {
		t.Fatal(err)
	}

	e := coreEnv{owner: owner, ws: ids.NewV7(), subject: ids.NewV7(), user: ids.NewV7()}
	if _, err := owner.Exec(ctx, `INSERT INTO workspace (id) VALUES ($1)`, e.ws); err != nil {
		t.Fatalf("seeding the workspace: %v", err)
	}
	if _, err := owner.Exec(ctx,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Filer')`,
		e.user, "filer-"+e.user.String()+"@ext.test"); err != nil {
		t.Fatalf("seeding the caller: %v", err)
	}
	if _, err := owner.Exec(ctx, `
		INSERT INTO person (id, owner_id, full_name, source, captured_by)
		VALUES ($1, $2, 'Subject Person', 'manual', 'human:x')`, e.subject, e.user); err != nil {
		t.Fatalf("seeding the subject: %v", err)
	}

	cfg, err := pgxpool.ParseConfig(appDSN)
	if err != nil {
		t.Fatalf("parsing the app DSN: %v", err)
	}
	// ONE connection, which is the whole fixture for the last case and costs
	// the others nothing: every claim here is about work done inside the
	// caller's transaction, so a port that behaves correctly never wants a
	// second. MaxConns alone — MinConns is the owner constructor's to set.
	cfg.MaxConns = 1
	pool, err := testdb.OwnPoolFromConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("opening the single-connection pool: %v", err)
	}
	t.Cleanup(pool.Close)
	e.pool = pool
	return e
}

// invocation is the scope a served call arrives with: the tenant, the human
// acting, and a correlation id — the three a core write resolves from the
// context rather than inventing.
func (e coreEnv) invocation() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.user.String(), UserID: e.user,
		Permissions: principal.Permissions{
			RoleKeys: []string{"admin"},
			Objects: map[string]principal.ObjectGrant{
				"activity": {Create: true, Read: true, Update: true},
				"person":   {Create: true, Read: true, Update: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

// runtime is the port a served call is handed, built by the production
// constructor.
func (e coreEnv) runtime(t *testing.T) (*callRuntime, context.Context, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(e.invocation(), coreBudget)
	return runtimeFor(ctx, coreUnit, coreVersion, coreVia,
		extensionRuntimeBinding{pool: e.pool}), ctx, cancel
}

// aNote is the request a unit files: a `note`, which names no transport, so the
// declared-channel rule that governs a `message` has nothing to hold it to.
func (e coreEnv) aNote(subject ids.UUID) crm.CreateActivityRequest {
	body := "filed by the unit"
	return crm.CreateActivityRequest{
		Kind: crm.CreateActivityRequestKindNote, Body: &body, Source: "extension:" + coreUnit,
	}.LinkTo(crm.CreateActivityRequestLinksEntityTypePerson, subject.String())
}

// file runs one filing through the served entry point and answers what the port
// answered. `after` runs inside the same transaction, so a case can decide
// whether it commits.
func (e coreEnv) file(t *testing.T, subject ids.UUID, after func() error) (crm.Activity, error) {
	t.Helper()
	rt, ctx, cancel := e.runtime(t)
	defer cancel()
	var filed crm.Activity
	err := rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		var inner error
		filed, inner = tx.Core().Activities().Create(ctx, e.aNote(subject))
		if inner != nil {
			return inner
		}
		if after != nil {
			return after()
		}
		return nil
	})
	return filed, err
}

func (e coreEnv) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := e.owner.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("counting rows: %v", err)
	}
	return n
}

// The whole write, committed together.
func TestAUnitsFilingLandsTheActivityItsAuditRowAndItsEvent(t *testing.T) {
	e := setupCore(t)

	filed, err := e.file(t, e.subject, nil)
	if err != nil {
		t.Fatalf("filing a note: %v", err)
	}
	if filed.Id == "" {
		t.Fatal("the port answered an activity with no id")
	}

	if n := e.count(t, `SELECT count(*) FROM activity WHERE id = $1`, filed.Id); n != 1 {
		t.Errorf("%d activity row(s) for %s, want 1", n, filed.Id)
	}
	// The LINK as well as the row: a filing whose subject did not survive is a
	// note attached to nobody, which reads as filed and is not.
	if n := e.count(t, `SELECT count(*) FROM activity_link
		WHERE activity_id = $1 AND entity_type = 'person' AND person_id = $2`,
		filed.Id, e.subject); n != 1 {
		t.Errorf("%d link(s) from %s to the subject, want 1", n, filed.Id)
	}
	// The event, which is what makes the write the product's own shape rather
	// than an insert wearing its name.
	if n := e.count(t, `SELECT count(*) FROM event_outbox`); n == 0 {
		t.Error("the filing published no event — a core write that skips the outbox is a record no subscriber ever hears about")
	}

	// And the attribution, read from the row a real write produced. This is
	// the claim no fake can make: `scoped` binds it and `storekit` merges it,
	// and only audit_log says whether the two met.
	var evidence []byte
	if err := e.owner.QueryRow(context.Background(),
		`SELECT evidence FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1`,
		filed.Id).Scan(&evidence); err != nil {
		t.Fatalf("reading the filing's audit row: %v", err)
	}
	// An empty column is the shape a lost binding takes, and it deserves its
	// own sentence: parsed as JSON it reads "unexpected end of input", which
	// sends the reader to the marshaller rather than to the attribution.
	if len(evidence) == 0 {
		t.Fatal("the filing's audit row carries no evidence at all — nothing bound the unit's attribution, so this write is anonymous in the log")
	}
	var carried struct {
		Extension struct {
			Unit    string `json:"unit"`
			Version string `json:"version"`
			Via     string `json:"via"`
		} `json:"extension"`
	}
	if err := json.Unmarshal(evidence, &carried); err != nil {
		t.Fatalf("reading the audit row's evidence: %v", err)
	}
	if carried.Extension.Unit != coreUnit || carried.Extension.Version != coreVersion ||
		carried.Extension.Via != coreVia {
		t.Errorf("the audit row carries %+v, want the unit, its version and the surface the call arrived on — "+
			"without it every core write a unit makes is anonymous in the log", carried.Extension)
	}
}

// The reverse direction, which is the claim the port is chosen for: the unit's
// own write failing takes the core record with it.
//
// Modelled as the unit returning an error AFTER the filing, because that is
// what a failed insert in the unit's own table does — the callback returns,
// and the transaction the port ran on is the caller's to roll back.
func TestAUnitsLaterFailureTakesTheFiledActivityWithIt(t *testing.T) {
	e := setupCore(t)
	unitFailed := errors.New("the unit's own row could not be written")

	filed, err := e.file(t, e.subject, func() error { return unitFailed })
	if !errors.Is(err, unitFailed) {
		t.Fatalf("the port answered %v, want the unit's own failure", err)
	}

	if n := e.count(t, `SELECT count(*) FROM activity WHERE id = $1`, filed.Id); n != 0 {
		t.Errorf("%d activity row(s) survived a rolled-back filing — the port committed separately from the unit it was serving", n)
	}
	if n := e.count(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1`, filed.Id); n != 0 {
		t.Errorf("%d link(s) survived", n)
	}
	if n := e.count(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1`, filed.Id); n != 0 {
		t.Errorf("%d audit row(s) survived, describing a record that does not exist", n)
	}
	if n := e.count(t, `SELECT count(*) FROM event_outbox`); n != 0 {
		t.Errorf("%d event(s) survived — a subscriber would be told about a record the database never kept", n)
	}
}

// An overlay workspace's native tables are not the live ones, so a core write
// there lands where nothing reads it. The port refuses instead, and the refusal
// is the DECLARED one.
func TestACoreWriteInAnOverlayWorkspaceIsRefusedAndWritesNothing(t *testing.T) {
	e := setupCore(t)
	if _, err := e.owner.Exec(context.Background(),
		// The incumbent rides along: overlay_mode_overlay_iff_incumbent holds the
		// two together, because an overlay installation with nothing to mirror is
		// a state no reader of the mode could act on.
		`UPDATE overlay_mode SET sor_mode = 'overlay', incumbent = 'hubspot'`); err != nil {
		t.Fatalf("putting the workspace in overlay mode: %v", err)
	}

	_, err := e.file(t, e.subject, nil)
	if !errors.Is(err, extension.ErrOverlayUnsupported) {
		t.Fatalf("filing in an overlay workspace = %v, want ErrOverlayUnsupported", err)
	}
	if n := e.count(t, `SELECT count(*) FROM activity`); n != 0 {
		t.Errorf("%d activity row(s) written in overlay mode, want none", n)
	}
}

// A subject the caller cannot see is a NOT FOUND, and nothing is written. The
// port inherits the store's row-scope gate rather than re-implementing it, and
// this is what says the gate is actually reached.
func TestFilingAgainstASubjectThatDoesNotExistIsRefusedAndWritesNothing(t *testing.T) {
	e := setupCore(t)

	_, err := e.file(t, ids.NewV7(), nil)
	if err == nil {
		t.Fatal("filing against a subject that does not exist was accepted")
	}
	if !errors.Is(err, extension.ErrNotFound) {
		t.Fatalf("filing against an invisible subject = %v, want ErrNotFound — an existence-hiding refusal is what keeps a unit from enumerating records", err)
	}
	if n := e.count(t, `SELECT count(*) FROM activity`); n != 0 {
		t.Errorf("%d activity row(s) written for a refused filing, want none", n)
	}
}

// The fixture's own claim, stated rather than left to be inferred.
//
// Every case above runs against a pool holding ONE connection, which is what
// makes them a regression test for the second acquire this port already took
// once: a verb reaching for the pool waits on the connection its own
// transaction is holding, and that wait does not end. Dropped from setupCore,
// the pool goes back to the shipped sixteen, all four cases keep passing, and
// the cover disappears with nothing to say so.
func TestTheCoreSuiteRunsOnOneConnection(t *testing.T) {
	e := setupCore(t)
	if got := e.pool.Config().MaxConns; got != 1 {
		t.Fatalf("the suite's pool holds %d connections, want 1 — with more than one, a verb that "+
			"acquires simply succeeds and every case above passes over the defect they exist for", got)
	}
}
