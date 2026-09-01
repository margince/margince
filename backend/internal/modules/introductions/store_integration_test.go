// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package introductions

// The ask's whole life against a real database. Three of the properties this
// file holds cannot be seen without Postgres: the partial unique index that is
// the duplicate guard, the row-scope predicate that decides who may read an
// ask, and the write shape's audit and outbox rows.

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var testNow = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

type introEnv struct {
	owner      *pgx.Conn
	pool       *pgxpool.Pool
	store      *Store
	ws         ids.UUID
	requester  ids.UUID
	introducer ids.UUID
	stranger   ids.UUID
	contact    ids.UUID
	unseen     ids.UUID
}

func setupIntro(t *testing.T) *introEnv {
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
	if err := testdb.Reset(ctx, owner); err != nil {
		t.Fatal(err)
	}
	e := &introEnv{
		owner: owner, ws: ids.NewV7(),
		requester: ids.NewV7(), introducer: ids.NewV7(), stranger: ids.NewV7(),
		contact: ids.NewV7(), unseen: ids.NewV7(),
	}
	if _, err := owner.Exec(ctx, `INSERT INTO workspace (id) VALUES ($1)`, e.ws); err != nil {
		t.Fatal(err)
	}
	for _, u := range []ids.UUID{e.requester, e.introducer, e.stranger} {
		if _, err := owner.Exec(ctx,
			`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Rep')`,
			u, "rep-"+u.String()+"@intro.test"); err != nil {
			t.Fatal(err)
		}
	}
	// The contact is OWNED by the requester, so a rep on own-scope can open it
	// — which is what makes the unseen one below a real refusal rather than a
	// caller who could not read any contact at all.
	if _, err := owner.Exec(ctx,
		`INSERT INTO person (id, full_name, source, captured_by, owner_id)
		 VALUES ($1, 'Dana Buyer', 'manual', 'test', $2)`, e.contact, e.requester); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx,
		`INSERT INTO person (id, full_name, source, captured_by, owner_id)
		 VALUES ($1, 'Someone Else', 'manual', 'test', $2)`, e.unseen, e.stranger); err != nil {
		t.Fatal(err)
	}
	pool, err := testdb.Pool(ctx, appDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testdb.AssertPoolsQuiesced(t) })
	e.pool = pool
	e.store = NewStore(
		database.BindTo(pool, ids.From[ids.WorkspaceKind](e.ws)),
		func() time.Time { return testNow },
	)
	return e
}

// asUser is a rep holding the grants the role defaults give them. The grant
// admits them to the surface; which PARTY they are on a given ask is the row's
// own check, which is the thing most of this file is about.
func (e *introEnv) asUser(u ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + u.String(), UserID: u,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects: map[string]principal.ObjectGrant{
				"introduction": {Create: true, Read: true, Update: true},
				"person":       {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}

func (e *introEnv) ask() NewRequest {
	return NewRequest{
		PersonID:       e.contact,
		IntroducerUser: e.introducer,
		RouteType:      "direct",
		InternalReason: "Dana reopened the retrofit conversation after 41 days.",
		DueAt:          testNow.AddDate(0, 0, 7),
	}
}

// The write shape, and the guard that only a real index can hold: two tabs
// pressing send both pass any read-then-write check, and only one may pass the
// partial unique index.
func TestAnAskLandsInTheWriteShapeAndCannotBeMadeTwice(t *testing.T) {
	e := setupIntro(t)
	id, err := e.store.Create(e.asUser(e.requester), e.ask())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	var audits, events int
	ctx := context.Background()
	if err := e.owner.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE entity_type = 'intro_request' AND entity_id = $1`,
		id).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if err := e.owner.QueryRow(ctx,
		`SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'intro_request.created'`).
		Scan(&events); err != nil {
		t.Fatal(err)
	}
	if audits != 1 || events != 1 {
		t.Fatalf("write shape: %d audit rows, %d events — want one of each", audits, events)
	}

	_, err = e.store.Create(e.asUser(e.requester), e.ask())
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("a second open ask on the same route gave %v; want a conflict", err)
	}
}

// A colleague who declines has answered. Asking them again about the same
// contact is the product forgetting a refusal it was told about — but once the
// ask is closed, a NEW one is a fresh question and must be allowed.
func TestAClosedAskFreesTheRouteAgain(t *testing.T) {
	e := setupIntro(t)
	id, err := e.store.Create(e.asUser(e.requester), e.ask())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := e.store.Decide(
		e.asUser(e.introducer), id, StatusDeclined, "not close enough", nil, 1); err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if _, err := e.store.Create(e.asUser(e.requester), e.ask()); err != nil {
		t.Errorf("a settled ask still blocked the route: %v", err)
	}
}

// The colleague's answer is the colleague's to give, and a stranger cannot
// even see that the ask exists: telling them it is not theirs would disclose
// that this colleague was asked about this contact, which is the fact the row
// is for.
func TestOnlyThePartiesReachTheAsk(t *testing.T) {
	e := setupIntro(t)
	id, err := e.store.Create(e.asUser(e.requester), e.ask())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	err = e.store.Decide(e.asUser(e.requester), id, StatusAccepted, "", nil, 1)
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("the requester accepted their own ask (%v)", err)
	}
	err = e.store.Decide(e.asUser(e.stranger), id, StatusAccepted, "", nil, 1)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a stranger got %v; want not-found, which does not disclose the ask", err)
	}
	// The admit case, without which the two refusals above would pass against
	// a store that refused everybody.
	if err := e.store.Decide(e.asUser(e.introducer), id, StatusAccepted, "", nil, 1); err != nil {
		t.Fatalf("the colleague could not answer their own ask: %v", err)
	}
}

// Permission to mention a name is not a handshake. Completing a name-drop
// records a name-drop, and the state the ask is in decides that — never the
// caller, who would otherwise be able to report a door that never opened.
func TestCompletingANameDropRecordsANameDrop(t *testing.T) {
	e := setupIntro(t)
	id, err := e.store.Create(e.asUser(e.requester), e.ask())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := e.store.Decide(
		e.asUser(e.introducer), id, StatusNameDropApproved, "", nil, 1); err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if err := e.store.Complete(e.asUser(e.requester), id, nil, 2); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	var status string
	var introducedAt, nameDroppedAt *time.Time
	if err := e.owner.QueryRow(context.Background(),
		`SELECT status, introduced_at, name_dropped_at FROM intro_request WHERE id = $1`, id).
		Scan(&status, &introducedAt, &nameDroppedAt); err != nil {
		t.Fatal(err)
	}
	if status != string(StatusNameDropped) {
		t.Errorf("a completed name-drop reads as %q", status)
	}
	if introducedAt != nil {
		t.Error("a name-drop stamped introduced_at — the record now claims a handshake")
	}
	if nameDroppedAt == nil {
		t.Error("a name-drop recorded no time for itself")
	}
}

// Two tabs open on one ask, one accepting and one declining, must not both
// win: the loser is told the record moved rather than overwriting an answer
// already given.
func TestAStaleVersionCannotOverwriteAnAnswer(t *testing.T) {
	e := setupIntro(t)
	id, err := e.store.Create(e.asUser(e.requester), e.ask())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := e.store.Decide(e.asUser(e.introducer), id, StatusAccepted, "", nil, 1); err != nil {
		t.Fatalf("Decide: %v", err)
	}
	err = e.store.Decide(e.asUser(e.introducer), id, StatusDeclined, "", nil, 1)
	if !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Errorf("the second tab's decline gave %v; want a version skew", err)
	}
}

// Naming a record is reading it, and the probe is strict: the contact must
// EXIST and be live. Art. 17 erasure anonymizes a person in place and stamps
// archived_at, so an ask that could still name the tombstone would keep a
// erased person's name in front of a colleague.
//
// Row scope is deliberately NOT what this holds. Customer identity is
// workspace-readable here — every rep reads every contact — so the guard that
// matters is existence and liveness, not ownership.
func TestAnAskCannotNameAContactThatIsGoneOrErased(t *testing.T) {
	e := setupIntro(t)

	missing := e.ask()
	missing.PersonID = ids.NewV7()
	if _, err := e.store.Create(e.asUser(e.requester), missing); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("an ask about a contact that does not exist gave %v; want not-found", err)
	}

	if _, err := e.owner.Exec(context.Background(),
		`UPDATE person SET archived_at = now() WHERE id = $1`, e.unseen); err != nil {
		t.Fatal(err)
	}
	erased := e.ask()
	erased.PersonID = e.unseen
	if _, err := e.store.Create(e.asUser(e.requester), erased); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("an ask named an erased contact (%v)", err)
	}

	// The intermediary goes through the same probe, and this is the case that
	// would otherwise leak: routing an ask THROUGH an erased contact.
	through := e.ask()
	through.RouteType = "through_contact"
	through.ThroughPersonID = &e.unseen
	if _, err := e.store.Create(e.asUser(e.requester), through); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("an ask routed through an erased contact (%v)", err)
	}

	// The admit case, without which every refusal above would pass against a
	// store that refused every ask.
	if _, err := e.store.Create(e.asUser(e.requester), e.ask()); err != nil {
		t.Errorf("a live contact could not be asked about: %v", err)
	}
}
