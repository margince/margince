// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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

func (e roleShareEnv) shareContact(t *testing.T, subjectType, subject, access string) limitedGrantBody {
	t.Helper()
	var got limitedGrantBody
	if status := e.Call(t, "POST", "/v1/record-grants", AnyMap{
		"record_type": "contact", "record_id": e.contactID,
		"subject_type": subjectType, "subject_id": subject, "access": access,
	}, nil, &got); status != http.StatusCreated {
		t.Fatalf("share %s with %s %s → %d, want 201", access, subjectType, subject, status)
	}
	return got
}

func TestAnEditShareToAColleagueWhoseRoleCannotEditIsSavedAsViewOnly(t *testing.T) {
	e := setupRoleShare(t)

	limited := e.shareContact(t, "user", e.viewer, "write")
	if limited.Access != "read" || limited.LimitedByRole == nil || !*limited.LimitedByRole {
		t.Fatalf("edit share to a read_only role answered %+v, want access read and limited_by_role true", limited)
	}
	var stored, audited, requested string
	if err := e.Owner.QueryRow(t.Context(), `SELECT g.access, a.after->>'access', a.evidence->>'requested_access'
		FROM record_grant g JOIN audit_log a ON a.action = 'record_share' AND a.after->>'subject_id' = g.subject_id::text
		WHERE g.id = $1`, limited.ID).Scan(&stored, &audited, &requested); err != nil {
		t.Fatalf("reading the saved grant and its audit row: %v", err)
	}
	if stored != "read" || audited != "read" || requested != "write" {
		t.Fatalf("stored %q, audited %q, requested %q; want read, read, write", stored, audited, requested)
	}

	full := e.shareContact(t, "user", e.editor, "write")
	if full.Access != "write" || full.LimitedByRole != nil {
		t.Fatalf("edit share to a rep answered %+v, want access write and no limited_by_role", full)
	}
}

// A team's members hold different roles, so a team keeps the edit it was given.
func TestAnEditShareToATeamIsNeverLimitedByRole(t *testing.T) {
	e := setupRoleShare(t)
	team := createdID(t, e.AppEnv, "/v1/teams", AnyMap{"name": "Mixed roles"})
	for _, member := range []string{e.viewer, e.editor} {
		if status := e.Call(t, http.MethodPut, "/v1/teams/"+team+"/members/"+member, nil, nil, nil); status >= http.StatusBadRequest {
			t.Fatalf("adding %s to the team → %d", member, status)
		}
	}

	got := e.shareContact(t, "team", team, "write")
	if got.Access != "write" || got.LimitedByRole != nil {
		t.Fatalf("edit share to a team answered %+v, want access write and no limited_by_role", got)
	}
}

func TestSharingCompanyContactsForEditWithAViewOnlyRoleSaysSo(t *testing.T) {
	e := setupRoleShare(t)
	for subject, want := range map[string]bool{e.viewer: true, e.editor: false} {
		var result struct {
			LimitedByRole bool `json:"limited_by_role"`
		}
		if status := e.Call(t, "POST", "/v1/companies/"+e.companyID+"/share-contacts", AnyMap{
			"subject_type": "user", "subject_id": subject, "access": "write",
		}, nil, &result); status != http.StatusOK {
			t.Fatalf("share company contacts with %s → %d, want 200", subject, status)
		}
		if result.LimitedByRole != want {
			t.Errorf("company contacts shared for edit with %s: limited_by_role = %v, want %v", subject, result.LimitedByRole, want)
		}
	}
}

// A share request's accept and a merge's owner carry ask ShareAccessForTx.
func TestTheWidestShareFollowsTheRole(t *testing.T) {
	e := setupRoleShare(t)
	tx, err := e.Owner.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(t.Context()); err != nil {
			t.Errorf("rolling back: %v", err)
		}
	}()
	for member, want := range map[string]string{e.viewer: "read", e.editor: "write"} {
		got, err := identity.ShareAccessForTx(t.Context(), tx, "contact", ids.MustParse(member))
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("widest share for %s = %q, want %q", member, got, want)
		}
	}
}
