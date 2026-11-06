// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// recordedRechecks is capture's side of the seam, recording which members a
// role write asked it to look at again. A member refuses capture when they may
// not create activities.
type recordedRechecks struct {
	asked  []ids.UUID
	counts map[ids.UUID]int
}

func (*recordedRechecks) Refuses(perms principal.Permissions, seat principal.SeatType) bool {
	return !seat.CanMutate() || !perms.Allows("activity", principal.ActionCreate)
}

func (r *recordedRechecks) LiveMailboxesTx(context.Context, pgx.Tx, []ids.UUID) (map[ids.UUID]int, error) {
	return r.counts, nil
}

func (r *recordedRechecks) RecheckSeatsTx(_ context.Context, _ pgx.Tx, userIDs []ids.UUID) error {
	r.asked = append(r.asked, userIDs...)
	return nil
}

func TestChangingAMembersRoleRechecksTheirMailboxes(t *testing.T) {
	e := setupRevocationEnv(t, "mail-recheck-change")
	seam := &recordedRechecks{}
	e.svc.WithMailCapture(seam)

	if err := e.svc.ChangeUserRole(e.wsCtx(e.admin), e.admin, e.member.UserID, "read_only"); err != nil {
		t.Fatalf("changing the role: %v", err)
	}
	if !slices.Equal(seam.asked, []ids.UUID{e.member.UserID.UUID}) {
		t.Fatalf("rechecked %v, want exactly the member whose role changed", seam.asked)
	}
}

func TestEditingARolesGrantRechecksEveryHolderAndChangesWhetherItCaptures(t *testing.T) {
	e := setupRevocationEnv(t, "mail-recheck-grant")
	ctx := e.wsCtx(e.admin)
	if err := e.svc.ChangeUserRole(ctx, e.admin, e.member.UserID, "rep"); err != nil {
		t.Fatalf("assigning rep: %v", err)
	}
	seam := &recordedRechecks{}
	e.svc.WithMailCapture(seam)

	if _, err := e.svc.SetRoleObjectGrant(ctx, e.admin, "rep", "activity", storedGrant{Read: true}, nil); err != nil {
		t.Fatalf("narrowing rep: %v", err)
	}
	if !slices.Contains(seam.asked, e.member.UserID.UUID) {
		t.Fatalf("rechecked %v, want the rep holder %s among them", seam.asked, e.member.UserID)
	}
	roles, err := e.svc.ListAssignableRoles(ctx, e.admin)
	if err != nil {
		t.Fatal(err)
	}
	captures := map[string]bool{}
	for _, role := range roles {
		if role.CapturesMail == nil {
			t.Fatalf("role %s carries no captures_mail with the seam wired", role.Key)
		}
		captures[role.Key] = *role.CapturesMail
	}
	if captures["rep"] || captures["read_only"] || !captures["admin"] {
		t.Fatalf("captures_mail = %v, want rep and read_only false after the narrowing, admin true", captures)
	}
}

// The administration roster carries each member's live connection count, which
// the role dialog warns with.
func TestTheAdministrationRosterCountsEachMembersMailboxes(t *testing.T) {
	e := setupRevocationEnv(t, "mail-roster-count")
	e.svc.WithMailCapture(&recordedRechecks{counts: map[ids.UUID]int{e.member.UserID.UUID: 2}})

	page, err := e.svc.ListUsers(e.wsCtx(e.admin), ListUsersInput{Actor: &e.admin})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, u := range page.Users {
		want := 0
		if u.ID == e.member.UserID.UUID {
			want, found = 2, true
		}
		if u.ConnectedMailboxes == nil || *u.ConnectedMailboxes != want {
			t.Errorf("%s connected_mailboxes = %v, want %d", u.Email, u.ConnectedMailboxes, want)
		}
	}
	if !found {
		t.Fatal("the member is not on the roster; the counts above prove nothing")
	}
}

// A role the identity provider's groups grant at sign-in reaches the member's
// mailboxes like a role an admin gave.
func TestAGroupGrantedRoleRechecksTheMembersMailboxes(t *testing.T) {
	svc, _, userID, email := seedSSOEnv(t, "sso-group-recheck")
	svc.WithGroupRoleMap(func(context.Context, pgx.Tx) (map[string]string, error) {
		return map[string]string{"crm-users": "rep"}, nil
	})
	seam := &recordedRechecks{}
	svc.WithMailCapture(seam)

	if _, err := svc.LoginViaFederatedIdentity(groupSyncCtx(), "google",
		OIDCClaims{Subject: "sub-1", Email: email, Groups: []string{"crm-users"}}); err != nil {
		t.Fatalf("LoginViaFederatedIdentity: %v", err)
	}
	if !slices.Equal(seam.asked, []ids.UUID{userID.UUID}) {
		t.Fatalf("rechecked %v, want exactly the member the group granted a role", seam.asked)
	}
}
