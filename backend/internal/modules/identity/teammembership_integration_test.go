// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// Whether a named person is on a team with the CALLER, against real rows.
//
// This answer gates a Team Lead opening somebody else's queue, so each arm is
// held separately: sharing a team admits, sharing none refuses, and an archived
// team admits nobody — matching how row scope resolves membership, because an
// answer wider than that predicate would grant a reach the rows then deny.
//
// The caller is taken from the principal rather than passed, so a test drives
// it by acting AS the person asking, which is also how the product reaches it.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestATeammateSharesALiveTeamAndAStrangerDoesNot(t *testing.T) {
	e := setupRevocationEnv(t, "team-membership")
	ctx := context.Background()

	sales, err := e.svc.CreateTeam(e.wsCtx(e.admin), e.admin, "DACH Sales")
	if err != nil {
		t.Fatalf("creating the team: %v", err)
	}
	support, err := e.svc.CreateTeam(e.wsCtx(e.admin), e.admin, "Support")
	if err != nil {
		t.Fatalf("creating the second team: %v", err)
	}

	teammate := inviteMember(ctx, t, e, "teammate@acme.test", "Team Mate")
	stranger := inviteMember(ctx, t, e, "stranger@acme.test", "Strange R")

	for _, member := range []ids.UUID{e.admin.UserID.UUID, teammate.UUID} {
		if err := e.svc.SetTeamMember(e.wsCtx(e.admin), e.admin, sales.ID, member, true); err != nil {
			t.Fatalf("adding %s to DACH Sales: %v", member, err)
		}
	}
	// The stranger is in a team of their OWN, not in no team at all: a refusal
	// that only holds for the teamless would miss the case this gate exists for.
	if err := e.svc.SetTeamMember(e.wsCtx(e.admin), e.admin, support.ID, stranger.UUID, true); err != nil {
		t.Fatalf("adding the stranger to Support: %v", err)
	}

	shares, err := e.svc.SharesLiveTeamWithCaller(e.wsCtx(e.admin), teammate)
	if err != nil {
		t.Fatalf("asking about a teammate: %v", err)
	}
	if !shares {
		t.Fatal("two members of DACH Sales did not read as teammates")
	}

	shares, err = e.svc.SharesLiveTeamWithCaller(e.wsCtx(e.admin), stranger)
	if err != nil {
		t.Fatalf("asking about somebody on another team: %v", err)
	}
	if shares {
		t.Fatal("a member of Support read as a teammate of DACH Sales")
	}
}

// An archived team grants nothing, matching the row-scope predicate. The
// memberships survive the archive so a restore brings them back, which is
// exactly why this needs asserting rather than assuming.
func TestAnArchivedTeamMakesNobodyTeammates(t *testing.T) {
	e := setupRevocationEnv(t, "team-membership-archived")
	ctx := context.Background()

	sales, err := e.svc.CreateTeam(e.wsCtx(e.admin), e.admin, "DACH Sales")
	if err != nil {
		t.Fatalf("creating the team: %v", err)
	}
	teammate := inviteMember(ctx, t, e, "teammate@acme.test", "Team Mate")
	for _, member := range []ids.UUID{e.admin.UserID.UUID, teammate.UUID} {
		if err := e.svc.SetTeamMember(e.wsCtx(e.admin), e.admin, sales.ID, member, true); err != nil {
			t.Fatalf("adding %s to DACH Sales: %v", member, err)
		}
	}

	// Teammates BEFORE the archive, so the assertion after it is a change and
	// not a query that never matched.
	shares, err := e.svc.SharesLiveTeamWithCaller(e.wsCtx(e.admin), teammate)
	if err != nil {
		t.Fatalf("asking about a teammate: %v", err)
	}
	if !shares {
		t.Fatal("two members of a live team did not read as teammates")
	}

	archived := true
	if _, err := e.svc.UpdateTeam(e.wsCtx(e.admin), e.admin, sales.ID, UpdateTeamInput{Archived: &archived}); err != nil {
		t.Fatalf("archiving the team: %v", err)
	}

	shares, err = e.svc.SharesLiveTeamWithCaller(e.wsCtx(e.admin), teammate)
	if err != nil {
		t.Fatalf("asking after the archive: %v", err)
	}
	if shares {
		t.Fatal("an archived team still made two people teammates")
	}
}

// A deactivated colleague is nobody's teammate any more.
//
// team_membership survives a deactivation — SetTeamMember refuses to ADD a
// suspended member, but nothing removes one who leaves — so a membership-only
// answer keeps calling them a teammate. Both callers then act on it: one opens
// a departed person's queue, and the other puts a coaching notice in an inbox
// nobody will ever read.
func TestADeactivatedColleagueIsNoLongerATeammate(t *testing.T) {
	e := setupRevocationEnv(t, "team-membership-deactivated")
	ctx := context.Background()

	sales, err := e.svc.CreateTeam(e.wsCtx(e.admin), e.admin, "DACH Sales")
	if err != nil {
		t.Fatalf("creating the team: %v", err)
	}
	leaver := inviteMember(ctx, t, e, "leaver@acme.test", "Leigh Ver")
	for _, member := range []ids.UUID{e.admin.UserID.UUID, leaver.UUID} {
		if err := e.svc.SetTeamMember(e.wsCtx(e.admin), e.admin, sales.ID, member, true); err != nil {
			t.Fatalf("adding %s to DACH Sales: %v", member, err)
		}
	}

	// Teammates BEFORE, so the assertion after is a change rather than a query
	// that never matched.
	shares, err := e.svc.SharesLiveTeamWithCaller(e.wsCtx(e.admin), leaver)
	if err != nil {
		t.Fatalf("asking about a teammate: %v", err)
	}
	if !shares {
		t.Fatal("two members of a live team did not read as teammates")
	}

	if err := e.svc.DeactivateUser(e.wsCtx(e.admin), e.admin,
		DeactivateUserInput{UserID: leaver}); err != nil {
		t.Fatalf("deactivating the leaver: %v", err)
	}

	// The membership row is still there — this is the case that needs the
	// status check rather than the join.
	var memberships int
	if err := e.owner.QueryRow(ctx,
		`SELECT count(*) FROM team_membership WHERE user_id = $1`, leaver).Scan(&memberships); err != nil {
		t.Fatalf("counting the leaver's memberships: %v", err)
	}
	if memberships == 0 {
		t.Fatal("the deactivation removed the membership row, so this test proves nothing about the status check")
	}

	shares, err = e.svc.SharesLiveTeamWithCaller(e.wsCtx(e.admin), leaver)
	if err != nil {
		t.Fatalf("asking after the deactivation: %v", err)
	}
	if shares {
		t.Fatal("a deactivated colleague still read as a teammate")
	}
}

// A caller is their own teammate. A reader naming their own id follows the same
// path as any other ask, and refusing them their own queue would be a bug the
// two-person cases above cannot see.
func TestACallerIsTheirOwnTeammate(t *testing.T) {
	e := setupRevocationEnv(t, "team-membership-self")

	// No team anywhere: the answer must not depend on membership rows existing.
	shares, err := e.svc.SharesLiveTeamWithCaller(e.wsCtx(e.admin), e.admin.UserID)
	if err != nil {
		t.Fatalf("asking about themselves: %v", err)
	}
	if !shares {
		t.Fatal("a caller did not read as their own teammate")
	}
}

// A caller with no human behind it is refused rather than answered.
//
// "false" would read as a fact about the organization chart — that these two
// are not teammates — where the truth is that the asker has no place on it. The
// refusal keeps an agent seat or a background pass from probing the chart.
func TestACallerWithNoHumanBehindItIsRefused(t *testing.T) {
	e := setupRevocationEnv(t, "team-membership-nonhuman")
	ctx := principal.WithWorkspaceID(context.Background(), e.ws.UUID)

	// An agent seat is refused with the permission sentinel, which is the answer
	// a caller acts on: it is a decision about authority rather than a
	// malformed request.
	agent := principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:probe", UserID: e.admin.UserID.UUID,
	})
	shares, err := e.svc.SharesLiveTeamWithCaller(agent, e.admin.UserID)
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("an agent seat asking about the chart got %v, wanted the permission sentinel", err)
	}
	if shares {
		t.Fatal("a refused agent seat was still told the two are teammates")
	}

	// No actor at all is refused too. The sentinel differs — there is no
	// principal to make a decision about — and what matters is that it errors
	// rather than answering.
	shares, err = e.svc.SharesLiveTeamWithCaller(ctx, e.admin.UserID)
	if err == nil {
		t.Fatal("a call with no actor at all was answered rather than refused")
	}
	if shares {
		t.Fatal("a refused call was still told the two are teammates")
	}
}

// inviteMember resolves a second seat through the real invite → redeem path, so
// the row under test is one the product writes rather than one the test does.
func inviteMember(ctx context.Context, t *testing.T, e *revocationEnv, email, name string) ids.UserID {
	t.Helper()
	created, rawToken, err := e.svc.InviteUser(e.wsCtx(e.admin), e.admin, InviteUserInput{
		Email: email, DisplayName: name, Role: "rep",
	})
	if err != nil {
		t.Fatalf("inviting %s: %v", email, err)
	}
	redeem := principal.WithCorrelationID(principal.WithWorkspaceID(ctx, e.ws.UUID), ids.NewV7())
	if err := e.svc.RedeemPasswordReset(redeem, rawToken, "a member password!"); err != nil {
		t.Fatalf("redeeming the invite for %s: %v", email, err)
	}
	return created
}
