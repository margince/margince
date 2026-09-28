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
}

// ContactsAwaitingDecisionFor lists the caller's own contacts that are waiting
// on a sender decision, oldest first.
func ContactsAwaitingDecisionFor(ctx context.Context, db *database.DB) ([]AwaitingContact, error) {
	if err := auth.RequireHuman(ctx); err != nil {
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
			if err := rows.Scan(&c.ContactID, &c.DisplayName, &c.Emails, &c.CapturedAt); err != nil {
				return err
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
