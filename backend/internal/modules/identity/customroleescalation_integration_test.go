// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// A delegated member administrator acts only on members their own authority
// contains, now that a custom role can be wider than theirs without being
// admin. Deactivating needs the target's administration grants and row scope;
// a password link or a role change needs everything the target holds.
//
// Every case carries an admitted arm beside the refused one, so a guard that
// refused everybody would fail here rather than pass.

import (
	"errors"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// memberVerbs are the three member verbs a delegated administrator carries, as
// calls against one target.
func (e *revocationEnv) memberVerbs(caller Identity, target ids.UserID, toRole string) map[string]func() error {
	ctx := e.wsCtx(caller)
	return map[string]func() error{
		"password link": func() error {
			_, _, err := e.svc.IssuePasswordLink(ctx, caller, target)
			return err
		},
		"role change": func() error { return e.svc.ChangeUserRole(ctx, caller, target, toRole) },
		"deactivate": func() error {
			return e.svc.DeactivateUser(ctx, caller, DeactivateUserInput{UserID: target})
		},
	}
}

func TestADelegatedMemberAdministratorActsOnlyOnSeatsTheirAuthorityContains(t *testing.T) {
	e := setupRevocationEnv(t, "contain")
	caller := e.seat(t, "delegate", e.customRole(t, "Member admin", "rep", map[string]storedGrant{objectUserAdmin: fullGrant}))
	repCopy := e.customRole(t, "Rep copy", "rep", nil)
	auditor := e.customRole(t, "Auditor", "rep", map[string]storedGrant{"audit_log": {Read: true}})
	closer := e.customRole(t, "Closer", "rep", map[string]storedGrant{"deal": fullGrant})

	for _, tc := range []struct {
		name    string
		role    string
		refused map[string]bool
	}{
		{"a plain rep", "rep", map[string]bool{}},
		{"a wider administration grant", auditor, map[string]bool{"password link": true, "role change": true, "deactivate": true}},
		{"a wider row scope", "read_only", map[string]bool{"password link": true, "role change": true, "deactivate": true}},
		{"a wider record grant", closer, map[string]bool{"password link": true, "role change": true}},
	} {
		target := e.seat(t, "target-"+tc.role, tc.role)
		verbs := e.memberVerbs(caller, target.UserID, repCopy)
		for _, verb := range []string{"password link", "role change", "deactivate"} {
			err := verbs[verb]()
			if tc.refused[verb] && !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Errorf("%s on %s: %v, want permission denied", verb, tc.name, err)
			}
			if !tc.refused[verb] && err != nil {
				t.Errorf("%s on %s: %v, want admitted", verb, tc.name, err)
			}
		}
	}

	if err := e.svc.ChangeUserRole(e.wsCtx(caller), caller, caller.UserID, repCopy); !errors.Is(err, errOwnRole) {
		t.Errorf("the delegate changing their own role: %v, want errOwnRole", err)
	}
}

// A field the caller cannot read and the target can is access the target has
// and the caller lacks.
func TestAPasswordLinkNeedsEveryFieldTheTargetReads(t *testing.T) {
	e := setupRevocationEnv(t, "contain-mask")
	callerRole := e.customRole(t, "Masked member admin", "rep", map[string]storedGrant{objectUserAdmin: fullGrant})
	e.maskRole(t, callerRole)
	caller := e.seat(t, "delegate", callerRole)
	// A copy carries the source's mask; taking user_admin away leaves a rep
	// who withholds the same field the caller does.
	maskedRep := e.customRole(t, "Masked rep", callerRole, map[string]storedGrant{objectUserAdmin: {}})

	unmasked := e.seat(t, "unmasked", "rep")
	if _, _, err := e.svc.IssuePasswordLink(e.wsCtx(caller), caller, unmasked.UserID); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a link for a rep who reads a field the caller cannot: %v, want permission denied", err)
	}
	masked := e.seat(t, "masked", maskedRep)
	if _, _, err := e.svc.IssuePasswordLink(e.wsCtx(caller), caller, masked.UserID); err != nil {
		t.Errorf("a link for a rep withholding the same field: %v, want admitted", err)
	}
}

// A team-scoped target on a team the caller is not on reads records the caller
// cannot, and a lead on such a team coaches people the caller cannot.
func TestAPasswordLinkNeedsTheTeamsTheTargetReaches(t *testing.T) {
	e := setupRevocationEnv(t, "contain-team")
	ctx := e.wsCtx(e.admin)
	north, err := e.svc.CreateTeam(ctx, e.admin, "North")
	if err != nil {
		t.Fatal(err)
	}
	south, err := e.svc.CreateTeam(ctx, e.admin, "South")
	if err != nil {
		t.Fatal(err)
	}
	// The caller leads too, so a lead target is decided by its team and not by
	// a grant the caller lacks.
	callerRole := e.customRole(t, "Team member admin", "rep", map[string]storedGrant{
		objectUserAdmin: fullGrant, objectTeamLead: {Read: true},
	})
	e.scopeRole(t, callerRole, principal.RowScopeTeam)
	caller := e.seat(t, "delegate", callerRole, north.ID)
	teamRep := e.customRole(t, "Team rep", "rep", nil)
	e.scopeRole(t, teamRep, principal.RowScopeTeam)
	lead := e.customRole(t, "Own-scoped lead", "rep", map[string]storedGrant{objectTeamLead: {Read: true}})

	for i, tc := range []struct {
		name    string
		role    string
		team    ids.UUID
		refused bool
	}{
		{"a team rep on the caller's team", teamRep, north.ID, false},
		{"a team rep on another team", teamRep, south.ID, true},
		{"an own-scoped rep on another team", "rep", south.ID, false},
		{"a lead of the caller's team", lead, north.ID, false},
		{"a lead of another team", lead, south.ID, true},
	} {
		target := e.seat(t, "target-"+strconv.Itoa(i), tc.role, tc.team)
		_, _, err := e.svc.IssuePasswordLink(e.wsCtx(caller), caller, target.UserID)
		if tc.refused && !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("%s: %v, want permission denied", tc.name, err)
		}
		if !tc.refused && err != nil {
			t.Errorf("%s: %v, want admitted", tc.name, err)
		}
	}
}

// A copy of the admin role contains everything the admin holds and is still
// not an admin: only a literal admin acts on one.
func TestAnAdminCopyNeverActsOnTheAdmin(t *testing.T) {
	e := setupRevocationEnv(t, "contain-admin")
	caller := e.seat(t, "deputy", e.customRole(t, "Deputy admin", roleAdmin, nil))
	rep := e.seat(t, "rep", "rep")
	ctx := e.wsCtx(caller)
	if err := e.svc.DeactivateUser(ctx, caller, DeactivateUserInput{UserID: rep.UserID}); err != nil {
		t.Errorf("an admin copy deactivating a rep: %v, want admitted", err)
	}
	if err := e.svc.DeactivateUser(ctx, caller, DeactivateUserInput{UserID: e.admin.UserID}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("an admin copy deactivating the admin: %v, want permission denied", err)
	}
}
