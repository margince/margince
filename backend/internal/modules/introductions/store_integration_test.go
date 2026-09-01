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

	// The row is only half the record. A dispute about whether an introduction
	// actually happened is settled from the audit trail, so an after-image
	// reading `introduced` would put the claimed handshake back exactly where
	// it carries the most weight.
	var after string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT after ->> 'status' FROM audit_log
		  WHERE entity_type = 'intro_request' AND entity_id = $1 AND action = 'update'
		  ORDER BY occurred_at DESC LIMIT 1`, id).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != string(StatusNameDropped) {
		t.Errorf("the audit trail records a completed name-drop as %q", after)
	}
}

// The version check in Go reads a row it does not lock, so two transactions can
// both pass it. Only `AND version = $N` inside the UPDATE is atomic, and it
// reports a loss by matching zero rows. If nothing reads that count, the loser
// still writes an audit row and emits an event for a move it never made.
func TestALostRaceWritesNoAuditTrail(t *testing.T) {
	e := setupIntro(t)
	id, err := e.store.Create(e.asUser(e.requester), e.ask())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := e.store.Decide(e.asUser(e.introducer), id, StatusAccepted, "", nil, 1); err != nil {
		t.Fatalf("Decide: %v", err)
	}
	// The race has to happen the way a real one does. An interloper that
	// commits BEFORE the call starts is caught by the Go-side version check,
	// which proves nothing about the UPDATE. So: open a transaction, take a
	// row lock, let Complete read version 2 and block on the lock, then commit
	// the interloper. Complete resumes with a version the row no longer has,
	// and only its own UPDATE can still tell.
	ctx := context.Background()
	blocker, err := e.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := blocker.Exec(ctx,
		`SELECT 1 FROM intro_request WHERE id = $1 FOR UPDATE`, id); err != nil {
		t.Fatal(err)
	}

	before := auditRowsFor(t, e, id)
	done := make(chan error, 1)
	go func() { done <- e.store.Complete(e.asUser(e.requester), id, nil, 2) }()

	// Let the goroutine reach the lock. It is waiting on Postgres, so this is
	// a poll of pg_locks and not a sleep-and-hope.
	waitForLockOn(t, e, id)
	if _, err := blocker.Exec(ctx,
		`UPDATE intro_request SET version = version + 1 WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if err := blocker.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	if err := <-done; !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Errorf("a caller whose UPDATE matched no row was told: %v", err)
	}
	if got := auditRowsFor(t, e, id); got != before {
		t.Errorf("a lost race wrote %d audit rows for a move it did not make", got-before)
	}
}

// waitForLockOn blocks until a backend is waiting for a lock on THIS ask's row,
// which is the observable fact that the racing call reached it and stopped.
//
// The wait is scoped to the row rather than to "any ungranted lock": a shared
// database has other traffic, and a test that took somebody else's contention
// as its signal would race past the setup it is trying to establish and then
// fail somewhere else.
func waitForLockOn(t *testing.T, e *introEnv, id ids.UUID) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var waiting bool
		// A backend blocked on a row lock in THIS database, while the ask still
		// exists. pg_blocking_pids is the direct question — "is somebody stuck
		// behind somebody else" — and it names the waiter rather than counting
		// ungranted locks the rest of the machine happens to hold.
		if err := e.owner.QueryRow(context.Background(), `
			SELECT EXISTS (
				SELECT 1 FROM pg_stat_activity
				 WHERE datname = current_database()
				   AND cardinality(pg_blocking_pids(pid)) > 0)
			   AND EXISTS (SELECT 1 FROM intro_request WHERE id = $1)`,
			id).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
	}
	t.Fatal("no backend ever blocked on the row — the race never happened")
}

func auditRowsFor(t *testing.T, e *introEnv, id ids.UUID) int {
	t.Helper()
	var n int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE entity_type = 'intro_request' AND entity_id = $1`,
		id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
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

// asCapture is the reply consumer's own principal: a system actor, unbounded by
// design, because an ask between two colleagues is not readable by anybody the
// consumer could pretend to be.
func (e *introEnv) asCapture() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:intro-advance",
	})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}

// evidence seeds a real captured message and answers its id.
//
// source_activity_id carries a foreign key to activity, which is the contract:
// the evidence a reply rests on must be a message somebody can open. A test
// passing a made-up id would be describing a row the database refuses.
func (e *introEnv) evidence(t *testing.T) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO activity (id, kind, subject, direction, source, captured_by)
		VALUES ($1, 'email', 'Re: retrofit', 'inbound', 'sync', 'test')`, id); err != nil {
		t.Fatal(err)
	}
	return id
}

// introduced walks a fresh ask to the handshake and answers its id.
func (e *introEnv) introduced(t *testing.T) ids.UUID {
	t.Helper()
	id, err := e.store.Create(e.asUser(e.requester), e.ask())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := e.store.Decide(e.asUser(e.introducer), id, StatusAccepted, "", nil, 1); err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if err := e.store.Complete(e.asUser(e.introducer), id, nil, 2); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	return id
}

// The reply the product may record, and the write shape that carries it.
//
// `replied` is the one status no endpoint can reach, so this is the ONLY path
// that produces it — and it still audits and emits like every other mutation.
func TestACapturedReplyClosesTheAskAndLeavesATrail(t *testing.T) {
	e := setupIntro(t)
	id := e.introduced(t)
	evidence := e.evidence(t)
	repliedAt := testNow.Add(time.Hour)

	recorded, err := e.store.RecordReply(e.asCapture(), id, evidence, repliedAt)
	if err != nil {
		t.Fatalf("RecordReply: %v", err)
	}
	if !recorded {
		t.Fatal("a reply to an introduced ask recorded nothing")
	}

	ctx := context.Background()
	var status string
	var at *time.Time
	var source *ids.UUID
	if err := e.owner.QueryRow(ctx,
		`SELECT status, replied_at, source_activity_id FROM intro_request WHERE id = $1`,
		id).Scan(&status, &at, &source); err != nil {
		t.Fatal(err)
	}
	if status != string(StatusReplied) {
		t.Errorf("status is %q; want replied", status)
	}
	// The message's own instant, not the consumer's clock: a mail sent Friday
	// and captured Monday was answered on Friday.
	if at == nil || !at.Equal(repliedAt) {
		t.Errorf("replied_at is %v; want the message's own %v", at, repliedAt)
	}
	// The evidence the claim rests on, so a reader can open the message.
	if source == nil || *source != evidence {
		t.Errorf("source_activity_id is %v; want the message %v", source, evidence)
	}

	var events int
	if err := e.owner.QueryRow(ctx,
		`SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'intro_request.replied'`).
		Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Errorf("%d intro_request.replied events; want one", events)
	}
}

// A second message on the same thread is the ordinary case, not a fault.
//
// The bus is at-least-once and a captured thread carries many messages, so this
// path is walked constantly. It must write nothing, emit nothing, and report
// itself as having done nothing — an error here would wedge the consumer group
// on a message that will never stop being a duplicate.
func TestASecondReplyChangesNothingAndIsNotAnError(t *testing.T) {
	e := setupIntro(t)
	id := e.introduced(t)
	first := e.evidence(t)
	if _, err := e.store.RecordReply(e.asCapture(), id, first, testNow.Add(time.Hour)); err != nil {
		t.Fatalf("the first reply: %v", err)
	}

	recorded, err := e.store.RecordReply(
		e.asCapture(), id, e.evidence(t), testNow.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("a second reply errored: %v", err)
	}
	if recorded {
		t.Error("a second reply reported itself as having recorded something")
	}

	ctx := context.Background()
	var source ids.UUID
	var version int
	if err := e.owner.QueryRow(ctx,
		`SELECT source_activity_id, version FROM intro_request WHERE id = $1`,
		id).Scan(&source, &version); err != nil {
		t.Fatal(err)
	}
	// The FIRST message stays the evidence. A later one overwriting it would
	// re-point the trail at a message that did not close the ask.
	if source != first {
		t.Errorf("the evidence moved to a later message (%v); want the first %v", source, first)
	}

	var events int
	if err := e.owner.QueryRow(ctx,
		`SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'intro_request.replied'`).
		Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Errorf("%d replied events after two messages; want one", events)
	}
}

// A reply can only follow a handshake or a lent name.
//
// An ask still waiting on the colleague has had no introduction to answer, so a
// message from the contact — which arrives constantly, since the rep is already
// corresponding with them — must not close it. This is the clause that keeps
// `requested` from silently becoming `replied` on ordinary traffic.
func TestOnlyAHandshakeCanBeReplied(t *testing.T) {
	e := setupIntro(t)
	id, err := e.store.Create(e.asUser(e.requester), e.ask())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	recorded, err := e.store.RecordReply(
		e.asCapture(), id, e.evidence(t), testNow.Add(time.Hour))
	if err != nil {
		t.Fatalf("RecordReply on a pending ask errored: %v", err)
	}
	if recorded {
		t.Error("a message closed an ask nobody had answered yet")
	}
	var status string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT status FROM intro_request WHERE id = $1`, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != string(StatusRequested) {
		t.Errorf("a pending ask moved to %q", status)
	}
}

// AwaitingReply answers only what a reply could close, and says when the wait
// started.
//
// `Since` is the whole reason this method exists rather than the consumer
// reading statuses itself: capture backfills years of correspondence, and every
// one of those messages is inbound and from the contact. Without the instant to
// compare against, the first import would mark every introduction ever made as
// answered.
func TestAwaitingReplyNamesTheHandshakeInstant(t *testing.T) {
	e := setupIntro(t)
	id := e.introduced(t)

	pending, err := e.store.AwaitingReply(e.asCapture(), e.contact)
	if err != nil {
		t.Fatalf("AwaitingReply: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != id {
		t.Fatalf("AwaitingReply gave %v; want the one introduced ask %v", pending, id)
	}
	// The handshake instant, which is what a message's occurred_at is compared
	// against. The store's clock is testNow, so that is when Complete landed.
	if !pending[0].Since.Equal(testNow) {
		t.Errorf("Since is %v; want the handshake instant %v", pending[0].Since, testNow)
	}

	if _, err := e.store.RecordReply(
		e.asCapture(), id, e.evidence(t), testNow.Add(time.Hour)); err != nil {
		t.Fatalf("RecordReply: %v", err)
	}
	after, err := e.store.AwaitingReply(e.asCapture(), e.contact)
	if err != nil {
		t.Fatalf("AwaitingReply after the reply: %v", err)
	}
	if len(after) != 0 {
		t.Errorf("an answered ask is still awaiting a reply: %v", after)
	}
}

// A lent name that was used is answerable too, and it stays a name-drop.
//
// The two paths reach `replied` from different states, and the transition table
// admits both. A test that only walked the handshake would leave the name-drop
// path unreachable without anything failing.
func TestAUsedNameDropCanBeAnsweredToo(t *testing.T) {
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

	recorded, err := e.store.RecordReply(
		e.asCapture(), id, e.evidence(t), testNow.Add(time.Hour))
	if err != nil {
		t.Fatalf("RecordReply: %v", err)
	}
	if !recorded {
		t.Fatal("a contact answering a name-drop recorded nothing")
	}
	// The before-image in the trail says name_dropped, not introduced. A
	// dispute about whether a door was opened is settled by this row.
	var before string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT before->>'status' FROM audit_log
		  WHERE entity_type = 'intro_request' AND entity_id = $1
		    AND after->>'status' = 'replied'`, id).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if before != string(StatusNameDropped) {
		t.Errorf("the trail says the reply followed %q; want name_dropped", before)
	}
}

// The receipt names the message the CURRENT claim rests on.
//
// One column serves three claims — introduced, name_dropped, replied — so a
// reply has to REPLACE the handshake's evidence rather than sit behind it. A
// row saying `replied` whose source_activity_id is the introduction sends a
// reader who opens the receipt to the wrong mail: they check the claim and find
// the message that preceded it.
//
// This is the branch a coalescing write leaves untested — every other case here
// completes with no evidence, so the old value was null and any rule looked
// right.
func TestAReplyReplacesTheHandshakesEvidence(t *testing.T) {
	e := setupIntro(t)
	id, err := e.store.Create(e.asUser(e.requester), e.ask())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := e.store.Decide(e.asUser(e.introducer), id, StatusAccepted, "", nil, 1); err != nil {
		t.Fatalf("Decide: %v", err)
	}
	// The handshake cites its own message, which is the case the reply must
	// then overwrite.
	handshake := e.evidence(t)
	linkEvidence(t, e, handshake, e.contact)
	if err := e.store.Complete(e.asUser(e.introducer), id, &handshake, 2); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	answer := e.evidence(t)
	if _, err := e.store.RecordReply(
		e.asCapture(), id, answer, testNow.Add(time.Hour)); err != nil {
		t.Fatalf("RecordReply: %v", err)
	}

	var source ids.UUID
	if err := e.owner.QueryRow(context.Background(),
		`SELECT source_activity_id FROM intro_request WHERE id = $1`, id).Scan(&source); err != nil {
		t.Fatal(err)
	}
	if source != answer {
		t.Errorf("the receipt on a replied ask is %v; want the reply %v, not the "+
			"handshake %v — a reader opening it would check the wrong message",
			source, answer, handshake)
	}

	// The handshake's evidence is not lost, only moved: the audit row for that
	// transition still carries it, which is where a dispute about whether the
	// introduction happened is settled.
	var trail int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log
		  WHERE entity_type = 'intro_request' AND entity_id = $1
		    AND after->>'status' = 'introduced'`, id).Scan(&trail); err != nil {
		t.Fatal(err)
	}
	if trail != 1 {
		t.Errorf("%d audit rows record the handshake; want one", trail)
	}
}

// linkEvidence files a message under a contact, which is what Complete's
// evidence check requires: an activity cited as proof has to be about the ask's
// own contact.
func linkEvidence(t *testing.T, e *introEnv, activity, person ids.UUID) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO activity_link (activity_id, entity_type, person_id)
		VALUES ($1, 'person', $2)`, activity, person); err != nil {
		t.Fatal(err)
	}
}
