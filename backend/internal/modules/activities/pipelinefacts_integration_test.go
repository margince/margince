// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// THE AGREEMENT TEST.
//
// The pipeline trace tells a member why the attention classifier did not read
// their message. That answer is only true while it agrees with the backlog query
// the classifier actually runs, and the two share one SQL fragment precisely so
// they cannot drift. This proves the fragment serves both callers over real
// rows, rather than trusting that sharing a string is enough.
//
// One row PER EXCLUSION, each excluded for exactly that reason, plus one
// eligible row. A test that seeded a single ineligible row would pass against a
// reader that always returned the same class — which is the bug that matters
// here, because a WRONG why is worse than no why: it sends a member looking for
// a transport problem when their message was simply archived.

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/pipelinetrace"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// factsEnv is one workspace, its owner connection and the store under test.
type factsEnv struct {
	owner *pgx.Conn
	store *Store
	ws    ids.UUID
	user  ids.UUID
}

func setupFacts(t *testing.T) *factsEnv {
	t.Helper()
	ownerDSN, appDSN := os.Getenv("MARGINCE_TEST_DSN"), os.Getenv("MARGINCE_TEST_APP_DSN")
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
	// To head before anything else touches this database: testdb.Pool refuses
	// until EnsureSchema has run, and EnsureSchema still REBUILDS whenever it
	// cannot prove the database is a fresh lane clone — so a seed written
	// before it would be dropped rather than reset.
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}
	e := &factsEnv{owner: owner, ws: ids.NewV7(), user: ids.NewV7()}
	e.exec(t, `INSERT INTO workspace (id) VALUES ($1)`, e.ws)
	e.exec(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Rep')`, e.user, "rep-"+e.user.String()+"@facts.test")
	pool, err := testdb.Pool(ctx, appDSN)
	if err != nil {
		t.Fatal(err)
	}
	// Registered where the pool is handed out, before the test adds any cleanup
	// of its own, so it runs last and sees a package that has genuinely stopped.
	// The pool outlives the test now, so a goroutine still holding a connection
	// would go on writing into the database the NEXT test just reset.
	t.Cleanup(func() { testdb.AssertPoolsQuiesced(t) })
	e.store = NewStore(database.BindTo(pool, ids.From[ids.WorkspaceKind](e.ws)))
	return e
}

func (e *factsEnv) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("seeding: %v", err)
	}
}

func (e *factsEnv) as() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.user.String(), UserID: e.user,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects:  map[string]principal.ObjectGrant{"activity": {Read: true}},
			RowScope: principal.RowScopeAll,
		},
	})
}

// capturedRow is one seeded message, described by what makes it (in)eligible.
type capturedRow struct {
	kind            string
	capturedBy      string
	archived        bool
	audience        string
	undecidedSender bool
	withPerson      bool
}

func (e *factsEnv) seed(t *testing.T, row capturedRow) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	capturedBy := row.capturedBy
	if capturedBy == "" {
		capturedBy = "connector:gmail"
	}
	email := "sender-" + id.String() + "@outside.test"
	audience := row.audience
	if audience == "" {
		audience = "workspace"
	}
	e.exec(t, `
		INSERT INTO activity (id, kind, occurred_at, source, captured_by, counterparty_email, archived_at, channel_provider, audience)
		VALUES ($1, $2, now(), 'test', $3, $4,
		        CASE WHEN $5 THEN now() ELSE NULL END,
		        CASE WHEN $2 = 'message' THEN 'telegram' ELSE NULL END,
		        $6)`,
		id, row.kind, capturedBy, email, row.archived, audience)
	if row.undecidedSender {
		e.exec(t, `
			INSERT INTO capture_pending_counterparty (id, owner_id, email, status, activity_id)
			VALUES ($1, $2, $3, 'pending', $4)`, ids.NewV7(), e.user, email, id)
	}
	if row.withPerson {
		person := ids.NewV7()
		e.exec(t, `INSERT INTO person (id, full_name, source, captured_by)
			VALUES ($1, 'Linked Person', 'test', 'connector:gmail')`, person)
		e.exec(t, `INSERT INTO activity_link (activity_id, entity_type, person_id)
			VALUES ($1, 'person', $2)`, id, person)
	}
	return id
}

func TestTheBacklogAndTheExplanationAgreeOnEveryExclusion(t *testing.T) {
	e := setupFacts(t)
	ctx := e.as()

	eligible := e.seed(t, capturedRow{kind: "email"})
	want := map[ids.UUID]pipelinetrace.Reason{
		eligible:                                pipelinetrace.ReasonAwaitingBatch,
		e.seed(t, capturedRow{kind: "message"}): pipelinetrace.ReasonTransportNotRead,
		e.seed(t, capturedRow{kind: "email", archived: true}):              pipelinetrace.ReasonArchived,
		e.seed(t, capturedRow{kind: "email", capturedBy: "human:someone"}): pipelinetrace.ReasonNotConnectorCaptured,
		e.seed(t, capturedRow{kind: "email", undecidedSender: true}):       pipelinetrace.ReasonSenderUndecided,
	}
	// The audience exclusion is asserted apart from the loop above, because the
	// trace read is itself audience-gated: to a reader outside the audience the
	// honest answer is "not found", and only the mailbox owner gets far enough
	// to be told WHY the classifier skipped it. Seeded with this fixture's own
	// user as the capturing mailbox so the read reaches the reason.
	limited := e.seed(t, capturedRow{
		kind: "email", audience: "participants",
		capturedBy: "connector:gmail:" + e.user.String(),
	})

	limitedFacts, err := e.store.ReadPipelineFacts(ctx, limited)
	if err != nil {
		t.Fatalf("the mailbox owner could not read their own limited message's trace: %v", err)
	}
	if limitedFacts.ClassifyReason != pipelinetrace.ReasonAudienceLimited {
		t.Errorf("reason for a limited message = %q, want %q — without its own reason the owner is told the batch "+
			"simply has not reached it, and waits for a pass that will never run",
			limitedFacts.ClassifyReason, pipelinetrace.ReasonAudienceLimited)
	}
	if limitedFacts.ClassifyEligible {
		t.Error("a limited message reports eligible — the reader and the backlog disagree, so the shared predicate is no longer shared")
	}

	// Half one: the classifier's own backlog selects EXACTLY the eligible row.
	backlog, err := e.store.UnlabeledCaptureEmails(ctx, 50, 200)
	if err != nil {
		t.Fatalf("reading the backlog: %v", err)
	}
	if len(backlog) != 1 || backlog[0].ID != eligible {
		t.Fatalf("the backlog selected %d row(s), want exactly the eligible one (%s)",
			len(backlog), eligible)
	}

	// Half two: the reader names the exclusion that applied to each of the rest,
	// and agrees with the backlog about which one was eligible.
	for id, reason := range want {
		facts, err := e.store.ReadPipelineFacts(ctx, id)
		if err != nil {
			t.Fatalf("reading pipeline facts for %s: %v", id, err)
		}
		if facts.ClassifyReason != reason {
			t.Errorf("reason for %s = %q, want %q", id, facts.ClassifyReason, reason)
		}
		if wantEligible := id == eligible; facts.ClassifyEligible != wantEligible {
			t.Errorf("eligible for %s = %v, want %v — the reader and the backlog "+
				"disagree, so the shared predicate is no longer shared",
				id, facts.ClassifyEligible, wantEligible)
		}
	}
}

func TestThePersonLinkIsWhatThePersonCreationRungReads(t *testing.T) {
	// The person-creation rung is derived by elimination, and the link is the
	// only durable signal it has — the same signal the nightly reconcile scans
	// for. If this read stopped seeing it, the rung would report "no contact
	// linked yet" for messages that have one.
	e := setupFacts(t)
	ctx := e.as()
	for id, want := range map[ids.UUID]bool{
		e.seed(t, capturedRow{kind: "email"}):                   false,
		e.seed(t, capturedRow{kind: "email", withPerson: true}): true,
	} {
		facts, err := e.store.ReadPipelineFacts(ctx, id)
		if err != nil {
			t.Fatalf("reading pipeline facts: %v", err)
		}
		if facts.HasPersonLink != want {
			t.Errorf("HasPersonLink for %s = %v, want %v", id, facts.HasPersonLink, want)
		}
	}
}

func TestReadingPipelineFactsTakesTheRowScopeNotJustTheGrant(t *testing.T) {
	// The object grant and the row scope are two different gates, and the
	// sibling below only removes the first. Every principal there reads the
	// seed's links, against which EnsureActivityContentVisible is a no-op by
	// construction — so deleting that call left nothing red.
	//
	// This is the other half: the grant is HELD, and the activity is linked only
	// to another rep's capture-private person (visibility='owner' — ownership
	// alone no longer hides a person), so only the link-walk can refuse.
	e := setupFacts(t)
	id := e.seed(t, capturedRow{kind: "email"})
	other := ids.NewV7()
	e.exec(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Other')`, other, "other-"+other.String()+"@facts.test")
	person := ids.NewV7()
	e.exec(t, `INSERT INTO person (id, full_name, source, captured_by, owner_id, visibility)
		VALUES ($1, 'Theirs', 'test', 'connector:gmail', $2, 'owner')`, person, other)
	e.exec(t, `INSERT INTO activity_link (activity_id, entity_type, person_id)
		VALUES ($1, 'person', $2)`, id, person)

	asUser := func(userID ids.UUID, scope principal.RowScope) context.Context {
		return principal.WithActor(
			principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.ws), ids.NewV7()),
			principal.Principal{
				Type: principal.PrincipalHuman, ID: "human:" + userID.String(), UserID: userID,
				Permissions: principal.Permissions{
					RoleKeys: []string{"rep"},
					Objects:  map[string]principal.ObjectGrant{"activity": {Read: true}},
					RowScope: scope,
				},
			})
	}

	// The allow arm over the same seed: the contact's owner reads it. Without
	// this, a link that failed to land would make the refusal below meaningless.
	if _, err := e.store.ReadPipelineFacts(asUser(other, principal.RowScopeOwn), id); err != nil {
		t.Fatalf("the private contact's owner could not read the seed: %v", err)
	}

	if _, err := e.store.ReadPipelineFacts(asUser(e.user, principal.RowScopeOwn), id); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound — the grant is held, so only the row "+
			"scope can refuse, and it must hide existence rather than deny", err)
	}
}

func TestReadingPipelineFactsTakesTheActivityGate(t *testing.T) {
	// The compose assembler gates the activity first, and that is still not
	// enough: a guard the caller supplies is one the next caller can forget.
	e := setupFacts(t)
	id := e.seed(t, capturedRow{kind: "email"})
	ungranted := principal.WithActor(
		principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.ws), ids.NewV7()),
		principal.Principal{
			Type: principal.PrincipalHuman, ID: "human:" + e.user.String(), UserID: e.user,
			Permissions: principal.Permissions{RoleKeys: []string{"rep"}, RowScope: principal.RowScopeAll},
		})
	if _, err := e.store.ReadPipelineFacts(ungranted, id); err == nil {
		t.Error("a caller with no activity grant read the pipeline facts")
	}
}

// The classifier's write re-tests the audience, not only the backlog that
// selected the row.
//
// The pass reads a batch, spends a model call per message and writes the
// answers back. A human or a verdict can limit a message inside that window.
// With the test only in the backlog query, the write lands after the narrowing
// and re-labels a message whose text the worklist's readers may no longer open
// — and nothing clears it a second time, because the narrowing already ran.
func TestTheLabelWriteLosesToANarrowingThatLandedWhileTheModelThought(t *testing.T) {
	e := setupFacts(t)
	ctx := e.as()

	open := e.seed(t, capturedRow{kind: "email"})
	// The row is OFFERED, which is what makes the write below a race rather
	// than a refusal that never had a chance. Asked of this row rather than of
	// the backlog's size: the package shares one database, so a sibling test's
	// rows are in it too and a count assertion would fail for their reason.
	backlog, err := e.store.UnlabeledCaptureEmails(ctx, 200, 200)
	if err != nil {
		t.Fatalf("reading the backlog: %v", err)
	}
	offered := false
	for _, row := range backlog {
		if row.ID == open {
			offered = true
		}
	}
	if !offered {
		t.Fatalf("the open message was not offered to the classifier — the fixture proves nothing about the race")
	}

	// The model call happens here, in production. The narrowing lands during it.
	e.exec(t, `UPDATE activity SET audience = 'participants' WHERE id = $1`, open)

	applied, err := e.store.SetCaptureLabel(ctx, open, "commitment")
	if err != nil {
		t.Fatalf("writing the label: %v", err)
	}
	if applied {
		t.Error("the classifier labelled a message limited while it was thinking — the label is that message's content on a worklist its audience excludes")
	}
	var label *string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT capture_label FROM activity WHERE id = $1`, open).Scan(&label); err != nil {
		t.Fatal(err)
	}
	if label != nil {
		t.Errorf("the limited message carries label %q", *label)
	}

	// An open message still gets its label: a re-check that refused everything
	// would pass every assertion above and stop attention routing entirely.
	stillOpen := e.seed(t, capturedRow{kind: "email"})
	if wrote, err := e.store.SetCaptureLabel(ctx, stillOpen, "commitment"); err != nil || !wrote {
		t.Errorf("an open message was not labelled (wrote=%v err=%v) — the re-check refuses more than the audience", wrote, err)
	}
}
