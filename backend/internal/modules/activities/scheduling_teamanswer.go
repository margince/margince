// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// teamAudit is the member list as the History tab shows it.
type teamAudit struct {
	TeamMembers []crmcontracts.SchedulingTeamMember `json:"team_members"`
}

// AnswerSchedulingTeamRequest records the acting colleague's answer on the
// team link that names them. Confirming needs a calendar Margince can read.
func (s *Store) AnswerSchedulingTeamRequest(ctx context.Context, page ids.UUID, answer crmcontracts.SchedulingTeamAnswerState) error {
	me, err := schedulingHost(ctx)
	if err != nil {
		return err
	}
	if !answer.Valid() {
		return &SchedulingArgumentError{Field: fieldTeamAnswer, Code: faultInvalid, Message: "Answer confirmed or left"}
	}
	if answer == crmcontracts.SchedulingTeamAnswerStateConfirmed {
		if _, standing, err := s.memberCalendar(ctx, me); err != nil {
			return err
		} else if standing != memberReadable {
			return errCalendarNotConnected()
		}
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		stored, err := lockLivePageTx(ctx, tx, page)
		if err != nil {
			return err
		}
		if stored == nil {
			return apperrors.ErrNotFound
		}
		if err := writeMemberStateTx(ctx, tx, page, stored, me, crmcontracts.SchedulingTeamMemberState(answer)); err != nil || s.teamRequests == nil {
			return err
		}
		return s.teamRequests.TeamRequestSettledTx(ctx, tx, page, me)
	})
}

// lockLivePageTx takes a live page's host lock, then the page row, and answers
// its policy. The host lock comes first on every path that takes both.
func lockLivePageTx(ctx context.Context, tx pgx.Tx, page ids.UUID) (*storedSchedulingProfile, error) {
	var host ids.UserID
	args := schedulingArgs{}
	err := tx.QueryRow(ctx, `SELECT host_user_id FROM booking_page WHERE id=`+args.add(page)+` AND revoked_at IS NULL`, args...).Scan(&host)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := storekit.LockWriteIdentity(ctx, tx, "booking_page", host.String()); err != nil {
		return nil, err
	}
	var stored *storedSchedulingProfile
	args = schedulingArgs{}
	err = tx.QueryRow(ctx, `SELECT scheduling_policy FROM booking_page WHERE `+livePageSQL(&args, host, page)+` FOR UPDATE`, args...).Scan(&stored)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	return stored, err
}

// RetireTeamMemberTx marks a departing colleague unavailable on every live
// booking link that names them. Each such link stays paused, and its owner
// sees why on the link, until they remove the colleague.
func RetireTeamMemberTx(ctx context.Context, tx pgx.Tx, member ids.UserID) error {
	args := schedulingArgs{}
	rows, err := tx.Query(ctx, `SELECT id, host_user_id FROM booking_page WHERE revoked_at IS NULL
 AND scheduling_policy->'team_members' @> jsonb_build_array(jsonb_build_object('user_id', `+args.add(member.String())+`::text))
 ORDER BY host_user_id`, args...)
	if err != nil {
		return err
	}
	type namingPage struct {
		ID   ids.UUID
		Host ids.UserID
	}
	pages, err := pgx.CollectRows(rows, pgx.RowToStructByPos[namingPage])
	if err != nil {
		return err
	}
	for _, page := range pages {
		if err := storekit.LockWriteIdentity(ctx, tx, "booking_page", page.Host.String()); err != nil {
			return err
		}
		var stored *storedSchedulingProfile
		args := schedulingArgs{}
		if err := tx.QueryRow(ctx, `SELECT scheduling_policy FROM booking_page WHERE id=`+args.add(page.ID)+` FOR UPDATE`, args...).Scan(&stored); err != nil {
			return err
		}
		if stored == nil {
			continue
		}
		if err := writeMemberStateTx(ctx, tx, page.ID, stored, member, crmcontracts.SchedulingTeamMemberStateUnavailable); err != nil && !errors.Is(err, apperrors.ErrNotFound) {
			return err
		}
	}
	return nil
}

// writeMemberStateTx writes a member's own state on a locked page. The
// member's answer and their departure share it.
func writeMemberStateTx(ctx context.Context, tx pgx.Tx, page ids.UUID, stored *storedSchedulingProfile, member ids.UserID, state crmcontracts.SchedulingTeamMemberState) error {
	before := slices.Clone(teamMembers(stored.SchedulingProfile))
	i := slices.IndexFunc(before, func(m crmcontracts.SchedulingTeamMember) bool { return memberUser(m) == member })
	if i < 0 {
		return apperrors.ErrNotFound
	}
	if memberState(before[i]) == state {
		return nil
	}
	after := slices.Clone(before)
	after[i].State = &state
	stored.TeamMembers = &after
	args := schedulingArgs{}
	if _, err := tx.Exec(ctx, `UPDATE booking_page SET scheduling_policy=`+args.add(stored)+` WHERE id=`+args.add(page), args...); err != nil {
		return err
	}
	audit, err := storekit.Audit(ctx, tx, "update", "booking_page", page, teamAudit{before}, teamAudit{after})
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, audit, page, crmcontracts.InternalEventBookingPageUpdated{Enabled: stored.Enabled})
}
