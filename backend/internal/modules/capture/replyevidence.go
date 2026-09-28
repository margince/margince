// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Whether an address has answered a mailbox owner, in a way that may publish
// the contact that owner is keeping.
//
// wroteBackTx asks whether the address ever answered ANYBODY here, which is the
// right question for what a record is worth and the wrong one for publishing.
// A reply that only a colleague's mailbox caught, or one on a held thread, is
// no evidence the owner's prospect became a relationship the workspace may see.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ownerAnswered is the evidence, with $1 the owner and $2 the folded address:
// an inbound, non-bulk message from the address, captured into the owner's own
// mailbox, answering a thread we wrote on, and not held.
func ownerAnswered() string {
	return `EXISTS (
		SELECT 1
		  FROM activity reply
		  JOIN capture_import own ON own.activity_id = reply.id AND own.user_id = $1
		 WHERE reply.counterparty_email = $2
		   AND reply.direction = 'inbound'
		   AND NOT reply.bulk_mail_attested
		   AND reply.thread_key <> ''
		   AND EXISTS (
		         SELECT 1 FROM activity ours
		          WHERE ours.thread_key = reply.thread_key
		            AND ours.counterparty_email = $2
		            AND ours.kind = reply.kind
		            AND ours.counterparty_outbound_attested
		            AND ` + auth.ActivityAvailableClause("ours") + `)
		   AND NOT ` + counterpartyHeldOn("reply") + `)`
}

// OwnerWasAnsweredTx reports whether the address has answered this owner in a
// way that may publish the contact they are keeping.
func OwnerWasAnsweredTx(ctx context.Context, tx pgx.Tx, owner ids.UUID, email string) (bool, error) {
	folded := normalizeEmail(email)
	if folded == "" {
		return false, nil
	}
	var answered bool
	if err := tx.QueryRow(ctx, `SELECT `+ownerAnswered(), owner, folded).Scan(&answered); err != nil {
		return false, fmt.Errorf("capture: reading whether the owner was answered: %w", err)
	}
	return answered, nil
}

// AnsweredContact names one owner-scoped contact the evidence above says may
// be published, and the address that answered.
type AnsweredContact struct {
	ContactID ids.UUID
	OwnerID   ids.UUID
	Email     string
}

// AnsweredContactsTx lists owner-scoped contacts narrowed for `reason` whose
// owner has been answered, so a reconciling pass can publish what the live
// capture hook missed.
//
// The reason is the caller's value, not spelled here: the vocabulary belongs to
// the contacts module. The evidence is ownerAnswered, the fragment the live
// check runs, with the row's own owner and address substituted.
func AnsweredContactsTx(ctx context.Context, tx pgx.Tx, reason string, limit int) ([]AnsweredContact, error) {
	evidence := strings.NewReplacer("$1", "p.owner_id", "$2", "pe.email").Replace(ownerAnswered())
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT ON (p.id) p.id, p.owner_id, pe.email
		  FROM contact p
		  JOIN contact_email pe ON pe.contact_id = p.id AND pe.archived_at IS NULL
		 WHERE p.visibility = 'owner' AND p.narrowing_reason = $1
		   AND p.archived_at IS NULL AND p.merged_into_id IS NULL
		   AND `+evidence+`
		 ORDER BY p.id, pe.email
		 LIMIT $2`, reason, limit)
	if err != nil {
		return nil, fmt.Errorf("capture: listing contacts whose owner was answered: %w", err)
	}
	defer rows.Close()
	var out []AnsweredContact
	for rows.Next() {
		var c AnsweredContact
		if err := rows.Scan(&c.ContactID, &c.OwnerID, &c.Email); err != nil {
			return nil, fmt.Errorf("capture: reading a contact whose owner was answered: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("capture: listing contacts whose owner was answered: %w", err)
	}
	return out, nil
}
