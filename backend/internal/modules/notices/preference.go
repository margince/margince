// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package notices

// How a seat wants to be reached, per CLASS rather than per kind.
//
// A reader who muted "an automation fired" has not muted "somebody is waiting
// on your approval", and a producer that spells one more kind must not arrive
// under a setting nobody was ever shown. ClassFor is the one place a produced
// kind becomes a class, so a kind nothing places is refused there rather than
// delivered as though a decision had been made about it.
//
// The choice is the SEAT's own, taken from the principal exactly as their
// notices are: holding the seat is the whole authority needed to decide how
// often the product may interrupt you, and no colleague decides it for them.

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// Where a class of notice is delivered. The table's own CHECK holds the same
// four words, and a fifth one added here without the migration beside it is a
// write the database refuses.
const (
	DeliveryOff    = "off"
	DeliveryInApp  = "in_app"
	DeliveryEmail  = "email"
	DeliveryDigest = "digest"
)

var deliveryChoices = []string{DeliveryOff, DeliveryInApp, DeliveryEmail, DeliveryDigest}

// The classes a seat decides about.
//
// KindApprovalPending is both the kind an approval raises and the name of its
// class, spelled once: a class holding exactly one kind IS that kind, and a
// second spelling would be two words to keep in step for no reader's benefit.
const (
	classAutomation = "automation"
	classLeadSLA    = "lead_sla"
	classCapture    = "capture"
	classCoach      = "coach"
	classSystem     = "system"

	KindApprovalPending  = "approval_pending"
	ClassApprovalPending = KindApprovalPending
)

// noticeClasses is the closed set a seat is offered, in the order a settings
// screen reads them: the lanes that carry the most first, the housekeeping
// last.
var noticeClasses = []string{
	classAutomation, classLeadSLA, ClassApprovalPending, classCapture, classCoach, classSystem,
}

// classByKind places every kind a system flow raises.
//
// The kinds are spelled by their producers — compose's noticesseam.go and
// vcardstagereview.go — and a module may not import compose, so this map is
// where the two meet. A kind missing from it is refused by ClassFor rather than
// delivered under whatever the fallback happened to be.
var classByKind = map[string]string{
	"automation":              classAutomation,
	"lead_sla":                classLeadSLA,
	"capture_backlog_stalled": classCapture,
	"vcard_staging_failed":    classSystem,
	KindApprovalPending:       ClassApprovalPending,
}

// ClassFor answers which class a notice kind is decided about under.
//
// The coaching kinds are read off the CONTRACT's own vocabulary rather than
// copied into the map above: NoticeKind is closed and coach-only by its own
// note — the kinds an automation raises are deliberately absent from it — so a
// kind the contract admits is a colleague's words, and a fifth one placed
// itself the day the contract grew it.
func ClassFor(kind string) (string, error) {
	if class, placed := classByKind[kind]; placed {
		return class, nil
	}
	if crmcontracts.NoticeKind(kind).Valid() {
		return classCoach, nil
	}
	return "", fmt.Errorf("notices: no notification class for kind %q", kind)
}

// DefaultDelivery is what the installation decides for a seat that never chose.
//
// An approval is the one class whose notice asks the reader to DO something and
// holds a colleague up until they do, so it leaves the product. Everything else
// waits on the screen they are already looking at, which is where they will see
// it without the product mailing them about its own housekeeping.
func DefaultDelivery(class string) string {
	if class == ClassApprovalPending {
		return DeliveryEmail
	}
	return DeliveryInApp
}

// Preference is one class's effective setting for one seat.
//
// Chosen tells a DECISION from a DEFAULT. A seat that never chose follows the
// installation and moves when that default moves; one that chose in_app stays
// there. Collapsing the two would silently re-decide for everybody on the day a
// default changed, which is the distinction the app_user delivery columns keep
// with pointers for the same reason.
type Preference struct {
	Class    string
	Delivery string
	Chosen   bool
}

// MyNotificationPreferences answers the CALLING contact's own settings — one
// entry per class, the effective delivery in each, and whether it was chosen.
// The seat is not a parameter: another contact's settings cannot be expressed.
func (s *Store) MyNotificationPreferences(ctx context.Context) ([]Preference, error) {
	human, err := actingSeat(ctx, "reading your notification settings")
	if err != nil {
		return nil, err
	}
	var chosen map[string]string
	if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var txErr error
		chosen, txErr = chosenBy(ctx, tx, human)
		return txErr
	}); err != nil {
		return nil, fmt.Errorf("notices: reading your notification settings: %w", err)
	}
	return effective(chosen), nil
}

// SaveNotificationPreference records how the caller wants one class delivered
// and answers their WHOLE set, so a screen rendering one row per class cannot
// take a stale copy of the others from its own memory.
func (s *Store) SaveNotificationPreference(ctx context.Context, class, delivery string) ([]Preference, error) {
	human, err := actingSeat(ctx, "changing your notification settings")
	if err != nil {
		return nil, err
	}
	if err := checkPreference(class, delivery); err != nil {
		return nil, err
	}
	var chosen map[string]string
	if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// FIRST, because everything below depends on one answer to "what has
		// this seat decided about this class". READ COMMITTED gives each
		// statement its own view, so two tabs saving the same class both read
		// "nothing decided here", both pass the no-op skip, and the second's
		// write lands with a before-image naming a null the first had already
		// filled — two ledger entries and two announcements for one net change,
		// and a ledger that cannot say what the change was from.
		//
		// The write identity rather than SELECT … FOR UPDATE: a FIRST save has
		// no row to lock, and that is precisely the racing case.
		if err := storekit.LockWriteIdentity(ctx, tx, "notification_preference",
			preferenceIdentity(human, class)); err != nil {
			return err
		}
		var txErr error
		if chosen, txErr = chosenBy(ctx, tx, human); txErr != nil {
			return txErr
		}
		return savePreference(ctx, tx, human, class, delivery, chosen)
	}); err != nil {
		return nil, fmt.Errorf("notices: changing your notification settings: %w", err)
	}
	return effective(chosen), nil
}

// savePreference writes the one class and announces it, leaving chosen holding
// what the seat has decided after the write.
//
// Split from its caller because the transaction body is the whole write shape —
// row, ledger entry, announcement — and reading it beside the principal check
// and the merge made the reader hold three things at once.
func savePreference(ctx context.Context, tx pgx.Tx, human ids.UUID, class, delivery string, chosen map[string]string) error {
	before, decided := chosen[class]
	if decided && before == delivery {
		// Nothing moved. A settings page that saves on every render would
		// otherwise fill the ledger with a change nobody made — the ruling
		// identity's SaveMyDelivery makes about the same screen.
		return nil
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO notification_preference (user_id, class, delivery)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, class)
		DO UPDATE SET delivery = EXCLUDED.delivery, updated_at = now()`,
		human, class, delivery); err != nil {
		return fmt.Errorf("recording the choice: %w", err)
	}
	var previous *string
	if decided {
		previous = &before
	}
	chosen[class] = delivery
	// The entity is the SEAT, the way their delivery settings are audited: the
	// preference row has no id of its own, and what changed is a fact about the
	// person. The images carry the value, because the ledger is what an operator
	// reads to put back what somebody had — the class alone could not.
	auditID, err := storekit.Audit(ctx, tx, "update", "user", human,
		preferenceImage(class, previous), preferenceImage(class, &delivery))
	if err != nil {
		return err
	}
	// The NAME of the class and not the choice: what somebody decided about
	// their own interruptions is theirs, and a fan-out carrying the value would
	// tell every subscription owner who had switched their mail off.
	return storekit.EmitEvent(ctx, tx, auditID, human,
		crmcontracts.PublicEventNotificationPreferenceChanged{Class: class})
}

// DeliveryFor answers where one class reaches one recipient, inside a
// transaction the caller already holds.
//
// The recipient IS a parameter here, unlike every other read in this file, and
// that is the signature of a different question: the delivery legs run under
// the system principal for a seat they are already delivering to, and there is
// no acting human to compare against. Absence is the installation's default,
// which is why a seat who never chose costs no row.
func DeliveryFor(ctx context.Context, tx pgx.Tx, recipient ids.UserID, class string) (string, error) {
	var delivery string
	err := tx.QueryRow(ctx,
		`SELECT delivery FROM notification_preference WHERE user_id = $1 AND class = $2`,
		recipient, class).Scan(&delivery)
	if errors.Is(err, pgx.ErrNoRows) {
		return DefaultDelivery(class), nil
	}
	if err != nil {
		return "", fmt.Errorf("notices: reading the recipient's notification setting: %w", err)
	}
	return delivery, nil
}

// preferenceIdentity names the logical record a save decides about: one seat's
// choice for ONE class.
//
// Per class rather than per seat, so two tabs changing different classes do not
// wait on each other — they decide about different rows and read each other's
// nothing.
func preferenceIdentity(human ids.UUID, class string) string {
	return human.String() + ":" + class
}

// actingSeat is the acting human, named by the act for the refusal message.
//
// The CONTACT and not merely a user id: an agent or system principal can carry
// a human's id, and an agent acting under its grantor's authority must not
// decide what reaches its grantor, nor read or settle what already has — the
// same ruling identity's delivery settings make, for the same inbox.
//
// One gate for the whole module: reading a notice, settling one, settling them
// all and deciding where a class is delivered ask the same question of the same
// principal, and five inline copies of it would drift the first time the answer
// moved.
func actingSeat(ctx context.Context, act string) (ids.UUID, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || actor.UserID.IsZero() {
		return ids.Nil, fmt.Errorf("notices: %s needs an authenticated contact: %w", act, apperrors.ErrPermissionDenied)
	}
	return actor.UserID, nil
}

// checkPreference refuses a choice outside the vocabulary.
func checkPreference(class, delivery string) error {
	if !slices.Contains(noticeClasses, class) {
		return &values.ParseError{
			Field: "class", Code: "unknown",
			Message: "that is not a class of notification this product sends",
		}
	}
	if !slices.Contains(deliveryChoices, delivery) {
		return &values.ParseError{
			Field: "delivery", Code: "unknown",
			Message: "delivery is off, in_app, email or digest",
		}
	}
	if class == classCoach && delivery == DeliveryOff {
		// A coaching notice is a COLLEAGUE's words placed in this seat's queue,
		// not the product's own housekeeping. A reader may decide where those
		// words reach them; dropping them silently would leave the lead who
		// wrote them believing they had been read.
		return &values.ParseError{
			Field: "delivery", Code: "value_not_allowed",
			Message: "a colleague's coaching can be routed but not switched off",
		}
	}
	return nil
}

// chosenBy reads the rows this seat has actually decided.
//
// Absence is the answer for every other class, so the merge happens in Go
// rather than in SQL: the closed class list is the product's, not the
// database's, and a LEFT JOIN against a values list would put half of it into a
// statement where the next reader would not look for it.
func chosenBy(ctx context.Context, tx pgx.Tx, human ids.UUID) (map[string]string, error) {
	rows, err := tx.Query(ctx,
		`SELECT class, delivery FROM notification_preference WHERE user_id = $1`, human)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	chosen := map[string]string{}
	for rows.Next() {
		var class, delivery string
		if err := rows.Scan(&class, &delivery); err != nil {
			return nil, err
		}
		chosen[class] = delivery
	}
	return chosen, rows.Err()
}

// effective merges what a seat chose over the classes the product offers.
//
// A stored class the product no longer sends is left out rather than rendered:
// a retired class is not a decision anybody can act on, and the row stays where
// it is in case the class comes back.
func effective(chosen map[string]string) []Preference {
	out := make([]Preference, 0, len(noticeClasses))
	for _, class := range noticeClasses {
		if delivery, decided := chosen[class]; decided {
			out = append(out, Preference{Class: class, Delivery: delivery, Chosen: true})
			continue
		}
		out = append(out, Preference{Class: class, Delivery: DefaultDelivery(class), Chosen: false})
	}
	return out
}

// preferenceImage renders one side of the audit's pair: the class is the field
// and the choice is its value, the way the delivery columns audit on app_user.
//
// A seat who never chose shows a JSON null where one who chose shows the word —
// the distinction the whole Chosen flag exists for, and the one an operator
// reading the ledger needs to put back what was there.
//
//craft:ignore naked-any the audit seam takes an entity's own snapshot shape, serialized to jsonb
func preferenceImage(class string, delivery *string) map[string]any {
	return map[string]any{class: delivery}
}
