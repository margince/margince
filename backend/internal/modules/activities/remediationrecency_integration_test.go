// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// The system asking about a silent deal must not make that deal look touched.
//
// last_activity_at is maintained by SQL — four helpers and two triggers — so a
// unit test over this package's Go cannot see this behaviour at all. Write a
// remediation task through the real LogActivity door, read the column back, and
// the answer is either the buyer's last message or the system's own question.
// Only one of those is engagement.
//
// This is the invariant forecast assurance depends on: it files review tasks on
// exactly the deals that have gone quiet, and if those tasks counted, the rule
// that noticed the silence would stop firing on every deal it touched.

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type recencyEnv struct {
	owner *pgx.Conn
	pool  *pgxpool.Pool
	ws    ids.UUID
	rep   ids.UUID
}

func setupRecency(t *testing.T) *recencyEnv {
	t.Helper()
	ownerDSN := os.Getenv("MARGINCE_TEST_DSN")
	appDSN := os.Getenv("MARGINCE_TEST_APP_DSN")
	if ownerDSN == "" || appDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN / MARGINCE_TEST_APP_DSN not set — run `make db-up` " +
			"(integration tests fail loudly, they never skip)")
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
	e := &recencyEnv{owner: owner, ws: ids.NewV7(), rep: ids.NewV7()}
	if _, err := owner.Exec(ctx, `INSERT INTO workspace (id) VALUES ($1)`, e.ws); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Rep')`,
		e.rep, "rep-"+e.rep.String()+"@recency.test"); err != nil {
		t.Fatal(err)
	}
	pool, err := testdb.Pool(ctx, appDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testdb.AssertPoolsQuiesced(t) })
	e.pool = pool
	return e
}

func (e *recencyEnv) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("seeding: %v", err)
	}
}

func (e *recencyEnv) as() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.rep.String(), UserID: e.rep,
		Permissions: principal.Permissions{
			RoleKeys: []string{"manager"},
			Objects: map[string]principal.ObjectGrant{
				"activity": {Read: true, Create: true},
				"deal":     {Read: true, Update: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

// seedQuietDeal returns a deal whose only activity is an old buyer message.
func (e *recencyEnv) seedQuietDeal(t *testing.T, buyerSpoke time.Time) ids.UUID {
	t.Helper()
	deal, pipeline, stage := ids.NewV7(), ids.NewV7(), ids.NewV7()
	e.exec(t, `INSERT INTO pipeline (id, name) VALUES ($1, $2)`, pipeline, "Pipeline "+pipeline.String())
	e.exec(t, `INSERT INTO stage (id, pipeline_id, name, "position") VALUES ($1, $2, 'Qualified', 1)`,
		stage, pipeline)
	e.exec(t, `INSERT INTO deal (id, name, status, owner_id, pipeline_id, stage_id, source, captured_by)
		VALUES ($1, 'Quiet renewal', 'open', $2, $3, $4, 'seed', 'system')`,
		deal, e.rep, pipeline, stage)

	buyer := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, subject, occurred_at, direction, audience, source, captured_by)
		VALUES ($1, 'note', 'Buyer asked about pricing', $2, 'inbound', 'workspace', 'seed', 'system')`,
		buyer, buyerSpoke)
	e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, deal_id)
		VALUES ($1, $2, 'deal', $3)`, ids.NewV7(), buyer, deal)
	return deal
}

func (e *recencyEnv) lastActivity(t *testing.T, deal ids.UUID) *time.Time {
	t.Helper()
	var at *time.Time
	if err := e.owner.QueryRow(context.Background(),
		`SELECT last_activity_at FROM deal WHERE id = $1`, deal).Scan(&at); err != nil {
		t.Fatalf("reading last_activity_at: %v", err)
	}
	return at
}

// logAgainstDeal writes one activity through the real store and links it.
func (e *recencyEnv) logAgainstDeal(t *testing.T, deal ids.UUID, subject, origin string) {
	t.Helper()
	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
	subj := subject
	_, created, err := store.LogActivity(e.as(), LogActivityInput{
		Kind:    "task",
		Subject: &subj,
		Source:  "test",
		Origin:  origin,
		Links:   []ActivityLinkInput{{EntityType: "deal", EntityID: deal}},
	})
	if err != nil {
		t.Fatalf("logging %s activity: %v", origin, err)
	}
	if !created {
		t.Fatal("LogActivity replayed an existing row instead of writing one")
	}
}

// TestRemediationWorkDoesNotRefreshTheDealClock is the whole point: the same
// write, twice, differing only in origin. A human task moves the clock because
// a person did something; a system review task does not, because nobody did.
func TestRemediationWorkDoesNotRefreshTheDealClock(t *testing.T) {
	e := setupRecency(t)
	quietSince := time.Now().Add(-90 * 24 * time.Hour).Truncate(time.Second)

	deal := e.seedQuietDeal(t, quietSince)
	before := e.lastActivity(t, deal)
	if before == nil || !before.UTC().Equal(quietSince.UTC()) {
		t.Fatalf("seeded clock is %v, wanted the buyer's message at %v — the rest of "+
			"this test means nothing if the fixture is not the quiet deal it claims",
			before, quietSince)
	}

	e.logAgainstDeal(t, deal, "Confirm the close date", OriginSystemRemediation)

	if got := e.lastActivity(t, deal); got == nil || !got.UTC().Equal(quietSince.UTC()) {
		t.Errorf("after a system review task the deal reads as last touched %v, "+
			"wanted the buyer's own message at %v — the system asking about a silent "+
			"deal has made it look engaged, and the staleness rule that filed the "+
			"task will now skip this deal", got, quietSince)
	}

	// The admitting half. Without it, a helper that excluded EVERY activity
	// would pass the assertion above and this test would prove nothing.
	e.logAgainstDeal(t, deal, "Call the buyer back", OriginHuman)

	after := e.lastActivity(t, deal)
	if after == nil || !after.After(quietSince) {
		t.Errorf("after a human task the deal still reads as last touched %v — the "+
			"exclusion is swallowing real work too", after)
	}
}
