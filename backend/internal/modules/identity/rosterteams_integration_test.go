// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// The roster carries each user's team memberships so the admin screen can edit
// them, and that is a disclosure: the same endpoint answers every authenticated
// user because the share and assignee pickers read it. These tests hold the two
// halves of the obligation against real rows — an admin sees the memberships,
// nobody else does, and an archived team is absent because its memberships
// resolve nothing.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestRosterCarriesTeamsForAnAdminAndForNobodyElse(t *testing.T) {
	e := setupRevocationEnv(t, "roster-teams")
	ctx := context.Background()

	sales, err := e.svc.CreateTeam(e.wsCtx(e.admin), e.admin, "DACH Sales")
	if err != nil {
		t.Fatalf("creating the team: %v", err)
	}
	if err := e.svc.SetTeamMember(e.wsCtx(e.admin), e.admin, sales.ID, e.admin.UserID.UUID, true); err != nil {
		t.Fatalf("adding the admin to the team: %v", err)
	}

	// A second seat, resolved through the real invite → set-password → login
	// path: a hand-built Identity would prove only that the test spelled the
	// role key right, and the disclosure gate reads the role the login carries.
	_, rawToken, err := e.svc.InviteUser(e.wsCtx(e.admin), e.admin, InviteUserInput{
		Email: "rep@acme.test", DisplayName: "Rep One", Role: "rep",
	})
	if err != nil {
		t.Fatalf("inviting the rep: %v", err)
	}
	if err := e.svc.RedeemPasswordReset(principal.WithCorrelationID(principal.WithWorkspaceID(ctx, e.ws.UUID), ids.NewV7()), rawToken, "a rep password!"); err != nil {
		t.Fatalf("redeeming the invite token: %v", err)
	}
	rep, _, err := e.svc.Login(principal.WithWorkspaceID(ctx, e.ws.UUID), "rep@acme.test", "a rep password!")
	if err != nil {
		t.Fatalf("rep login: %v", err)
	}

	adminRow := findRosterUser(t, e, e.admin, true, e.admin.UserID.UUID)
	if adminRow.TeamIDs == nil {
		t.Fatal("an admin's roster read asked for team ids and got nil, which means it was never read")
	}
	if len(adminRow.TeamIDs) != 1 || adminRow.TeamIDs[0] != sales.ID {
		t.Fatalf("the admin is in DACH Sales; the roster says %v", adminRow.TeamIDs)
	}

	// The rep's own read of the same roster. WithRoles is what the handler
	// derives from the caller's admin role, so a non-admin page must not carry
	// the memberships out of the database at all.
	repRow := findRosterUser(t, e, rep, false, e.admin.UserID.UUID)
	if repRow.TeamIDs != nil {
		t.Fatalf("a rep read the admin's team memberships: %v", repRow.TeamIDs)
	}
}

func TestAnArchivedTeamLeavesTheRoster(t *testing.T) {
	e := setupRevocationEnv(t, "roster-archived-team")

	sales, err := e.svc.CreateTeam(e.wsCtx(e.admin), e.admin, "DACH Sales")
	if err != nil {
		t.Fatalf("creating the team: %v", err)
	}
	if err := e.svc.SetTeamMember(e.wsCtx(e.admin), e.admin, sales.ID, e.admin.UserID.UUID, true); err != nil {
		t.Fatalf("adding the admin to the team: %v", err)
	}
	// The membership is still there after this; what changes is that it now
	// resolves no scope and no share, so the roster must stop reporting it.
	archived := true
	if _, err := e.svc.UpdateTeam(e.wsCtx(e.admin), e.admin, sales.ID, UpdateTeamInput{Archived: &archived}); err != nil {
		t.Fatalf("archiving the team: %v", err)
	}

	row := findRosterUser(t, e, e.admin, true, e.admin.UserID.UUID)
	if row.TeamIDs == nil {
		t.Fatal("an admin's roster read asked for team ids and got nil, which means it was never read")
	}
	if len(row.TeamIDs) != 0 {
		t.Fatalf("an archived team still reads as a live membership: %v", row.TeamIDs)
	}
}

// findRosterUser reads the roster the way the handler does — withRoles is the
// flag it derives from the caller's admin role — and returns the one row for
// subject. It fails the test rather than returning a zero row, because every
// caller here asserts about a user it just created.
func findRosterUser(t *testing.T, e *revocationEnv, actor Identity, withRoles bool, subject ids.UUID) userRow {
	t.Helper()
	rows, _, err := e.svc.ListUsers(e.wsCtx(actor), ListUsersInput{IncludeInactive: withRoles, WithRoles: withRoles})
	if err != nil {
		t.Fatalf("reading the roster: %v", err)
	}
	for _, row := range rows {
		if row.ID == subject {
			return row
		}
	}
	t.Fatalf("the roster does not carry %s", subject)
	return userRow{}
}
