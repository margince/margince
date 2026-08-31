// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Every decision this product made about one seat's senders, in one list.
//
// The decisions are already recorded — the ledger holds what the classifier
// concluded, the override table holds what the owner said instead — but they
// live in two tables and neither is readable by the person they are about. A
// product that decides silently and shows nobody is one an owner has to trust
// rather than check, and the whole posture rests on them being able to check.
//
// One seat's own senders and nobody else's. Whose mail a person keeps out is
// itself private: there is no id here that reaches a colleague's list and no
// admin view of one.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SenderDecision is one sender and what became of them.
type SenderDecision struct {
	Address string
	// Kind is what the classifier concluded — person, newsletter, personal,
	// advisor and the rest — or empty when it has not answered yet.
	Kind string
	// Status is the ledger's own lifecycle: pending, real, noise, unsure.
	Status string
	// Decision is what the OWNER said instead, empty when they have not.
	Decision string
	// OverruledKind is what their decision overruled, so the page can say
	// "you corrected this" rather than only showing the current answer.
	OverruledKind string
	// RecordExists reports whether a contact was actually created, which is
	// the consequence an owner can see on the rest of the product.
	RecordExists bool
}

// Overruled reports whether the owner's decision differs from the machine's.
func (d SenderDecision) Overruled() bool { return d.Decision != "" }

// SendersFor lists every sender this seat's mailbox has produced a decision
// about, the owner's own corrections included.
//
// A FULL OUTER JOIN in effect, because the two halves do not cover each other:
// the ledger holds senders nobody has corrected, and the override table holds
// addresses the classifier never reached — including senders whose ledger row a
// purge already deleted, which is exactly the case an owner most needs to still
// see. A list built from either alone would quietly omit one of them.
func SendersFor(ctx context.Context, db *database.DB) ([]SenderDecision, error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return nil, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID == ids.Nil {
		return nil, apperrors.ErrPermissionDenied
	}
	var out []SenderDecision
	err := db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT coalesce(p.email, o.address) AS address,
			       coalesce(p.kind, '')         AS kind,
			       coalesce(p.status, '')       AS status,
			       coalesce(o.decision, '')     AS decision,
			       coalesce(o.overruled_kind, '') AS overruled_kind,
			       EXISTS (
			         SELECT 1 FROM person_email pe
			           JOIN person pr ON pr.id = pe.person_id AND pr.archived_at IS NULL
			          WHERE pe.email = coalesce(p.email, o.address)
			            AND pe.archived_at IS NULL) AS record_exists
			  FROM capture_pending_counterparty p
			  FULL OUTER JOIN capture_sender_override o
			    ON o.address = p.email AND o.user_id = $1
			 WHERE p.owner_id = $1 OR o.user_id = $1
			 ORDER BY address`, actor.UserID)
		if err != nil {
			return fmt.Errorf("capture: listing a seat's sender decisions: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var d SenderDecision
			if err := rows.Scan(&d.Address, &d.Kind, &d.Status,
				&d.Decision, &d.OverruledKind, &d.RecordExists); err != nil {
				return fmt.Errorf("capture: listing a seat's sender decisions: %w", err)
			}
			out = append(out, d)
		}
		return rows.Err()
	})
	return out, err
}
