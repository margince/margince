// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// MeetingNotesAccess carries capture's account policy and reading window.
type MeetingNotesAccess struct {
	Concern string
	Domains []string
	Since   time.Time
}

// MeetingNotesAccessReader supplies capture policy without a module dependency.
type MeetingNotesAccessReader interface {
	ForReaderTx(context.Context, pgx.Tx, time.Time) (MeetingNotesAccess, error)
}

// WithMeetingNotesAccess binds the shared capture policy to HTTP detail reads.
func (h Handlers) WithMeetingNotesAccess(reader MeetingNotesAccessReader) Handlers {
	h.store = h.store.WithMeetingNotesAccess(reader)
	return h
}

// WithMeetingNotesAccess copies the store so bindings remain independently scoped.
func (s *Store) WithMeetingNotesAccess(reader MeetingNotesAccessReader) *Store {
	c := *s
	c.meetingNotesAccess = reader
	return &c
}

func (s *Store) stampMeetingNotesConnection(ctx context.Context, tx pgx.Tx, activity *crmcontracts.Activity) error {
	if s.meetingNotesAccess == nil || activity.Kind != crmcontracts.ActivityKindMeeting || activity.ArchivedAt != nil ||
		(activity.ContentState != nil && *activity.ContentState == crmcontracts.ActivityContentStateWithheld) {
		return nil
	}
	actor, ok := principal.Actor(ctx)
	if !ok || !actor.HumanOrItsAgent() {
		return nil
	}
	now := s.now()
	access, err := s.meetingNotesAccess.ForReaderTx(ctx, tx, now)
	if err != nil || access.Concern == "" {
		return err
	}
	meeting := ids.From[ids.ActivityKind](ids.UUID(activity.Id))
	pending, err := s.DueMeetingNotesTx(ctx, tx, MeetingNotesQuery{Meeting: &meeting, Owner: ids.From[ids.UserKind](actor.UserID), Domains: access.Domains, Since: access.Since, Until: now, Limit: 1})
	if err != nil {
		return err
	}
	if len(pending) != 0 {
		activity.MeetingNotesConnection = new(crmcontracts.ActivityMeetingNotesConnection(access.Concern))
	}
	return nil
}

// WithMeetingNotesAccess binds the same policy to tool detail reads.
func (p *Provider) WithMeetingNotesAccess(reader MeetingNotesAccessReader) *Provider {
	p.store = p.store.WithMeetingNotesAccess(reader)
	return p
}
