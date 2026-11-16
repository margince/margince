// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package personalchannels

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

// What became of one pushed message.
const (
	StatusLanded    = "landed"
	StatusDuplicate = "duplicate"
	StatusSkipped   = "skipped"
	StatusRefused   = "refused"
	StatusDeferred  = "deferred"
)

// MaxPush is how many messages one push may carry.
const MaxPush = 50

// LandResult is what the capture pipeline did with one message.
type LandResult struct {
	Status     string
	ActivityID ids.UUID
	Reason     string
}

// Lander lands one member's message through capture. compose supplies it,
// because capture is a sibling this module may not import.
type Lander interface {
	Land(ctx context.Context, member ids.UUID, m NormalizedMessage, files []extension.InboundFile) (LandResult, error)
}

// PushResult is one message's answer, in the order it was sent.
type PushResult struct {
	MessageID  string
	Status     string
	ActivityID *ids.UUID
	Reason     string
}

// WithLander returns a copy of the store that lands messages through l.
func (s *Store) WithLander(l Lander) *Store {
	out := *s
	out.lander = l
	return &out
}

// origin is which client sent a message in: a passport, an AI assistant's
// connection, or the member's own session.
func origin(ctx context.Context) (kind string, id *ids.UUID) {
	p, ok := principal.Actor(ctx)
	switch {
	case !ok || p.Type != principal.PrincipalAgent:
		return "session", nil
	case !p.ConnectionID.IsZero():
		return "connection", &p.ConnectionID
	}
	return "passport", &p.PassportID
}

// Push lands msgs for the caller. One message's refusal does not refuse the
// rest; messages left when deadline passes answer deferred, and a resend lands
// them safely because each is keyed by its message_id.
func (s *Store) Push(ctx context.Context, msgs []PushMessage, deadline time.Time) ([]PushResult, error) {
	if err := auth.Require(ctx, rbacObject, principal.ActionCreate); err != nil {
		return nil, err
	}
	if len(msgs) == 0 || len(msgs) > MaxPush {
		return nil, invalid("send 1 to %d messages at a time", MaxPush)
	}
	if s.lander == nil {
		return nil, errors.New("personal channels: no lander is composed")
	}
	user, err := member(ctx)
	if err != nil {
		return nil, err
	}
	var conn Connection
	if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		conn, err = s.RequireEnabled(ctx, tx, user)
		return err
	}); err != nil {
		return nil, err
	}
	out := make([]PushResult, 0, len(msgs))
	for _, m := range msgs {
		if !s.now().Before(deadline) {
			out = append(out, PushResult{MessageID: m.MessageID, Status: StatusDeferred, Reason: "send it again; the request ran out of time"})
			continue
		}
		res, err := s.pushOne(ctx, conn.ID, user, m)
		if err != nil {
			return out, err
		}
		out = append(out, res)
	}
	return out, nil
}

func (s *Store) pushOne(ctx context.Context, connection, user ids.UUID, m PushMessage) (PushResult, error) {
	res := PushResult{MessageID: m.MessageID}
	n, err := Normalize(connection, m)
	if err != nil {
		return refused(res, err), nil
	}
	var files []extension.InboundFile
	if len(m.UploadIDs) > 0 {
		if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
			files, err = readUploadsTx(ctx, tx, user, m.UploadIDs)
			return err
		}); err != nil {
			return refused(res, err), nil
		}
	}
	landed, err := s.lander.Land(ctx, user, n, files)
	if err != nil {
		return refused(res, err), nil
	}
	res.Status, res.Reason = landed.Status, landed.Reason
	if landed.Status == StatusLanded || landed.Status == StatusDuplicate {
		id := landed.ActivityID
		res.ActivityID = &id
	}
	if landed.Status != StatusLanded {
		return res, nil
	}
	return res, s.recordLanding(ctx, connection, n, landed.ActivityID, m.UploadIDs)
}

// recordLanding keeps which network and client the landed message came from, and
// deletes the staged files it carried.
func (s *Store) recordLanding(ctx context.Context, connection ids.UUID, n NormalizedMessage, activity ids.UUID, uploads []ids.UUID) error {
	kind, id := origin(ctx)
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO personal_channel_message (activity_id, connection_id, network, origin_kind, origin_id)
			VALUES ($1, $2, $3, $4, $5) ON CONFLICT (activity_id) DO NOTHING`,
			activity, connection, n.Network, kind, id); err != nil {
			return err
		}
		if len(uploads) == 0 {
			return nil
		}
		_, err := tx.Exec(ctx, `DELETE FROM personal_channel_upload WHERE connection_id = $1 AND id = ANY($2)`, connection, uploads)
		return err
	})
}

// refused answers a message the push cannot land, with the reason a client can
// act on. Only an invalid or unknown message is the client's to fix; anything
// else is reported as a refusal too, so one message never fails the batch.
func refused(res PushResult, err error) PushResult {
	res.Status, res.Reason = StatusRefused, err.Error()
	if !errors.Is(err, apperrors.ErrInvalidArgument) && !errors.Is(err, apperrors.ErrNotFound) {
		res.Reason = fmt.Sprintf("Margince could not land it: %v", err)
	}
	return res
}
