// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// A host's calendar that refuses Margince parks their whole queue.
//
// Retrying each invitation five times against a calendar that has said no only
// spends the attempts and the host's patience. The first refusal parks the
// finishing invitation. It also parks every queued one of the same host and
// calendar that no worker holds. A held one parks itself when its attempt ends. All
// of them share one episode, so the host is told once. Each delivery pass first
// resumes the rows whose calendar can take bookings again.

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const (
	attentionDeliveryFailed       = string(crmcontracts.MeetingInvitationAttentionReasonDeliveryFailed)
	attentionCalendarDisconnected = string(crmcontracts.MeetingInvitationAttentionReasonCalendarDisconnected)
)

// InvitationAttention tells a host that an invitation of theirs waits for them,
// in the transaction that parked it. episode is the key: one notice per episode.
type InvitationAttention interface {
	InvitationAttentionTx(ctx context.Context, tx pgx.Tx, host ids.UserID, activity, episode ids.UUID, reason string) error
}

// WithInvitationAttention binds the host notice an invitation raises when it parks.
func (s *Store) WithInvitationAttention(attention InvitationAttention) *Store {
	clone := *s
	clone.invitationAttention = attention
	return &clone
}

// calendarAuthFailure is the one answer to whether a provider call failed
// because the host's calendar refused Margince, whichever call it was.
func calendarAuthFailure(err error) bool {
	if errors.Is(err, connector.ErrAuthRejected) {
		return true
	}
	refusal, ok := errors.AsType[*SchedulingArgumentError](err)
	return ok && *refusal == calendarNotConnected
}

// pendingFor is the queued state a command waits in before delivery.
func pendingFor(command string) string {
	switch command {
	case invitationUpdate:
		return "rescheduling"
	case invitationCancel:
		return invitationCanceling
	}
	return invitationPending
}

// markAttention parks row in memory: it waits for its host, for reason.
func markAttention(row *invitationRow, reason string, episode ids.UUID) {
	row.Status, row.AttentionReason, row.AttentionEpisode = invitationNeedsAttention, &reason, &episode
}

// parkInvitationAndQueue parks the finishing row and every queued row of the
// same host and calendar that no worker holds under lease. It parks nothing
// for a refusal whose attempt began before the host's last episode resumed:
// that refusal is news about the calendar the host has since reconnected.
func (s *Store) parkInvitationAndQueue(ctx context.Context, tx pgx.Tx, row *invitationRow, attemptedAt time.Time) (bool, error) {
	episode, current, err := s.disconnectEpisode(ctx, tx, row.Host, row.Provider, attemptedAt)
	if err != nil || !current {
		return false, err
	}
	markAttention(row, attentionCalendarDisconnected, episode)
	args := schedulingArgs{}
	rows, err := tx.Query(ctx, `SELECT activity_id FROM meeting_invitation WHERE host_user_id=`+args.add(row.Host)+` AND provider=`+args.add(row.Provider)+
		` AND activity_id<>`+args.add(row.ID)+` AND status IN ('pending','rescheduling','canceling') AND (lease_until IS NULL OR lease_until<`+args.add(s.now())+`) FOR UPDATE SKIP LOCKED`, args...)
	if err != nil {
		return false, err
	}
	queued, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return false, err
	}
	for _, id := range queued {
		if err := s.parkQueued(ctx, tx, id, episode); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (s *Store) parkQueued(ctx context.Context, tx pgx.Tx, id, episode ids.UUID) error {
	row, err := readInvitation(ctx, tx, id, true)
	if err != nil {
		return err
	}
	before := row
	markAttention(&row, attentionCalendarDisconnected, episode)
	row.Version++
	if err := writeAttention(ctx, tx, row); err != nil {
		return err
	}
	return auditInvitation(ctx, tx, &before, row)
}

// disconnectEpisode is the episode a refusal belongs to. An open episode is
// joined. A resumed one is joined by nothing: a refusal whose attempt began
// before it resumed is stale (current false), and a later one starts a new
// episode. The row outlives the invitations it parked for that reason.
func (s *Store) disconnectEpisode(ctx context.Context, tx pgx.Tx, host ids.UserID, provider string, attemptedAt time.Time) (ids.UUID, bool, error) {
	args := schedulingArgs{}
	var episode ids.UUID
	var resumed *time.Time
	err := tx.QueryRow(ctx, `SELECT episode, resumed_at FROM meeting_calendar_episode WHERE host_user_id=`+args.add(host)+` AND provider=`+args.add(provider)+` FOR UPDATE`, args...).Scan(&episode, &resumed)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return ids.Nil, false, err
	case resumed == nil:
		return episode, true, nil
	case attemptedAt.Before(*resumed):
		return ids.Nil, false, nil
	}
	episode = ids.NewV7()
	args = schedulingArgs{}
	values := []string{args.add(host), args.add(provider), args.add(episode), args.add(s.now())}
	_, err = tx.Exec(ctx, `INSERT INTO meeting_calendar_episode (host_user_id, provider, episode, started_at) VALUES (`+strings.Join(values, ",")+`)
		ON CONFLICT (host_user_id, provider) DO UPDATE SET episode=EXCLUDED.episode, started_at=EXCLUDED.started_at, resumed_at=NULL`, args...)
	return episode, true, err
}

func writeAttention(ctx context.Context, tx pgx.Tx, row invitationRow) error {
	args := schedulingArgs{}
	_, err := tx.Exec(ctx, `UPDATE meeting_invitation SET status=`+args.add(row.Status)+`,attention_reason=`+args.add(row.AttentionReason)+
		`,attention_episode=`+args.add(row.AttentionEpisode)+`,version=`+args.add(row.Version)+`,lease_until=NULL,updated_at=now() WHERE activity_id=`+args.add(row.ID), args...)
	return err
}

// tellHost raises the host's notice for a row that has just begun to wait.
func (s *Store) tellHost(ctx context.Context, tx pgx.Tx, row invitationRow) error {
	if s.invitationAttention == nil || row.AttentionEpisode == nil || row.AttentionReason == nil {
		return nil
	}
	return s.invitationAttention.InvitationAttentionTx(ctx, tx, row.Host, row.ID, *row.AttentionEpisode, *row.AttentionReason)
}

// resumeParkedInvitations sends a parked queue on once its host reconnected
// the calendar after the episode began and it can take bookings again. A
// calendar that merely stopped refusing for a moment is not evidence.
func (s *Store) resumeParkedInvitations(ctx context.Context) error {
	type queue struct {
		host     ids.UserID
		provider string
	}
	var open []queue
	err := s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT host_user_id, provider FROM meeting_calendar_episode WHERE resumed_at IS NULL`)
		if err != nil {
			return err
		}
		open, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (queue, error) {
			var q queue
			return q, row.Scan(&q.host, &q.provider)
		})
		return err
	})
	if err != nil {
		return err
	}
	faults := make([]error, 0, len(open))
	for _, q := range open {
		faults = append(faults, s.resumeQueue(ctx, q.host, q.provider))
	}
	return errors.Join(faults...)
}

// resumeQueue takes the host's lock first and judges the calendar under it, so
// a disconnect that parks after the judgement cannot be undone by it.
func (s *Store) resumeQueue(ctx context.Context, host ids.UserID, provider string) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		if err := storekit.LockWriteIdentity(ctx, tx, "meeting_host", host.String()); err != nil {
			return err
		}
		episode, ready, err := s.resumableEpisode(ctx, tx, host, provider)
		if err != nil || !ready {
			return err
		}
		args := schedulingArgs{}
		rows, err := tx.Query(ctx, `SELECT activity_id FROM meeting_invitation WHERE attention_episode=`+args.add(episode)+
			` AND status='needs_attention' AND attention_reason=`+args.add(attentionCalendarDisconnected)+` FOR UPDATE`, args...)
		if err != nil {
			return err
		}
		parked, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		if err != nil {
			return err
		}
		for _, id := range parked {
			if err := s.resumeParked(ctx, tx, id); err != nil {
				return err
			}
		}
		args = schedulingArgs{}
		_, err = tx.Exec(ctx, `UPDATE meeting_calendar_episode SET resumed_at=`+args.add(s.now())+` WHERE host_user_id=`+args.add(host)+` AND provider=`+args.add(provider), args...)
		return err
	})
}

// resumableEpisode is the host's open episode when, judged under the lock, the
// calendar was reconnected after it began and can take bookings now.
func (s *Store) resumableEpisode(ctx context.Context, tx pgx.Tx, host ids.UserID, provider string) (ids.UUID, bool, error) {
	args := schedulingArgs{}
	var episode ids.UUID
	var started time.Time
	err := tx.QueryRow(ctx, `SELECT episode, started_at FROM meeting_calendar_episode WHERE host_user_id=`+args.add(host)+` AND provider=`+args.add(provider)+` AND resumed_at IS NULL FOR UPDATE`, args...).Scan(&episode, &started)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.Nil, false, nil
	}
	if err != nil {
		return ids.Nil, false, err
	}
	reconnected, err := s.calendar.ReconnectedSince(ctx, host, provider, started)
	if err != nil || !reconnected {
		return ids.Nil, false, err
	}
	blocker, err := s.calendar.Blocker(ctx, host, provider)
	return episode, blocker == crmcontracts.SchedulingProfileBlockerNone, err
}

func (s *Store) resumeParked(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	row, err := readInvitation(ctx, tx, id, true)
	if err != nil {
		return err
	}
	before := row
	row.Status, row.AttentionReason, row.AttentionEpisode = pendingFor(row.Command), nil, nil
	row.Attempts, row.Version = 0, row.Version+1
	args := schedulingArgs{}
	if _, err := tx.Exec(ctx, `UPDATE meeting_invitation SET status=`+args.add(row.Status)+`,attempts=0,version=`+args.add(row.Version)+
		`,next_attempt_at=`+args.add(s.now())+`,updated_at=now() WHERE activity_id=`+args.add(row.ID), args...); err != nil {
		return err
	}
	return auditInvitation(ctx, tx, &before, row)
}
