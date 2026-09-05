// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A delegated member administrator cannot reach an administrator's account.
//
// This is the case the whole of escalation.go exists for, and it did not exist
// before this change: while these verbs gated on the literal admin role, every
// caller already held every authority every target could hold, so no call site
// read the target's role and none needed to. Handing user_admin out as a grant
// removes that guarantee.
//
// Each verb is asserted twice. The refusal alone would pass against an authority
// that refuses EVERYONE — a gate accidentally denying admins too looks identical
// from the deny side — so every case carries the admit arm that proves the door
// is open to the seat that should hold it.
//
// The gap this closes was real: InviteUser shipped in an earlier revision of
// this change with no assignment ceiling at all, and nothing failed. It creates
// a user, assigns whatever role the caller names, and returns the raw
// set-password token, so a delegated user_admin.create holder could mint
// themselves an admin and walk in with the token in the response body.
func TestADelegatedMemberAdministratorCannotReachAnAdmin(t *testing.T) {
	e := setupRevocationEnv(t, "escalation")

	// The delegation this PR ships: user_admin in full, and nothing else that
	// matters. Deliberately NOT the admin role key, because holding that key is
	// its own ceiling and would short-circuit every guard under test.
	delegated := Identity{
		UserID: ids.New[ids.UserKind](), WorkspaceID: e.admin.WorkspaceID,
		Roles: []string{"ops"}, SeatType: string(principal.SeatFull),
		Permissions: principal.Permissions{
			RoleKeys: []string{"ops"},
			Objects: map[string]principal.ObjectGrant{
				objectUserAdmin: {Create: true, Read: true, Update: true, Delete: true},
			},
			RowScope: principal.RowScopeAll,
		},
	}

	for _, tt := range []struct {
		verb string
		call func(Identity) error
	}{
		{"issue a password link", func(actor Identity) error {
			_, _, err := e.svc.IssuePasswordLink(escalationCtx(actor), actor,
				ids.From[ids.UserKind](e.admin.UserID.UUID))
			return err
		}},
		{"deactivate", func(actor Identity) error {
			return e.svc.DeactivateUser(escalationCtx(actor), actor,
				DeactivateUserInput{UserID: ids.From[ids.UserKind](e.admin.UserID.UUID)})
		}},
		{"change the role of", func(actor Identity) error {
			return e.svc.ChangeUserRole(escalationCtx(actor), actor,
				ids.From[ids.UserKind](e.admin.UserID.UUID), "rep")
		}},
	} {
		t.Run(tt.verb+" an admin", func(t *testing.T) {
			if err := tt.call(delegated); !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Errorf("a delegated user_admin holder tried to %s an admin: err=%v, want "+
					"permission denied — this is an account-takeover path", tt.verb, err)
			}
			// The admit arm. Without it a gate refusing everybody passes above.
			if err := tt.call(e.admin); errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Errorf("an admin tried to %s another admin: permission denied — the guard "+
					"refuses the seat that is supposed to hold this authority", tt.verb)
			}
		})
	}

	// Inviting is the fourth path and the sharpest: it needs no existing target,
	// so a ceiling that only asks "who may I act on" would never fire.
	t.Run("invite a new admin", func(t *testing.T) {
		_, _, err := e.svc.InviteUser(escalationCtx(delegated), delegated, InviteUserInput{
			Email: "mallory@" + e.slug + ".test", DisplayName: "Backup Admin", Role: "admin",
		})
		if !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("a delegated user_admin holder invited a new admin: err=%v, want permission "+
				"denied — the invite returns the raw set-password token, so this is a complete "+
				"takeover in one call", err)
		}
		if _, _, err := e.svc.InviteUser(escalationCtx(e.admin), e.admin, InviteUserInput{
			Email: "genuine@" + e.slug + ".test", DisplayName: "Genuine Admin", Role: "admin",
		}); err != nil {
			t.Errorf("an admin invited an admin: %v — the ceiling refuses the seat that holds "+
				"the authority", err)
		}
	})

	// A role the caller CAN assign still works, so the ceiling is about the
	// authority being handed out and not about the verb being unusable.
	t.Run("invite a rep, which the delegation allows", func(t *testing.T) {
		if _, _, err := e.svc.InviteUser(escalationCtx(delegated), delegated, InviteUserInput{
			Email: "newrep@" + e.slug + ".test", DisplayName: "New Rep", Role: "rep",
		}); err != nil {
			t.Errorf("a delegated user_admin holder invited a rep: %v — the delegation is "+
				"supposed to make exactly this possible", err)
		}
	})
}

// escalationCtx binds what the HTTP middleware binds, PERMISSIONS INCLUDED.
//
// revocationEnv.wsCtx builds a principal carrying no grants, which was complete
// while these paths gated on a role key read off the Identity argument. They
// read the context now, so a caller built that way holds nothing and every case
// here would pass by being refused for the wrong reason.
func escalationCtx(id Identity) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), id.WorkspaceID.UUID)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + id.UserID.String(),
		UserID: id.UserID.UUID, SeatType: principal.SeatType(id.SeatType),
		Permissions: id.Permissions,
	})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}
