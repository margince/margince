// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// An agent whose colleague has been deactivated lends nobody's authority, even
// while it still holds the passport the colleague issued.
func TestAnAgentOfADeactivatedMemberActsForNobody(t *testing.T) {
	t.Parallel()
	e := setupRevocationEnv(t, "agent-actor-departed")
	// The suite's database holds one workspace per env, so the singleton read
	// would find several; this env's own workspace is the installation here.
	e.svc.installation.Store(&e.ws)
	issued, err := e.svc.IssuePassport(e.wsCtx(e.member), e.member, IssuePassportInput{Label: new(testPassportLabel), Scopes: []string{"read"}})
	if err != nil {
		t.Fatalf("issue passport: %v", err)
	}
	agentCtx := principal.WithActor(e.wsCtx(e.member), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:" + issued.ID.String(),
		PassportID: issued.ID.UUID, OnBehalfOf: e.member.UserID.UUID,
	})
	if acting, err := e.svc.agentsUser(agentCtx); err != nil || acting.UserID != e.member.UserID {
		t.Fatalf("a live member's agent acts for %v (%v), want the member", acting.UserID, err)
	}

	reason := "left the company"
	if err := e.svc.DeactivateUser(e.wsCtx(e.admin), e.admin, DeactivateUserInput{
		UserID: e.member.UserID, Reason: &reason,
	}); err != nil {
		t.Fatalf("deactivate: %v", err)
	}

	if _, err := e.svc.agentsUser(agentCtx); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a departed member's agent answered %v, want ErrPermissionDenied", err)
	}
}
