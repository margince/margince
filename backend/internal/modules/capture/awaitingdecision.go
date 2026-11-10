// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The captured contacts still waiting on a sender decision, as their owner
// sees them.
//
// The admin capture-health page counts these per mailbox and names none; the
// owner is the one reader allowed to see which they are. Both read the same
// predicate, so the owner's list and the admin's count agree by construction.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// AwaitingSenderDecisionClause selects, over the contact aliased `alias`, a
// captured contact still owner-private because no settled answer about any of
// its addresses exists in the sender ledger. A contact whose address has no
// ledger row at all is waiting too.
//
// The capture-health count and ContactsAwaitingDecisionFor both use it.
func AwaitingSenderDecisionClause(alias string) string {
	return alias + `.archived_at IS NULL
	   AND ` + alias + `.visibility = 'owner'
	   AND ` + alias + `.captured_by LIKE 'connector:%'
	   AND NOT EXISTS (
	         SELECT 1 FROM capture_pending_counterparty q
	          JOIN contact_email pe ON pe.contact_id = ` + alias + `.id AND pe.archived_at IS NULL
	         WHERE q.email = pe.email
	           AND q.status IN ('real', 'noise', 'suppressed', 'rejected'))`
}

// AwaitingContact is one of the caller's contacts waiting on a decision.
type AwaitingContact struct {
	ContactID   ids.UUID
	DisplayName string
	Emails      []string
	CapturedAt  time.Time
	// Addresses says, for each of Emails in the same order, what it waits on.
	Addresses []AwaitingAddress
}

// AwaitingAddress is one address of a waiting contact and what it waits on.
type AwaitingAddress struct {
	Email string
	State AwaitingState
}

// AwaitingState names what one address of a waiting contact waits on.
type AwaitingState string

// The states, most specific first: awaitingStateSQL asks in this order.
const (
	// AwaitingYourAnswerApplying: the owner decided, and a pass that will
	// apply it has not run yet.
	AwaitingYourAnswerApplying AwaitingState = "decided"
	// AwaitingClassifier: a question about the sender is open and due.
	AwaitingClassifier AwaitingState = "classifying"
	// AwaitingUnsure: the classifier could not tell, so the owner decides.
	AwaitingUnsure AwaitingState = "unsure"
	// AwaitingQueued: nothing was asked yet, and the stranded sweep will ask
	// over a captured message from the address.
	AwaitingQueued AwaitingState = "queued"
	// AwaitingNothingToJudge: nothing was asked and nothing will be: no
	// captured message from the address remains, or a user already worked
	// on the contact. Only its owner can settle it.
	AwaitingNothingToJudge AwaitingState = "unjudgeable"
)

// Processing reports whether the product, not the owner, takes the next step.
func (s AwaitingState) Processing() bool {
	return s == AwaitingYourAnswerApplying || s == AwaitingClassifier || s == AwaitingQueued
}

// awaitingStateSQL answers the AwaitingState of the address email on the
// contact aliased contact. The queued arm is StrandedContacts' own filter.
//
// A decision counts as applying only where a pass will reach it. That is the
// owner's open question, the stranded sweep for a business answer, or the
// noise sweep for a keep_out. Elsewhere it falls through, so the drawer keeps
// the owner's own ways to settle the contact.
func awaitingStateSQL(contact, email string) string {
	untouched := `NOT ` + contactTouchedByAUser(contact)
	return `CASE
	  WHEN EXISTS (SELECT 1 FROM capture_sender_override o
	                WHERE o.user_id = ` + contact + `.owner_id AND o.address = ` + email + `
	                  AND (EXISTS (SELECT 1 FROM capture_pending_counterparty q
	                                WHERE q.email = ` + email + ` AND q.owner_id = o.user_id
	                                  AND q.status IN ('pending', 'unsure'))
	                       OR (` + untouched + `
	                           AND (o.decision = 'keep_out'
	                                OR (NOT EXISTS (SELECT 1 FROM capture_pending_counterparty q
	                                                 WHERE q.email = ` + email + `)
	                                    AND EXISTS (SELECT 1 ` + judgeableMessageFrom(contact, email) + `))))))
	    THEN '` + string(AwaitingYourAnswerApplying) + `'
	  WHEN EXISTS (SELECT 1 FROM capture_pending_counterparty q
	                WHERE q.email = ` + email + ` AND q.status = 'pending')
	    THEN '` + string(AwaitingClassifier) + `'
	  WHEN EXISTS (SELECT 1 FROM capture_pending_counterparty q
	                WHERE q.email = ` + email + ` AND q.status = 'unsure')
	    THEN '` + string(AwaitingUnsure) + `'
	  WHEN ` + untouched + `
	   AND EXISTS (SELECT 1 ` + judgeableMessageFrom(contact, email) + `)
	    THEN '` + string(AwaitingQueued) + `'
	  ELSE '` + string(AwaitingNothingToJudge) + `' END`
}

// ContactsAwaitingDecisionFor lists the caller's own contacts that are waiting
// on a sender decision, oldest first.
func ContactsAwaitingDecisionFor(ctx context.Context, db *database.DB) ([]AwaitingContact, error) {
	if err := auth.RequireHumanOrItsAgent(ctx); err != nil {
		return nil, err
	}
	// The list names contacts, which is what the contact grant governs.
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return nil, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID == ids.Nil {
		return nil, apperrors.ErrPermissionDenied
	}
	out := []AwaitingContact{}
	err := db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT p.id, coalesce(p.full_name, ''),
			       coalesce(array(SELECT e.email FROM contact_email e
			                       WHERE e.contact_id = p.id AND e.archived_at IS NULL
			                       ORDER BY e.email), '{}'),
			       coalesce(array(SELECT `+awaitingStateSQL("p", "e.email")+` FROM contact_email e
			                       WHERE e.contact_id = p.id AND e.archived_at IS NULL
			                       ORDER BY e.email), '{}'),
			       p.created_at
			  FROM contact p
			 WHERE `+AwaitingSenderDecisionClause("p")+`
			   AND p.owner_id = $1
			 ORDER BY p.created_at, p.id`, actor.UserID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c AwaitingContact
			var states []string
			if err := rows.Scan(&c.ContactID, &c.DisplayName, &c.Emails, &states, &c.CapturedAt); err != nil {
				return err
			}
			for i, email := range c.Emails {
				c.Addresses = append(c.Addresses, AwaitingAddress{Email: email, State: AwaitingState(states[i])})
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("capture: listing a seat's contacts awaiting a sender decision: %w", err)
	}
	return out, nil
}
