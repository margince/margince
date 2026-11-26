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
	me, err := s.admitTeamAnswer(ctx, answer)
	if err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error { return s.answerTeamLinkTx(ctx, tx, page, me, answer) })
}

// AnswerHostTeamRequests records the acting colleague's answer on every live
// team link of host that names them and that the answer changes. It serves
// clients that answer a host rather than a link. A repeat succeeds, as on one
// link. It is not found when no live link holds the caller in a state this
// answer moves or repeats. So a confirm on a link they left is not found.
func (s *Store) AnswerHostTeamRequests(ctx context.Context, host ids.UserID, answer crmcontracts.SchedulingTeamAnswerState) error {
	me, err := s.admitTeamAnswer(ctx, answer)
	if err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		// The host lock comes before the read, so the links are judged as they
		// stand once no other answer on them can commit.
		if err := storekit.LockWriteIdentity(ctx, tx, "booking_page", host.String()); err != nil {
			return err
		}
		args := schedulingArgs{}
		rows, err := tx.Query(ctx, `SELECT id, scheduling_policy FROM booking_page WHERE host_user_id=`+args.add(host)+` AND revoked_at IS NULL
 AND scheduling_policy->'team_members' @> jsonb_build_array(jsonb_build_object('user_id', `+args.add(me.String())+`::text))
 ORDER BY created_at, id FOR UPDATE`, args...)
		if err != nil {
			return err
		}
		type namingPage struct {
			ID     ids.UUID
			Policy storedSchedulingProfile
		}
		pages, err := pgx.CollectRows(rows, pgx.RowToStructByPos[namingPage])
		if err != nil {
			return err
		}
		reached := 0
		for _, page := range pages {
			switch {
			case answerChanges(page.Policy.SchedulingProfile, me, answer):
				if err := s.writeTeamAnswerTx(ctx, tx, page.ID, &page.Policy, me, answer); err != nil {
					return err
				}
				reached++
			case answerRepeats(page.Policy.SchedulingProfile, me, answer):
				reached++
			}
		}
		if reached == 0 {
			return apperrors.ErrNotFound
		}
		return nil
	})
}

// answerRepeats is whether member already stands where answer would put them.
func answerRepeats(profile crmcontracts.SchedulingProfile, member ids.UserID, answer crmcontracts.SchedulingTeamAnswerState) bool {
	for _, m := range teamMembers(profile) {
		if memberUser(m) == member {
			return memberState(m) == crmcontracts.SchedulingTeamMemberState(answer)
		}
	}
	return false
}

// answerChanges is whether answer moves member on a page: confirming a pending
// request, or leaving one not yet left.
func answerChanges(profile crmcontracts.SchedulingProfile, member ids.UserID, answer crmcontracts.SchedulingTeamAnswerState) bool {
	for _, m := range teamMembers(profile) {
		if memberUser(m) != member {
			continue
		}
		state := memberState(m)
		if answer == crmcontracts.SchedulingTeamAnswerStateConfirmed {
			return state == crmcontracts.SchedulingTeamMemberStatePending
		}
		return state != crmcontracts.SchedulingTeamMemberStateLeft
	}
	return false
}

// admitTeamAnswer is the acting colleague, when they may give answer.
func (s *Store) admitTeamAnswer(ctx context.Context, answer crmcontracts.SchedulingTeamAnswerState) (ids.UserID, error) {
	me, err := schedulingHost(ctx)
	if err != nil {
		return me, err
	}
	if !answer.Valid() {
		return me, &SchedulingArgumentError{Field: fieldTeamAnswer, Code: faultInvalid, Message: "Answer confirmed or left"}
	}
	if answer == crmcontracts.SchedulingTeamAnswerStateConfirmed {
		if _, standing, err := s.memberCalendar(ctx, me); err != nil {
			return me, err
		} else if standing != memberReadable {
			return me, errCalendarNotConnected()
		}
	}
	return me, nil
}

// answerTeamLinkTx writes one answer on one live page under its host's lock.
func (s *Store) answerTeamLinkTx(ctx context.Context, tx pgx.Tx, page ids.UUID, me ids.UserID, answer crmcontracts.SchedulingTeamAnswerState) error {
	stored, err := lockLivePageTx(ctx, tx, page)
	if err != nil {
		return err
	}
	if stored == nil {
		return apperrors.ErrNotFound
	}
	return s.writeTeamAnswerTx(ctx, tx, page, stored, me, answer)
}

// writeTeamAnswerTx writes answer on a page the caller holds the lock of, and
// settles the request it answers.
func (s *Store) writeTeamAnswerTx(ctx context.Context, tx pgx.Tx, page ids.UUID, stored *storedSchedulingProfile, me ids.UserID, answer crmcontracts.SchedulingTeamAnswerState) error {
	if err := writeMemberStateTx(ctx, tx, page, stored, me, crmcontracts.SchedulingTeamMemberState(answer)); err != nil || s.teamRequests == nil {
		return err
	}
	return s.teamRequests.TeamRequestSettledTx(ctx, tx, page, me)
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
