// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// Admin-side session management: a user_admin holder lists the sessions open
// under another member's account and ends one — the counterpart to the
// self-service surface, gated on the object rather than self-scoped, and
// answering a stranger's user or session id with the same not-found the rest of
// the tree gives an out-of-scope row.

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAdminListsAndRevokesAMembersSession(t *testing.T) {
	e := setupRevocationEnv(t, "admin-sessions")

	memberToken := e.signIn(t, "member's laptop")

	sessions, err := e.svc.ListUserSessions(e.wsOnlyCtx(), e.admin, e.member.UserID)
	if err != nil {
		t.Fatalf("ListUserSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("admin sees %d of the member's sessions, want 1", len(sessions))
	}
	if sessions[0].UserAgent == nil || *sessions[0].UserAgent != "member's laptop" {
		t.Errorf("admin's view of the member's session lost its device: %v", sessions[0].UserAgent)
	}

	if err := e.svc.RevokeUserSession(e.wsOnlyCtx(), e.admin, e.member.UserID, sessions[0].ID); err != nil {
		t.Fatalf("RevokeUserSession: %v", err)
	}
	if _, err := e.svc.Authenticate(e.wsOnlyCtx(), memberToken); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("the member's session survived an admin revoke: %v", err)
	}
}

func TestManagingSessionsNeedsTheUserAdminGrant(t *testing.T) {
	e := setupRevocationEnv(t, "admin-sessions-rbac")

	// The member holds no user_admin grant. Acting as themselves against the
	// admin, both verbs are refused at the object gate — a 403, before any row.
	if _, err := e.svc.ListUserSessions(e.wsOnlyCtx(), e.member, e.admin.UserID); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("list without user_admin: err=%v, want ErrPermissionDenied", err)
	}
	if err := e.svc.RevokeUserSession(e.wsOnlyCtx(), e.member, e.admin.UserID, ids.NewV7()); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("revoke without user_admin: err=%v, want ErrPermissionDenied", err)
	}
}

func TestAdminSessionActionsOnAnUnknownUserAreNotFound(t *testing.T) {
	e := setupRevocationEnv(t, "admin-sessions-unknown")

	ghost := ids.New[ids.UserKind]()
	if _, err := e.svc.ListUserSessions(e.wsOnlyCtx(), e.admin, ghost); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("list of an unknown user: err=%v, want ErrNotFound", err)
	}
	if err := e.svc.RevokeUserSession(e.wsOnlyCtx(), e.admin, ghost, ids.NewV7()); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("revoke on an unknown user: err=%v, want ErrNotFound", err)
	}
}

func TestAdminRevokeOfAnUnknownSessionForAKnownUserIsNotFound(t *testing.T) {
	e := setupRevocationEnv(t, "admin-sessions-nosession")

	if err := e.svc.RevokeUserSession(e.wsOnlyCtx(), e.admin, e.member.UserID, ids.NewV7()); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("revoke of a session the member does not hold: err=%v, want ErrNotFound", err)
	}
}
