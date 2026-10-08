// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// The roster's allowed actions agree with the member verbs: every action it
// offers on a member, the real handler accepts, and every one it withholds,
// the handler refuses or answers without changing the member.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// rosterActions reads the management roster through the handler, as caller.
func rosterActions(t *testing.T, e *revocationEnv, h Handlers, caller Identity) map[ids.UUID][]crmcontracts.UserAllowedActions {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/users?include_inactive=true", nil).
		WithContext(withIdentity(e.wsCtx(caller), caller))
	inactive, limit := true, 200
	h.ListUsers(rec, req, crmcontracts.ListUsersParams{IncludeInactive: &inactive, Limit: &limit})
	if rec.Code != http.StatusOK {
		t.Fatalf("roster status = %d: %s", rec.Code, rec.Body)
	}
	var page crmcontracts.UserListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	out := map[ids.UUID][]crmcontracts.UserAllowedActions{}
	for _, user := range page.Data {
		if user.AllowedActions == nil {
			t.Fatalf("%s carries no allowed_actions on a member administrator's roster", user.Email)
		}
		out[ids.UUID(user.Id)] = *user.AllowedActions
	}
	return out
}

// callMemberVerb runs one member verb through its handler and answers the
// status code.
func callMemberVerb(e *revocationEnv, h Handlers, caller Identity, action memberAction, target ids.UserID, toRole string) int {
	rec := httptest.NewRecorder()
	body := "{}"
	if action == actionChangeRole {
		body = `{"role":"` + toRole + `"}`
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/users/"+target.String(), strings.NewReader(body)).
		WithContext(withIdentity(e.wsCtx(caller), caller))
	id := crmcontracts.Id(target.UUID)
	switch action {
	case actionChangeRole:
		h.ChangeUserRole(rec, req, id)
	case actionIssuePasswordLink:
		h.IssueUserPasswordLink(rec, req, id)
	case actionDeactivate:
		h.DeactivateUser(rec, req, id)
	case actionReactivate:
		h.ReactivateUser(rec, req, id)
	}
	return rec.Code
}

// memberState is what a member verb changes, read as the admin.
func memberState(t *testing.T, e *revocationEnv, userID ids.UserID) string {
	t.Helper()
	row, err := e.svc.GetUser(e.wsCtx(e.admin), e.admin, userID)
	if err != nil {
		t.Fatal(err)
	}
	return row.Status + " " + strings.Join(row.Roles, ",")
}

// assertOfferAgrees calls action on target and holds the answer to the offer.
func assertOfferAgrees(t *testing.T, e *revocationEnv, h Handlers, caller Identity, offered []crmcontracts.UserAllowedActions,
	action memberAction, name string, target ids.UserID, toRole string,
) {
	t.Helper()
	listed := slices.Contains(offered, crmcontracts.UserAllowedActions(action))
	before := memberState(t, e, target)
	code := callMemberVerb(e, h, caller, action, target, toRole)
	accepted := code < http.StatusBadRequest
	switch {
	case listed && !accepted:
		t.Errorf("%s on %s: offered, and the handler answered %d", action, name, code)
	case !listed && accepted && action != actionIssuePasswordLink && memberState(t, e, target) != before:
		t.Errorf("%s on %s: withheld, and the handler changed the member (%d)", action, name, code)
	case !listed && accepted && action == actionIssuePasswordLink:
		t.Errorf("%s on %s: withheld, and the handler minted a link", action, name)
	}
}

func TestTheRosterOffersADelegatedAdministratorExactlyTheVerbsTheHandlersAccept(t *testing.T) {
	e := setupRevocationEnv(t, "allowed-actions")
	h := NewHandlers(e.svc).WithPasswordLinkBase("https://crm.example.test")
	delegate := e.customRole(t, "Member admin", "rep", map[string]storedGrant{objectUserAdmin: fullGrant})
	repCopy := e.customRole(t, "Rep copy", "rep", nil)
	auditor := e.customRole(t, "Auditor", "rep", map[string]storedGrant{"audit_log": {Read: true}})
	closer := e.customRole(t, "Closer", "rep", map[string]storedGrant{"deal": fullGrant})

	// Each verb gets its own caller and targets: every verb it admits changes
	// the member, and the next verb must meet the member as the offer saw it.
	for _, action := range []memberAction{actionChangeRole, actionIssuePasswordLink, actionDeactivate, actionReactivate} {
		tag := string(action)
		caller := e.seat(t, "delegate-"+tag, delegate)
		deactivated := e.seat(t, "gone-"+tag, "rep")
		if err := e.svc.DeactivateUser(e.wsCtx(e.admin), e.admin, DeactivateUserInput{UserID: deactivated.UserID}); err != nil {
			t.Fatal(err)
		}
		targets := map[string]ids.UserID{
			"an admin":                      e.seat(t, "admin-"+tag, roleAdmin).UserID,
			"a wider administration grant":  e.seat(t, "auditor-"+tag, auditor).UserID,
			"a wider record grant":          e.seat(t, "closer-"+tag, closer).UserID,
			"a narrower role":               e.seat(t, "rep-"+tag, "rep").UserID,
			"a deactivated narrower member": deactivated.UserID,
			"the caller":                    caller.UserID,
		}
		offers := rosterActions(t, e, h, caller)
		offeredAnywhere := false
		for name, target := range targets {
			offeredAnywhere = offeredAnywhere || slices.Contains(offers[target.UUID], crmcontracts.UserAllowedActions(action))
			assertOfferAgrees(t, e, h, caller, offers[target.UUID], action, name, target, repCopy)
		}
		// A roster offering nothing would agree with every refusal above.
		if !offeredAnywhere {
			t.Errorf("%s is offered on no target; the agreement above held vacuously", action)
		}
	}
}

// The literal admin is the sole admin here, so the roster withholds the two
// verbs that would leave nobody able to administer members, and the handlers
// refuse them.
func TestTheRosterWithholdsWhatWouldRemoveTheLastAdmin(t *testing.T) {
	e := setupRevocationEnv(t, "allowed-last-admin")
	h := NewHandlers(e.svc).WithPasswordLinkBase("https://crm.example.test")
	offered := rosterActions(t, e, h, e.admin)[e.admin.UserID.UUID]
	for _, action := range []memberAction{actionChangeRole, actionDeactivate} {
		if slices.Contains(offered, crmcontracts.UserAllowedActions(action)) {
			t.Errorf("%s is offered on the sole admin", action)
		}
		assertOfferAgrees(t, e, h, e.admin, offered, action, "the sole admin", e.admin.UserID, "rep")
	}
	if !slices.Contains(offered, crmcontracts.UserAllowedActions(actionIssuePasswordLink)) {
		t.Errorf("the sole admin's own password link is withheld: %v", offered)
	}
}

// An installation with no public base URL builds no link, so the roster never
// offers one.
func TestTheRosterOffersNoPasswordLinkWithoutABaseURL(t *testing.T) {
	e := setupRevocationEnv(t, "allowed-no-base")
	offers := rosterActions(t, e, NewHandlers(e.svc), e.admin)
	for user, offered := range offers {
		if slices.Contains(offered, crmcontracts.UserAllowedActions(actionIssuePasswordLink)) {
			t.Errorf("%s is offered a password link on an installation that cannot build one", user)
		}
	}
	if !slices.Contains(offers[e.member.UserID.UUID], crmcontracts.UserAllowedActions(actionDeactivate)) {
		t.Errorf("the member's offer %v lost more than the link", offers[e.member.UserID.UUID])
	}
}

// A caller holding one user_admin verb is offered exactly the actions that
// verb opens, and the handlers agree for the ones it does not.
func TestTheRosterOffersOnlyTheVerbsTheCallerHolds(t *testing.T) {
	e := setupRevocationEnv(t, "allowed-verbs")
	h := NewHandlers(e.svc).WithPasswordLinkBase("https://crm.example.test")
	repCopy := e.customRole(t, "Rep copy", "rep", nil)
	for verb, want := range map[string][]memberAction{
		"delete": {actionDeactivate},
		"update": {actionChangeRole, actionIssuePasswordLink},
	} {
		grant := storedGrant{Read: true, Delete: verb == "delete", Update: verb == "update"}
		caller := e.seat(t, "holder-"+verb, e.customRole(t, "Holds "+verb, "rep", map[string]storedGrant{objectUserAdmin: grant}))
		target := e.seat(t, "target-"+verb, "rep")
		offered := rosterActions(t, e, h, caller)[target.UserID.UUID]
		got := make([]memberAction, 0, len(offered))
		for _, action := range offered {
			got = append(got, memberAction(action))
		}
		if !slices.Equal(got, want) {
			t.Errorf("a %s holder is offered %v on a rep, want %v", verb, got, want)
		}
		for _, action := range []memberAction{actionChangeRole, actionIssuePasswordLink, actionDeactivate} {
			assertOfferAgrees(t, e, h, caller, offered, action, "a rep", target.UserID, repCopy)
		}
	}
}

// The sole admin holding a second role may still be given admin alone, which
// keeps an administrator; the roster offers the change, and every other
// choice for them is refused.
func TestTheRosterOffersTheSoleAdminARoleChangeThatKeepsAdmin(t *testing.T) {
	e := setupRevocationEnv(t, "allowed-sole-admin-extra")
	h := NewHandlers(e.svc).WithPasswordLinkBase("https://crm.example.test")
	extra := e.customRole(t, "Extra", "rep", nil)
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO role_assignment (role_id, user_id) VALUES ($1, $2)`,
		e.roleID(t, extra), e.admin.UserID); err != nil {
		t.Fatal(err)
	}
	offered := rosterActions(t, e, h, e.admin)[e.admin.UserID.UUID]
	if !slices.Contains(offered, crmcontracts.UserAllowedActions(actionChangeRole)) {
		t.Fatalf("the sole admin holding [admin, %s] is offered %v, want change_role", extra, offered)
	}
	if code := callMemberVerb(e, h, e.admin, actionChangeRole, e.admin.UserID, "rep"); code != http.StatusConflict {
		t.Errorf("demoting the sole admin to rep answered %d, want 409", code)
	}
	assertOfferAgrees(t, e, h, e.admin, offered, actionChangeRole, "the sole admin", e.admin.UserID, roleAdmin)
	if got := memberState(t, e, e.admin.UserID); got != "active admin" {
		t.Errorf("the sole admin after keeping admin alone = %q, want %q", got, "active admin")
	}
}
