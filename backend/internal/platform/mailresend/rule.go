// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package mailresend states, once, when a bounced message may be sent again
// and which contact it belongs to. The Worklist row, the endpoint behind it
// and the timeline all read the answer from here, so they give one answer.
package mailresend

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Rule is the SQL for one statement. It reads the delivery under the alias
// `Delivery` (comms_outbound) and its activity under `Activity`.
type Rule struct {
	Delivery, Activity string
	owner              string
	eligible           string
	join               string
}

// For binds the rule to the signed-in user and to what that user may read. arg
// appends one placeholder value to the statement and returns its number.
func For(ctx context.Context, delivery, activity string, arg func(any) int) (Rule, error) {
	return bind(ctx, delivery, activity, arg, true)
}

// ForGatedRows is For for a statement whose activities the caller's content
// gate already admitted, so the gate is not planned a second time.
func ForGatedRows(ctx context.Context, delivery, activity string, arg func(any) int) (Rule, error) {
	return bind(ctx, delivery, activity, arg, false)
}

func bind(ctx context.Context, delivery, activity string, arg func(any) int, gate bool) (Rule, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID.IsZero() {
		return Rule{}, fmt.Errorf("sending a bounced message again needs a signed-in user: %w", apperrors.ErrPermissionDenied)
	}
	r := Rule{Delivery: delivery, Activity: activity, owner: "NULL", eligible: "FALSE"}
	// The owner is a contact, and naming one discloses it: a reader without the
	// contact grant is offered nothing.
	if !auth.ReadGranted(ctx, "contact") {
		return r, nil
	}
	visible, err := auth.LinkTargetReadableClause(ctx, "al", arg)
	if err != nil {
		return Rule{}, err
	}
	content := "TRUE"
	if gate {
		var err error
		if content, err = auth.ActivityContentClause(ctx, activity, arg); err != nil {
			return Rule{}, err
		}
	}
	if visible == "" {
		visible = "TRUE"
	}
	r.join = "JOIN activity " + activity + " ON " + activity + ".id = " + delivery + ".activity_id"
	// The contact whose address refused the message: filed on the activity,
	// readable by the caller, live, and holding the bounced address. A live
	// holder wins over one that only held the address before. Without the
	// address as the key, a message filed under several contacts would hand
	// the corrected address and the consent question to whichever sorted first.
	r.owner = `(SELECT al.contact_id FROM activity_link al
	  JOIN contact c ON c.id = al.contact_id AND c.archived_at IS NULL AND c.merged_into_id IS NULL
	  JOIN contact_email ce ON ce.contact_id = c.id AND ce.email = lower(btrim(` + delivery + `.bounce_recipient))
	 WHERE al.activity_id = ` + delivery + `.activity_id AND al.entity_type = 'contact' AND ` + visible + `
	 ORDER BY (ce.archived_at IS NULL) DESC, al.contact_id LIMIT 1)`
	// The caller's own mail that bounced hard and named the address that
	// refused. It kept its author's text and no agent wrote it. It is not
	// itself a repeat and has not been repeated.
	r.eligible = delivery + `.user_id = ` + fmt.Sprintf("$%d", arg(actor.UserID)) + `
   AND ` + delivery + `.sender_kind = 'user'
   AND ` + delivery + `.channel_user_id IS NULL
   AND ` + delivery + `.bounce_kind = 'hard'
   AND ` + delivery + `.bounce_recipient IS NOT NULL
   AND ` + delivery + `.authored_body_len IS NOT NULL
   AND ` + delivery + `.resent_from IS NULL
   AND ` + activity + `.captured_by NOT LIKE 'agent:%'
   AND NOT EXISTS (SELECT 1 FROM comms_outbound r WHERE r.resent_from = ` + delivery + `.id)
   AND ` + content + `
   AND ` + r.owner + ` IS NOT NULL`
	return r, nil
}

// OwnerSQL is a scalar expression: the contact id that owns the delivery, or
// NULL when none does.
func (r Rule) OwnerSQL() string { return r.owner }

// EligibleSQL is a boolean expression: the caller may send the delivery again.
func (r Rule) EligibleSQL() string { return r.eligible }

// JoinSQL joins the delivery to its activity, for a statement that does not
// already read it.
func (r Rule) JoinSQL() string { return r.join }
