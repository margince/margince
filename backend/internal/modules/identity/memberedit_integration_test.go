// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAnImportedMemberCanBeEditedAndPlacedOnATeamBeforeFirstSignIn(t *testing.T) {
	e := setupRevocationEnv(t, "imported-member-edit")
	ctx := e.wsCtx(e.admin)
	member, err := e.svc.CreateFormerMember(ctx, e.admin, FormerMemberInput{Email: "imported@example.test", DisplayName: "Imported Name", Role: "rep"})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.ReactivateUser(ctx, e.admin, member); err != nil {
		t.Fatal(err)
	}
	row, err := e.svc.GetUser(ctx, e.admin, member)
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != userStatusInvited {
		t.Fatalf("status = %s, want invited", row.Status)
	}
	team, err := e.svc.CreateTeam(ctx, e.admin, "Delivery")
	if err != nil {
		t.Fatal(err)
	}
	edit := MemberEdit{Name: "Corrected Name", Email: row.Email, Teams: new([]ids.UUID{team.ID}), ExpectedUpdatedAt: row.UpdatedAt}
	if err := e.svc.UpdateMember(ctx, e.admin, member, edit); err != nil {
		t.Fatal(err)
	}
	saved, err := e.svc.GetUser(ctx, e.admin, member)
	if err != nil {
		t.Fatal(err)
	}
	if saved.DisplayName != "Corrected Name" || len(saved.TeamIDs) != 1 || saved.TeamIDs[0] != team.ID || saved.Status != userStatusInvited {
		t.Fatalf("saved member = %+v", saved)
	}
	if len(e.identityEvents(t, "user.updated", member.UUID)) != 1 {
		t.Fatal("profile change did not publish its event")
	}
	if err := e.svc.UpdateMember(ctx, e.admin, member, edit); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("stale edit = %v", err)
	}
}

func TestARejectedMemberTeamEditRollsBackTheProfile(t *testing.T) {
	e := setupRevocationEnv(t, "atomic-member-edit")
	ctx := e.wsCtx(e.admin)
	row, err := e.svc.GetUser(ctx, e.admin, e.member.UserID)
	if err != nil {
		t.Fatal(err)
	}
	edit := MemberEdit{Name: "Must Roll Back", Email: row.Email, Teams: new([]ids.UUID{ids.NewV7()}), ExpectedUpdatedAt: row.UpdatedAt}
	if err := e.svc.UpdateMember(ctx, e.admin, e.member.UserID, edit); err == nil {
		t.Fatal("unknown team accepted")
	}
	after, err := e.svc.GetUser(ctx, e.admin, e.member.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if after.DisplayName != row.DisplayName || !after.UpdatedAt.Equal(row.UpdatedAt) {
		t.Fatal("failed edit changed the member")
	}
	if len(e.identityEvents(t, "user.updated", e.member.UserID.UUID)) != 0 {
		t.Fatal("rolled-back edit published an event")
	}
	if err := e.svc.UpdateMember(e.wsCtx(e.member), e.member, e.admin.UserID, edit); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("non-admin edit = %v", err)
	}
}

func TestAMemberEmailEditCannotTakeAnotherAccount(t *testing.T) {
	e := setupRevocationEnv(t, "member-email-conflict")
	ctx := e.wsCtx(e.admin)
	row, err := e.svc.GetUser(ctx, e.admin, e.member.UserID)
	if err != nil {
		t.Fatal(err)
	}
	err = e.svc.UpdateMember(ctx, e.admin, e.member.UserID, MemberEdit{Name: row.DisplayName, Email: e.admin.Email, ExpectedUpdatedAt: row.UpdatedAt})
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("duplicate email = %v", err)
	}
	after, err := e.svc.GetUser(ctx, e.admin, e.member.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Email != row.Email {
		t.Fatal("duplicate email changed the account")
	}
}

func TestAUsedSignInAddressCannotBeChangedByTheMemberEditor(t *testing.T) {
	e := setupRevocationEnv(t, "protected-member-email")
	ctx := e.wsCtx(e.admin)
	row, err := e.svc.GetUser(ctx, e.admin, e.member.UserID)
	if err != nil {
		t.Fatal(err)
	}
	err = e.svc.UpdateMember(ctx, e.admin, e.member.UserID, MemberEdit{Name: row.DisplayName, Email: "changed@example.test", ExpectedUpdatedAt: row.UpdatedAt})
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("used email edit = %v", err)
	}
	saved, err := e.svc.GetUser(ctx, e.admin, e.member.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Email != row.Email || !saved.UpdatedAt.Equal(row.UpdatedAt) {
		t.Fatal("refused edit changed the account")
	}
}

func TestAnUnusedImportedSignInAddressCanBeCorrected(t *testing.T) {
	e := setupRevocationEnv(t, "unused-member-email")
	ctx := e.wsCtx(e.admin)
	member, err := e.svc.CreateFormerMember(ctx, e.admin, FormerMemberInput{Email: "mistyped@example.test", DisplayName: "Imported colleague"})
	if err != nil {
		t.Fatal(err)
	}
	row, err := e.svc.GetUser(ctx, e.admin, member)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.UpdateMember(ctx, e.admin, member, MemberEdit{Name: row.DisplayName, Email: "corrected@example.test", ExpectedUpdatedAt: row.UpdatedAt}); err != nil {
		t.Fatal(err)
	}
	saved, err := e.svc.GetUser(ctx, e.admin, member)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Email != "corrected@example.test" {
		t.Fatalf("email = %s", saved.Email)
	}
}
