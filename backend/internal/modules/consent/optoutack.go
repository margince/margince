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
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// acknowledgeWithdrawalTx stages the receipt one withdrawal of a marketing-class
// purpose owes. recordAdmittedTx calls it for every changed withdrawal. So every
// door that records one is covered, the record-confirmation page included.
//
// One receipt per act, not per purpose: a press that withdrew four purposes is
// one refusal. withdrawalMessageID names the act by subject and transaction.
// The subject's own press arrives and stops sending at the instant it is
// recorded, withdrawnAt.
func (s *Store) acknowledgeWithdrawalTx(ctx context.Context, tx pgx.Tx, sub subject, purposeClass string, withdrawnAt time.Time) error {
	if !slices.Contains(commsauthz.MarketingPurposeClasses(), purposeClass) {
		return nil
	}
	act, err := withdrawalActTx(ctx, tx)
	if err != nil {
		return err
	}
	messageID := withdrawalMessageID(sub.id, act)
	first, err := firstAcknowledgementOfActTx(ctx, tx, messageID)
	if err != nil || !first {
		return err
	}
	return s.stageAcknowledgementTx(ctx, tx, sub, messageID, ids.UUID{},
		RefusalTimes{ReceivedAt: withdrawnAt, StoppedAt: withdrawnAt})
}

// withdrawalFloorTx answers until when a withdrawal of advertising stays out of
// retention's reach, nil where the rules set none. The proof row carries it, so
// it holds when no stop or receipt is written beside the withdrawal.
func (s *Store) withdrawalFloorTx(
	ctx context.Context, tx pgx.Tx, sub subject, state ConsentState, purposeClass string, withdrawnAt time.Time,
) (*time.Time, error) {
	if state != StateWithdrawn || !slices.Contains(commsauthz.MarketingPurposeClasses(), purposeClass) {
		return nil, nil //nolint:nilnil // a nil end is the absence of a retention floor, which the proof row stores as null
	}
	terms, _, err := s.keepUnderReceiptRulesTx(ctx, tx, sub, ids.UUID{},
		RefusalTimes{ReceivedAt: withdrawnAt, StoppedAt: withdrawnAt})
	return terms.keptUntil, err
}

// acknowledgedActsSetting is the transaction-local setting that lists the
// receipts this transaction has staged. set_config(..., true) drops it at commit
// or rollback, so it never reaches the next transaction on the connection.
const acknowledgedActsSetting = "margince.acknowledged_refusals"

// firstAcknowledgementOfActTx marks messageID as acknowledged in this
// transaction, and answers whether it was not marked before.
func firstAcknowledgementOfActTx(ctx context.Context, tx pgx.Tx, messageID string) (bool, error) {
	var acts string
	if err := tx.QueryRow(ctx, `SELECT coalesce(current_setting($1, true), '')`,
		acknowledgedActsSetting).Scan(&acts); err != nil {
		return false, fmt.Errorf("consent: reading the refusals this transaction acknowledged: %w", err)
	}
	if slices.Contains(strings.Fields(acts), messageID) {
		return false, nil
	}
	if _, err := tx.Exec(ctx, `SELECT set_config($1, $2, true)`,
		acknowledgedActsSetting, acts+" "+messageID); err != nil {
		return false, fmt.Errorf("consent: marking the refusal this receipt acknowledges: %w", err)
	}
	return true, nil
}

// acknowledgeOptOutTx stages the acknowledgement this stop owes, if any.
//
// ANSWERS NOTHING on a stop that owes none, which is the ordinary case: no
// applicable pack, a pack that declares no acknowledgement, or a stop that is
// not a refusal of advertising. A stop recorded by a rep relaying a phone call
// owes one exactly as a self-service press does — the decree is about the
// refusal, not about which door recorded it.
func (s *Store) acknowledgeOptOutTx(
	ctx context.Context, tx pgx.Tx, sub subject, kind string, stopID ids.UUID, times RefusalTimes,
) error {
	if !acknowledgeableStop(kind) {
		return nil
	}
	return s.stageAcknowledgementTx(ctx, tx, sub, acknowledgementMessageID(stopID), stopID, times)
}

// stageAcknowledgementTx is the one place an acknowledgement is composed and
// queued, for both the suppression door and the unsubscribe press.
//
// ONE PLACE, because the two refusals owe the same message and a second copy of
// this would be a second answer to what the decree requires. What differs is
// the message id, which each caller derives from the act it is acknowledging.
//
// stopID names the suppression this receipt acknowledges, zero for a
// withdrawal, which records none. times are the refusal's, which it prints.
func (s *Store) stageAcknowledgementTx(
	ctx context.Context, tx pgx.Tx, sub subject, messageID string, stopID ids.UUID, times RefusalTimes,
) error {
	terms, owed, err := s.keepUnderReceiptRulesTx(ctx, tx, sub, stopID, times)
	if err != nil || !owed {
		return err
	}
	nested, err := tx.Begin(ctx)
	if err != nil {
		return fmt.Errorf("consent: opening receipt preparation: %w", err)
	}
	missing, err := s.stageOwedReceiptTx(ctx, nested, sub, terms, messageID, stopID, times)
	missed := MissedReceipt{SubjectType: sub.entityType, SubjectID: sub.id, MessageID: messageID, Reason: missing}
	if err != nil {
		if rollbackErr := nested.Rollback(ctx); rollbackErr != nil {
			return fmt.Errorf("consent: rolling back receipt preparation: %w", rollbackErr)
		}
		slog.ErrorContext(ctx, "opt-out receipt could not be staged; the refusal remains recorded",
			"subject_type", sub.entityType, "subject_id", sub.id, "cause", err)
		missed.Reason = MissedReceiptNotPrepared
	} else if err := nested.Commit(ctx); err != nil {
		return fmt.Errorf("consent: committing receipt preparation: %w", err)
	}
	if missed.Reason != "" {
		s.raiseMissedReceiptTx(ctx, tx, missed)
	}
	return nil
}

// keepUnderReceiptRulesTx reads the subject's receipt terms and stamps the
// refusal's retention floor, in a savepoint apart from the receipt's. So a
// receipt that fails to stage cannot take the floor down with it.
func (s *Store) keepUnderReceiptRulesTx(
	ctx context.Context, tx pgx.Tx, sub subject, stopID ids.UUID, times RefusalTimes,
) (receiptTerms, bool, error) {
	nested, err := tx.Begin(ctx)
	if err != nil {
		return receiptTerms{}, false, fmt.Errorf("consent: opening the refusal's retention floor: %w", err)
	}
	terms, owed, err := s.receiptOwedTx(ctx, nested, sub)
	if err == nil {
		terms.keptUntil, err = keepRefusalTx(ctx, nested, sub, stopID, times, terms.keep)
	}
	if err != nil {
		if rollbackErr := nested.Rollback(ctx); rollbackErr != nil {
			return receiptTerms{}, false, fmt.Errorf("consent: rolling back the refusal's retention floor: %w", rollbackErr)
		}
		// A failure before the rules answered cannot say a receipt was owed.
		slog.ErrorContext(ctx, "opt-out receipt rules could not be read; the refusal remains recorded",
			"subject_type", sub.entityType, "subject_id", sub.id, "cause", err)
		return receiptTerms{}, false, nil
	}
	if err := nested.Commit(ctx); err != nil {
		return receiptTerms{}, false, fmt.Errorf("consent: committing the refusal's retention floor: %w", err)
	}
	return terms, owed, nil
}

// stageOwedReceiptTx stages the receipt the rules owe, and answers why it
// could not be staged when it was not.
func (s *Store) stageOwedReceiptTx(
	ctx context.Context, tx pgx.Tx, sub subject, terms receiptTerms, messageID string, stopID ids.UUID, times RefusalTimes,
) (string, error) {
	times.UTCOffset = terms.utcOffset
	if s.confirmSender == nil {
		return MissedReceiptNoMailLane, nil
	}
	address, err := acknowledgementAddressTx(ctx, tx, sub, stopID)
	if err != nil {
		return "", err
	}
	if address == "" {
		return MissedReceiptNoAddress, nil
	}
	return "", s.prepareAcknowledgementTx(ctx, tx, sub, terms, address, messageID, times)
}

// receiptTerms are what the rules ask of one refusal and its receipt. dueAt is
// when an unsent receipt raises its alarm, nil for never. utcOffset is where its
// times print. keep is how long the refusal and receipt stay out of retention's
// reach, and keptUntil the instant that ends for this refusal.
type receiptTerms struct {
	dueAt     *time.Time
	utcOffset time.Duration
	keep      time.Duration
	keptUntil *time.Time
}

// keepRefusalTx stamps the stop with the end of its retention floor, and
// answers that end for the receipt. Nil when the rules set no floor.
func keepRefusalTx(
	ctx context.Context, tx pgx.Tx, sub subject, stopID ids.UUID, times RefusalTimes, keep time.Duration,
) (*time.Time, error) {
	if keep <= 0 {
		return nil, nil //nolint:nilnil // a nil end is the absence of a retention floor, which the receipt reads as none
	}
	kept := times.StoppedAt.Add(keep)
	if !stopID.IsZero() {
		var err error
		if kept, err = stampRefusalFloorTx(ctx, tx, sub, stopID, kept); err != nil {
			return nil, err
		}
	}
	return &kept, nil
}

// stampRefusalFloorTx sets the stop's retention floor to kept, and answers the
// floor the stop carries. A stop recorded again keeps the floor it already has.
func stampRefusalFloorTx(ctx context.Context, tx pgx.Tx, sub subject, stopID ids.UUID, kept time.Time) (time.Time, error) {
	if err := storekit.LockSubjectsStopAddresses(ctx, tx, sub.id); err != nil {
		return time.Time{}, err
	}
	tag, err := tx.Exec(ctx, `UPDATE communication_suppression SET kept_until = $2 WHERE id = $1 AND kept_until IS NULL`, stopID, kept)
	if err != nil {
		return time.Time{}, fmt.Errorf("consent: keeping the refusal for its retention floor: %w", err)
	}
	if tag.RowsAffected() == 0 {
		if err := tx.QueryRow(ctx, `SELECT kept_until FROM communication_suppression WHERE id = $1`, stopID).Scan(&kept); err != nil {
			return time.Time{}, fmt.Errorf("consent: reading the refusal's retention floor: %w", err)
		}
	}
	return kept, nil
}

// receiptOwedTx asks the stopping subject's own country's rules whether this
// refusal owes a receipt, and on which terms.
func (s *Store) receiptOwedTx(ctx context.Context, tx pgx.Tx, sub subject) (receiptTerms, bool, error) {
	country, err := recipientCountryTx(ctx, tx, ruleSubject{Kind: sub.entityType, ID: sub.id.String()})
	if err != nil {
		return receiptTerms{}, false, err
	}
	rules, _, applicable := applicableRules(country.Code)
	if !applicable {
		return receiptTerms{}, false, nil
	}
	terms := receiptTerms{utcOffset: rules.OptOutAcknowledgementUTCOffset, keep: rules.OptOutRecordsKeep}
	if rules.OptOutAcknowledgementWithin > 0 {
		due := s.now().Add(rules.OptOutAcknowledgementWithin)
		terms.dueAt = &due
	}
	return terms, rules.OptOutAcknowledgement, nil
}

func (s *Store) prepareAcknowledgementTx(
	ctx context.Context, tx pgx.Tx, sub subject, terms receiptTerms, address, messageID string, times RefusalTimes,
) error {
	rendered, category, err := RenderOptOutAcknowledgement(s.replyLanguage(ctx, tx, sub), times)
	if err != nil {
		return err
	}
	rendered, err = s.controllerView(ctx, tx, rendered, category, address)
	if err != nil {
		return err
	}
	send := ConfirmationSend{
		Recipient: address, Category: category, MessageID: messageID, DueAt: terms.dueAt, KeptUntil: terms.keptUntil, Rendered: rendered,
	}
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

// acknowledgementAddressTx reads where to send it. A refusal made from an
// address, as a link press is, is answered at that address. The stop records
// it, and the subject may have changed the one on file since. Otherwise a
// contact's is its primary address, by the ordering the preference centre and
// the confirm card share. A lead's is the one on its own row while it is live.
//
// Answers EMPTY rather than an error when there is none. A stop can be recorded
// against a subject whose address the product does not hold, or has erased.
// That is a fact about the subject rather than a failure.
func acknowledgementAddressTx(ctx context.Context, tx pgx.Tx, sub subject, stopID ids.UUID) (string, error) {
	if !stopID.IsZero() {
		var refusedFrom string
		if err := tx.QueryRow(ctx,
			`SELECT coalesce(address, '') FROM communication_suppression WHERE id = $1`, stopID).Scan(&refusedFrom); err != nil {
			return "", fmt.Errorf("consent: reading the address this stop was made from: %w", err)
		}
		if refusedFrom != "" {
			return refusedFrom, nil
		}
	}
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

// withdrawalMessageID names one withdrawal by its subject and its act.
//
// A second press of the same purposes on one day, after a re-grant, is a
// second refusal. It owes its own receipt.
func withdrawalMessageID(subjectID ids.UUID, act string) string {
	return withdrawalMessagePrefix(subjectID) + act + "@margince.invalid"
}

// withdrawalMessagePrefix opens the id of every withdrawal receipt of one
// subject, so a reader finds them by the subject they answer.
func withdrawalMessagePrefix(subjectID ids.UUID) string {
	return "optout-ack-" + strings.ReplaceAll(subjectID.String(), "-", "") + "-"
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
