// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

// A member's connection made before connections carried a sending company gets
// its owner's company: the legal entity the member belongs to, else the
// default one. A connection with no owner stays unscoped, and one that already
// names a company keeps it.
func TestConnectionsMadeBeforeTheScopeGetTheirOwnersCompany(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()

	core, err := migrations.CoreHistory()
	if err != nil {
		t.Fatalf("loading core: %v", err)
	}
	through := namespaceThroughVersion(t, core, "1795180850")
	if _, err := dbmigrate.Up(ctx, conn, through); err != nil {
		t.Fatalf("up: %v", err)
	}
	if _, err := dbmigrate.Down(ctx, conn, through, 1); err != nil {
		t.Fatalf("undoing the backfill: %v", err)
	}

	branchRep := seedAppUserForConnection(t, conn, "branch@scope.example")
	defaultRep := seedAppUserForConnection(t, conn, "default@scope.example")
	branch, defaultEntity := vouchEntities(t, conn, branchRep)
	kept := scalar(t, conn, `INSERT INTO legal_entity (display_name, legal_name, country, captured_by)
		VALUES ('Kept', 'Kept Ltd', 'at', 'human:x') RETURNING id`)

	byBranch := seedConnection(t, conn, &branchRep, nil)
	byDefault := seedConnection(t, conn, &defaultRep, nil)
	alreadyScoped := seedConnection(t, conn, &defaultRep, &kept)
	team := scalar(t, conn, `INSERT INTO team (name) VALUES ('Desk') RETURNING id`)
	ownerless := scalar(t, conn, `INSERT INTO capture_connection (provider, user_id, reader_team_id, connected_by, status, credential_ref, account_label, mail_posture)
		VALUES ('gmail', NULL, $1, $2, 'connected', 'vault:test', 'desk@scope.example', 'shared') RETURNING id`, team, defaultRep)

	if _, err := dbmigrate.Up(ctx, conn, core); err != nil {
		t.Fatalf("migrating back up: %v", err)
	}
	for name, tc := range map[string]struct{ id, want string }{
		"a branch member's":       {byBranch, branch},
		"a default member's":      {byDefault, defaultEntity},
		"one that already had it": {alreadyScoped, kept},
		"one with no owner":       {ownerless, ""},
	} {
		var got *string
		if err := conn.QueryRow(ctx, `SELECT legal_entity_id FROM capture_connection WHERE id = $1`, tc.id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if (tc.want == "" && got != nil) || (tc.want != "" && (got == nil || *got != tc.want)) {
			t.Errorf("%s connection has company %v, want %q", name, got, tc.want)
		}
	}
}

func seedConnection(t *testing.T, conn *pgx.Conn, user, entity *string) string {
	t.Helper()
	return scalar(t, conn, `INSERT INTO capture_connection (provider, user_id, status, credential_ref, account_label, legal_entity_id)
		VALUES ('gmail', $1, 'connected', 'vault:test', 'mailbox-' || gen_random_uuid()::text, $2) RETURNING id`, user, entity)
}
