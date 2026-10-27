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

// The assignment picker asks for invited seats, and it searches with or
// without the marks a name was typed in.
func TestMemberSearchFoldsDiacriticsAndOffersInvitedSeatsWhenAsked(t *testing.T) {
	e := setupRevocationEnv(t, "roster-member-fold")
	invited := InviteUserInput{Email: "thang.tran@acme.test", DisplayName: "Trần Văn Thắng", Role: "rep"}
	if _, _, err := e.svc.InviteUser(e.wsCtx(e.admin), e.admin, invited); err != nil {
		t.Fatalf("inviting %s: %v", invited.Email, err)
	}
	active := InviteUserInput{Email: "thang.le@acme.test", DisplayName: "Thang Le", Role: "rep"}
	_, rawToken, err := e.svc.InviteUser(e.wsCtx(e.admin), e.admin, active)
	if err != nil {
		t.Fatalf("inviting %s: %v", active.Email, err)
	}
	reset := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.ws.UUID), ids.NewV7())
	if err := e.svc.RedeemPasswordReset(reset, rawToken, "a rep password!"); err != nil {
		t.Fatalf("redeeming the invite for %s: %v", active.Email, err)
	}

	for _, q := range []string{"thang", "Thắng", "THẮNG"} {
		page, err := e.svc.ListUsers(e.wsCtx(e.admin), ListUsersInput{Q: &q, IncludeInvited: true})
		if err != nil {
			t.Fatalf("searching %q: %v", q, err)
		}
		statuses := map[string]string{}
		for _, u := range page.Users {
			statuses[u.Email] = u.Status
		}
		if statuses[invited.Email] != "invited" || statuses[active.Email] != "active" {
			t.Errorf("search %q with invited seats = %v; want both, the invited one marked invited", q, statuses)
		}
	}

	q := "Thắng"
	page, err := e.svc.ListUsers(e.wsCtx(e.admin), ListUsersInput{Q: &q})
	if err != nil {
		t.Fatalf("searching %q: %v", q, err)
	}
	if len(page.Users) != 1 || page.Users[0].Email != active.Email {
		t.Errorf("search %q without invited seats returned %d rows; want only %s", q, len(page.Users), active.Email)
	}
}

func TestTeamSearchFoldsDiacritics(t *testing.T) {
	e := setupRevocationEnv(t, "roster-team-fold")
	if _, err := e.svc.CreateTeam(e.wsCtx(e.admin), e.admin, "Đội Hà Nội"); err != nil {
		t.Fatalf("creating the team: %v", err)
	}
	for _, q := range []string{"ha noi", "Hà Nội"} {
		teams, _, err := e.svc.ListTeams(e.wsCtx(e.admin), ListTeamsInput{Q: &q})
		if err != nil {
			t.Fatalf("searching teams for %q: %v", q, err)
		}
		if len(teams) != 1 || teams[0].Name != "Đội Hà Nội" {
			t.Errorf("team search %q returned %d rows; want the Hà Nội team", q, len(teams))
		}
	}
}
