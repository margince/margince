// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

type limitedGrantBody struct {
	ID            string `json:"id"`
	Access        string `json:"access"`
	LimitedByRole *bool  `json:"limited_by_role"`
}

// roleShareEnv holds a contact, a company and two colleagues: one whose role
// only reads contacts and one whose role edits them.
type roleShareEnv struct {
	*relEnv
	viewer, editor string
}

func setupRoleShare(t *testing.T) roleShareEnv {
	t.Helper()
	e := setupRelationships(t)
	e.DescribeCompany(t)
	return roleShareEnv{
		relEnv: e,
		viewer: createdID(t, e.AppEnv, "/v1/users", AnyMap{"email": "viewer@grantrole.test", "display_name": "Viewer", "role": "read_only"}),
		editor: createdID(t, e.AppEnv, "/v1/users", AnyMap{"email": "editor@grantrole.test", "display_name": "Editor", "role": "rep"}),
	}
}

// shareContact shares the contact for editing.
func (e roleShareEnv) shareContact(t *testing.T, subjectType, subject string) limitedGrantBody {
	t.Helper()
	var got limitedGrantBody
	if status := e.Call(t, "POST", "/v1/record-grants", AnyMap{
		"record_type": "contact", "record_id": e.contactID,
		"subject_type": subjectType, "subject_id": subject, "access": "write",
	}, nil, &got); status != http.StatusCreated {
		t.Fatalf("share write with %s %s → %d, want 201", subjectType, subject, status)
	}
	return got
}

// limitedShareAudits counts the record_share rows that say a write share to
// the subject was cut to read by their role.
func (e roleShareEnv) limitedShareAudits(t *testing.T, subject string) int {
	t.Helper()
	var n int
	if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM audit_log
		WHERE action = 'record_share' AND after->>'subject_id' = $1 AND after->>'access' = 'read'
		  AND evidence->>'requested_access' = 'write' AND evidence->>'limited_by_role' = 'true'`, subject).Scan(&n); err != nil {
		t.Fatalf("counting the limited share audit rows: %v", err)
	}
	return n
}

func TestAnEditShareToAColleagueWhoseRoleCannotEditIsSavedAsViewOnly(t *testing.T) {
	t.Parallel()
	e := setupRoleShare(t)

	limited := e.shareContact(t, "user", e.viewer)
	if limited.Access != "read" || limited.LimitedByRole == nil || !*limited.LimitedByRole {
		t.Fatalf("edit share to a read_only role answered %+v, want access read and limited_by_role true", limited)
	}
	if n := e.limitedShareAudits(t, e.viewer); n != 1 {
		t.Fatalf("%d audit rows record the cut, want 1", n)
	}

	full := e.shareContact(t, "user", e.editor)
	if full.Access != "write" || full.LimitedByRole != nil {
		t.Fatalf("edit share to a rep answered %+v, want access write and no limited_by_role", full)
	}
}

// Attaching an activity uses a write share. So a role that may log activities
// but not change contacts keeps write, and can attach.
func TestAnEditShareToARoleThatOnlyLogsActivitiesKeepsWrite(t *testing.T) {
	t.Parallel()
	e := setupRoleShare(t)
	var role struct {
		Key string `json:"key"`
	}
	mustCall(t, e.AppEnv, "POST", "/v1/roles", AnyMap{"copy_from": "rep", "name": "Logs only"}, http.StatusCreated, &role)
	mustCall(t, e.AppEnv, "PATCH", "/v1/roles/"+role.Key+"/objects/contact",
		AnyMap{"create": false, "read": true, "update": false, "delete": false}, http.StatusOK, nil)
	const email, password = "logger@grantrole.test", "grantrole-logger-password"
	logger := createdID(t, e.AppEnv, "/v1/users", AnyMap{"email": email, "display_name": "Logger", "role": role.Key})

	got := e.shareContact(t, "user", logger)
	if got.Access != "write" || got.LimitedByRole != nil {
		t.Fatalf("edit share to a role that logs activities answered %+v, want access write", got)
	}
	apptest.SetPasswordByLink(t, e.AppEnv, logger, password)
	if status := e.AsSecondSeat(t, email, password).Call(t, "POST", "/v1/activities", AnyMap{
		"kind": "note", "subject": "Called about the renewal",
		"links": []AnyMap{{"entity_type": "contact", "entity_id": e.contactID}},
	}, nil, nil); status != http.StatusCreated {
		t.Fatalf("logging a note on the shared contact → %d, want 201", status)
	}
}

// A team's members hold different roles, so a team keeps the edit it was given.
func TestAnEditShareToATeamIsNeverLimitedByRole(t *testing.T) {
	t.Parallel()
	e := setupRoleShare(t)
	team := createdID(t, e.AppEnv, "/v1/teams", AnyMap{"name": "Mixed roles"})
	for _, member := range []string{e.viewer, e.editor} {
		if status := e.Call(t, http.MethodPut, "/v1/teams/"+team+"/members/"+member, nil, nil, nil); status >= http.StatusBadRequest {
			t.Fatalf("adding %s to the team → %d", member, status)
		}
	}

	got := e.shareContact(t, "team", team)
	if got.Access != "write" || got.LimitedByRole != nil {
		t.Fatalf("edit share to a team answered %+v, want access write and no limited_by_role", got)
	}
}

func shareCompanyContacts(t *testing.T, e roleShareEnv, subject string) *bool {
	t.Helper()
	var result struct {
		LimitedByRole *bool `json:"limited_by_role"`
	}
	if status := e.Call(t, "POST", "/v1/companies/"+e.companyID+"/share-contacts", AnyMap{
		"subject_type": "user", "subject_id": subject, "access": "write",
	}, nil, &result); status != http.StatusOK {
		t.Fatalf("share company contacts with %s → %d, want 200", subject, status)
	}
	return result.LimitedByRole
}

func TestSharingCompanyContactsForEditWithAViewOnlyRoleSaysSo(t *testing.T) {
	t.Parallel()
	e := setupRoleShare(t)
	// No contact works there yet: no grant is written, so the answer reveals nothing.
	if limited := shareCompanyContacts(t, e, e.viewer); limited != nil {
		t.Fatalf("an empty company answered limited_by_role = %v, want it absent", *limited)
	}
	mustCall(t, e.AppEnv, "POST", "/v1/relationships", AnyMap{
		"kind": "employment", "contact_id": e.contactID, "company_id": e.companyID,
		"is_current_primary": true, "source": "manual",
	}, http.StatusCreated, nil)

	if limited := shareCompanyContacts(t, e, e.viewer); limited == nil || !*limited {
		t.Fatal("an edit share of company contacts to a read_only role did not say it was limited")
	}
	if n := e.limitedShareAudits(t, e.viewer); n != 1 {
		t.Fatalf("%d audit rows record the cut, want 1", n)
	}
	if limited := shareCompanyContacts(t, e, e.editor); limited != nil {
		t.Fatalf("company contacts shared for edit with a rep answered limited_by_role = %v", *limited)
	}
}
