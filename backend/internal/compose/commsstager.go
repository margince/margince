// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Staging one outbound message: the delivery row, the decision recording why it
// was allowed to be queued, and the job that will send it — all on the caller's
// transaction.
//
// Its own file because it is the seam where three modules meet and none may
// import another: activities hands down the message, comms owns the delivery
// row, consent owns the decision. Compose is the only place that may see all
// three, which is exactly why the wiring belongs here rather than in any of
// them.

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// commsStager records an accepted send for transmission: the delivery row and
// the job that will carry it, both on the caller's transaction. One commit, one
// fact — a crash between them would either promise a send nothing queued or
// queue one with no timeline entry behind it.
type commsStager struct {
	store  *comms.Store
	runner *jobs.Runner
	// authority records why this message was allowed to be queued, on the same
	// transaction that queues it. Held here because compose is the only place
	// that may see both modules: comms owns the delivery row, consent owns the
	// decision, and neither may import the other.
	authority *consent.Gate
}

var (
	_ activities.DeliveryStager        = commsStager{}
	_ activities.ChannelDeliveryStager = commsStager{}
)

// DeliveryMachinery is the ONE delivery path in both the shapes a message can be
// staged in. It is a single seam rather than two because there is a single
// machinery behind it — one delivery table, one status machine, one retry ladder,
// one dispatcher — and a role able to wire mail staging without channel staging
// could serve a reply surface that accepts a message nothing will ever carry.
type DeliveryMachinery interface {
	activities.DeliveryStager
	activities.ChannelDeliveryStager
}

// NewDeliveryStager builds the delivery machinery every send transport is
// composed with (compose.WithDelivery). The runner is insert-only in the api
// role; the worker role works what it inserts.
//
//nolint:ireturn // returns the DeliveryMachinery seam by design: the concrete type is unexported and every caller holds the interface
func NewDeliveryStager(pool *pgxpool.Pool, runner *jobs.Runner) DeliveryMachinery {
	return commsStager{
		store:     comms.NewStore(InstallationDB(pool), time.Now, activities.NewStore(InstallationDB(pool))),
		runner:    runner,
		authority: consentGateFor(pool),
	}
}

func (s commsStager) StageTx(ctx context.Context, tx pgx.Tx, in activities.DeliveryRequest) error {
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		return errors.New("comms: staging a delivery outside workspace context")
	}
	id, err := s.store.StageTx(ctx, tx, comms.StageInput{
		ActivityID:      in.ActivityID,
		Provider:        in.Provider,
		MessageID:       in.MessageID,
		Recipients:      in.Recipients,
		Cc:              in.Cc,
		Bcc:             in.Bcc,
		Subject:         in.Subject,
		Body:            in.Body,
		HTMLBody:        in.HTMLBody,
		FromName:        in.FromName,
		Attachments:     commsFiles(in.Attachments),
		ConsentPurpose:  in.ConsentPurpose,
		InReplyTo:       in.InReplyTo,
		References:      in.References,
		ThreadKey:       in.ThreadKey,
		ListUnsubscribe: in.ListUnsubscribe,
	})
	if err != nil {
		return err
	}
	// Why this was allowed to be queued, written before the job that will send
	// it. In the same transaction as the activity, the delivery row, the audit
	// entry and the outbox event: all of them or none, so a decision can never
	// describe a message that rolled back, and a delivery can never reach the
	// worker with nothing on record about why it exists.
	if s.authority != nil {
		if _, err := s.authority.AuthorizeStagingTx(ctx, tx, id, in.Authorization); err != nil {
			return err
		}
	}
	return s.runner.EnqueueTx(ctx, tx, SendEmailArgs{
		Workspace: ws, DeliveryID: id.String(),
	}, sendInsertOpts())
}

// StageChannelTx is the same staging for a channel reply: the channel-shaped row
// and the SAME transmit job, on the caller's transaction.
//
// One job kind carries both shapes deliberately. The worker loads the delivery
// and dispatches it, and the dispatcher branches on the ROW's shape exactly once
// (comms/sendseam.go) — a second job kind would be a second path to keep in step
// with the first, and the channel is the one that would fall behind.
func (s commsStager) StageChannelTx(ctx context.Context, tx pgx.Tx, in activities.ChannelDeliveryRequest) error {
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		return errors.New("comms: staging a channel delivery outside workspace context")
	}
	id, err := s.store.StageChannelTx(ctx, tx, comms.StageChannelInput{
		ActivityID:     in.ActivityID,
		Provider:       in.Provider,
		Recipient:      in.Recipient,
		Body:           in.Body,
		Attachments:    commsFiles(in.Attachments),
		ConsentPurpose: in.ConsentPurpose,
	})
	if err != nil {
		return err
	}
	// The channel path is a second implementation of staging, and this is the
	// half a fix to the mail path does not reach. It records the same decision
	// for the same reason.
	if s.authority != nil {
		if _, err := s.authority.AuthorizeStagingTx(ctx, tx, id, in.Authorization); err != nil {
			return err
		}
	}
	return s.runner.EnqueueTx(ctx, tx, SendEmailArgs{
		Workspace: ws, DeliveryID: id.String(),
	}, sendInsertOpts())
}
