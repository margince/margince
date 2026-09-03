// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A finding is visible exactly where the deal it is about is.
//
// Deals are an IDENTITY table — workspace-readable by every seat, with the
// write arm rather than the read arm keeping one the owner's — so a rep sees a
// colleague's finding today, and that is correct rather than a leak.
//
// What this test holds is the COUPLING: the list reaches a reader only through
// the deal's own visibility, so the day a record type arrives scoped, or a deal
// stops being workspace-readable, this narrows with it. Reading the exception
// table directly would leave a second answer to "who may see this deal" that
// nothing keeps in step — and the archived case below is where the two already
// differ.
//
// A unit test cannot see any of it: the predicate is SQL, built from the
// caller's own grants.

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAFindingIsVisibleExactlyWhereItsDealIs(t *testing.T) {
	t.Parallel()
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

	wsTyped := ids.New[ids.WorkspaceKind]()
	ws := wsTyped.UUID
	mine, theirs := ids.NewV7(), ids.NewV7()
	if _, err := owner.Exec(ctx, `INSERT INTO workspace (id) VALUES ($1)`, ws); err != nil {
		t.Fatal(err)
	}
	for _, user := range []ids.UUID{mine, theirs} {
		if _, err := owner.Exec(ctx,
			`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Rep')`,
			user, "rep-"+user.String()+"@scope.test"); err != nil {
			t.Fatal(err)
		}
	}

	// Two deals, two owners, and a finding on each. The scan writes both,
	// because it reads the whole pipeline.
	myDeal, theirDeal, goneDeal := ids.NewV7(), ids.NewV7(), ids.NewV7()
	// A deal needs its pipeline and stage: seeded the way the rest of the tree
	// seeds one, so the row is the shape the real writer produces rather than
	// the minimum the columns admit.
	pipeline, stage := ids.NewV7(), ids.NewV7()
	if _, err := owner.Exec(ctx,
		`INSERT INTO pipeline (id, name) VALUES ($1, 'Scope')`, pipeline); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx,
		`INSERT INTO stage (id, pipeline_id, name, "position") VALUES ($1, $2, 'Qualified', 1)`,
		stage, pipeline); err != nil {
		t.Fatal(err)
	}
	for deal, dealOwner := range map[ids.UUID]ids.UUID{
		myDeal: mine, theirDeal: theirs, goneDeal: mine,
	} {
		if _, err := owner.Exec(ctx,
			`INSERT INTO deal (id, name, status, owner_id, pipeline_id, stage_id, source, captured_by)
			 VALUES ($1, 'Deal', 'open', $2, $3, $4, 'seed', 'test')`,
			deal, dealOwner, pipeline, stage); err != nil {
			t.Fatal(err)
		}
		if _, err := owner.Exec(ctx,
			`INSERT INTO assurance_exception
			     (logical_key, type, subject_kind, subject_id, severity, owner_id, captured_by)
			 VALUES ($1, 'close_past', 'deal', $2, 'high', $3, 'test')`,
			"close_past:"+deal.String(), deal, dealOwner); err != nil {
			t.Fatal(err)
		}
	}

	// One of them is archived after the fact, the way a real deal leaves.
	if _, err := owner.Exec(ctx,
		`UPDATE deal SET archived_at = now() WHERE id = $1`, goneDeal); err != nil {
		t.Fatal(err)
	}

	pool, err := testdb.Pool(ctx, appDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testdb.AssertPoolsQuiesced(t) })

	// A rep who may read only their OWN deals.
	repCtx := principal.WithWorkspaceID(context.Background(), ws)
	repCtx = principal.WithCorrelationID(repCtx, ids.NewV7())
	repCtx = principal.WithActor(repCtx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + mine.String(), UserID: mine,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects: map[string]principal.ObjectGrant{
				"forecast": {Read: true},
				"deal":     {Read: true},
			},
			RowScope: principal.RowScopeOwn,
		},
	})

	var found []assuranceExceptionSubject
	if err := database.WithWorkspaceTx(repCtx, database.BindTo(pool, wsTyped).Pool(),
		func(tx pgx.Tx) error {
			rows, err := AssuranceExceptions(repCtx, tx)
			if err != nil {
				return err
			}
			for _, row := range rows {
				found = append(found, assuranceExceptionSubject{row.SubjectID})
			}
			return nil
		}); err != nil {
		t.Fatalf("listing the findings: %v", err)
	}

	sawMine, sawTheirs, sawArchived := false, false, false
	for _, row := range found {
		switch row.subject {
		case myDeal:
			sawMine = true
		case theirDeal:
			sawTheirs = true
		case goneDeal:
			sawArchived = true
		}
	}
	if !sawMine {
		t.Error("the caller's own finding was not listed — the review screen would sit " +
			"empty while the pipeline had problems")
	}
	// A colleague's deal IS readable: deals are workspace-readable and the
	// write arm is what keeps one theirs. Asserting the opposite would pin a
	// rule the product does not have.
	if !sawTheirs {
		t.Error("a colleague's finding was withheld — deals are workspace-readable, so " +
			"this narrowed past the product's own rule")
	}
	// The coupling, and the case where reading assurance_exception directly
	// would already differ: an archived deal is invisible, so a finding about
	// one must be too. It is still in the table.
	if sawArchived {
		t.Error("a finding about an ARCHIVED deal was listed — the deal is gone from " +
			"every live read and its finding has to go with it")
	}
}

type assuranceExceptionSubject struct{ subject ids.UUID }
