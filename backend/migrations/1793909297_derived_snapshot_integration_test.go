// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/migrations"
)

func TestDerivedSnapshotsKeepAuditStorageAndTopLevelTransactionIdentity(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	core, err := migrations.Core()
	if err != nil {
		t.Fatal(err)
	}
	through := namespaceThroughVersion(t, core, "1793958488")
	migration := through.Migrations[len(through.Migrations)-1]
	ctx := context.Background()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Error(err)
		}
	}()
	if _, err := tx.Exec(ctx, migration.DownSQL); err != nil {
		t.Fatal(err)
	}
	var before, after uint32
	if err := tx.QueryRow(ctx, `SELECT pg_relation_filenode('audit_log')`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	const insertAudit = `INSERT INTO audit_log(actor_type,actor_id,action,entity_type,entity_id)
 VALUES('system','agent:test','create','contact',uuidv7())`
	if _, err := tx.Exec(ctx, insertAudit); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, migration.UpSQL); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT pg_relation_filenode('audit_log')`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("adding audit provenance rewrote storage: %d -> %d", before, after)
	}
	var legacy int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE actor_id='agent:test' AND source_xid IS NULL`).Scan(&legacy); err != nil {
		t.Fatal(err)
	}
	if legacy == 0 {
		t.Fatal("existing audit rows were rewritten instead of retaining conservative unknown provenance")
	}
	var top, source string
	if err := tx.QueryRow(ctx, `SELECT pg_current_xact_id()::text`).Scan(&top); err != nil {
		t.Fatal(err)
	}
	nested, err := tx.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := nested.QueryRow(ctx, insertAudit+` RETURNING source_xid::text`).Scan(&source); err != nil {
		t.Fatal(err)
	}
	if err := nested.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if top != source {
		t.Fatalf("audit source %s names a subtransaction instead of top-level %s", source, top)
	}
}
