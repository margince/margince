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

	"github.com/margince/margince/backend/internal/platform/auth"
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

// findRosterUser reads the roster AS actor and returns the one row for subject.
// What the row carries is the service's decision from that actor's grants, not
// this helper's — which is the point: a caller cannot pass a flag that widens
// it. It fails the test rather than returning a zero row, because every caller
// here asserts about a user it just created.
func findRosterUser(t *testing.T, e *revocationEnv, actor Identity, includeInactive bool, subject ids.UUID) userRow {
	t.Helper()
	roster, err := e.svc.ListUsers(e.wsCtx(actor), ListUsersInput{IncludeInactive: includeInactive})
	if err != nil {
		t.Fatalf("reading the roster: %v", err)
	}
	for _, row := range roster.Users {
		if row.ID == subject {
			return row
		}
	}
	t.Fatalf("the roster does not carry %s", subject)
	return userRow{}
}

// A caller with no principal on the context reads nothing, and neither does a
// Deal Room buyer. Both are refused by the service itself.
//
// Before this, all five roster reads carried no gate of their own and were safe
// only because the HTTP handler in front of them checked first. That is a rule
// every future caller has to remember, and this is the test that makes the
// service answer for itself. The buyer arm matters most: a buyer is minted
// carrying no permissions, so an object gate alone would refuse it by accident
// rather than on purpose, and one careless constructor would end that.
func TestTheRosterRefusesACallerWhoIsNotAMember(t *testing.T) {
	e := setupRevocationEnv(t, "roster-non-member")

	buyerCtx := principal.WithActor(
		principal.WithWorkspaceID(context.Background(), e.ws.UUID),
		principal.Principal{Type: principal.PrincipalBuyer, ID: "buyer:room-guest"})

	for _, tc := range []struct {
		name string
		ctx  context.Context
	}{
		{"no principal at all", principal.WithWorkspaceID(context.Background(), e.ws.UUID)},
		{"a Deal Room buyer", buyerCtx},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := e.svc.ListUsers(tc.ctx, ListUsersInput{}); err == nil {
				t.Error("ListUsers served the member roster")
			}
			if _, _, err := e.svc.ListTeams(tc.ctx, ListTeamsInput{}); err == nil {
				t.Error("ListTeams served the team list")
			}
			if _, _, err := e.svc.Colleagues(tc.ctx, ""); err == nil {
				t.Error("Colleagues served the seat list")
			}
			if _, err := e.svc.SeatNames(tc.ctx, []ids.UserID{e.admin.UserID}); err == nil {
				t.Error("SeatNames named a colleague")
			}
			if _, err := e.svc.GetUser(tc.ctx, e.member.UserID); err == nil {
				t.Error("GetUser served one member")
			}
		})
	}
}

// A member who may not administer members reads the roster and is refused the
// single-member read, which always carries role keys and team memberships.
//
// The pair is the whole point: the same seat is admitted to one read and
// refused the other, so a failure here cannot be a seat that was refused
// everything.
func TestTheSingleMemberReadIsForMemberAdministratorsOnly(t *testing.T) {
	e := setupRevocationEnv(t, "roster-single-read")

	if _, err := e.svc.ListUsers(e.wsCtx(e.member), ListUsersInput{}); err != nil {
		t.Fatalf("a member was refused the roster every seat may read: %v", err)
	}
	if _, err := e.svc.GetUser(e.wsCtx(e.member), e.admin.UserID); err == nil {
		t.Error("a member without user_admin read a single member's roles and teams")
	}
	if _, err := e.svc.GetUser(e.wsCtx(e.admin), e.member.UserID); err != nil {
		t.Errorf("a member administrator was refused the read they administer with: %v", err)
	}
}

// A caller who may not administer members gets the active-only roster however
// loudly they ask for the rest.
//
// The deactivated seat is what makes the assertion non-vacuous: without one in
// the fixture, both arms return the same rows and the test would pass for a
// service that honours the flag for everybody. The seat is deactivated through
// the real writer rather than an UPDATE, so the row is the one production
// produces.
func TestAskingToIncludeInactiveDoesNotWidenAMembersRoster(t *testing.T) {
	e := setupRevocationEnv(t, "roster-include-inactive")

	// A THIRD seat is deactivated, so e.member survives as a caller who holds no
	// user_admin grant. Deactivating e.member itself would leave nobody to read
	// as, and the test would prove only that an administrator sees the row.
	gone, _, err := e.svc.InviteUser(e.wsCtx(e.admin), e.admin, InviteUserInput{
		Email: "left-the-company@" + e.slug + ".test", DisplayName: "Left The Company", Role: "rep",
	})
	if err != nil {
		t.Fatalf("seeding the seat this test deactivates: %v", err)
	}
	if err := e.svc.DeactivateUser(e.wsCtx(e.admin), e.admin,
		DeactivateUserInput{UserID: gone}); err != nil {
		t.Fatalf("deactivating it through the real writer: %v", err)
	}

	asAdmin, err := e.svc.ListUsers(e.wsCtx(e.admin), ListUsersInput{IncludeInactive: true})
	if err != nil {
		t.Fatalf("administrator's widened roster: %v", err)
	}
	if !carriesUser(asAdmin.Users, gone.UUID) {
		t.Fatal("the deactivated seat is absent from the administrator's widened roster, so the arm below proves nothing")
	}
	if !asAdmin.Management {
		t.Error("the administrator's page does not report itself as the management view")
	}

	// The same request by a seat that may not administer members. Same flag,
	// same URL, and the inactive seat must not appear.
	asMember, err := e.svc.ListUsers(e.wsCtx(e.member), ListUsersInput{IncludeInactive: true})
	if err != nil {
		t.Fatalf("member's roster: %v", err)
	}
	if carriesUser(asMember.Users, gone.UUID) {
		t.Error("a member asked for inactive seats and was given them")
	}
	if asMember.Management {
		t.Error("a member's page reports itself as the management view, so the wire mapping would disclose role keys")
	}
}

func carriesUser(rows []userRow, id ids.UUID) bool {
	for _, row := range rows {
		if row.ID == id {
			return true
		}
	}
	return false
}

// A delegated member administrator holding a WRITE verb and not read still gets
// the row back after the write.
//
// The verbs on user_admin are grantable apart, and the writes admit on them
// separately: DeactivateUser asks for delete, ChangeUserRole for update. The
// read that every one of those writes returns through is the same read, so
// demanding user_admin.read there would let the deactivation commit and then
// answer 403 — the seat is locked out and the response says the request failed.
//
// The role is edited through the real editor rather than an UPDATE, so the
// grant is one an operator can actually produce.
func TestTheMemberReadFollowsAnyAdministrationVerbNotReadAlone(t *testing.T) {
	e := setupRevocationEnv(t, "roster-write-verb-read")

	if _, err := e.svc.SetRoleObjectGrant(e.wsCtx(e.admin), e.admin, "ops", objectUserAdmin,
		storedGrant{Delete: true}, nil); err != nil {
		t.Fatalf("granting ops user_admin.delete and nothing else: %v", err)
	}
	deleter := e.admin
	deleter.Roles = []string{"ops"}
	deleter.Permissions = principal.Permissions{
		RoleKeys: []string{"ops"},
		Objects:  map[string]principal.ObjectGrant{objectUserAdmin: {Delete: true}},
	}

	// The premise: this seat may NOT read the object it may delete on.
	if err := auth.Require(e.wsCtx(deleter), objectUserAdmin, principal.ActionRead); err == nil {
		t.Fatal("the fixture holds user_admin.read, so the assertion below proves nothing")
	}
	if _, err := e.svc.GetUser(e.wsCtx(deleter), e.member.UserID); err != nil {
		t.Errorf("a holder of user_admin.delete cannot read back the seat it just wrote: %v", err)
	}
}
