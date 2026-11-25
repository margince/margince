// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// SchedulingTeamCandidates are the colleagues the acting host may name on
// their booking link: live colleagues whose calendar Margince can read.
func (s *Store) SchedulingTeamCandidates(ctx context.Context) ([]crmcontracts.SchedulingTeamCandidate, error) {
	me, err := schedulingHost(ctx)
	if err != nil {
		return nil, err
	}
	var colleagues []ids.UserID
	err = s.tx(ctx, func(tx pgx.Tx) error {
		args := schedulingArgs{}
		// Only a colleague whose personal page names a calendar provider can be
		// read, so the provider check below runs for those alone.
		rows, err := tx.Query(ctx, `SELECT u.id FROM app_user u WHERE u.id<>`+args.add(me)+` AND `+workforce.LiveHumanSQL("u")+`
 AND EXISTS (SELECT 1 FROM booking_page p WHERE p.host_user_id=u.id AND p.revoked_at IS NULL AND NOT p.team_link
   AND coalesce(p.scheduling_policy->>'provider','')<>'')
 ORDER BY u.display_name, u.id`, args...)
		if err != nil {
			return err
		}
		colleagues, err = pgx.CollectRows(rows, pgx.RowTo[ids.UserID])
		return err
	})
	if err != nil {
		return nil, err
	}
	out := []crmcontracts.SchedulingTeamCandidate{}
	for _, colleague := range colleagues {
		calendar, standing, err := s.memberCalendar(ctx, colleague)
		if err != nil {
			return nil, err
		}
		if standing == memberReadable && calendar.Profile.HostName != nil {
			out = append(out, crmcontracts.SchedulingTeamCandidate{UserId: openapi_types.UUID(colleague.UUID), Name: *calendar.Profile.HostName})
		}
	}
	return out, nil
}

// SchedulingTeamRequests are the live team links that name the acting
// colleague and that they have not left, one row per link.
func (s *Store) SchedulingTeamRequests(ctx context.Context) ([]crmcontracts.SchedulingTeamRequest, error) {
	me, err := schedulingHost(ctx)
	if err != nil {
		return nil, err
	}
	var pages []BookingPage
	err = s.tx(ctx, func(tx pgx.Tx) error {
		args := schedulingArgs{}
		rows, err := tx.Query(ctx, `SELECT p.host_user_id, p.id, p.team_link FROM booking_page p JOIN app_user u ON u.id=p.host_user_id
 WHERE p.revoked_at IS NULL AND `+workforce.LiveMemberSQL("u")+`
 AND p.scheduling_policy->'team_members' @> jsonb_build_array(jsonb_build_object('user_id', `+args.add(me.String())+`::text))
 ORDER BY u.display_name, u.id, p.created_at, p.id`, args...)
		if err != nil {
			return err
		}
		pages, err = pgx.CollectRows(rows, pgx.RowToStructByPos[BookingPage])
		return err
	})
	if err != nil {
		return nil, err
	}
	out := []crmcontracts.SchedulingTeamRequest{}
	for _, page := range pages {
		profile, err := s.bookingPageProfile(ctx, page)
		if err != nil {
			return nil, err
		}
		for _, m := range teamMembers(profile) {
			state := memberState(m)
			if memberUser(m) != me || state == crmcontracts.SchedulingTeamMemberStateLeft {
				continue
			}
			request := crmcontracts.SchedulingTeamRequest{
				PageId: openapi_types.UUID(page.ID), HostUserId: openapi_types.UUID(page.HostUserID.UUID), Title: profile.Title,
				State: crmcontracts.SchedulingTeamRequestState(state),
			}
			if profile.HostName != nil {
				request.HostName = *profile.HostName
			}
			out = append(out, request)
		}
	}
	return out, nil
}

// ListSchedulingTeamCandidates implements GET /scheduling/team-candidates.
func (h Handlers) ListSchedulingTeamCandidates(w http.ResponseWriter, r *http.Request) {
	out, err := h.store.SchedulingTeamCandidates(r.Context())
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// ListSchedulingTeamRequests implements GET /scheduling/team-requests.
func (h Handlers) ListSchedulingTeamRequests(w http.ResponseWriter, r *http.Request) {
	out, err := h.store.SchedulingTeamRequests(r.Context())
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// AnswerSchedulingTeamLink implements PUT /scheduling/team-links/{page_id}/answer.
func (h Handlers) AnswerSchedulingTeamLink(w http.ResponseWriter, r *http.Request, pageID openapi_types.UUID) {
	var req crmcontracts.SchedulingTeamAnswer
	if !httperr.Decode(w, r, &req) {
		return
	}
	if err := h.store.AnswerSchedulingTeamRequest(r.Context(), ids.UUID(pageID), req.State); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AnswerSchedulingTeamRequest implements the deprecated PUT
// /scheduling/team-requests/{host_user_id}, which answers each team link of a host.
func (h Handlers) AnswerSchedulingTeamRequest(w http.ResponseWriter, r *http.Request, hostUserID openapi_types.UUID) {
	var req crmcontracts.SchedulingTeamAnswer
	if !httperr.Decode(w, r, &req) {
		return
	}
	if err := h.store.AnswerHostTeamRequests(r.Context(), ids.From[ids.UserKind](ids.UUID(hostUserID)), req.State); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
