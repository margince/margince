// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAStoredAgentActorNamingNoAgentIsNotActedAs(t *testing.T) {
	e := Setup(t)
	e.GrantRole(t, e.Rep1, "rep")
	run := queuedRun(agentOf(t, e), 1)
	run.Actor.ID = ""

	ctx, err := compose.ActAsImportRunForTest(e.wsCtx(), e.Pool, run)
	if err == nil || ctx != nil {
		t.Fatalf("acting as an agent with no id answered a context, %v; want an error and no principal", err)
	}
}

func TestAStoredAgentActorUnderAnotherHumansPassportIsRefused(t *testing.T) {
	e := Setup(t)
	e.GrantRole(t, e.Rep1, "rep")
	e.GrantRole(t, e.AdminUser, "admin")
	agent := agentOf(t, e)
	stored := migration.StoredActorOf(agent)
	// The passport is Rep1's; the stored human is somebody else.
	stored.UserID, stored.OnBehalfOf = e.AdminUser, e.AdminUser
	correlation := ids.NewV7()
	run := migration.Run{ID: migration.RunID(ids.NewV7()), JobAttempt: 1, Actor: &stored, AttemptCorrelationID: &correlation}

	_, err := compose.ActAsImportRunForTest(e.wsCtx(), e.Pool, run)
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("acting under a passport another human holds answered %v, want a permission refusal", err)
	}
}
