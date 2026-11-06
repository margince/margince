// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/identity/internal/policy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// MailCapture is capture's half of a role change, injected through compose
// because identity never imports capture.
type MailCapture interface {
	// Refuses reports whether a member holding these grants cannot have a
	// mailbox captured.
	Refuses(perms principal.Permissions, seat principal.SeatType) bool
	// LiveMailboxesTx counts each member's live mail and calendar connections.
	LiveMailboxesTx(ctx context.Context, tx pgx.Tx, userIDs []ids.UUID) (map[ids.UUID]int, error)
	// RecheckSeatsTx makes these members' connections due, so the next sweep
	// asks again whether they may capture.
	RecheckSeatsTx(ctx context.Context, tx pgx.Tx, userIDs []ids.UUID) error
}

// WithMailCapture connects role changes to the mailboxes they affect. Without
// it a changed role still reaches the mailbox, on its next normal sync.
func (s *Service) WithMailCapture(m MailCapture) *Service { s.mailCapture = m; return s }

// recheckMailboxesTx asks capture to look again at these members' mailboxes.
func (s *Service) recheckMailboxesTx(ctx context.Context, tx pgx.Tx, userIDs []ids.UUID) error {
	if s.mailCapture == nil {
		return nil
	}
	return s.mailCapture.RecheckSeatsTx(ctx, tx, userIDs)
}

// recheckRoleHoldersTx rechecks every member who holds the role, after its
// grants moved.
func (s *Service) recheckRoleHoldersTx(ctx context.Context, tx pgx.Tx, roleKey string) error {
	if s.mailCapture == nil {
		return nil
	}
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT ra.user_id FROM role_assignment ra JOIN role r ON r.id = ra.role_id
		 WHERE r.key = $1`, roleKey)
	if err != nil {
		return err
	}
	holders, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return err
	}
	return s.mailCapture.RecheckSeatsTx(ctx, tx, holders)
}

// roleCapturesMail answers whether a member on a full seat holding only this
// role document may have a mailbox captured; nil when no seam is wired.
func (s *Service) roleCapturesMail(key string, raw []byte) (*bool, error) {
	if s.mailCapture == nil {
		return nil, nil
	}
	doc, err := policy.Parse(raw)
	if err != nil {
		return nil, err
	}
	captures := !s.mailCapture.Refuses(policy.Merge(map[string]policy.Document{key: doc}), principal.SeatFull)
	return &captures, nil
}

// markConnectedMailboxesTx fills each row's live connection count for the
// administration view.
func (s *Service) markConnectedMailboxesTx(ctx context.Context, tx pgx.Tx, rows []userRow) error {
	if s.mailCapture == nil {
		return nil
	}
	counts, err := s.mailCapture.LiveMailboxesTx(ctx, tx, rowIDs(rows))
	if err != nil {
		return err
	}
	for i := range rows {
		count := counts[rows[i].ID]
		rows[i].ConnectedMailboxes = &count
	}
	return nil
}
