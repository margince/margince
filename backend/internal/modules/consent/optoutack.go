// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Acknowledging a refusal of advertising, where the applicable rules owe one.
//
// Decree 91/2020/ND-CP Art. 16 gives a Vietnamese recipient who refuses further
// advertising a confirmation that their refusal was received, within
// twenty-four hours and carrying no advertising of its own.
// gates/messagingruleapplied_test.go carried the gap in its register:
// OptOutAcknowledgement was declared by the pack and nothing sent one.
//
// THE CONTROLLER LANE IS THE ONLY ONE THAT MAY, which is the reason this exists
// here rather than as an ordinary send. Every other path is refused writing to
// somebody who has just suppressed themselves, and correctly: the
// acknowledgement is the single exception the category vocabulary already
// carries, and optout_confirmation survives a broad stop for exactly this.
//
// THE TWENTY-FOUR HOURS ARE NOT ENFORCED, and that is worth saying plainly
// rather than leaving a reader to assume the decree's deadline is handled. The
// acknowledgement is queued the moment the refusal commits, which is as prompt
// as this product sends anything — but nothing measures the gap, nothing alerts
// on it, and a worker outage lasting longer than a day sends it late with no
// error raised. Controller mail is not paced, so the age ceiling that bounds an
// ordinary send does not reach here either.
//
// Closing it means a deadline on the delivery and something that reads it,
// which is its own change.
//
// SAME TRANSACTION as the suppression it acknowledges. A separate consumer
// would have to answer what happens when the stop commits and the queue write
// fails — either a subject who is owed an acknowledgement and does not get one,
// or a job that has to re-derive whether it is still owed. Staging it here
// means the two are one fact.

import (
	"context"
	"fmt"
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
// ONE MESSAGE FOR THE PRESS, not one per purpose. The subject performed one
// act, and a stop that withdrew four subscriptions is not four refusals — the
// message id is derived from the contact and the day for exactly that reason,
// so a second press the same day is refused by the ledger's own uniqueness
// index rather than by a count somebody maintains.
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
	day, err := databaseDayTx(ctx, tx)
	if err != nil {
		return err
	}
	return s.stageAcknowledgementTx(ctx, tx,
		subject{id: contactID.UUID, entityType: entityContact},
		withdrawalMessageID(contactID, day))
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
func (s *Store) stageAcknowledgementTx(
	ctx context.Context, tx pgx.Tx, sub subject, messageID string,
) error {
	if s.confirmSender == nil {
		return nil
	}
	rules, _, applicable, err := s.applicableRules(ctx, tx)
	if err != nil || !applicable {
		return err
	}
	if !rules.OptOutAcknowledgement {
		return nil
	}
	address, err := acknowledgementAddressTx(ctx, tx, sub)
	if err != nil || address == "" {
		// NO ADDRESS, NO ACKNOWLEDGEMENT, and no error either. A stop can be
		// recorded against a contact whose address the product does not hold,
		// or has since erased — and refusing the STOP because the
		// acknowledgement cannot be addressed would cost the subject the thing
		// they actually asked for to satisfy the thing they did not.
		return err
	}
	// NO EXPIRY, because there is no link to expire. The renderer skips the
	// expiry line for a linkless template, so the zero time never reaches the
	// words.
	rendered, category, err := RenderControllerTemplate(
		TemplateOptOutAcknowledgement, time.Time{}, s.mailLanguage(ctx, tx))
	if err != nil {
		return err
	}
	// A SAVEPOINT, so a staging refusal cannot take the stop with it.
	//
	// THE STOP IS WHAT THE SUBJECT ASKED FOR. The acknowledgement is what the
	// decree adds, and letting the second roll back the first inverts their
	// importance: a contact whose address has hard-bounced cannot be
	// acknowledged, and without this that refusal would fail their objection
	// and leave advertising permitted — the exact opposite of what they said.
	//
	// A REPLAY lands here too. The message id is derived from the act, so a
	// second acknowledgement of one refusal collides on the ledger's own
	// uniqueness index and is contained here rather than sending twice.
	nested, err := tx.Begin(ctx)
	if err != nil {
		return fmt.Errorf("consent: opening the acknowledgement: %w", err)
	}
	_, err = s.confirmSender.QueueConfirmationTx(ctx, nested, ConfirmationSend{
		ContactID: ids.From[ids.ContactKind](sub.id),
		Recipient: address,
		Category:  category,
		MessageID: messageID,
		// NO LINK, NO EXPIRY, NO MATERIAL. The template is linkless, and comms
		// refuses a body whose placeholder count disagrees with what it was
		// staged with — so these staying empty is what the shape check reads.
		Rendered: rendered,
	})
	if err != nil {
		// A rollback that does not take leaves the outer transaction aborted,
		// which would fail the stop anyway — so that one is reported rather
		// than hidden behind the staging error it was trying to contain.
		if rbErr := nested.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("consent: the opt-out acknowledgement could not be rolled back: %w", rbErr)
		}
		return nil
	}
	if err := nested.Commit(ctx); err != nil {
		return fmt.Errorf("consent: committing the opt-out acknowledgement: %w", err)
	}
	return nil
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
	for _, k := range acknowledgeableKinds() {
		if kind == k {
			return true
		}
	}
	return false
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

// acknowledgementAddressTx reads where to send it: the contact's primary
// address, by the ordering the preference centre and the confirm card share.
//
// Answers EMPTY rather than an error when there is none. A stop can be recorded
// against a contact whose address the product does not hold, or has since
// erased, and that is a fact about the contact rather than a failure.
func acknowledgementAddressTx(ctx context.Context, tx pgx.Tx, sub subject) (string, error) {
	if sub.entityType != entityContact {
		// A LEAD holds no contact_email row, and the acknowledgement is
		// addressed by the same reader every other controller message uses.
		// Reaching for the lead's own address here would be a second spelling
		// of "where does this contact receive mail", which is the thing
		// primaryEmailSQL exists to keep singular.
		return "", nil
	}
	var address string
	err := tx.QueryRow(ctx,
		`SELECT `+primaryEmailSQL("$1"), sub.id).Scan(&address)
	if err != nil {
		return "", fmt.Errorf("consent: reading where to acknowledge this stop: %w", err)
	}
	return address, nil
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

// withdrawalMessageID derives the RFC822 identity from the contact and the day.
//
// THE DAY, not the moment, because one press is one refusal however many
// subscriptions it withdrew — and because a subject who presses twice in an
// afternoon has refused once. A second press the same day collides on the
// ledger's uniqueness index and is contained, which is the behaviour wanted:
// the decree owes an acknowledgement of the refusal, not of each click.
//
// The date comes from the DATABASE rather than a Go clock. Two application
// processes with skewed wall clocks would otherwise be able to place one press
// on either side of midnight and send two messages for one act; one clock
// narrows that to the midnight boundary itself, which is as far as a derived id
// can take it.
func withdrawalMessageID(contactID ids.ContactID, day string) string {
	return "optout-ack-" + strings.ReplaceAll(contactID.String(), "-", "") +
		"-" + day + "@margince.invalid"
}

// databaseDayTx reads today's date from the database.
//
// ONE CLOCK for every process, rather than each reading its own: the day is
// part of a message id, so two application hosts with skewed wall clocks could
// otherwise place one press on either side of midnight and stage two
// acknowledgements for one refusal.
func databaseDayTx(ctx context.Context, tx pgx.Tx) (string, error) {
	var day string
	if err := tx.QueryRow(ctx, `SELECT to_char(now(), 'YYYYMMDD')`).Scan(&day); err != nil {
		return "", fmt.Errorf("consent: reading the day this refusal belongs to: %w", err)
	}
	return day, nil
}
