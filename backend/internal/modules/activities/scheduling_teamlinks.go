// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// A host's booking pages: one personal page, which offers the host's own free
// time alone, and any number of team links beside it. A team link keeps only
// its name, its switch and its team; the calendar and every other booking
// setting come from the host's personal page.

import (
	"context"
	"crypto/rand"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// livePageSQL names one live booking page of host: page, or the host's
// personal page when page is Nil. Every read of one page goes through it.
func livePageSQL(args *schedulingArgs, host ids.UserID, page ids.UUID) string {
	where := `host_user_id=` + args.add(host) + ` AND revoked_at IS NULL AND `
	if page == ids.Nil {
		return where + `NOT team_link`
	}
	return where + `id=` + args.add(page)
}

// teamLinkRow is one live team link as stored.
type teamLinkRow struct {
	ID     ids.UUID
	Slug   string
	Policy storedSchedulingProfile
}

func readTeamLinkTx(ctx context.Context, tx pgx.Tx, host ids.UserID, page ids.UUID, lock bool) (teamLinkRow, error) {
	args := schedulingArgs{}
	query := `SELECT id, slug, scheduling_policy FROM booking_page WHERE ` + livePageSQL(&args, host, page) + ` AND team_link`
	if lock {
		query += ` FOR UPDATE`
	}
	var link teamLinkRow
	err := tx.QueryRow(ctx, query, args...).Scan(&link.ID, &link.Slug, &link.Policy)
	if errors.Is(err, pgx.ErrNoRows) {
		return link, apperrors.ErrNotFound
	}
	return link, err
}

// teamLinkProfile is a team link as its guests and its host read it.
func (s *Store) teamLinkProfile(personal crmcontracts.SchedulingProfile, link teamLinkRow) crmcontracts.SchedulingProfile {
	profile := personal
	profile.Title, profile.Enabled, profile.TeamMembers = link.Policy.Title, link.Policy.Enabled, link.Policy.TeamMembers
	profile.UseDefaultTitle = new(false)
	profile.Slug, profile.Alias, profile.PublicUrl = &link.Slug, nil, nil
	if address, ok := s.publicBookingURL(link.Slug); ok {
		profile.PublicUrl = &address
	}
	return profile
}

// bookingPageProfile is the settings of the page a slug resolved to.
func (s *Store) bookingPageProfile(ctx context.Context, page BookingPage) (crmcontracts.SchedulingProfile, error) {
	profile, err := s.hostSchedulingProfile(ctx, page.HostUserID)
	if err != nil || !page.TeamLink {
		return profile, err
	}
	var link teamLinkRow
	err = s.tx(ctx, func(tx pgx.Tx) error {
		link, err = readTeamLinkTx(ctx, tx, page.HostUserID, page.ID, false)
		return err
	})
	if err != nil {
		return profile, err
	}
	return s.teamLinkProfile(profile, link), nil
}

// teamLinkView is a team link as its host sees it in Settings.
func (s *Store) teamLinkView(ctx context.Context, host ids.UserID, personal crmcontracts.SchedulingProfile, link teamLinkRow) (crmcontracts.SchedulingTeamLink, error) {
	profile := s.teamLinkProfile(personal, link)
	blocker, err := s.pageBlocker(ctx, host, profile)
	if err != nil {
		return crmcontracts.SchedulingTeamLink{}, err
	}
	if err := s.describeTeam(ctx, &profile); err != nil {
		return crmcontracts.SchedulingTeamLink{}, err
	}
	return crmcontracts.SchedulingTeamLink{
		Id: openapi_types.UUID(link.ID), Title: profile.Title, Enabled: profile.Enabled, TeamMembers: teamMembers(profile),
		Slug: link.Slug, PublicUrl: profile.PublicUrl, Blocker: crmcontracts.SchedulingTeamLinkBlocker(blocker),
	}, nil
}

// SchedulingTeamLinks lists the acting host's live team links, oldest first.
func (s *Store) SchedulingTeamLinks(ctx context.Context) ([]crmcontracts.SchedulingTeamLink, error) {
	host, err := schedulingHost(ctx)
	if err != nil {
		return nil, err
	}
	var links []teamLinkRow
	err = s.tx(ctx, func(tx pgx.Tx) error {
		args := schedulingArgs{}
		rows, err := tx.Query(ctx, `SELECT id, slug, scheduling_policy FROM booking_page WHERE host_user_id=`+args.add(host)+
			` AND revoked_at IS NULL AND team_link ORDER BY created_at, id`, args...)
		if err != nil {
			return err
		}
		links, err = pgx.CollectRows(rows, pgx.RowToStructByPos[teamLinkRow])
		return err
	})
	if err != nil {
		return nil, err
	}
	personal, err := s.hostSchedulingProfile(ctx, host)
	if err != nil {
		return nil, err
	}
	out := make([]crmcontracts.SchedulingTeamLink, 0, len(links))
	for _, link := range links {
		view, err := s.teamLinkView(ctx, host, personal, link)
		if err != nil {
			return nil, err
		}
		out = append(out, view)
	}
	return out, nil
}

func (s *Store) ownTeamLink(ctx context.Context, host ids.UserID, page ids.UUID) (crmcontracts.SchedulingTeamLink, error) {
	var link teamLinkRow
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		link, err = readTeamLinkTx(ctx, tx, host, page, false)
		return err
	})
	if err != nil {
		return crmcontracts.SchedulingTeamLink{}, err
	}
	personal, err := s.hostSchedulingProfile(ctx, host)
	if err != nil {
		return crmcontracts.SchedulingTeamLink{}, err
	}
	return s.teamLinkView(ctx, host, personal, link)
}

// teamLinkSettings is what a request may set on a team link. Only each
// member's user_id is read: their answer is theirs to give.
func teamLinkSettings(req crmcontracts.SchedulingTeamLinkRequest) (crmcontracts.SchedulingProfile, error) {
	title := strings.TrimSpace(req.Title)
	if err := validateSchedulingTitle(title); err != nil {
		return crmcontracts.SchedulingProfile{}, err
	}
	team := make([]crmcontracts.SchedulingTeamMember, 0, len(req.TeamMembers))
	for _, member := range req.TeamMembers {
		team = append(team, crmcontracts.SchedulingTeamMember{UserId: member.UserId})
	}
	return crmcontracts.SchedulingProfile{Title: title, Enabled: req.Enabled == nil || *req.Enabled, TeamMembers: &team}, nil
}

// CreateSchedulingTeamLink gives the acting host a new team link with its own
// address. The link exists only because its host made it, so it counts as used.
func (s *Store) CreateSchedulingTeamLink(ctx context.Context, req crmcontracts.SchedulingTeamLinkRequest) (crmcontracts.SchedulingTeamLink, error) {
	host, err := schedulingHost(ctx)
	if err != nil {
		return crmcontracts.SchedulingTeamLink{}, err
	}
	next, err := teamLinkSettings(req)
	if err != nil {
		return crmcontracts.SchedulingTeamLink{}, err
	}
	if err := s.admitTeam(ctx, host, crmcontracts.SchedulingProfile{}, next); err != nil {
		return crmcontracts.SchedulingTeamLink{}, err
	}
	var page ids.UUID
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := storekit.LockWriteIdentity(ctx, tx, "booking_page", host.String()); err != nil {
			return err
		}
		slug, err := mintBookingSlug(rand.Read)
		if err != nil {
			return err
		}
		next.TeamMembers = mergeTeam(nil, next.TeamMembers)
		stored := storedSchedulingProfile{SchedulingProfile: next}
		args := schedulingArgs{}
		if err := tx.QueryRow(ctx, `INSERT INTO booking_page (host_user_id, slug, team_link, scheduling_policy, first_used_at) VALUES (`+
			args.add(host)+`, `+args.add(slug)+`, true, `+args.add(stored)+`, `+args.add(s.now())+`) RETURNING id`, args...).Scan(&page); err != nil {
			return err
		}
		if err := s.tellTeamChanges(ctx, tx, host, page, nil, next); err != nil {
			return err
		}
		audit, err := storekit.AuditEvent(ctx, tx, "create", "booking_page", page, stored)
		if err != nil {
			return err
		}
		return storekit.EmitEvent(ctx, tx, audit, page, crmcontracts.InternalEventBookingPageUpdated{Enabled: next.Enabled})
	})
	if err != nil {
		return crmcontracts.SchedulingTeamLink{}, err
	}
	return s.ownTeamLink(ctx, host, page)
}

// UpdateSchedulingTeamLink renames, switches or re-teams one of the acting
// host's team links. A member's answer is theirs: a kept member keeps it.
func (s *Store) UpdateSchedulingTeamLink(ctx context.Context, page ids.UUID, req crmcontracts.SchedulingTeamLinkRequest) (crmcontracts.SchedulingTeamLink, error) {
	host, err := schedulingHost(ctx)
	if err != nil {
		return crmcontracts.SchedulingTeamLink{}, err
	}
	next, err := teamLinkSettings(req)
	if err != nil {
		return crmcontracts.SchedulingTeamLink{}, err
	}
	current, err := s.ownTeamLink(ctx, host, page)
	if err != nil {
		return current, err
	}
	if err := s.admitTeam(ctx, host, crmcontracts.SchedulingProfile{TeamMembers: &current.TeamMembers}, next); err != nil {
		return current, err
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := storekit.LockWriteIdentity(ctx, tx, "booking_page", host.String()); err != nil {
			return err
		}
		link, err := readTeamLinkTx(ctx, tx, host, page, true)
		if err != nil {
			return err
		}
		before := link.Policy
		next.TeamMembers = mergeTeam(&before, next.TeamMembers)
		after := storedSchedulingProfile{SchedulingProfile: next}
		args := schedulingArgs{}
		if _, err := tx.Exec(ctx, `UPDATE booking_page SET scheduling_policy=`+args.add(after)+` WHERE id=`+args.add(page), args...); err != nil {
			return err
		}
		if err := s.tellTeamChanges(ctx, tx, host, page, &before, next); err != nil {
			return err
		}
		audit, err := storekit.Audit(ctx, tx, "update", "booking_page", page, before, after)
		if err != nil {
			return err
		}
		return storekit.EmitEvent(ctx, tx, audit, page, crmcontracts.InternalEventBookingPageUpdated{Enabled: next.Enabled})
	})
	if err != nil {
		return crmcontracts.SchedulingTeamLink{}, err
	}
	return s.ownTeamLink(ctx, host, page)
}

// teamLinkRevocation is the audited value of a team link's deletion.
type teamLinkRevocation struct {
	Revoked bool `json:"revoked"`
}

// DeleteSchedulingTeamLink revokes one of the acting host's team links and
// settles each member's request. The host's personal page is not touched, and
// meetings booked through the link stand.
func (s *Store) DeleteSchedulingTeamLink(ctx context.Context, page ids.UUID) error {
	host, err := schedulingHost(ctx)
	if err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		if err := storekit.LockWriteIdentity(ctx, tx, "booking_page", host.String()); err != nil {
			return err
		}
		link, err := readTeamLinkTx(ctx, tx, host, page, true)
		if err != nil {
			return err
		}
		args := schedulingArgs{}
		if _, err := tx.Exec(ctx, `UPDATE booking_page SET revoked_at=`+args.add(s.now())+` WHERE id=`+args.add(page), args...); err != nil {
			return err
		}
		if err := s.tellTeamChanges(ctx, tx, host, page, &link.Policy, crmcontracts.SchedulingProfile{}); err != nil {
			return err
		}
		audit, err := storekit.Audit(ctx, tx, "revoke", "booking_page", page, teamLinkRevocation{}, teamLinkRevocation{Revoked: true})
		if err != nil {
			return err
		}
		return storekit.EmitEvent(ctx, tx, audit, page, crmcontracts.InternalEventBookingPageUpdated{Enabled: false})
	})
}

// ListSchedulingTeamLinks implements GET /scheduling/team-links.
func (h Handlers) ListSchedulingTeamLinks(w http.ResponseWriter, r *http.Request) {
	out, err := h.store.SchedulingTeamLinks(r.Context())
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// CreateSchedulingTeamLink implements POST /scheduling/team-links.
func (h Handlers) CreateSchedulingTeamLink(w http.ResponseWriter, r *http.Request) {
	var req crmcontracts.SchedulingTeamLinkRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	out, err := h.store.CreateSchedulingTeamLink(r.Context(), req)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, out)
}

// UpdateSchedulingTeamLink implements PUT /scheduling/team-links/{page_id}.
func (h Handlers) UpdateSchedulingTeamLink(w http.ResponseWriter, r *http.Request, pageID openapi_types.UUID) {
	var req crmcontracts.SchedulingTeamLinkRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	out, err := h.store.UpdateSchedulingTeamLink(r.Context(), ids.UUID(pageID), req)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// DeleteSchedulingTeamLink implements DELETE /scheduling/team-links/{page_id}.
func (h Handlers) DeleteSchedulingTeamLink(w http.ResponseWriter, r *http.Request, pageID openapi_types.UUID) {
	if err := h.store.DeleteSchedulingTeamLink(r.Context(), ids.UUID(pageID)); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
