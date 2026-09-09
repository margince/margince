// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// One reset at a time, and the fleet pause belongs to the reset that took it.

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/deployconfig"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A second reset is refused while the first holds the lock, and refused BEFORE
// it pauses anything.
//
// The ordering is the assertion that matters. A refusal that had already
// quiesced the fleet would be an outage caused by the check meant to prevent
// one — and it would arm the trap this closes, because that second reset would
// then run its own deferred resume and lift the first one's pause mid-sweep.
func TestASecondResetIsRefusedWithoutTouchingTheFleet(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()

	held, err := takeResetLock(ctx, e.Pool)
	if err != nil {
		t.Fatalf("taking the reset lock: %v", err)
	}
	defer held()

	quiesced := false
	h := dataResetHandlers{
		pool:             e.Pool,
		seeds:            deployconfig.Seeds{},
		dataResetAllowed: true,
		log:              slog.New(slog.NewTextHandler(io.Discard, nil)),
		runtime: &ResetRuntime{
			QuiesceQueues: func(context.Context) (bool, error) {
				quiesced = true
				return true, nil
			},
			ResumeQueues: func(context.Context) error { return nil },
		},
	}

	_, err = h.run(ctx, "Authz")

	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("a second reset answered %v, want a conflict — two resets interleaving their sweeps and "+
			"their fleet pauses is what the lock exists to refuse", err)
	}
	if quiesced {
		t.Error("the refused reset paused the job fleet before finding out it could not run — it then " +
			"resumes on its way out and lifts the pause the running reset is still sweeping under")
	}
	if got := e.WsCount(t, "SELECT count(*) FROM audit_log WHERE action='reset_data'"); got != 0 {
		t.Errorf("%d reset_data audit row(s) after a refused reset, want 0 — it swept", got)
	}
}

// The lock is released when the reset finishes, so the next one runs.
//
// Asserted because the refusal above is only correct if it is temporary: a lock
// that leaked would turn one reset into an installation that can never be reset
// again, which is a worse failure than the interleaving it prevents.
func TestTheResetLockIsReleasedForTheNextReset(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	h := dataResetHandlers{
		pool:             e.Pool,
		seeds:            deployconfig.Seeds{},
		dataResetAllowed: true,
		log:              slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	if _, err := h.run(ctx, "Authz"); err != nil {
		t.Fatalf("the first reset: %v", err)
	}
	if _, err := h.run(ctx, "Authz"); err != nil {
		t.Fatalf("the second reset answered %v — the first one kept the lock, and this installation can "+
			"no longer be reset without a restart", err)
	}
}

// A caller that never took the lock does not resume the fleet.
//
// This is what the owner token is for. The resume is deferred on every exit —
// deliberately, since a pause nobody lifts wedges every queue — so without the
// token a refused reset would run that defer too and lift a pause it never took.
func TestARefusedResetDoesNotResumeTheFleet(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()

	held, err := takeResetLock(ctx, e.Pool)
	if err != nil {
		t.Fatalf("taking the reset lock: %v", err)
	}
	defer held()

	resumed := false
	h := dataResetHandlers{
		pool:             e.Pool,
		seeds:            deployconfig.Seeds{},
		dataResetAllowed: true,
		log:              slog.New(slog.NewTextHandler(io.Discard, nil)),
		runtime: &ResetRuntime{
			QuiesceQueues: func(context.Context) (bool, error) { return true, nil },
			ResumeQueues:  func(context.Context) error { resumed = true; return nil },
		},
	}

	if _, err := h.run(ctx, "Authz"); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("the second reset answered %v, want a conflict", err)
	}
	if resumed {
		t.Error("a reset that never acquired the pause resumed the queues anyway — it lifted the pause " +
			"the running reset is sweeping under, and the fleet writes into a half-swept installation")
	}
}

// A sealed ref written after the sweep read the handles is still purged.
//
// THE ESCAPE. collectWorkspaceSecretRefs takes its list inside the sweep's
// transaction, and a list is what was referenced THEN. Nothing stops concurrent
// HTTP writes during a reset, so a connection write that repoints a handle to a
// fresh ref — committing between that read and the sweep's delete — leaves the
// OLD ref purged and the new one resident, with the row that named it gone. The
// reset then reports a clean installation over credential material nobody can
// reach and nothing will ever collect.
//
// Simulated by sealing a ref and giving it to no row at all, which is the state
// that write leaves behind and the state the snapshot cannot see. Doing it that
// way rather than racing a real write is deliberate: a test that raced would
// pass or fail on scheduling, and what is under test is whether the purge asks
// the post-commit INVARIANT — a vault_secret row no live handle names is
// unreachable however it got there — rather than replaying a snapshot.
func TestASealedRefTheSweepNeverSawIsStillPurged(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()

	vault := resetTestVault(t, e)
	wsID := ids.From[ids.WorkspaceKind](e.WS)
	referenced, err := vault.Put(ctx, wsID, []byte("an incumbent's oauth refresh token"))
	if err != nil {
		t.Fatalf("sealing the referenced credential: %v", err)
	}
	escaped, err := vault.Put(ctx, wsID, []byte("the token a concurrent write repointed to"))
	if err != nil {
		t.Fatalf("sealing the escaped credential: %v", err)
	}
	e.WsExec(t, `INSERT INTO incumbent_connection (id, incumbent, region, status, credential_ref)
		VALUES ($1, 'hubspot', 'eu', 'active', $2)`, ids.NewV7(), string(referenced))

	h := dataResetHandlers{
		pool:             e.Pool,
		seeds:            deployconfig.Seeds{},
		dataResetAllowed: true,
		log:              slog.New(slog.NewTextHandler(io.Discard, nil)),
		vault:            vault,
	}
	if _, err := h.run(ctx, "Authz"); err != nil {
		t.Fatalf("run: %v", err)
	}

	for _, c := range []struct {
		what string
		ref  keyvault.Ref
	}{
		{"the ref the sweep's snapshot saw", referenced},
		{"the ref written after the snapshot, referenced by nothing", escaped},
	} {
		if _, err := vault.Get(ctx, wsID, c.ref); !errors.Is(err, keyvault.ErrNotFound) {
			t.Errorf("%s outlived the reset (Get returned %v) — a wipe that leaves credential material "+
				"resident is not a clean slate, and this one is unreachable so nothing else will ever "+
				"collect it", c.what, err)
		}
	}
}
