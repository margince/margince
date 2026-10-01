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
	e.dropMasksAfter(t, maskedRep)

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
// cannot, and a lead on such a team coaches colleagues the caller cannot.
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

	// A role change is held against the teams the member is already on: an
	// own-scoped rep on South gains South's records by becoming team-scoped.
	for team, refused := range map[ids.UUID]bool{north.ID: false, south.ID: true} {
		member := e.seat(t, "rescoped-"+strconv.FormatBool(refused), "rep", team)
		err := e.svc.ChangeUserRole(e.wsCtx(caller), caller, member.UserID, teamRep)
		if refused && !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("making a rep on another team team-scoped: %v, want permission denied", err)
		}
		if !refused && err != nil {
			t.Errorf("making a rep on the caller's team team-scoped: %v, want admitted", err)
		}
	}

	// Only an admin invites onto a team, whichever team it is.
	for _, team := range []ids.UUID{north.ID, south.ID} {
		_, _, err := e.svc.InviteUser(e.wsCtx(caller), caller, InviteUserInput{
			Email: "invited@" + e.slug + ".test", DisplayName: "Invited",
			Role: teamRep, TeamIDs: []ids.UUID{team},
		})
		if !errors.Is(err, errTeamMembershipRequiresAdmin) {
			t.Errorf("a delegate inviting onto a team: %v, want errTeamMembershipRequiresAdmin", err)
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

// The pickers' list and the invite agree: every role offered is accepted, and
// every live role withheld is refused.
func TestTheAssignableRolesAreExactlyTheRolesAnInviteAccepts(t *testing.T) {
	e := setupRevocationEnv(t, "assignable")
	caller := e.seat(t, "delegate", e.customRole(t, "Member admin", "rep", map[string]storedGrant{objectUserAdmin: fullGrant}))
	e.customRole(t, "Field sales", "rep", nil)
	e.customRole(t, "Role editor", "rep", map[string]storedGrant{objectRoleAdmin: fullGrant})

	offered, err := e.svc.ListAssignableRoles(e.wsCtx(caller), caller)
	if err != nil {
		t.Fatal(err)
	}
	every, err := e.svc.ListRoles(e.wsCtx(e.admin), e.admin, false)
	if err != nil {
		t.Fatal(err)
	}
	isOffered := map[string]bool{}
	for _, role := range offered {
		isOffered[role.Key] = true
	}
	if !isOffered["rep"] || !isOffered["custom_field_sales"] || isOffered[roleAdmin] || isOffered["custom_role_editor"] {
		t.Errorf("offered %v; want rep and custom_field_sales, and neither admin nor custom_role_editor", isOffered)
	}
	for i, role := range every {
		_, _, err := e.svc.InviteUser(e.wsCtx(caller), caller, InviteUserInput{
			Email: "agree-" + strconv.Itoa(i) + "@" + e.slug + ".test", DisplayName: "Agree", Role: role.Key,
		})
		if isOffered[role.Key] && err != nil {
			t.Errorf("%s was offered and the invite refused it: %v", role.Key, err)
		}
		if !isOffered[role.Key] && !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("%s was withheld and the invite answered %v, want permission denied", role.Key, err)
		}
	}

	if _, err := e.svc.ListAssignableRoles(e.wsCtx(e.member), e.member); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a member without user_admin reading the assignable roles: %v, want permission denied", err)
	}
}

// The account an invite or a role change produces is held against everything
// the delegate holds, read off the stored role: a record grant, an extension
// grant this build no longer composes, and a field the delegate cannot read.
func TestADelegateProducesNoAccountWiderThanTheirOwn(t *testing.T) {
	e := setupRevocationEnv(t, "assign-whole")
	registerExtObject(t)
	delegateRole := e.customRole(t, "Member admin", "rep", map[string]storedGrant{
		objectUserAdmin: fullGrant, "deal": {},
	})
	e.maskRole(t, delegateRole)
	delegate := e.seat(t, "delegate", delegateRole)
	masked := e.customRole(t, "Masked rep", "rep", map[string]storedGrant{"deal": {}})
	e.maskRole(t, masked)
	withDeals := e.customRole(t, "Rep with deals", "rep", map[string]storedGrant{"deal": {Read: true}})
	withExt := e.customRole(t, "Rep with notes", "rep", map[string]storedGrant{"deal": {}, extObject: {Read: true}})
	ResetRbacObjectsForTest()
	// Each role below differs from the admitted one on ONE axis, so each carries
	// the delegate's mask unless the mask is the axis it tests.
	e.maskRole(t, withDeals)
	e.maskRole(t, withExt)
	unmasked := e.customRole(t, "Unmasked rep", "rep", map[string]storedGrant{"deal": {}})
	target := e.seat(t, "target", masked)
	ctx := e.wsCtx(delegate)

	for i, tc := range []struct {
		name    string
		role    string
		refused bool
	}{
		{"a role inside the delegate's own", masked, false},
		{"a role reading deals", withDeals, true},
		{"a role holding an extension grant", withExt, true},
		{"a role reading a field the delegate cannot", unmasked, true},
	} {
		_, _, invited := e.svc.InviteUser(ctx, delegate, InviteUserInput{
			Email: "whole-" + strconv.Itoa(i) + "@" + e.slug + ".test", DisplayName: "Whole", Role: tc.role,
		})
		assigned := e.svc.ChangeUserRole(ctx, delegate, target.UserID, tc.role)
		for verb, err := range map[string]error{"invite into": invited, "assign": assigned} {
			if tc.refused && !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Errorf("%s %s: %v, want permission denied", verb, tc.name, err)
			}
			if !tc.refused && err != nil {
				t.Errorf("%s %s: %v, want admitted", verb, tc.name, err)
			}
		}
	}
}

// Archive, reactivate, link, restore: the chain that would hand a delegate an
// account on a role wider than theirs. It stops at reactivation, and a member
// on an archived role gets no link and no passport either.
func TestAMemberOnAnArchivedRoleIsNotHandedBack(t *testing.T) {
	e := setupRevocationEnv(t, "archived-held")
	ctx := e.wsCtx(e.admin)
	strong := e.customRole(t, "Strong", "management", nil)
	holder := e.seat(t, "holder", strong)
	if err := e.svc.DeactivateUser(ctx, e.admin, DeactivateUserInput{UserID: holder.UserID}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.ArchiveRole(ctx, e.admin, strong); err != nil {
		t.Fatalf("archiving a role only a deactivated member holds: %v", err)
	}

	if err := e.svc.ReactivateUser(ctx, e.admin, holder.UserID); !errors.Is(err, errArchivedRoleHeld) {
		t.Errorf("reactivating a member on an archived role: %v, want errArchivedRoleHeld", err)
	}
	if _, _, err := e.svc.IssuePasswordLink(ctx, e.admin, holder.UserID); !errors.Is(err, errArchivedRoleHeld) {
		t.Errorf("a link for a member on an archived role: %v, want errArchivedRoleHeld", err)
	}
	if _, err := e.svc.IssuePassport(e.wsCtx(holder), holder, IssuePassportInput{Scopes: []string{"read"}}); !errors.Is(err, errArchivedRoleHeld) {
		t.Errorf("a passport for a member on an archived role: %v, want errArchivedRoleHeld", err)
	}

	// Given a live role, the same member comes back.
	if err := e.svc.ChangeUserRole(ctx, e.admin, holder.UserID, "rep"); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.ReactivateUser(ctx, e.admin, holder.UserID); err != nil {
		t.Errorf("reactivating the member once reassigned: %v", err)
	}
}

// Only an admin changes who is on a team. A holder of team_admin who is not an
// admin may create and rename a team, and may not add anybody — themselves
// included — take anybody off, or archive or restore the team.
func TestOnlyAnAdminChangesWhoIsOnATeam(t *testing.T) {
	e := setupRevocationEnv(t, "team-members")
	delegate := e.seat(t, "delegate", e.customRole(t, "Team admin", "rep", map[string]storedGrant{
		objectTeamAdmin: {Create: true, Read: true, Update: true},
	}))
	colleague := e.seat(t, "colleague", "rep")
	ctx := e.wsCtx(delegate)
	team, err := e.svc.CreateTeam(ctx, delegate, "North")
	if err != nil {
		t.Fatalf("a team_admin holder creating a team: %v", err)
	}
	renamed := "North DACH"
	if _, err := e.svc.UpdateTeam(ctx, delegate, team.ID, UpdateTeamInput{Name: &renamed}); err != nil {
		t.Errorf("a team_admin holder renaming a team: %v", err)
	}

	archived := true
	for verb, err := range map[string]error{
		"add themselves":  e.svc.SetTeamMember(ctx, delegate, team.ID, delegate.UserID.UUID, true),
		"add a colleague": e.svc.SetTeamMember(ctx, delegate, team.ID, colleague.UserID.UUID, true),
		"archive the team": func() error {
			_, err := e.svc.UpdateTeam(ctx, delegate, team.ID, UpdateTeamInput{Archived: &archived})
			return err
		}(),
	} {
		if !errors.Is(err, errTeamMembershipRequiresAdmin) {
			t.Errorf("a team_admin holder tried to %s: %v, want errTeamMembershipRequiresAdmin", verb, err)
		}
	}

	if err := e.svc.SetTeamMember(e.wsCtx(e.admin), e.admin, team.ID, colleague.UserID.UUID, true); err != nil {
		t.Fatalf("an admin adding a colleague: %v", err)
	}
	if err := e.svc.SetTeamMember(ctx, delegate, team.ID, colleague.UserID.UUID, false); !errors.Is(err, errTeamMembershipRequiresAdmin) {
		t.Errorf("a team_admin holder removing a colleague: %v, want errTeamMembershipRequiresAdmin", err)
	}
	if err := e.svc.SetTeamMember(e.wsCtx(e.admin), e.admin, team.ID, colleague.UserID.UUID, false); err != nil {
		t.Errorf("an admin removing a colleague: %v", err)
	}
}
