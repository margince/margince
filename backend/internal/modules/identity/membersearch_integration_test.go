// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestMemberSearchTreatsUnderscoreAsTypedText(t *testing.T) {
	e := setupRevocationEnv(t, "roster-member-search")
	for _, in := range []InviteUserInput{
		{Email: "snake_case@acme.test", DisplayName: "Snake Sam", Role: "rep"},
		{Email: "plainpat@acme.test", DisplayName: "Plain Pat", Role: "rep"},
	} {
		_, rawToken, err := e.svc.InviteUser(e.wsCtx(e.admin), e.admin, in)
		if err != nil {
			t.Fatalf("inviting %s: %v", in.Email, err)
		}
		reset := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.ws.UUID), ids.NewV7())
		if err := e.svc.RedeemPasswordReset(reset, rawToken, "a rep password!"); err != nil {
			t.Fatalf("redeeming the invite for %s: %v", in.Email, err)
		}
	}
	q := "_"
	page, err := e.svc.ListUsers(e.wsCtx(e.admin), ListUsersInput{Q: &q})
	if err != nil {
		t.Fatalf("searching members for %q: %v", q, err)
	}
	if len(page.Users) != 1 || page.Users[0].Email != "snake_case@acme.test" {
		t.Errorf("member search %q returned %d rows; want only the address holding the underscore", q, len(page.Users))
	}
}
