// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// A dead address stops being written to.
//
// The engine has always been able to refuse on a hard bounce: `hard_bounce` is
// in communication_suppression's kind CHECK, and authorizetransmitrecord.go
// maps it to commsauthz.ReasonHardBounce. Nothing ever wrote one. The refusal
// was reachable and unreachable at the same time — a rule with no writer behind
// it, which reads in the code as though the product handles dead addresses and
// in production as though it does not.
//
// What DID exist is comms/deadaddresses.go, a derived read that answers "which
// of these addresses last refused a delivery" for one contact panel. That is a
// display, and deliberately so: it is computed at read time and a later
// delivery that arrives clears the mark with no writer and nothing to erase.
// It is not consulted by the send path, so an operator could see "this address
// bounced" on the record and send to it again in the next breath.
//
// This writes the stop the engine was already prepared to read. The derived
// view stays exactly as it is: one answers "what happened to this address",
// the other "may we write to it", and the second is the one the send path asks.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// HardBounceFact is one delivery report the provider judged permanent.
type HardBounceFact struct {
	// Address is the recipient that refused. Lowercased here, matching how the
	// live-address index and the engine's own read both spell it.
	Address string
	// DeliveryID is the comms_outbound row the report named. Carried into the
	// source text so a later reader can find the message that died, which is
	// the only evidence that this stop was earned rather than typed.
	DeliveryID ids.UUID
}

// RecordHardBounceTx writes the address stop a permanent delivery failure earns.
//
// ADDRESS-SCOPED, never contact-scoped, and that is the substantive decision
// here. A bounce says one mailbox is gone; it says nothing about whoever owns
// it. A
// contact-scoped stop would silence every address they have, so correcting a
// typo in one address would leave the other two stopped — and the engine reads
// address rows already, so the narrow stop is also the one that works.
//
// THE ROW NAMES NO CONTACT, and this is the decision the whole scope rests on.
// Naming one looks helpful — a stop with a contact on it is findable from the
// record — and it silently converts an address stop into a contact stop,
// because the engine matches `contact_id = $1 OR ... lower(address) = ...`
// (authorizetransmitrecord.go) and the contact arm fires first. Every address
// that contact has is then refused, so correcting a typo in one leaves the
// other two dead. A test proved this: with the stop written against a
// deliberately wrong address, the send was still refused with reason
// hard_bounce, because the contact arm was matching.
//
// The cost is real and smaller: a reader on the contact page cannot see the
// stop by joining on contact_id. They can see the DEAD ADDRESS, which is what
// the page already shows through comms/deadaddresses.go, and the audit row
// below names the contact so the history still records that this happened to
// them.
//
// IDEMPOTENT BY THE DATABASE. A dead mailbox refuses every message sent to it,
// providers redeliver reports, and a backfill replays a captured mailbox, so
// this is reached repeatedly for the same address. The partial unique index
// (migration 1789190000) refuses the duplicate and ON CONFLICT DO NOTHING makes
// that the expected outcome rather than an error the caller has to tell apart
// from a real one. This is the opposite call from Suppress's, and deliberately:
// somebody asking a second time to be left alone is a second occasion worth
// recording,
// while a second bounce from a mailbox already known to be dead is the same
// fact arriving twice.
//
// MACHINE LEVEL. A bounce is not a legal act by the subject and must not rank
// with one — CanOverrule refuses to rank anything above LevelSubject, so a
// subject-level bounce stop would be unliftable by the whole staff.
//
// It is NOT lifted through Lift, and the level is not what decides that. Lift
// is contact-anchored end to end and reads the row by `id AND contact_id`,
// which a row naming no contact can never satisfy. LiftAddressStop below is the
// path, and it exists because without one a mistyped address would be stopped
// permanently with a database edit as the only remedy.
//
// Runs on the CALLER'S transaction, so the stop, the audit row and the outbox
// event commit with the bounce mark that earned them. A stop recorded against a
// delivery report that rolled back would refuse mail on the strength of a
// failure that never happened.
func RecordHardBounceTx(ctx context.Context, tx pgx.Tx, fact HardBounceFact) error {
	address := strings.ToLower(strings.TrimSpace(fact.Address))
	if address == "" {
		return errors.New("consent: a hard bounce names no address")
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	// Which record owns this address, if any — for the AUDIT row alone, never
	// for the suppression. Resolved here rather than handed across the seam:
	// it is a question about contacts, and this module already reads
	// contact_email to answer questions like it, while the send ledger has no
	// other reason to know records exist.
	//
	// Often nobody. A bounce is a fact about an ADDRESS, and the address may
	// belong to a lead, to somebody who was never a record, or to a contact
	// whose email row was deleted since the send. LIMIT 1 because two contacts
	// can carry the same address and the stop is about neither of them in
	// particular — it is about the mailbox.
	var contactID *ids.ContactID
	if err := tx.QueryRow(ctx, `
		SELECT contact_id FROM contact_email
		 WHERE lower(email) = $1 LIMIT 1`, address).Scan(&contactID); err != nil &&
		!errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("consent: resolving whose address refused delivery: %w", err)
	}
	// RETURNING id, so everything below fires only when a row was actually
	// written. A redelivered report is a no-op, and auditing one would record a
	// stop being made that already existed — an audit trail saying something
	// happened twice is worse than one that is quiet.
	var stopID ids.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO communication_suppression
		    (address, kind, source, captured_by, decided_by_level)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (lower(address))
		  WHERE kind = 'hard_bounce' AND address IS NOT NULL AND revoked_at IS NULL
		  DO NOTHING
		RETURNING id`,
		address, kindHardBounce, bounceSource(fact.DeliveryID), by,
		string(commsauthz.LevelMachine)).Scan(&stopID)
	if errors.Is(err, pgx.ErrNoRows) {
		// This address is already known to be dead, so nothing is audited and
		// nothing is announced — the stop that exists says everything a second
		// one would.
		//
		// THE NOTICE CASE STILL REOPENS. A dead mailbox refuses every message
		// sent to it, so a second outstanding delivery to the same address
		// bounces too, and that delivery may have been carrying a disclosure of
		// its own. Returning here would leave its case in `queued` forever —
		// the stop is about the ADDRESS and is rightly written once, while the
		// duty is about a MESSAGE and there is one per delivery.
		_, failErr := MarkNoticeDeliveryFailedTx(ctx, tx, fact.DeliveryID)
		return failErr
	}
	if err != nil {
		return fmt.Errorf("consent: recording that this address refused delivery: %w", err)
	}

	// Audited on the CONTACT when there is one, because that is the record a
	// reader opens to ask why a message was refused. With no contact the audit
	// still has to land somewhere the row can be found, so it names the
	// delivery that died — which is the only other identity this fact has.
	var entityType string
	var entityID ids.UUID
	if contactID != nil {
		entityType, entityID = "contact", contactID.UUID
	} else {
		entityType, entityID = entityActivity, fact.DeliveryID
	}
	// AuditEvent and not Audit: there is no prior state to image. A stop is a
	// new fact, not an edit to a field that held something before, and Audit
	// refuses an update it cannot describe.
	//
	// The ADDRESS is not in the payload. audit_log is append-only, so an
	// address written here outlives the Art. 17 erase that clears the
	// suppression row — and a recipient address is exactly what an erasure is
	// asked to destroy. The row carries it, erasure reaches the row, and the
	// audit records that an address was stopped without naming which.
	auditID, err := storekit.AuditEvent(ctx, tx, "update", entityType, entityID,
		map[string]any{
			auditFieldSuppressionKind: kindHardBounce,
			"decided_by_level":        string(commsauthz.LevelMachine),
		})
	if err != nil {
		return err
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, entityID,
		suppressionRecordedPayload(kindHardBounce, commsauthz.LevelMachine)); err != nil {
		return err
	}
	// A disclosure that did not arrive leaves its duty owed. The same delivery
	// that just proved this address dead may have been carrying an Art. 13 or
	// Art. 14 notice, and the case for it is sitting in `queued` — which reads
	// as handled and is not. Reopened in this transaction, so the dead address
	// and the duty it failed to discharge move together.
	//
	// The count is deliberately dropped: zero is the ordinary answer, because
	// most bounced mail carries no disclosure, and a caller has nothing to do
	// differently either way.
	if _, err := MarkNoticeDeliveryFailedTx(ctx, tx, fact.DeliveryID); err != nil {
		return err
	}
	return nil
}

// kindHardBounce is the stop's kind. The table's CHECK, the engine's reason map
// (authorizetransmitrecord.go) and this writer all have to agree on it.
const kindHardBounce = "hard_bounce"

// entityActivity names the entity a delivery-shaped audit row hangs from. The
// send ledger, the qualifying-event source and this audit all spell it, and
// three literals would be three chances to disagree with the audit reader.
const entityActivity = "activity"

// bounceSource says where the stop came from, in the free-text `source` column
// every suppression carries.
//
// It names the DELIVERY rather than the provider's reason text. The reason is
// external input written by the receiving system, it is already stored on the
// comms_outbound row this points at, and copying it here would put unbounded
// remote text into a second table with its own retention.
func bounceSource(deliveryID ids.UUID) string {
	return "delivery refused permanently: " + deliveryID.String()
}

// LiftAddressStop clears the bounce stop on an address that works again.
//
// SEPARATE FROM Lift, and the separation is forced rather than chosen. Lift is
// anchored on a contact from end to end: it resolves the subject, takes an
// advisory lock on that subject's stops, checks EnsureRetractable against the
// contact row, and reads the suppression by `id AND contact_id`. A bounce stop
// carries no contact — deliberately, because a contact-scoped row would refuse
// every address that record has — so it is unreachable through that path. The
// first version of this slice shipped without noticing, which made every bounce
// stop permanent and left a comment claiming any seat could lift it.
//
// The typo case is why this has to exist. Somebody mistypes an address, the
// mail bounces, the address is stopped, and the correction has to clear it —
// otherwise the mistake is permanent and the fix is a database edit.
//
// MACHINE LEVEL ONLY. This clears a stop the machinery wrote about a mailbox
// and must never reach a stop somebody made: a subject's own objection is not
// undone by somebody correcting an address. The kind filter is what holds that,
// and it is narrower than the level check Lift makes — this cannot touch a
// marketing_objection even if one somehow carried the machine level.
func (s *Store) LiftAddressStop(ctx context.Context, address, reason string) error {
	address = strings.ToLower(strings.TrimSpace(address))
	if address == "" {
		return &ValidationError{Field: "address", Reason: "lifting a stop names the address"}
	}
	if err := boundReason(reason); err != nil {
		return err
	}
	if strings.TrimSpace(reason) == "" {
		return &ValidationError{
			Field:  fieldReason,
			Reason: "lifting an address stop needs a reason somebody can review later",
		}
	}
	// The same gate a stop is recorded behind. An address stop is about who may
	// be written to, which is the contact object's question even when the row
	// names no contact.
	if err := auth.Require(ctx, "contact", principal.ActionUpdate); err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		var stopID ids.UUID
		err := tx.QueryRow(ctx, `
			UPDATE communication_suppression
			   SET revoked_at = now()
			 WHERE lower(address) = $1
			   AND kind = $2
			   AND contact_id IS NULL
			   AND revoked_at IS NULL
			RETURNING id`, address, kindHardBounce).Scan(&stopID)
		if errors.Is(err, pgx.ErrNoRows) {
			// No live bounce stop on this address. Answered as not-found rather
			// than silently, so a caller correcting an address learns whether
			// anything was actually cleared.
			return apperrors.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("consent: lifting the stop on this address: %w", err)
		}
		// Audited on the DELIVERY-less side: there is no contact to hang this
		// from, so the suppression row itself is the entity. That is the one
		// identity this fact reliably has, and it is what a later reader asking
		// "who decided this address works again" opens.
		if _, err := storekit.AuditEvent(ctx, tx, "update", "communication_suppression", stopID,
			map[string]any{
				auditFieldSuppressionKind: kindHardBounce,
				"revoked":                 true,
				fieldReason:               reason,
			}); err != nil {
			return err
		}
		return nil
	})
}
