// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package storekit_test

// Audit and Emit are the one writer of the audit and outbox rows. So the write
// shape every store relies on is theirs to keep. Both rows land in the caller's
// transaction, and the event names the audit row and the request.

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// storePool opens a pool as the runtime role on a schema at head.
func storePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	ownerDSN, appDSN := os.Getenv("MARGINCE_TEST_DSN"), os.Getenv("MARGINCE_TEST_APP_DSN")
	if ownerDSN == "" || appDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN / MARGINCE_TEST_APP_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	owner, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := owner.Close(ctx); err != nil {
			t.Errorf("closing the owner connection: %v", err)
		}
	}()
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}
	pool, err := testdb.OwnPool(ctx, appDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// writingAs binds what a request binds before it reaches a store. That is the
// actor the audit row is filed under and the correlation id the event carries.
func writingAs(correlation ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	ctx = principal.WithCorrelationID(ctx, correlation)
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + ids.NewV7().String(),
	})
}

// createCompany is a store's create: the domain row, then its audit row, then
// the event that names that audit row.
func createCompany(ctx context.Context, tx pgx.Tx, company ids.UUID) (ids.UUID, error) {
	if _, err := tx.Exec(ctx, `INSERT INTO company (id, display_name, source, captured_by)
		VALUES ($1, 'Write Shape GmbH', 'manual', 'human:write-shape')`, company); err != nil {
		return ids.Nil, err
	}
	auditID, err := storekit.Audit(ctx, tx, "create", "company", company, nil,
		map[string]any{"display_name": "Write Shape GmbH"})
	if err != nil {
		return ids.Nil, err
	}
	return auditID, storekit.Emit(ctx, tx, auditID, "company.created", "company", company,
		map[string]any{"display_name": "Write Shape GmbH"})
}

func TestARowItsAuditAndItsEventCommitInOneTransaction(t *testing.T) {
	pool := storePool(t)
	correlation, company := ids.NewV7(), ids.NewV7()
	ctx := writingAs(correlation)
	var auditID ids.UUID
	if err := database.WithTx(ctx, pool, func(tx pgx.Tx) error {
		var err error
		auditID, err = createCompany(ctx, tx, company)
		return err
	}); err != nil {
		t.Fatalf("creating the company: %v", err)
	}

	// xmin names the transaction that wrote each row version.
	var rowXact, auditXact, eventXact *string
	var envelope []byte
	if err := pool.QueryRow(ctx, `
		SELECT (SELECT xmin::text FROM company WHERE id = $1),
		       (SELECT xmin::text FROM audit_log WHERE id = $2 AND entity_id = $1),
		       (SELECT xmin::text FROM event_outbox WHERE envelope #>> '{trace,audit_log_id}' = $2::text),
		       (SELECT envelope FROM event_outbox WHERE envelope #>> '{trace,audit_log_id}' = $2::text)`,
		company, auditID).Scan(&rowXact, &auditXact, &eventXact, &envelope); err != nil {
		t.Fatalf("reading the three rows back: %v", err)
	}
	if rowXact == nil || auditXact == nil || eventXact == nil {
		t.Fatalf("a committed write left row=%v audit=%v event=%v; all three must exist", rowXact, auditXact, eventXact)
	}
	if *auditXact != *rowXact || *eventXact != *rowXact {
		t.Errorf("the row, audit and event were written by transactions %s, %s and %s; they must be one",
			*rowXact, *auditXact, *eventXact)
	}

	var env events.Envelope
	if err := json.Unmarshal(envelope, &env); err != nil {
		t.Fatalf("decoding the staged envelope: %v", err)
	}
	if env.Trace.CorrelationID != correlation {
		t.Errorf("the event carries correlation %s, want the request's %s", env.Trace.CorrelationID, correlation)
	}
	if env.Entity.Type != "company" || env.Entity.ID != company {
		t.Errorf("the event names %s %s, want company %s", env.Entity.Type, env.Entity.ID, company)
	}
}

func TestARolledBackWriteLeavesNeitherItsAuditNorItsEvent(t *testing.T) {
	pool := storePool(t)
	company := ids.NewV7()
	ctx := writingAs(ids.NewV7())
	failed := errors.New("the step after the event failed")
	err := database.WithTx(ctx, pool, func(tx pgx.Tx) error {
		if _, err := createCompany(ctx, tx, company); err != nil {
			return err
		}
		return failed
	})
	if !errors.Is(err, failed) {
		t.Fatalf("the transaction answered %v, want the step's own failure", err)
	}

	var rows, audits, outbox int
	if err := pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM company WHERE id = $1),
		       (SELECT count(*) FROM audit_log WHERE entity_id = $1),
		       (SELECT count(*) FROM event_outbox WHERE envelope #>> '{entity,id}' = $1::text)`,
		company).Scan(&rows, &audits, &outbox); err != nil {
		t.Fatalf("counting what the rollback left: %v", err)
	}
	if rows+audits+outbox != 0 {
		t.Errorf("a rolled-back write left %d row(s), %d audit row(s) and %d event(s): an audit or event "+
			"written outside the caller's transaction outlives the change it describes", rows, audits, outbox)
	}
}
