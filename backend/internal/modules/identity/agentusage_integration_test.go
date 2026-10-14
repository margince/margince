// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAgentUsageListsEachUserWithALivePassport(t *testing.T) {
	e := setupRevocationEnv(t, "agent-usage")
	rep := e.seat(t, "rep", "rep")
	first, second := e.mintOwnPassport(t, e.admin, []string{"read"}), e.mintOwnPassport(t, e.admin, []string{"read"})
	revoked := e.mintOwnPassport(t, rep, []string{"read"})
	if err := e.svc.RevokePassport(e.wsCtx(rep), rep, revoked); err != nil {
		t.Fatalf("revoking: %v", err)
	}

	// A bootstrap admin owes a password rotation, and authentication refuses
	// the passports of one who does; this admin has rotated.
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE app_user SET must_change_password = false WHERE id = $1`, e.admin.UserID); err != nil {
		t.Fatal(err)
	}

	holders, err := e.svc.AgentUsageHolders(e.wsCtx(e.admin), e.admin)
	if err != nil {
		t.Fatalf("listing agent usage: %v", err)
	}

	if len(holders) != 1 || holders[0].UserID != e.admin.UserID.UUID {
		t.Fatalf("listed %+v, want only the admin: the rep's one passport is revoked", holders)
	}
	want := map[string]bool{
		agentvolume.SubjectOf(first.UUID, ids.UUID{}): true, agentvolume.SubjectOf(second.UUID, ids.UUID{}): true,
	}
	if got := holders[0].Subjects; len(got) != 2 || !want[got[0]] || !want[got[1]] {
		t.Errorf("the admin's subjects are %v, want both live passports", got)
	}
}

func TestAgentUsageIsRefusedWithoutTheAIUsageGrant(t *testing.T) {
	e := setupRevocationEnv(t, "agent-usage-rep")
	rep := e.seat(t, "rep", "rep")

	if _, err := e.svc.AgentUsageHolders(e.wsCtx(rep), rep); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a rep listing agent usage → %v, want permission denied", err)
	}
}

func TestAnAgentCannotReadAgentUsage(t *testing.T) {
	e := setupRevocationEnv(t, "agent-usage-agent")
	ctx := principal.WithActor(e.wsCtx(e.admin), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:x", PassportID: ids.NewV7(),
		OnBehalfOf: e.admin.UserID.UUID, Permissions: e.admin.Permissions,
	})

	if _, err := e.svc.AgentUsageHolders(ctx, e.admin); err == nil {
		t.Fatal("an agent read every user's agent usage")
	}
}
