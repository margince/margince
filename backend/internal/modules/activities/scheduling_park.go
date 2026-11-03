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
// same host and calendar that no worker holds under lease.
func (s *Store) parkInvitationAndQueue(ctx context.Context, tx pgx.Tx, row *invitationRow) error {
	episode, err := s.disconnectEpisode(ctx, tx, row.Host, row.Provider)
	if err != nil {
		return err
	}
	markAttention(row, attentionCalendarDisconnected, episode)
	args := schedulingArgs{}
	rows, err := tx.Query(ctx, `SELECT activity_id FROM meeting_invitation WHERE host_user_id=`+args.add(row.Host)+` AND provider=`+args.add(row.Provider)+
		` AND activity_id<>`+args.add(row.ID)+` AND status IN ('pending','rescheduling','canceling') AND (lease_until IS NULL OR lease_until<`+args.add(s.now())+`) FOR UPDATE SKIP LOCKED`, args...)
	if err != nil {
		return err
	}
	queued, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return err
	}
	for _, id := range queued {
		if err := s.parkQueued(ctx, tx, id, episode); err != nil {
			return err
		}
	}
	return nil
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

// disconnectEpisode is the open episode of this host and calendar, or a new
// one. A row a worker held while the queue parked joins the open one.
func (s *Store) disconnectEpisode(ctx context.Context, tx pgx.Tx, host ids.UserID, provider string) (ids.UUID, error) {
	args := schedulingArgs{}
	var episode ids.UUID
	err := tx.QueryRow(ctx, `SELECT attention_episode FROM meeting_invitation WHERE host_user_id=`+args.add(host)+` AND provider=`+args.add(provider)+
		` AND status='needs_attention' AND attention_reason=`+args.add(attentionCalendarDisconnected)+` AND attention_episode IS NOT NULL LIMIT 1`, args...).Scan(&episode)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.NewV7(), nil
	}
	return episode, err
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

// resumeParkedInvitations sends a parked queue on once its calendar can take
// bookings again. A host whose calendar still cannot is left waiting.
func (s *Store) resumeParkedInvitations(ctx context.Context) error {
	type queue struct {
		host     ids.UserID
		provider string
	}
	var parked []queue
	err := s.tx(ctx, func(tx pgx.Tx) error {
		args := schedulingArgs{}
		rows, err := tx.Query(ctx, `SELECT DISTINCT host_user_id, provider FROM meeting_invitation WHERE status='needs_attention' AND attention_reason=`+args.add(attentionCalendarDisconnected), args...)
		if err != nil {
			return err
		}
		parked, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (queue, error) {
			var q queue
			return q, row.Scan(&q.host, &q.provider)
		})
		return err
	})
	if err != nil {
		return err
	}
	var faults []error
	for _, q := range parked {
		blocker, err := s.calendar.Blocker(ctx, q.host, q.provider)
		if err != nil {
			faults = append(faults, err)
			continue
		}
		if blocker == crmcontracts.SchedulingProfileBlockerNone {
			faults = append(faults, s.resumeQueue(ctx, q.host, q.provider))
		}
	}
	return errors.Join(faults...)
}

func (s *Store) resumeQueue(ctx context.Context, host ids.UserID, provider string) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		if err := storekit.LockWriteIdentity(ctx, tx, "meeting_host", host.String()); err != nil {
			return err
		}
		args := schedulingArgs{}
		rows, err := tx.Query(ctx, `SELECT activity_id FROM meeting_invitation WHERE host_user_id=`+args.add(host)+` AND provider=`+args.add(provider)+
			` AND status='needs_attention' AND attention_reason=`+args.add(attentionCalendarDisconnected)+` FOR UPDATE`, args...)
		if err != nil {
			return err
		}
		parked, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		if err != nil {
			return err
		}
		for _, id := range parked {
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
			if err := auditInvitation(ctx, tx, &before, row); err != nil {
				return err
			}
		}
		return nil
	})
}
