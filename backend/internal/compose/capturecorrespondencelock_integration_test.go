// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The correspondence guard and the write that can flip it.
//
// "Does this workspace correspond with them" is the T1 fact that calls the noise
// effects off, and it is read inside the verdict's own transaction and acted on
// there: read `no`, archive the sender's mail. The only write that turns that
// answer from no to yes is an attested OUTBOUND row — inbound mail cannot — so
// that insert and this read are the pair that has to serialize.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Read and write, serialized.
//
// CorrespondsWith is read inside the verdict's own transaction and acted on
// there: read `no`, archive the sender's mail. An attested outbound insert
// committing in between is a workspace that has just written to them, and under
// READ COMMITTED the archive never sees it — the mail of a real counterparty is
// hidden on an answer that was already stale. Re-reading cannot close that; the
// two paths have to take one key.
//
// Deterministic rather than timed: the reader holds its transaction open and the
// probe asks whether the key is taken, so this fails when the lock is absent
// rather than when a machine is slow.
func TestTheCorrespondenceGuardAndAnAttestedSendSerializeOnOneKey(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	const victim = "cfo@bigcorp.example"

	held := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)

	go func() {
		done <- database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			if _, err := capture.NewPendingStore(InstallationDB(e.Pool)).
				CorrespondsWith(ctx, tx, victim); err != nil {
				return err
			}
			close(held)
			<-release
			return nil
		})
	}()
	<-held

	probe := func() bool {
		t.Helper()
		var free bool
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx,
				`SELECT pg_try_advisory_xact_lock(hashtextextended($1 || ':' || $2, 0))`,
				correspondence.LockEntity+"_write", correspondence.LockIdentity(victim)).Scan(&free)
		}); err != nil {
			t.Fatalf("probing the correspondence key: %v", err)
		}
		return free
	}

	if probe() {
		t.Error("an attested send could take the correspondence key while the verdict's " +
			"guard held it — the read and the archive it drives are not serialized " +
			"against the one write that can change the answer")
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("the holding transaction: %v", err)
	}
	if !probe() {
		t.Error("the key stayed held after the transaction committed — an xact lock that " +
			"outlived its transaction would stall every later send to this address")
	}
}
