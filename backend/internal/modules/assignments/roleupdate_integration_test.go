// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package assignments

// The role vocabulary's write path, against a real database.
//
// This file exists because of a defect no unit test could have caught: the
// patch was applied with ApplyWithVersion, which always narrows with
// `archived_at IS NULL`, and record_role has no such column — retirement here
// is `active = false`. Every relabel, reorder and retirement failed with an
// undefined-column error, and nothing noticed, because the write path was
// never once exercised against real SQL. The three cases below are the ones a
// settings page actually performs.

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type roleEnv struct {
	store *Store
	pool  *pgxpool.Pool
	owner *pgx.Conn
	ws    ids.UUID
	admin ids.UUID
}

func setupRoleEnv(t *testing.T) *roleEnv {
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
	e := &roleEnv{owner: owner, ws: ids.NewV7(), admin: ids.NewV7()}
	if _, err := owner.Exec(ctx, `INSERT INTO workspace (id) VALUES ($1)`, e.ws); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Admin')`,
		e.admin, "admin-"+e.admin.String()+"@roles.test"); err != nil {
		t.Fatal(err)
	}
	pool, err := testdb.Pool(ctx, appDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testdb.AssertPoolsQuiesced(t) })
	e.pool = pool
	e.store = NewStore(database.BindTo(pool, ids.From[ids.WorkspaceKind](e.ws)))
	return e
}

func (e *roleEnv) as() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.admin.String(), UserID: e.admin,
		Permissions: principal.Permissions{
			RoleKeys: []string{"admin"},
			Objects: map[string]principal.ObjectGrant{
				"custom_field": {Create: true, Read: true, Update: true},
			},
		},
	})
}

// TestAdministeringARoleReachesTheDatabase holds the three writes a settings
// page performs. Each asserts the row afterwards, because the defect this file
// was written for returned an error the handler surfaced as a 500 — a test
// that only checked "no error" from a call it never made would have passed.
func TestAdministeringARoleReachesTheDatabase(t *testing.T) {
	e := setupRoleEnv(t)
	ctx := e.as()

	created, err := e.store.CreateRecordRole(ctx, CreateRecordRoleInput{
		Label:         "Renewal owner",
		RecordTypes:   []crmcontracts.AssignmentRecordType{crmcontracts.AssignmentRecordTypeDeal},
		AssigneeKinds: []crmcontracts.AssignmentSubjectKind{crmcontracts.AssignmentSubjectKindUser},
		SortOrder:     90,
	})
	if err != nil {
		t.Fatalf("creating a role: %v", err)
	}
	if created.Key != "renewal_owner" {
		t.Fatalf("key derived from the label = %q, want renewal_owner", created.Key)
	}

	// Relabel. The failing case: this is the call that answered with an
	// undefined-column error for every role in the vocabulary.
	renamed, err := e.store.UpdateRecordRole(ctx, ids.UUID(created.Id), UpdateRecordRoleInput{
		Label: ptr("Renewal lead"),
	})
	if err != nil {
		t.Fatalf("relabelling a role: %v", err)
	}
	if renamed.Label != "Renewal lead" {
		t.Fatalf("label after relabel = %q, want Renewal lead", renamed.Label)
	}
	// The KEY does not move with the label, which is what makes a report keyed
	// on it comparable across the rename.
	if renamed.Key != created.Key {
		t.Fatalf("key moved with the label: %q -> %q", created.Key, renamed.Key)
	}

	// Retire. `active = false`, never a delete and never an archived_at.
	retired, err := e.store.UpdateRecordRole(ctx, ids.UUID(created.Id), UpdateRecordRoleInput{
		Active: ptr(false),
	})
	if err != nil {
		t.Fatalf("retiring a role: %v", err)
	}
	if retired.Active {
		t.Fatal("the role is still active after being retired")
	}

	// And back: a retired role can be reinstated, so an administrator who
	// retired the wrong one is not stuck with it.
	revived, err := e.store.UpdateRecordRole(ctx, ids.UUID(created.Id), UpdateRecordRoleInput{
		Active: ptr(true),
	})
	if err != nil {
		t.Fatalf("reinstating a role: %v", err)
	}
	if !revived.Active {
		t.Fatal("the role did not come back")
	}
}

// TestTheSeededVocabularyIsReadable proves the migration's seed survives the
// reset, which it did not until record_role joined the preserved reference
// tables: every integration test saw an empty vocabulary and a picker with
// nothing in it.
func TestTheSeededVocabularyIsReadable(t *testing.T) {
	e := setupRoleEnv(t)
	roles, err := e.store.ListRecordRoles(e.as())
	if err != nil {
		t.Fatalf("listing roles: %v", err)
	}
	if len(roles) < 7 {
		t.Fatalf("the seeded vocabulary has %d roles, want at least the 7 the migration writes", len(roles))
	}
	var found bool
	for _, role := range roles {
		if role.Key == "account_manager" {
			found = true
		}
	}
	if !found {
		t.Fatal("account_manager is missing from the seeded vocabulary")
	}
}

func ptr[T any](v T) *T { return &v }
