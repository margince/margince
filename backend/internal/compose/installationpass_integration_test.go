// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"

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
	e.WsExec(t, `UPDATE workspace SET archived_at = now()`)

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
	e.WsExec(t, `UPDATE workspace SET archived_at = now()`)

	err := runInstallationPass(context.Background(), e.Pool, func(context.Context, ids.UUID) error {
		t.Error("the pass ran with no live company")
		return nil
	})
	if err != nil {
		t.Fatalf("a pass with nothing to pass over must end cleanly, got %v", err)
	}
}
