// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Postgres names the TABLE a constraint refused, for a real refusal.
//
// httperr's classifier reads that name to tell a constraint on the record the
// request wrote from one on the audit or outbox row written beside it — the
// first is the caller's input to fix, the second is ours. Every unit case for
// that classifier builds the error itself, so all of them would keep passing if
// Postgres sent no table at all and the classifier silently stopped separating
// the two. Only a real violation can say the field is there.
//
// One case, on one constraint, because the claim is about the WIRE PROTOCOL
// rather than about any table: if `TableName` arrives for this violation, it
// arrives for the rest.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestARefusedAuditRowNamesItsOwnTable(t *testing.T) {
	e := Setup(t)
	// An actor type audit_log_actor_type_check does not admit. The verb would
	// have been the closer analogue of the original defect, but writing an
	// illegal one is a thing the tree already forbids: TestEveryAuditVerbTheCodeWritesIsLegal
	// scans every storekit.Audit call and fails on a literal the DDL rejects,
	// which is the STATIC half of this same fix. So the fixture breaks the
	// audit row's other CHECK — the same table, the same class of defect, the
	// same 23514 — and leaves that gate looking at a tree it can still vouch for.
	ctx := principal.WithCorrelationID(
		principal.WithActor(principal.WithWorkspaceID(context.Background(), e.WS),
			principal.Principal{Type: principal.PrincipalType("neither_human_nor_machine"), ID: "system"}), ids.NewV7())

	// Written through the REAL writer, which is the point: an audit row is
	// filled by code the caller never sees, so nothing in a request could have
	// caused this and nothing in a request can fix it.
	// The refusal is RETURNED from the transaction rather than captured inside
	// it: the failed statement aborts the transaction, so a callback that
	// swallowed the error would meet a commit that cannot succeed and this test
	// would read the rollback instead of the constraint.
	refusal := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := storekit.AuditEvent(ctx, tx, "update", "organization", ids.NewV7(), nil)
		return err
	})
	if refusal == nil {
		t.Fatal("an unknown actor type was accepted — audit_log_actor_type_check is what this test reads, and it did not fire")
	}

	table, ok := storekit.ViolatedTable(refusal)
	if !ok {
		t.Fatalf("Postgres named no table for %v — the classifier cannot tell our own broken audit row from a value the caller sent, and answers every one of them as the caller's", refusal)
	}
	if table != storekit.TableAudit {
		t.Fatalf("the refusal names table %q, want %q", table, storekit.TableAudit)
	}
	if !storekit.IsSideEffectTable(table) {
		t.Fatalf("%q is not recognised as a row written beside the caller's own", table)
	}

	// And the whole way out: this is ours, so the caller gets the opaque 500
	// rather than advice about a value they never sent.
	fault, classified := httperr.Classify(refusal)
	if !classified || fault.Status != 500 || fault.Detail != "" {
		t.Fatalf("classified=%v status=%d detail=%q, want a 500 with nothing to say to the caller",
			classified, fault.Status, fault.Detail)
	}
}
