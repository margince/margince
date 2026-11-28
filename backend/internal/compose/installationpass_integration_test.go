// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestBindInstallationBindsTheInstallationsWorkspace(t *testing.T) {
	e := integration.Setup(t)

	ctx, err := bindInstallation(context.Background(), e.Pool)
	if err != nil {
		t.Fatalf("binding the installation: %v", err)
	}
	got, ok := principal.WorkspaceID(ctx)
	if !ok || got != e.WS {
		t.Fatalf("bound workspace = %s (bound=%v), want the installation's %s", got, ok, e.WS)
	}
}

func TestAJobBeforeTheFirstCompanyFailsToBindInsteadOfRunningUnbound(t *testing.T) {
	e := integration.Setup(t)
	archiveEveryWorkspace(t)

	if _, err := bindInstallation(context.Background(), e.Pool); !errors.Is(err, identity.ErrNotBootstrapped) {
		t.Fatalf("binding with no live company = %v, want ErrNotBootstrapped", err)
	}
}

func TestAnInstallationPassRunsOnceWithTheInstallationsWorkspace(t *testing.T) {
	e := integration.Setup(t)

	var seen []ids.UUID
	err := runInstallationPass(context.Background(), e.Pool, func(_ context.Context, ws ids.UUID) error {
		seen = append(seen, ws)
		return nil
	})
	if err != nil {
		t.Fatalf("the pass: %v", err)
	}
	if len(seen) != 1 || seen[0] != e.WS {
		t.Fatalf("the pass ran for %v, want once for %s", seen, e.WS)
	}
}

func TestAnInstallationPassReturnsItsErrorSoTheRowFails(t *testing.T) {
	e := integration.Setup(t)
	failure := errors.New("the pass failed")

	err := runInstallationPass(context.Background(), e.Pool, func(context.Context, ids.UUID) error { return failure })
	if !errors.Is(err, failure) {
		t.Fatalf("the pass returned %v, want its own error so River records the row as failed", err)
	}
}

func TestAnInstallationPassWithNoCompanyDoesNotRun(t *testing.T) {
	e := integration.Setup(t)
	archiveEveryWorkspace(t)

	err := runInstallationPass(context.Background(), e.Pool, func(context.Context, ids.UUID) error {
		t.Error("the pass ran with no live company")
		return nil
	})
	if err != nil {
		t.Fatalf("a pass with nothing to pass over must end cleanly, got %v", err)
	}
}

// archiveEveryWorkspace leaves the installation with no live company. The
// workspace row is the tenant boundary, which the app role may not write, so
// the owner connection does it.
func archiveEveryWorkspace(t *testing.T) {
	t.Helper()
	if _, err := integration.OwnerConn(t).Exec(context.Background(), `UPDATE workspace SET archived_at = now()`); err != nil {
		t.Fatalf("archiving the workspace: %v", err)
	}
}

// A privacy cleanup still reaches data in an archived workspace, and runs when
// no workspace is live: archiving does not un-store what the workspace holds.
func TestTheStoredObjectReapStillCleansAnArchivedWorkspace(t *testing.T) {
	e := integration.Setup(t)
	blob := newReapBlobstore()
	const key = "reap/attachment/archived-orphan"
	orphanedObject(t, e, blob, key)
	archiveEveryWorkspace(t)

	if err := reapWorkerOver(e, blob).Work(context.Background(), &river.Job[StoredObjectReapArgs]{}); err != nil {
		t.Fatalf("the reap: %v", err)
	}
	if !blob.deleted[key] {
		t.Fatal("the orphan in an archived workspace was left in the store; the cleanup must reach archived workspaces")
	}
}

func TestAnEveryWorkspacePassReachesAnArchivedWorkspaceAndSurvivesAFailure(t *testing.T) {
	e := integration.Setup(t)
	archiveEveryWorkspace(t)

	var seen []ids.UUID
	failure := errors.New("one workspace failed")
	err := runEveryWorkspacePass(context.Background(), e.Pool, func(_ context.Context, ws ids.UUID) error {
		seen = append(seen, ws)
		return failure
	})
	if len(seen) != 1 || seen[0] != e.WS {
		t.Fatalf("the pass ran for %v, want the archived %s", seen, e.WS)
	}
	if !errors.Is(err, failure) {
		t.Fatalf("the pass returned %v, want the failure joined so the row fails", err)
	}
}
