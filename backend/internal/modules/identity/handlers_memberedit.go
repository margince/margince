// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// WithMemberWork supplies cross-module retirement through the composition boundary.
func (h Handlers) WithMemberWork(work MemberWork) Handlers {
	if h.svc != nil {
		h.svc.WithMemberWork(work)
	}
	return h
}

// UpdateMember applies profile, role and team edits in one transaction.
func (h Handlers) UpdateMember(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	var req crmcontracts.UpdateMemberRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	var teams *[]ids.UUID
	if req.TeamIds != nil {
		selected := make([]ids.UUID, len(*req.TeamIds))
		for i, team := range *req.TeamIds {
			selected[i] = ids.UUID(team)
		}
		teams = &selected
	}

	err := h.svc.UpdateMember(r.Context(), actor, ids.From[ids.UserKind](ids.UUID(id)), MemberEdit{
		Name: req.DisplayName, Email: string(req.Email), Role: req.Role, Teams: teams, ExpectedUpdatedAt: req.ExpectedUpdatedAt,
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RemoveMember requires a reviewed dependency snapshot before permanent removal.
func (h Handlers) RemoveMember(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	var req crmcontracts.RemoveMemberRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	var target *ids.UUID
	if req.ToUserId != nil {
		value := ids.UUID(*req.ToUserId)
		target = &value
	}
	if err := h.svc.RetireMember(r.Context(), actor, ids.From[ids.UserKind](ids.UUID(id)), target, req.PreviewToken, false); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetMember returns the current revision for an administrator editing an account.
func (h Handlers) GetMember(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	h.writeUserByID(w, r, actor, ids.From[ids.UserKind](ids.UUID(id)), http.StatusOK)
}
