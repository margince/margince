// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// What a rep wants DELIVERED, as opposed to what they can open.
//
// The morning brief and the weekly review are both on the screen already; the
// mail is a nudge toward them. So this is one person's setting about their own
// inbox, self-scoped from the principal exactly as their display language is,
// and for the same reason: holding a seat is the whole authority needed to
// decide how often the product may interrupt you.

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

// The delivery choices. One vocabulary for both surfaces, so a reader who
// learns one has learned the other.
const (
	DeliveryNone  = "none"
	DeliveryEmail = "email"
)

var deliveryChoices = []string{DeliveryNone, DeliveryEmail}

// Delivery is what one seat has chosen about what arrives.
//
// Every field is a POINTER, and that is the shape rather than an accident: a
// member who has never chosen is not the same as one who chose "none". The
// first follows the installation's default and moves if that default moves; the
// second is a decision that stays. A plain string would collapse the two.
type Delivery struct {
	MorningBrief *string
	Weekly       *string
	QuietDay     *bool
	HourLocal    *int
}

// MyDelivery answers the caller's own delivery settings.
//
// Self-scoped: there is no argument by which a caller asks for somebody else's,
// because an admin does not read a colleague's inbox preferences through this
// API any more than they read their display language.
func (s *Service) MyDelivery(ctx context.Context) (Delivery, error) {
	human, err := deliveryUser(ctx)
	if err != nil {
		return Delivery{}, err
	}
	var out Delivery
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT morning_brief_delivery, weekly_delivery, quiet_day_notice, delivery_hour_local
			  FROM app_user WHERE id = $1 AND `+LiveMemberSQL("")+``, human).
			Scan(&out.MorningBrief, &out.Weekly, &out.QuietDay, &out.HourLocal)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Delivery{}, apperrors.ErrNotFound
	}
	if err != nil {
		return Delivery{}, fmt.Errorf("identity: reading the caller's delivery settings: %w", err)
	}
	return out, nil
}

// SaveMyDelivery records what the caller wants delivered.
//
// A PATCH in shape: a nil field means "leave it as it was", never "clear it".
// A client that renders three controls and sends two must not silently reset
// the third — and the settings page is exactly the surface where that mistake
// costs somebody their mail without anybody choosing it.
func (s *Service) SaveMyDelivery(ctx context.Context, in Delivery) (Delivery, error) {
	human, err := deliveryUser(ctx)
	if err != nil {
		return Delivery{}, err
	}
	if err := checkDelivery(in); err != nil {
		return Delivery{}, err
	}
	var out Delivery
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		var before Delivery
		if err := tx.QueryRow(ctx, `
			SELECT morning_brief_delivery, weekly_delivery, quiet_day_notice, delivery_hour_local
			  FROM app_user WHERE id = $1 AND `+LiveMemberSQL("")+``, human).
			Scan(&before.MorningBrief, &before.Weekly, &before.QuietDay, &before.HourLocal); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperrors.ErrNotFound
			}
			return err
		}
		out = merged(before, in)
		if sameDelivery(before, out) {
			// Nothing moved. A settings page that saves on every render would
			// otherwise fill the ledger with a change nobody made — the same
			// ruling SaveMyLocale makes one file over.
			return nil
		}
		if _, err := tx.Exec(ctx, `
			UPDATE app_user
			   SET morning_brief_delivery = $2, weekly_delivery = $3,
			       quiet_day_notice = $4, delivery_hour_local = $5
			 WHERE id = $1 AND `+LiveMemberSQL("")+``,
			human, out.MorningBrief, out.Weekly, out.QuietDay, out.HourLocal); err != nil {
			return err
		}
		auditID, err := storekit.Audit(ctx, tx, "update", "user", human,
			deliveryImage(before), deliveryImage(out))
		if err != nil {
			return err
		}
		return storekit.EmitEvent(ctx, tx, auditID, human,
			crmcontracts.PublicEventUserDeliveryChanged{
				ChangedFields: deliveryChanges(before, out),
			})
	})
	if err != nil {
		return Delivery{}, fmt.Errorf("identity: saving the caller's delivery settings: %w", err)
	}
	return out, nil
}

// deliveryUser is the acting human.
//
// Deliberately NOT actingHuman: that resolves the human an agent is acting
// UNDER, which is right for attributing work and wrong here. An agent carrying
// its grantor's authority must not change what lands in its grantor's inbox.
func deliveryUser(ctx context.Context) (ids.UUID, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || actor.UserID.IsZero() {
		return ids.Nil, apperrors.ErrPermissionDenied
	}
	return actor.UserID, nil
}

// checkDelivery refuses a choice outside the vocabulary.
func checkDelivery(in Delivery) error {
	if in.MorningBrief != nil && !slices.Contains(deliveryChoices, *in.MorningBrief) {
		return unknownDelivery("morning_brief_delivery")
	}
	if in.Weekly != nil && !slices.Contains(deliveryChoices, *in.Weekly) {
		return unknownDelivery("weekly_delivery")
	}
	if in.HourLocal != nil && (*in.HourLocal < 0 || *in.HourLocal > 23) {
		return &values.ParseError{
			Field: "delivery_hour_local", Code: "out_of_range",
			Message: "a local hour is 0 to 23",
		}
	}
	return nil
}

func unknownDelivery(field string) error {
	return &values.ParseError{
		Field: field, Code: "unknown", Message: "delivery is none or email",
	}
}

// merged applies the patch: a nil field leaves what was there.
func merged(before, patch Delivery) Delivery {
	out := before
	if patch.MorningBrief != nil {
		out.MorningBrief = patch.MorningBrief
	}
	if patch.Weekly != nil {
		out.Weekly = patch.Weekly
	}
	if patch.QuietDay != nil {
		out.QuietDay = patch.QuietDay
	}
	if patch.HourLocal != nil {
		out.HourLocal = patch.HourLocal
	}
	return out
}

// sameDelivery compares two settings by VALUE rather than by pointer.
//
// Two structs holding equal values behind different pointers are the same
// choice, and Go's own == would call them different — writing an audit row for
// a save that changed nothing.
func sameDelivery(a, b Delivery) bool {
	return same(a.MorningBrief, b.MorningBrief) &&
		same(a.Weekly, b.Weekly) &&
		same(a.QuietDay, b.QuietDay) &&
		same(a.HourLocal, b.HourLocal)
}

// same reports whether two optional values are the same choice — both unset, or
// both set to equal values.
func same[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// deliveryImage renders one side of the audit's before/after pair.
//
// The pointers carry through, so a member who never chose shows JSON null where
// one who chose "none" shows the word — the distinction the whole shape exists
// for, and the one an operator reading the ledger needs.
//
//craft:ignore naked-any the audit seam takes an entity's own snapshot shape, serialized to jsonb
func deliveryImage(d Delivery) map[string]any {
	return map[string]any{
		"morning_brief_delivery": d.MorningBrief,
		"weekly_delivery":        d.Weekly,
		"quiet_day_notice":       d.QuietDay,
		"delivery_hour_local":    d.HourLocal,
	}
}

// deliveryChanges names what moved, for the event.
//
// The NAMES rather than the values: what a person chose about their own inbox
// is theirs, and a fan-out carrying the values would tell every subscription
// owner who had switched their mail off.
func deliveryChanges(before, after Delivery) []string {
	var changed []string
	if !same(before.MorningBrief, after.MorningBrief) {
		changed = append(changed, "morning_brief_delivery")
	}
	if !same(before.Weekly, after.Weekly) {
		changed = append(changed, "weekly_delivery")
	}
	if !same(before.QuietDay, after.QuietDay) {
		changed = append(changed, "quiet_day_notice")
	}
	if !same(before.HourLocal, after.HourLocal) {
		changed = append(changed, "delivery_hour_local")
	}
	return changed
}
