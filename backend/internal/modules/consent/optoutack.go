// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Optional receipts share the refusal transaction through a savepoint.
// A receipt failure must not undo the subject's refusal.

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// acknowledgeWithdrawalTx stages the acknowledgement an UNSUBSCRIBE owes.
//
// THE ORDINARY REFUSAL, and the one Codex found this feature missing. A
// suppression is what a rep records and what the stronger public stop writes; a
// press of an unsubscribe link writes per-purpose withdrawals and no
// suppression at all. Acknowledging only the first would answer the rarer act
// and stay silent on the common one, which is the shape of an obligation that
// looks discharged and is not.
//
// One message for the press, not one per purpose: a stop that withdrew four
// subscriptions is one refusal. withdrawalMessageID names the press by contact
// and the transaction that recorded it, so every later press gets its own receipt.
//
// NOTHING WITHDRAWN, NOTHING TO ACKNOWLEDGE. A replayed press changes no state,
// and telling somebody again that we stopped is a second message they did not
// ask for.
func (s *Store) acknowledgeWithdrawalTx(
	ctx context.Context, tx pgx.Tx, contactID ids.ContactID, withdrawn []string,
) error {
	if len(withdrawn) == 0 {
		return nil
	}
	act, err := withdrawalActTx(ctx, tx)
	if err != nil {
		return err
	}
	return s.stageAcknowledgementTx(ctx, tx,
		subject{id: contactID.UUID, entityType: entityContact},
		withdrawalMessageID(contactID, act))
}

// acknowledgeOptOutTx stages the acknowledgement this stop owes, if any.
//
// ANSWERS NOTHING on a stop that owes none, which is the ordinary case: no
// applicable pack, a pack that declares no acknowledgement, or a stop that is
// not a refusal of advertising. A stop recorded by a rep relaying a phone call
// owes one exactly as a self-service press does — the decree is about the
// refusal, not about which door recorded it.
func (s *Store) acknowledgeOptOutTx(
	ctx context.Context, tx pgx.Tx, sub subject, kind string, stopID ids.UUID,
) error {
	if !acknowledgeableStop(kind) {
		return nil
	}
	return s.stageAcknowledgementTx(ctx, tx, sub, acknowledgementMessageID(stopID))
}

// stageAcknowledgementTx is the one place an acknowledgement is composed and
// queued, for both the suppression door and the unsubscribe press.
//
// ONE PLACE, because the two refusals owe the same message and a second copy of
// this would be a second answer to what the decree requires. What differs is
// the message id, which each caller derives from the act it is acknowledging.
func (s *Store) stageAcknowledgementTx(ctx context.Context, tx pgx.Tx, sub subject, messageID string) error {
	nested, err := tx.Begin(ctx)
	if err != nil {
		return fmt.Errorf("consent: opening receipt preparation: %w", err)
	}
	owed, missing, err := s.stageOwedReceiptTx(ctx, nested, sub, messageID)
	missed := MissedReceipt{SubjectType: sub.entityType, SubjectID: sub.id, MessageID: messageID, Reason: missing}
	if err != nil {
		if rollbackErr := nested.Rollback(ctx); rollbackErr != nil {
			return fmt.Errorf("consent: rolling back receipt preparation: %w", rollbackErr)
		}
		slog.ErrorContext(ctx, "opt-out receipt could not be staged; the refusal remains recorded",
			"subject_type", sub.entityType, "subject_id", sub.id, "cause", err)
		// A failure before the rules answered cannot say a receipt was owed.
		if owed {
			missed.Reason = MissedReceiptNotPrepared
			s.raiseMissedReceiptTx(ctx, tx, missed)
		}
		return nil
	}
	if err := nested.Commit(ctx); err != nil {
		return fmt.Errorf("consent: committing receipt preparation: %w", err)
	}
	if owed && missing != "" {
		s.raiseMissedReceiptTx(ctx, tx, missed)
	}
	return nil
}

// stageOwedReceiptTx stages the receipt when the rules owe one. It answers
// whether one is owed, and why it could not be staged when it was not.
func (s *Store) stageOwedReceiptTx(ctx context.Context, tx pgx.Tx, sub subject, messageID string) (bool, string, error) {
	owed, dueAt, err := s.receiptOwedTx(ctx, tx, sub)
	if err != nil || !owed {
		return false, "", err
	}
	if s.confirmSender == nil {
		return true, MissedReceiptNoMailLane, nil
	}
	address, err := acknowledgementAddressTx(ctx, tx, sub)
	if err != nil {
		return true, "", err
	}
	if address == "" {
		return true, MissedReceiptNoAddress, nil
	}
	return true, "", s.prepareAcknowledgementTx(ctx, tx, sub, dueAt, address, messageID)
}

// receiptOwedTx asks the stopping subject's own country's rules whether this
// refusal owes a receipt, and by when. A nil deadline is a rule that sets none.
func (s *Store) receiptOwedTx(ctx context.Context, tx pgx.Tx, sub subject) (bool, *time.Time, error) {
	country, err := recipientCountryTx(ctx, tx, ruleSubject{Kind: sub.entityType, ID: sub.id.String()})
	if err != nil {
		return false, nil, err
	}
	rules, _, applicable := applicableRules(country.Code)
	if !applicable || !rules.OptOutAcknowledgement {
		return false, nil, nil
	}
	if rules.OptOutAcknowledgementWithin <= 0 {
		return true, nil, nil
	}
	due := s.now().Add(rules.OptOutAcknowledgementWithin)
	return true, &due, nil
}

func (s *Store) prepareAcknowledgementTx(
	ctx context.Context, tx pgx.Tx, sub subject, dueAt *time.Time, address, messageID string,
) error {
	rendered, category, err := RenderControllerTemplate(TemplateOptOutAcknowledgement, time.Time{}, s.mailLanguage(ctx, tx))
	if err != nil {
		return err
	}
	rendered, err = s.controllerView(ctx, tx, rendered, category, address)
	if err != nil {
		return err
	}
	send := ConfirmationSend{Recipient: address, Category: category, MessageID: messageID, DueAt: dueAt, Rendered: rendered}
	// Filed under the subject who refused, so the receipt's timeline row, the
	// overdue sweep and an erasure all find it on that record.
	if sub.entityType == entityLead {
		send.LeadID = ids.From[ids.LeadKind](sub.id)
	} else {
		send.ContactID = ids.From[ids.ContactKind](sub.id)
	}
	_, err = s.confirmSender.QueueConfirmationTx(ctx, tx, send)
	return err
}

// acknowledgeableStop reports whether this kind of stop is a refusal of
// ADVERTISING, which is what the decree owes an acknowledgement for.
//
// A marketing objection plainly is. A broad subject request is too: somebody
// asking us to stop contacting them entirely has refused advertising along with
// everything else, and reading the wider request as not including the narrower
// one would deny an acknowledgement to the subject who asked for most.
//
// A hard bounce is NOT. Nothing was refused — an address stopped working, and
// mailing an acknowledgement to a dead address would be both futile and a
// second delivery attempt at one the provider already rejected.
func acknowledgeableStop(kind string) bool {
	return slices.Contains(acknowledgeableKinds(), kind)
}

// acknowledgeableKinds is the same answer as a list, for the validator that has
// to ask the database the question.
//
// ONE LIST, because the two ends must agree: the writer stages an
// acknowledgement for these kinds and the validator evidences it from these
// kinds, and a kind in one and not the other is either a message staged that
// can never send, or one that sends with nothing behind it.
func acknowledgeableKinds() []string {
	return []string{commsauthz.ReasonObjection, suppressibleKind}
}

// acknowledgementAddressTx reads where to send it. A contact's is its primary
// address, by the ordering the preference centre and the confirm card share. A
// lead holds one address on its own row, read while the lead is live.
//
// Answers EMPTY rather than an error when there is none. A stop can be recorded
// against a subject whose address the product does not hold, or has erased.
// That is a fact about the subject rather than a failure.
func acknowledgementAddressTx(ctx context.Context, tx pgx.Tx, sub subject) (string, error) {
	reader := primaryEmailSQL("$1")
	if sub.entityType == entityLead {
		reader = liveLeadEmailSQL("$1")
	}
	var address string
	err := tx.QueryRow(ctx, `SELECT `+reader, sub.id).Scan(&address)
	if err != nil {
		return "", fmt.Errorf("consent: reading where to acknowledge this stop: %w", err)
	}
	return address, nil
}

// liveLeadEmailSQL is a lead's own address, or empty when the lead has none or
// is archived. An archived lead was promoted, merged or erased. Mail to it would
// answer for a record that no longer speaks for its subject.
func liveLeadEmailSQL(ref string) string {
	return `coalesce((SELECT l.email FROM lead l WHERE l.id = ` + ref + ` AND l.archived_at IS NULL), '')`
}

// acknowledgementMessageID derives the RFC822 identity from the stop it
// acknowledges.
//
// ONE STOP, ONE MESSAGE. The ledger holds a uniqueness index on this column, so
// deriving it from the row means a second acknowledgement of the same stop is
// refused by the database rather than by a check somebody has to remember. A
// minted id would be unique every time, which is the opposite of what is wanted
// here: the point is that the same refusal cannot be acknowledged twice.
//
// It mirrors confirmMessageID's shape, for the same reason that one is derived
// rather than random.
func acknowledgementMessageID(stopID ids.UUID) string {
	return "optout-ack-" + strings.ReplaceAll(stopID.String(), "-", "") + "@margince.invalid"
}

// withdrawalMessageID names one press by its contact and its act.
//
// A second press of the same purposes on one day, after a re-grant, is a
// second refusal. It owes its own receipt.
func withdrawalMessageID(contactID ids.ContactID, act string) string {
	return "optout-ack-" + strings.ReplaceAll(contactID.String(), "-", "") + "-" + act + "@margince.invalid"
}

// withdrawalActTx names the transaction recording this press: the database day
// and its transaction id.
//
// The id is unique per act, and stable while the act runs. The day keeps an id
// from repeating after a restore into a new cluster, which counts transactions afresh.
func withdrawalActTx(ctx context.Context, tx pgx.Tx) (string, error) {
	var act string
	if err := tx.QueryRow(ctx, `SELECT to_char(now(), 'YYYYMMDD') || '-' || pg_current_xact_id()::text`).Scan(&act); err != nil {
		return "", fmt.Errorf("consent: naming the refusal this receipt acknowledges: %w", err)
	}
	return act, nil
}
