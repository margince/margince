// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// WHO owes the reply, and how many promises each person has missed — against a
// real database, because both answers are produced by SQL and neither can be
// checked any other way.
//
// The ownership walk is four LEFT JOINs and a COALESCE over four aggregates
// inside a statement that already groups. Nothing in the unit lane executes it:
// a wrong join, an arm reading the ungated link table, or a precedence in the
// wrong order all compile, and the store simply returns a different person.

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

// loadEnv is one workspace with two reps who own records between them.
type loadEnv struct {
	owner *pgx.Conn
	pool  *pgxpool.Pool
	ws    ids.UUID
	rep   ids.UUID
	other ids.UUID
}

func setupLoad(t *testing.T) *loadEnv {
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
	e := &loadEnv{owner: owner, ws: ids.NewV7(), rep: ids.NewV7(), other: ids.NewV7()}
	if _, err := owner.Exec(ctx, `INSERT INTO workspace (id) VALUES ($1)`, e.ws); err != nil {
		t.Fatal(err)
	}
	for _, user := range []ids.UUID{e.rep, e.other} {
		if _, err := owner.Exec(ctx,
			`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Rep')`,
			user, "rep-"+user.String()+"@load.test"); err != nil {
			t.Fatal(err)
		}
	}
	pool, err := testdb.Pool(ctx, appDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testdb.AssertPoolsQuiesced(t) })
	e.pool = pool
	return e
}

// as binds the reader at ALL row scope. The board is a team surface, so this is
// the tier that actually reads it — and it keeps these assertions about the
// OWNERSHIP walk rather than about the scope clauses, which have their own
// tests.
func (e *loadEnv) as() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.rep.String(), UserID: e.rep,
		Permissions: principal.Permissions{
			RoleKeys: []string{"manager"},
			Objects: map[string]principal.ObjectGrant{
				"activity": {Read: true}, "person": {Read: true},
				"deal": {Read: true}, "organization": {Read: true},
				"lead": {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

func (e *loadEnv) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("seeding: %v", err)
	}
}

// waitFor reads who is waiting and returns the ONE row for the given message.
//
// By id rather than by taking the only row: the lane template is shared across
// this package's tests, so a message another test seeded is present too. An
// assertion over the whole answer would pass or fail on which tests ran.
func (e *loadEnv) waitFor(t *testing.T, activity ids.UUID) WaitingReply {
	t.Helper()
	rows, err := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws))).
		WaitingReplies(e.as(), time.Now())
	if err != nil {
		t.Fatalf("reading who is waiting: %v", err)
	}
	for _, row := range rows {
		if row.ActivityID == activity {
			return row
		}
	}
	t.Fatalf("the seeded wait is absent from %d returned rows — it qualifies on every "+
		"eligibility rule, so its absence is the read and not the fixture", len(rows))
	return WaitingReply{}
}

// seedWait writes one unanswered inbound filed under the records given, and
// returns its id. Every rule the eligibility query applies is satisfied here —
// a sales link, inside the horizon, a human sender, no later reply — so what the
// assertions then vary is only the OWNERSHIP.
func (e *loadEnv) seedWait(t *testing.T, subject string, link string, target ids.UUID) ids.UUID {
	t.Helper()
	activity := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, thread_key, source, captured_by)
		VALUES ($1, 'email', 'inbound', $2, now() - interval '2 days', $3, 'seed', 'system')`,
		activity, subject, "thread-"+activity.String())
	e.exec(t, `INSERT INTO activity_participant (id, activity_id, role, address)
		VALUES ($1, $2, 'from', 'buyer@customer.test')`, ids.NewV7(), activity)
	e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, `+link+`)
		VALUES ($1, $2, $3, $4)`, ids.NewV7(), activity, link[:len(link)-3], target)
	return activity
}

// A wait is attributed to the owner of the record it is filed under.
//
// The whole ownership walk fails silently if it is wrong: the query still
// returns the message, still returns one row, and simply names the wrong person
// — so the board blames a colleague for somebody else's customer.
func TestAWaitIsAttributedToTheOwnerOfItsRecord(t *testing.T) {
	e := setupLoad(t)
	person := ids.NewV7()
	e.exec(t, `INSERT INTO person (id, full_name, owner_id, source, captured_by)
		VALUES ($1, 'Buyer Person', $2, 'seed', 'system')`, person, e.other)
	activity := e.seedWait(t, "Question about pricing", "person_id", person)

	if got := e.waitFor(t, activity); got.OwnerID != e.other {
		t.Fatalf("the wait was attributed to %v, wanted the person's owner %v",
			got.OwnerID, e.other)
	}
}

// A DEAL on the thread outranks the person on it.
//
// The precedence is a COALESCE over four arms in one order, and getting it
// backwards is invisible without this: both owners are real people, both
// answers are one row, and the board simply credits the account owner with a
// conversation the deal owner is answerable for.
func TestADealOnTheThreadOutranksThePersonOnIt(t *testing.T) {
	e := setupLoad(t)
	person, deal, org := ids.NewV7(), ids.NewV7(), ids.NewV7()
	e.exec(t, `INSERT INTO organization (id, display_name, owner_id, source, captured_by)
		VALUES ($1, 'Customer GmbH', $2, 'seed', 'system')`, org, e.rep)
	e.exec(t, `INSERT INTO person (id, full_name, owner_id, source, captured_by)
		VALUES ($1, 'Buyer Person', $2, 'seed', 'system')`, person, e.rep)
	pipeline, stage := ids.NewV7(), ids.NewV7()
	// Named for its own id: pipeline_name_unique is installation-wide and the
	// lane template is shared across this package, so a fixed name collides with
	// whatever another test in this file seeded first.
	e.exec(t, `INSERT INTO pipeline (id, name) VALUES ($1, $2)`, pipeline, "Pipeline "+pipeline.String())
	e.exec(t, `INSERT INTO stage (id, pipeline_id, name, "position") VALUES ($1, $2, 'Qualified', 1)`,
		stage, pipeline)
	e.exec(t, `INSERT INTO deal (id, name, status, owner_id, organization_id, pipeline_id, stage_id, source, captured_by)
		VALUES ($1, 'Zeta renewal', 'open', $2, $3, $4, $5, 'seed', 'system')`,
		deal, e.other, org, pipeline, stage)

	activity := e.seedWait(t, "Contract question", "person_id", person)
	e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, deal_id)
		VALUES ($1, $2, 'deal', $3)`, ids.NewV7(), activity, deal)

	if got := e.waitFor(t, activity); got.OwnerID != e.other {
		t.Fatalf("the wait was attributed to %v, wanted the DEAL owner %v — the deal "+
			"outranks the person the thread is also filed under",
			got.OwnerID, e.other)
	}
}

// A wait on a record nobody owns names nobody, rather than picking somebody.
func TestAWaitOnAnUnownedRecordNamesNobody(t *testing.T) {
	e := setupLoad(t)
	person := ids.NewV7()
	e.exec(t, `INSERT INTO person (id, full_name, source, captured_by)
		VALUES ($1, 'Unowned Buyer', 'seed', 'system')`, person)
	activity := e.seedWait(t, "Question about pricing", "person_id", person)

	if got := e.waitFor(t, activity); !got.OwnerID.IsZero() {
		t.Fatalf("a wait on a record nobody owns was attributed to %v", got.OwnerID)
	}
}

// The overdue count is per assignee, counts only what is actually late, and
// reports the unassigned pile under the zero id.
func TestOverdueTasksAreCountedPerAssignee(t *testing.T) {
	e := setupLoad(t)
	seed := func(assignee *ids.UUID, due string, done bool) {
		// done_at travels with is_done: the activity_done_at CHECK refuses a
		// finished task with no moment on it.
		var doneAt *time.Time
		if done {
			at := time.Now()
			doneAt = &at
		}
		e.exec(t, `INSERT INTO activity (id, kind, subject, occurred_at, due_at, assignee_id, is_done, done_at, source, captured_by)
			VALUES ($1, 'task', 'A promise', now(), now() + $2::interval, $3, $4, $5, 'seed', 'system')`,
			ids.NewV7(), due, assignee, done, doneAt)
	}
	seed(&e.other, "-2 days", false)
	seed(&e.other, "-1 day", false)
	// Not late yet, so it is not overdue.
	seed(&e.other, "3 days", false)
	// Late but finished, so nobody owes it.
	seed(&e.other, "-5 days", true)
	// Late and nobody's.
	seed(nil, "-4 days", false)

	rows, err := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws))).
		OverdueLoadByAssignee(e.as(), time.Now())
	if err != nil {
		t.Fatalf("counting overdue tasks: %v", err)
	}
	per := map[ids.UUID]int{}
	for _, row := range rows {
		per[row.OwnerID] = row.Overdue
	}
	if per[e.other] != 2 {
		t.Errorf("the colleague was credited with %d overdue tasks, wanted the 2 that are "+
			"open and already late", per[e.other])
	}
	if per[ids.UUID{}] != 1 {
		t.Errorf("the unassigned overdue count was %d, wanted the 1 nobody holds",
			per[ids.UUID{}])
	}
	if _, counted := per[e.rep]; counted {
		t.Error("the reader was credited with overdue tasks they were never assigned")
	}
}

// The owner named is the owner OF the record named, not the smallest owner id
// among the links.
//
// A message may be filed under two deals — uq_activity_link keys on (activity,
// type, id), so a second deal link is a legal row. The record id and the owner
// were picked by two independent orderings, so a message could report deal D1
// and bill its wait to whoever owned D2. Both figures are real people and real
// deals, the row count is unchanged, and nothing but this says which pairing is
// right.
//
// The fixture pins the two orderings AGAINST each other: the deal that sorts
// first is owned by the user that sorts second. Order by owner and the answer is
// the other rep; order by the record and it is this one.
func TestAWaitNamesTheOwnerOfTheRecordItNames(t *testing.T) {
	e := setupLoad(t)
	pipeline, stage := ids.NewV7(), ids.NewV7()
	// Named for its own id: pipeline_name_unique is installation-wide and the
	// lane template is shared across this package, so a fixed name collides with
	// whatever another test in this file seeded first.
	e.exec(t, `INSERT INTO pipeline (id, name) VALUES ($1, $2)`, pipeline, "Pipeline "+pipeline.String())
	e.exec(t, `INSERT INTO stage (id, pipeline_id, name, "position") VALUES ($1, $2, 'Qualified', 1)`,
		stage, pipeline)

	// Two owners, told apart by how their ids sort.
	firstOwner, secondOwner := e.rep, e.other
	if secondOwner.String() < firstOwner.String() {
		firstOwner, secondOwner = secondOwner, firstOwner
	}
	// Two deals, likewise — and the FIRST deal goes to the SECOND owner, which
	// is what makes the two orderings disagree.
	firstDeal, secondDeal := ids.NewV7(), ids.NewV7()
	if secondDeal.String() < firstDeal.String() {
		firstDeal, secondDeal = secondDeal, firstDeal
	}
	e.exec(t, `INSERT INTO deal (id, name, status, owner_id, pipeline_id, stage_id, source, captured_by)
		VALUES ($1, 'First by id', 'open', $2, $3, $4, 'seed', 'system')`,
		firstDeal, secondOwner, pipeline, stage)
	e.exec(t, `INSERT INTO deal (id, name, status, owner_id, pipeline_id, stage_id, source, captured_by)
		VALUES ($1, 'Second by id', 'open', $2, $3, $4, 'seed', 'system')`,
		secondDeal, firstOwner, pipeline, stage)

	activity := e.seedWait(t, "Which deal is this about", "deal_id", firstDeal)
	e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, deal_id)
		VALUES ($1, $2, 'deal', $3)`, ids.NewV7(), activity, secondDeal)

	got := e.waitFor(t, activity)
	if got.DealID != firstDeal {
		t.Fatalf("the row named deal %v, wanted the first by id %v — the fixture no longer "+
			"pins the two orderings against each other", got.DealID, firstDeal)
	}
	if got.OwnerID != secondOwner {
		t.Fatalf("the wait named deal %v but was billed to %v, who owns the OTHER deal on "+
			"this thread — the owner must be the owner of the record named, which is %v",
			got.DealID, got.OwnerID, secondOwner)
	}
}
