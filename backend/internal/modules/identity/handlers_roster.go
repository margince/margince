// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// ListUsers serves one keyset page of the workspace member roster.
func (h Handlers) ListUsers(w http.ResponseWriter, r *http.Request, params crmcontracts.ListUsersParams) {
	// The caller goes on the context; the SERVICE decides what they may see and
	// says which view it served. This handler renders that answer and does not
	// take its own — a second evaluation here disagreed with the service for an
	// agent request, which reaches the handler with a kernel principal and no
	// human Identity to ask about.
	if actor, ok := identityFrom(r.Context()); ok {
		r = r.WithContext(actorCtx(r.Context(), actor))
	}
	roster, err := h.svc.ListUsers(r.Context(), ListUsersInput{
		Q:               params.Q,
		Cursor:          params.Cursor,
		Limit:           params.Limit,
		IncludeInactive: params.IncludeInactive != nil && *params.IncludeInactive,
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	// Same URL, different body per caller: a member administrator's page carries
	// role keys and the widened status view, a rep's carries neither. A shared
	// cache keyed on the URL alone would serve one of them the other's answer.
	w.Header().Set("Cache-Control", "private, no-store")
	wire := rosterUserMapping(roster.Management)
	data := make([]crmcontracts.User, 0, len(roster.Users))
	for _, u := range roster.Users {
		data = append(data, wire(u))
	}
	httperr.WriteJSON(w, http.StatusOK,
		crmcontracts.UserListResponse{Data: data, Page: pageInfo(roster.Page)})
}

// ListTeams serves one keyset page of the workspace teams with their
// active member count.
func (h Handlers) ListTeams(w http.ResponseWriter, r *http.Request, params crmcontracts.ListTeamsParams) {
	rows, page, err := h.svc.ListTeams(r.Context(), ListTeamsInput{
		Q:      params.Q,
		Cursor: params.Cursor,
		Limit:  params.Limit,
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	data := make([]crmcontracts.Team, 0, len(rows))
	for _, tm := range rows {
		data = append(data, wireTeam(tm))
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.TeamListResponse{Data: data, Page: pageInfo(page)})
}

// pageInfo renders the store's keyset page onto the contract's PageInfo
// envelope — this module's own copy of the one-per-module spelling
// (contacts/deals/activities/signals each carry their own).
func pageInfo(p storekit.Page) crmcontracts.PageInfo {
	info := crmcontracts.PageInfo{HasMore: p.HasMore}
	if p.NextCursor != "" {
		info.NextCursor = &p.NextCursor
	}
	return info
}

// wireUser maps a roster row to the contract User. workspace_id is
// required on User; no credential column ever leaves the store — userRow
// carries none, so none can leak here. Role keys are deliberately absent:
// the roster answers every authenticated member, and only wireUserWithRoles
// adds them.
func wireUser(u userRow) crmcontracts.User {
	created := u.CreatedAt
	return crmcontracts.User{
		Id:          openapi_types.UUID(u.ID),
		Email:       openapi_types.Email(u.Email),
		DisplayName: u.DisplayName,
		Status:      crmcontracts.UserStatus(u.Status),
		IsAgent:     u.IsAgent,
		CreatedAt:   &created,
	}
}

// rosterUserMapping picks the User mapping this caller may see. Role keys ride
// the admin's roster only — an admin is the only caller who can act on a role,
// and the share/assignee pickers every other member reads this roster for have
// no use for who holds `admin`. Named rather than inlined so the disclosure
// decision is one testable thing instead of a branch inside a loop.
func rosterUserMapping(isAdmin bool) func(userRow) crmcontracts.User {
	if isAdmin {
		return wireUserWithRoles
	}
	return wireUser
}

// wireUserWithRoles is the admin view of a user: wireUser plus the role keys
// and team memberships the admin card renders and acts on. Splitting it from
// wireUser makes the disclosure gate structural — a caller that has not been
// checked for admin cannot reach either by forgetting a flag.
//
// A row whose read did not ask for a field (nil, not empty) is answered
// WITHOUT it rather than with an empty one: "[]" on the wire means the user
// holds no role and is in no team, and claiming that about someone who may
// hold several is worse than saying nothing.
func wireUserWithRoles(u userRow) crmcontracts.User {
	wire := wireUser(u)
	if u.Roles != nil {
		roles := u.Roles
		wire.Roles = &roles
	}
	if u.TeamIDs != nil {
		teams := make([]openapi_types.UUID, 0, len(u.TeamIDs))
		for _, id := range u.TeamIDs {
			teams = append(teams, openapi_types.UUID(id))
		}
		wire.TeamIds = &teams
	}
	return wire
}

// wireTeam maps a roster row to the contract Team, setting the optional
// member_count the roster read populates.
func wireTeam(tm teamRow) crmcontracts.Team {
	created := tm.CreatedAt
	count := tm.MemberCount
	return crmcontracts.Team{
		Id:          openapi_types.UUID(tm.ID),
		Name:        tm.Name,
		MemberCount: &count,
		CreatedAt:   &created,
	}
}
