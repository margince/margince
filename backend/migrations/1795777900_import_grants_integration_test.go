// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

func TestMissingImportGrantsAreBackfilledWithoutReplacingAnOperatorsDenial(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()
	core, err := migrations.CoreHistory()
	if err != nil {
		t.Fatal(err)
	}
	prior := namespaceThroughVersion(t, core, "1795777013")
	if _, err := dbmigrate.Up(ctx, conn, prior); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO role(key,name,is_system,permissions)
 SELECT key,key,key<>'custom','{"objects":{"tag":{"read":true}}}'::jsonb
 FROM unnest(ARRAY['admin','ops','management','manager','rep','read_only','custom']) AS key
 ON CONFLICT(key) DO UPDATE SET permissions=excluded.permissions;
 UPDATE role SET permissions=jsonb_set(permissions,'{objects,import_run}',
 '{"create":false,"read":false,"update":false,"delete":false}') WHERE key='manager'`); err != nil {
		t.Fatal(err)
	}
	through := namespaceThroughVersion(t, core, "1795777900")
	if _, err := dbmigrate.Up(ctx, conn, through); err != nil {
		t.Fatal(err)
	}
	assertGrants := func() {
		t.Helper()
		for key, wantRead := range map[string]bool{"admin": true, "ops": true, "management": true, "manager": false, "rep": true, "read_only": false, "custom": false} {
			var read, tag bool
			if err := conn.QueryRow(ctx, `SELECT coalesce((permissions#>>'{objects,import_run,read}')::boolean,false),
    (permissions#>>'{objects,tag,read}')::boolean FROM role WHERE key=$1`, key).Scan(&read, &tag); err != nil {
				t.Fatal(err)
			}
			if read != wantRead || !tag {
				t.Errorf("%s: import read=%v want%v; sibling grant=%v", key, read, wantRead, tag)
			}
		}
		var custom bool
		if err := conn.QueryRow(ctx, `SELECT permissions->'objects' ? 'import_run' FROM role WHERE key='custom'`).Scan(&custom); err != nil {
			t.Fatal(err)
		}
		if custom {
			t.Error("the backfill changed a custom role")
		}
	}
	assertGrants()
	if _, err := dbmigrate.Down(ctx, conn, through, 1); err != nil {
		t.Fatal(err)
	}
	assertGrants()
	if _, err := dbmigrate.Up(ctx, conn, through); err != nil {
		t.Fatal(err)
	}
	assertGrants()
}
