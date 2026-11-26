// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestPreparedMailboxProofUsesOneReadAndStaysInItsTransaction(t *testing.T) {
	e := Setup(t)
	e.WsExec(t, `INSERT INTO capture_connection(user_id,provider,status,credential_ref,account_label)
 VALUES(@proved,'gmail','connected','fixture-reference','proved@example.test'),
 (@typed,'imap','connected','fixture-reference','typed@example.test')`, pgx.NamedArgs{"proved": e.Rep1, "typed": e.Rep2})
	e.WsExec(t, `INSERT INTO capture_owner_identity(user_id,kind,value,source,created_by)
 VALUES(@seat,'address','alias@example.test','user','human:fixture')`, pgx.NamedArgs{"seat": e.Rep2})
	var prepared context.Context
	ctx, meter := database.WithStatementMeter(e.Admin())
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		before := meter.Cost().Statements
		var err error
		prepared, err = storekit.WithMailSeatIdentitySnapshot(ctx, tx)
		if err != nil {
			return err
		}
		for range 2 {
			labels, err := storekit.ProvedMailboxLabels(prepared, tx)
			if err != nil {
				return err
			}
			if len(labels) != 1 || len(labels[e.Rep1]) != 1 || labels[e.Rep1][0] != "proved@example.test" {
				t.Fatalf("provider proof admitted typed addresses: %v", labels)
			}
		}
		if meter.Cost().Statements-before != 1 {
			t.Fatalf("prepared proof used %d queries, want the original identity read", meter.Cost().Statements-before)
		}
		other, ok := principal.Actor(e.As(e.Rep1, nil, AdminPerms))
		if !ok {
			t.Fatal("reader principal missing")
		}
		// The identities are workspace facts with no row scope, so another
		// reader in this transaction gets the same answer from its memo.
		labels, err := storekit.ProvedMailboxLabels(principal.WithActor(prepared, other), tx)
		if err != nil {
			return err
		}
		if len(labels) != 1 || len(labels[e.Rep1]) != 1 || labels[e.Rep1][0] != "proved@example.test" {
			t.Fatalf("another reader in the transaction got other proof: %v", labels)
		}
		if meter.Cost().Statements-before != 1 {
			t.Fatalf("another reader in the transaction read the identities again: %d queries", meter.Cost().Statements-before)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	e.WsExec(t, `UPDATE capture_connection SET status='disconnected' WHERE user_id=@seat AND provider='gmail'`, pgx.NamedArgs{"seat": e.Rep1})
	if err := database.WithWorkspaceTx(prepared, e.Pool, func(tx pgx.Tx) error {
		labels, err := storekit.ProvedMailboxLabels(prepared, tx)
		if err == nil && len(labels) != 0 {
			t.Fatalf("another transaction reused withdrawn proof: %v", labels)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
