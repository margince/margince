// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The set-aside read over real Postgres, from a subscription delivery.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension"
)

// connect deposits the member's credential with the unit, the act that makes
// them one who connected it.
func (s *systemReadEnv) connect(t *testing.T, user ids.UUID) {
	t.Helper()
	s.WsExec(t, `INSERT INTO extension_secret (id, extension_name, user_id, key, vault_ref)
		VALUES ($1, 'dealmirror', $2, 'token', 'vault/x') ON CONFLICT DO NOTHING`, ids.NewV7(), user)
}

func (s *systemReadEnv) setAside(t *testing.T, user ids.UUID, contact ids.UUID) extension.SetAsideState {
	t.Helper()
	var got extension.SetAsideState
	s.connect(t, user)
	s.onDelivery(t, func(ctx context.Context, rt extension.Runtime) error {
		var err error
		got, err = rt.SetAside().Contact(ctx, extension.UserID(user.String()), contact.String())
		return err
	})
	return got
}

func TestASetAsideChoiceIsReadAsTheUsersOwnAndRunsOut(t *testing.T) {
	s := setupSystemRead(t)
	composeReader(t, extension.SystemReadSetAside)
	contact := ids.NewV7()
	s.WsExec(t, `INSERT INTO contact (id, full_name, source, captured_by, visibility) VALUES ($1, 'Lapsed', 'manual', 'system', 'workspace')`, contact)
	until := clocktest.Now(t).UTC().Add(30 * 24 * time.Hour).Truncate(time.Microsecond)
	dismiss := func(reader ids.UUID, until any) {
		s.WsExec(t, `INSERT INTO relationship_nudge_dismissal (contact_id, reader_id, dismissed_until, set_by, set_at)
			VALUES ($1, $2, $3, 'test', now() - interval '2 hours')
			ON CONFLICT (contact_id, reader_id) DO UPDATE SET dismissed_until = EXCLUDED.dismissed_until`, contact, reader, until)
	}

	dismiss(s.Rep1, until)
	if got := s.setAside(t, s.Rep1, contact); got.Kind != extension.SetAsideUntil || got.Until == nil || !got.Until.Equal(until) {
		t.Errorf("a 30 day dismissal read as %+v, want until %v", got, until)
	}
	if got := s.setAside(t, s.Rep3, contact); got.Kind != extension.SetAsideNone {
		t.Errorf("a colleague's read as %+v, want none", got)
	}
	dismiss(s.Rep1, nil)
	if got := s.setAside(t, s.Rep1, contact); got.Kind != extension.SetAsidePermanent {
		t.Errorf("a permanent dismissal read as %+v, want permanent", got)
	}
	s.WsExec(t, `DELETE FROM relationship_nudge_dismissal WHERE contact_id = $1`, contact)
	if got := s.setAside(t, s.Rep1, contact); got.Kind != extension.SetAsideNone {
		t.Errorf("a resumed contact read as %+v, want none", got)
	}
	dismiss(s.Rep1, clocktest.Now(t).UTC().Add(-time.Hour))
	if got := s.setAside(t, s.Rep1, contact); got.Kind != extension.SetAsideNone {
		t.Errorf("an expired dismissal read as %+v, want none", got)
	}
	if got := s.setAside(t, s.Rep1, ids.NewV7()); got.Kind != extension.SetAsideNone {
		t.Errorf("a contact that does not exist read as %+v, want none", got)
	}
}

func TestASetAsideReadIsRefusedForAMemberWhoDidNotConnectTheUnit(t *testing.T) {
	s := setupSystemRead(t)
	composeReader(t, extension.SystemReadSetAside)
	var err error
	s.onDelivery(t, func(ctx context.Context, rt extension.Runtime) error {
		_, err = rt.SetAside().Contact(ctx, extension.UserID(s.Rep1.String()), ids.NewV7().String())
		return nil
	})
	if !errors.Is(err, extension.ErrForbidden) {
		t.Errorf("a set-aside read for a member who did not connect = %v, want ErrForbidden", err)
	}
}
