// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Lifting a restriction of processing (GDPR Art. 18(3)). The contact is told
// before the lift. The admin's lift queues the notice, and the stop is revoked
// only once the notice is handed to the mail provider. While the notice has not
// gone out, the restriction stays in force.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

const (
	// restrictionLiftPrefix opens the message id of a lift notice. The stop id
	// follows it, so the receipt names the stop it releases.
	restrictionLiftPrefix = "restriction-lift-"
	// liftBasisNoticeSent marks the lift the sent notice released.
	liftBasisNoticeSent = "restriction_notice_sent"
	// liftReasonNoticeSent is the lift's own reason. The admin's words are on
	// the audit row of the request that queued the notice.
	liftReasonNoticeSent = "the notice of the lift was handed to the mail provider"
)

// RequestRestrictionLift queues the notice that lifting a restriction owes the
// contact. It lifts nothing: LiftNoticedRestriction and LiftSentRestrictions
// do, once the notice is sent. Only a human admin asks, as only one records a restriction.
func (s *Store) RequestRestrictionLift(ctx context.Context, in LiftInput) error {
	in.Reason = strings.TrimSpace(in.Reason)
	if err := admitRestriction(ctx); err != nil {
		return err
	}
	sub, err := consentSubject(RecordInput{ContactID: in.ContactID, LeadID: in.LeadID})
	if err != nil {
		return err
	}
	if sub.entityType != entityContact || in.SuppressionID.IsZero() {
		return &ValidationError{Field: suppressionIDField, Reason: "name the contact's restriction to lift"}
	}
	if err := requireReason(in.Reason, "lifting a restriction"); err != nil {
		return err
	}
	if err := requireUpdateOnSubject(ctx, sub.entityType); err != nil {
		return err
	}
	if s.confirmSender == nil {
		return &ValidationError{Field: suppressionIDField, Reason: "this installation sends no mail, so it cannot send the notice a lift owes; the restriction stays"}
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := lockSubjectSuppressions(ctx, tx, sub.id); err != nil {
			return err
		}
		if err := auth.EnsureRetractable(ctx, tx, sub.entityType, sub.id); err != nil {
			return err
		}
		if _, err := liveRestrictionTx(ctx, tx, in.SuppressionID, sub.id); err != nil {
			return err
		}
		if err := refuseLiftUnderwayTx(ctx, tx, sub.id); err != nil {
			return err
		}
		return s.queueRestrictionLiftTx(ctx, tx, sub, in)
	})
}

// queueRestrictionLiftTx stages the notice and records who asked and why.
func (s *Store) queueRestrictionLiftTx(ctx context.Context, tx pgx.Tx, sub subject, in LiftInput) error {
	address, err := acknowledgementAddressTx(ctx, tx, sub, in.SuppressionID)
	if err != nil {
		return err
	}
	if address == "" {
		return &ValidationError{Field: suppressionIDField, Reason: "the contact has no address to send the notice to, so the restriction stays"}
	}
	act, err := withdrawalActTx(ctx, tx)
	if err != nil {
		return err
	}
	rendered, category, err := RenderControllerTemplate(TemplateRestrictionLift, time.Time{}, s.mailLanguage(ctx, tx))
	if err != nil {
		return err
	}
	rendered, err = s.controllerView(ctx, tx, rendered, category, address)
	if err != nil {
		return err
	}
	messageID := restrictionLiftMessageID(in.SuppressionID, act)
	if _, err := s.confirmSender.QueueConfirmationTx(ctx, tx, ConfirmationSend{
		ContactID: in.ContactID, Recipient: address, Category: category, MessageID: messageID, Rendered: rendered,
	}); err != nil {
		return err
	}
	_, err = storekit.AuditEvent(ctx, tx, "update", sub.entityType, sub.id, map[string]any{
		"restriction_lift_requested": in.SuppressionID.String(),
		fieldReason:                  in.Reason,
	})
	return err
}

// LiftNoticedRestriction ends the restriction a sent lift notice names. It is
// what the delivery store runs once the notice's receipt has committed.
func (s *Store) LiftNoticedRestriction(ctx context.Context, messageID string) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	stopID, ok := restrictionLiftStop(messageID)
	if !ok {
		return nil
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error { return s.liftNoticedTx(ctx, tx, stopID) })
}

// LiftSentRestrictions ends every restriction whose lift notice comms records
// as sent, and answers how many it ended. It repeats what the receipt's own
// effect may have failed to do; one stop that fails does not hold the others.
func (s *Store) LiftSentRestrictions(ctx context.Context) (int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return 0, err
	}
	var owed []ids.UUID
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT s.id FROM communication_suppression s
			 WHERE `+auth.LiveRestrictionSQL("s")+` AND EXISTS (
			       SELECT 1 FROM comms_outbound o
			        WHERE o.status = '`+deliverySent+`' AND o.message_id LIKE `+liftNoticeLikeSQL("s.id")+`)`)
		if err != nil {
			return err
		}
		owed, err = pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("consent: reading restrictions whose lift notice was sent: %w", err)
	}
	lifted := 0
	for _, stopID := range owed {
		if err := s.db.Tx(ctx, func(tx pgx.Tx) error { return s.liftNoticedTx(ctx, tx, stopID) }); err != nil {
			slog.ErrorContext(ctx, "a restriction whose lift notice was sent is still in force", "stop_id", stopID, "cause", err)
			continue
		}
		lifted++
	}
	return lifted, nil
}

// liftNoticedTx revokes, through the lift writer, every live restriction the
// contact held when the notice for stopID was sent. A stop already lifted, or a
// notice not yet sent, changes nothing, so running it twice is harmless.
func (s *Store) liftNoticedTx(ctx context.Context, tx pgx.Tx, stopID ids.UUID) error {
	var contact ids.UUID
	err := tx.QueryRow(ctx, `SELECT contact_id FROM communication_suppression
		 WHERE id = $1 AND `+auth.LiveRestrictionSQL("communication_suppression"), stopID).Scan(&contact)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("consent: reading the restriction a lift notice names: %w", err)
	}
	if err := lockSubjectSuppressions(ctx, tx, contact); err != nil {
		return err
	}
	if err := auth.EnsureWritable(ctx, tx, entityContact, contact); err != nil {
		return err
	}
	// A sent notice ends the restrictions that stood when the admin asked. One
	// recorded while the notice waited is a newer objection, and it stays.
	var requestedAt *time.Time
	if err := tx.QueryRow(ctx, `SELECT max(o.created_at) FROM comms_outbound o
		 WHERE o.status = '`+deliverySent+`' AND o.message_id LIKE `+liftNoticeLikeSQL("$1::uuid"), stopID).Scan(&requestedAt); err != nil {
		return fmt.Errorf("consent: reading whether the lift notice was sent: %w", err)
	}
	if requestedAt == nil {
		return nil
	}
	rows, err := tx.Query(ctx, `SELECT s.id, s.decided_by_level FROM communication_suppression s
		 WHERE s.contact_id = $1 AND `+auth.LiveRestrictionSQL("s")+` AND s.recorded_at <= $2
		 ORDER BY s.id FOR UPDATE`, contact, *requestedAt)
	if err != nil {
		return fmt.Errorf("consent: reading the contact's restrictions: %w", err)
	}
	live, err := pgx.CollectRows(rows, pgx.RowToStructByPos[liveRestriction])
	if err != nil {
		return fmt.Errorf("consent: reading the contact's restrictions: %w", err)
	}
	sub := subject{entityType: entityContact, column: subjectColumnContact, id: contact}
	for _, stop := range live {
		if err := s.revokeStopTx(ctx, tx, sub, revocation{
			stopID: stop.ID, recordedAt: stop.Decided, by: commsauthz.LevelAdmin, reason: liftReasonNoticeSent, basis: liftBasisNoticeSent,
		}); err != nil {
			return err
		}
	}
	return nil
}

// liveRestriction is one restriction stop a sent notice ends.
type liveRestriction struct {
	ID      ids.UUID
	Decided string
}

// errLiftUnderway refuses a second lift while a notice is queued or sent.
var errLiftUnderway = fmt.Errorf(
	"a notice that lifts this contact's restriction is already queued or sent; wait until it is sent or fails: %w",
	apperrors.ErrConflict)

// refuseLiftUnderwayTx refuses a lift while a notice for any of the contact's
// live restrictions waits or has gone out.
func refuseLiftUnderwayTx(ctx context.Context, tx pgx.Tx, contact ids.UUID) error {
	var underway bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM communication_suppression s
		  JOIN comms_outbound o ON o.message_id LIKE `+liftNoticeLikeSQL("s.id")+`
		 WHERE s.contact_id = $1 AND `+auth.LiveRestrictionSQL("s")+`
		   AND o.status IN ('`+deliveryPending+`', '`+deliverySent+`'))`, contact).Scan(&underway); err != nil {
		return fmt.Errorf("consent: reading whether a lift notice is underway: %w", err)
	}
	if underway {
		return errLiftUnderway
	}
	return nil
}

// liveRestrictionTx locks one live restriction of the contact and answers the
// level that recorded it, or ErrNotFound.
func liveRestrictionTx(ctx context.Context, tx pgx.Tx, stopID, contact ids.UUID) (string, error) {
	var decided string
	err := tx.QueryRow(ctx, `SELECT s.decided_by_level FROM communication_suppression s
		 WHERE s.id = $1 AND s.contact_id = $2 AND `+auth.LiveRestrictionSQL("s")+`
		 FOR UPDATE`, stopID, contact).Scan(&decided)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", apperrors.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("consent: reading the restriction: %w", err)
	}
	return decided, nil
}

// restrictionLiftMessageID names one lift notice by its stop and the act that
// queued it. A new act gets a new id, so an admin can ask again after a failure.
func restrictionLiftMessageID(stopID ids.UUID, act string) string {
	return restrictionLiftPrefix + strings.ReplaceAll(stopID.String(), "-", "") + "-" + act + "@margince.invalid"
}

// restrictionLiftStop reads the stop back out of a lift notice's message id.
func restrictionLiftStop(messageID string) (ids.UUID, bool) {
	rest, ok := strings.CutPrefix(messageID, restrictionLiftPrefix)
	if !ok || len(rest) < 32 {
		return ids.UUID{}, false
	}
	h := rest[:32]
	stopID, err := ids.Parse(h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:])
	return stopID, err == nil
}

// liftNoticeLikeSQL matches the message ids of every lift notice of the stop
// the placeholder names, for the stops list.
func liftNoticeLikeSQL(stopIDExpr string) string {
	return `'` + restrictionLiftPrefix + `' || replace(` + stopIDExpr + `::text, '-', '') || '-%'`
}
