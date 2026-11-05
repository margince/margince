// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

// Package capturetest runs provider fixtures through the real capture sink.
package capturetest

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Mailbox supplies the real sink's tenant and import owner without compose wiring.
type Mailbox struct {
	Context context.Context
	DB      *database.DB
	Seat    ids.UUID
}

// NewMailbox creates an isolated provider identity for the capture sink.
func NewMailbox(t *testing.T, provider, address string) Mailbox {
	t.Helper()
	ctx := context.Background()
	ownerDSN, appDSN := os.Getenv("MARGINCE_TEST_DSN"), os.Getenv("MARGINCE_TEST_APP_DSN")
	if ownerDSN == "" || appDSN == "" {
		t.Fatal("integration database is required; run make test-it")
	}
	owner, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if err := testdb.Reset(ctx, owner); err != nil {
		t.Fatal(err)
	}
	workspace, seat := ids.NewV7(), ids.NewV7()
	if _, err := owner.Exec(ctx, `INSERT INTO workspace (id) VALUES ($1)`, workspace); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `INSERT INTO app_user (id,email,display_name,email_domain_folded) VALUES ($1,$2,'Mailbox owner',$3)`,
		seat, address, owndomain.StoredOfAddress(address)); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `INSERT INTO capture_connection (user_id,provider,status,credential_ref,account_label) VALUES ($1,$2,'connected','vault:test',$3)`, seat, provider, address); err != nil {
		t.Fatal(err)
	}
	pool, err := testdb.Pool(ctx, appDSN)
	if err != nil {
		t.Fatal(err)
	}
	testdb.AssertPoolsQuiesced(t)
	ctx = principal.WithCorrelationID(principal.WithWorkspaceID(ctx, workspace), ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:" + provider, UserID: seat, OnBehalfOf: seat,
		Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{"activity": {Create: true, Read: true}}, RowScope: principal.RowScopeAll},
	})
	return Mailbox{Context: ctx, DB: database.BindTo(pool, ids.From[ids.WorkspaceKind](workspace)), Seat: seat}
}

// Arrival reads the provider arrival time persisted by capture.
func (m Mailbox) Arrival(t *testing.T, messageID string) time.Time {
	t.Helper()
	var received time.Time
	if err := m.DB.Tx(m.Context, func(tx pgx.Tx) error {
		return tx.QueryRow(m.Context, `SELECT ci.provider_received_at FROM capture_import ci JOIN activity a ON a.id=ci.activity_id WHERE a.source_id=$1 AND ci.user_id=$2 AND a.archived_at IS NULL`, messageID, m.Seat).Scan(&received)
	}); err != nil {
		t.Fatal(err)
	}
	return received
}
