// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Staging one message the INSTALLATION sends about itself: the timeline row, the
// delivery, the decision recording why it was allowed, and the job that will
// carry it — all on one transaction.
//
// Its own file beside commsstager.go, which does the same for a message a REP
// sends. They are deliberately siblings rather than one function with a flag:
// what differs is not a parameter but the whole authority story. A rep's send
// resolves a connected mailbox and asks whether that rep may write to this
// person; the installation's own mail has no mailbox and no rep, and asks
// whether the record shows an obligation it owes them.
//
// What they SHARE is the part that must not diverge — the delivery row, the
// staging decision, the job, and one commit for all of them.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// controllerMailQueue stages the installation's own mail through the durable
// lane every other message uses.
type controllerMailQueue struct {
	activities *activities.Store
	comms      *comms.Store
	runner     *jobs.Runner
	templates  comms.ControllerTemplates
	// authority records why this message was allowed to be queued, on the same
	// transaction that queues it — exactly as commsStager does for user mail.
	// The lane is NOT a side door: a confirmation mail is asked the same
	// question, and answers it on its own evidence rather than skipping it.
	authority *consent.Gate
}

var _ consent.ConfirmationSender = controllerMailQueue{}

// NewControllerMailQueue builds the lane the installation's own notices ride.
//
//nolint:ireturn // returns the consent-side seam by design: the concrete type is unexported and every caller holds the interface
func NewControllerMailQueue(pool *pgxpool.Pool, runner *jobs.Runner) consent.ConfirmationSender {
	store := activities.NewStore(InstallationDB(pool))
	return controllerMailQueue{
		activities: store,
		comms:      comms.NewStore(InstallationDB(pool), time.Now, store),
		runner:     runner,
		templates:  consent.ControllerTemplateRegistry(),
		authority:  consentGateFor(pool),
	}
}

// QueueConfirmationTx records one confirmation message for transmission.
//
// The ORDER is the contract. The activity is written first because both privacy
// erasure and the subject-access export reach comms_outbound THROUGH it, so a
// delivery staged before its activity would be invisible to both for as long as
// the transaction ran — and permanently, if the activity write then failed.
func (q controllerMailQueue) QueueConfirmationTx(ctx context.Context, tx pgx.Tx, in consent.ConfirmationSend) (ids.UUID, error) {
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		return ids.UUID{}, errors.New("compose: staging a controller delivery outside workspace context")
	}
	subject, body := in.Rendered.Subject, in.Rendered.Body
	outbound := string(crmcontracts.ActivityDirectionOutbound)
	activity, _, err := q.activities.LogActivityTx(ctx, tx, activities.LogActivityInput{
		Kind:      string(crmcontracts.ActivityKindEmail),
		Subject:   &subject,
		Body:      &body,
		Direction: &outbound,
		// The installation writing to somebody about its own obligations is not
		// that person engaging, so it stays out of every last_activity_at.
		Origin: activities.OriginSystemNotice,
		Links: []activities.ActivityLinkInput{{
			EntityType: string(crmcontracts.ActivityLinkEntityTypePerson),
			EntityID:   in.PersonID.UUID,
		}},
	})
	if err != nil {
		return ids.UUID{}, fmt.Errorf("compose: filing the notice on the timeline: %w", err)
	}
	deliveryID, err := q.comms.StageControllerTx(ctx, tx, comms.StageControllerInput{
		ActivityID:       ids.From[ids.ActivityKind](ids.UUID(activity.Id)),
		Recipient:        in.Recipient,
		TemplateKey:      in.Rendered.Key,
		TemplateVersion:  in.Rendered.Version,
		Subject:          subject,
		Body:             body,
		MessageID:        in.MessageID,
		PayloadRef:       in.LinkRef,
		PayloadExpiresAt: &in.ExpiresAt,
		LinkID:           in.LinkID,
	}, q.templates)
	if err != nil {
		return ids.UUID{}, err
	}
	// Why this was allowed to be queued, on the same transaction — the same
	// obligation commsStager.StageTx carries, and NOT guarded on the authority
	// being wired for the same reason it states: a nil check here would read as
	// caution and behave as a bypass, staging every notice with no decision
	// while satisfying a census gate that sees the call and cannot see it
	// skipped.
	if q.authority == nil {
		return ids.UUID{}, errors.New("compose: no authorization authority is wired on the controller mail lane")
	}
	set, err := q.authority.AuthorizeStagingTx(ctx, tx, deliveryID, commsauthz.Request{
		Recipients: []connector.Recipient{{Email: in.Recipient}},
		Context:    in.Category,
		Subject:    subject,
		Body:       body,
	})
	if err != nil {
		return ids.UUID{}, err
	}
	if err := refuseAtStaging(set); err != nil {
		return ids.UUID{}, err
	}
	if err := q.runner.EnqueueTx(ctx, tx, SendEmailArgs{
		Workspace: ws, DeliveryID: deliveryID.String(),
	}, sendInsertOpts()); err != nil {
		return ids.UUID{}, err
	}
	return deliveryID, nil
}
