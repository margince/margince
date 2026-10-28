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
// contact; the installation's own mail has no mailbox and no rep, and asks
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
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/config"
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
	// from is the relay's sender address, recorded on each delivery it stages.
	from string
}

var _ consent.ConfirmationSender = controllerMailQueue{}

// NewControllerMailQueue builds the lane the installation's own notices ride.
//
//nolint:ireturn // returns the consent-side seam by design: the concrete type is unexported and every caller holds the interface
func NewControllerMailQueue(pool *pgxpool.Pool, runner *jobs.Runner, from string) consent.ConfirmationSender {
	store := activities.NewStore(InstallationDB(pool))
	return controllerMailQueue{
		activities: store,
		comms:      comms.NewStore(InstallationDB(pool), time.Now, store),
		runner:     runner,
		templates:  consent.ControllerTemplateRegistry(),
		authority:  consentGateFor(pool),
		from:       from,
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
	rendered, body, err := presentControllerMailTx(ctx, tx, in.Rendered)
	if err != nil {
		return ids.UUID{}, err
	}
	in.Rendered = rendered
	subject := rendered.Subject
	outbound := string(crmcontracts.ActivityDirectionOutbound)
	activity, _, err := q.activities.LogActivityTx(ctx, tx, activities.SentRecord(activities.LogActivityInput{
		Kind:      string(crmcontracts.ActivityKindEmail),
		Subject:   &subject,
		Body:      &body,
		Direction: &outbound,
		Source:    systemActor,
		// The installation writing to somebody about its own obligations is not
		// that contact engaging, so it stays out of every last_activity_at.
		Origin: activities.OriginSystemNotice,
		Links:  []activities.ActivityLinkInput{controllerMailSubject(in)},
	}))
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
		HTMLBody:         in.Rendered.HTMLBody,
		MessageID:        in.MessageID,
		PayloadRef:       in.LinkRef,
		PayloadExpiresAt: controllerExpiry(in.ExpiresAt),
		DueAt:            in.DueAt,
		LinkID:           in.LinkID,
		FromAddress:      q.from,
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
		HTMLBody:   in.Rendered.HTMLBody,
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

// PresentConfirmationTx is the mail QueueConfirmationTx would stage, so a
// preview shows the same From, subject and text.
func (q controllerMailQueue) PresentConfirmationTx(ctx context.Context, tx pgx.Tx, rendered consent.Rendered) (consent.PresentedMail, error) {
	presented, body, err := presentControllerMailTx(ctx, tx, rendered)
	if err != nil {
		return consent.PresentedMail{}, err
	}
	return consent.PresentedMail{From: q.from, Subject: presented.Subject, Text: body}, nil
}

// presentControllerMailTx adds the brand the installation's mail is sent
// under: an HTML alternative, and the brand line atop the plain text.
func presentControllerMailTx(ctx context.Context, tx pgx.Tx, rendered consent.Rendered) (consent.Rendered, string, error) {
	body := rendered.Body
	if rendered.Language == "" && rendered.HTMLBody == "" {
		return rendered, body, nil
	}
	brand, err := config.StoredApplyTx(ctx, tx, loadedConfig(), identity.Name)
	if err != nil {
		return consent.Rendered{}, "", err
	}
	rendered.HTMLBody = consent.RenderControllerPresentation(rendered, brand)
	if brand != "" {
		body = brand + "\n\n" + body
	}
	return rendered, body, nil
}

// controllerMailSubject files the notice under the record it was sent to: the
// lead for a website lead's privacy notice, the contact for everything else.
func controllerMailSubject(in consent.ConfirmationSend) activities.ActivityLinkInput {
	if !in.LeadID.IsZero() {
		return activities.ActivityLinkInput{
			EntityType: string(crmcontracts.ActivityLinkEntityTypeLead), EntityID: in.LeadID.UUID,
		}
	}
	return activities.ActivityLinkInput{
		EntityType: string(crmcontracts.ActivityLinkEntityTypeContact), EntityID: in.ContactID.UUID,
	}
}

func controllerExpiry(expiry time.Time) *time.Time {
	if expiry.IsZero() {
		return nil
	}
	return &expiry
}
