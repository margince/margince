// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"
)

// An admin's agent runs the member administration its admin runs, except the
// acts that would let a credential widen what a credential may do. Each is
// refused through the route-backed action an agent calls. The same act on an
// ordinary colleague goes through, so the refusal is the limit, not a gate.
func TestAnAgentIsRefusedTheMemberActsThatWidenRights(t *testing.T) {
	d := newTwoDoors(t, "agent-member-limits", "read", "write", "send")
	d.e.DescribeCompany(t)
	self := memberID(t, d, "agent-member-limits@fable.test")
	colleague := createdID(t, d.e, "/v1/users", AnyMap{"email": "rep@agent-member-limits.test", "display_name": "Rep", "role": "rep"})
	admin := createdID(t, d.e, "/v1/users", AnyMap{"email": "boss@agent-member-limits.test", "display_name": "Boss", "role": "admin"})
	team := createdRecord(t, d.e, "/v1/teams", AnyMap{"name": "Limits"})

	for name, call := range map[string]struct {
		tool string
		args map[string]any
	}{
		"inviting an admin": {"send_outbound", map[string]any{
			"as":           "invite_member",
			"request_body": map[string]any{"email": "new@agent-member-limits.test", "display_name": "New", "role": "admin"},
		}},
		"granting the admin role": {"change_access", map[string]any{
			"action": "change_role", "id": colleague,
			"request_body": map[string]any{"role": "admin"},
		}},
		"changing its own user's role": {"change_access", map[string]any{
			"action": "change_role", "id": self,
			"request_body": map[string]any{"role": "rep"},
		}},
		"deactivating an administrator": {"change_access", map[string]any{
			"action": "deactivate_member", "id": admin,
			"request_body": map[string]any{},
		}},
		"putting its own user on a team": {"change_access", map[string]any{"action": "add_team_member", "id": team, "userId": self}},
		"handing work to its own user": {"change_access", map[string]any{
			"action": "hand_over_work", "id": colleague,
			"request_body": map[string]any{"to_user_id": self},
		}},
	} {
		refusal := d.toolRefused(t, call.tool, call.args)
		if !strings.Contains(refusal, "an agent does not") {
			t.Errorf("%s: refused with %q, want the agent limit", name, refusal)
		}
	}

	// The same act on an ordinary colleague runs: d.tool fails on any refusal.
	d.tool(t, "change_access", map[string]any{"action": "add_team_member", "id": team, "userId": colleague})
}

// Archiving a team switches off every member's reach through it, and restoring
// switches it on, so each counts as acting on every member. An agent is refused
// when its own user or an administrator is on the team, and a rename stays free.
func TestAnAgentIsRefusedArchivingOrRestoringATeamHoldingALimitedMember(t *testing.T) {
	d := newTwoDoors(t, "agent-team-limits", "read", "write")
	d.e.DescribeCompany(t)
	self := memberID(t, d, "agent-team-limits@fable.test")
	colleague := createdID(t, d.e, "/v1/users", AnyMap{"email": "rep@agent-team-limits.test", "display_name": "Rep", "role": "rep"})
	admin := createdID(t, d.e, "/v1/users", AnyMap{"email": "boss@agent-team-limits.test", "display_name": "Boss", "role": "admin"})

	teamOf := func(name string, members ...string) string {
		team := createdRecord(t, d.e, "/v1/teams", AnyMap{"name": name})
		for _, member := range members {
			if status := d.e.Call(t, http.MethodPut, "/v1/teams/"+team+"/members/"+member, nil, nil, nil); status >= http.StatusBadRequest {
				t.Fatalf("seating %s on %s answered %d", member, name, status)
			}
		}
		return team
	}
	archive := func(team string, archived bool) map[string]any {
		return map[string]any{"action": "update_team", "id": team, "request_body": map[string]any{"archived": archived}}
	}

	for name, team := range map[string]string{
		"its own user's team":     teamOf("Own", self),
		"an administrator's team": teamOf("Admins", admin),
	} {
		if refusal := d.toolRefused(t, "change_access", archive(team, true)); !strings.Contains(refusal, "an agent does not") {
			t.Errorf("archiving %s: refused with %q, want the agent limit", name, refusal)
		}
		if status := d.e.Call(t, http.MethodPatch, "/v1/teams/"+team, AnyMap{"archived": true}, nil, nil); status != http.StatusOK {
			t.Fatalf("the admin archiving %s answered %d", name, status)
		}
		if refusal := d.toolRefused(t, "change_access", archive(team, false)); !strings.Contains(refusal, "an agent does not") {
			t.Errorf("restoring %s: refused with %q, want the agent limit", name, refusal)
		}
	}

	// A team of ordinary colleagues archives and restores, and any team renames.
	ordinary := teamOf("Ordinary", colleague)
	d.tool(t, "change_access", archive(ordinary, true))
	d.tool(t, "change_access", archive(ordinary, false))
	own := teamOf("Mine", self)
	d.tool(t, "change_access", map[string]any{"action": "update_team", "id": own, "request_body": map[string]any{"name": "Mine renamed"}})
}

// memberID is the roster id of the member signed in with email.
func memberID(t *testing.T, d *twoDoors, email string) string {
	t.Helper()
	var roster any
	if status := d.e.Call(t, http.MethodGet, "/v1/users", nil, nil, &roster); status != http.StatusOK {
		t.Fatalf("reading the roster answered %d", status)
	}
	if id := idOfEmail(roster, email); id != "" {
		return id
	}
	t.Fatalf("no member %s on the roster", email)
	return ""
}

// idOfEmail finds the object carrying email and answers its id.
//
//craft:ignore naked-any the roster is walked as decoded JSON, whatever its page shape
func idOfEmail(node any, email string) string {
	switch v := node.(type) {
	case map[string]any:
		if v["email"] == email {
			id, _ := v["id"].(string)
			return id
		}
		for _, member := range v {
			if id := idOfEmail(member, email); id != "" {
				return id
			}
		}
	case []any:
		for _, member := range v {
			if id := idOfEmail(member, email); id != "" {
				return id
			}
		}
	}
	return ""
}
