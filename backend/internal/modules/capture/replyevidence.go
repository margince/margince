// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Whether an address has answered the workspace in a way that may publish the
// contact capture is keeping to one seat.
//
// One question with two askers: the verdict, which decides whether a `contact`
// answer stays the owner's for want of a reply, and the reply hook and its
// reconciling pass, which publish such a contact once a reply exists. Both read
// AddressAnsweredTx, so a reply counts the same whichever of them sees it first.
//
// A reply to any seat counts, not only to the seat that owns the record. A
// colleague who corresponds with the person is the workspace working with them,
// and the narrowing it lifts exists only to keep a rep's unanswered prospecting
// quiet. A reply on a held thread never counts.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// addressAnswered is the evidence, with $1 the folded address: an inbound,
// non-bulk message from the address, answering a thread we wrote on, and not
// held. The exchange is wroteBackTx's, with the hold added.
func addressAnswered() string {
	return `EXISTS (
		SELECT 1
		  FROM activity reply
		 WHERE reply.counterparty_email = $1
		   AND reply.direction = 'inbound'
		   AND NOT reply.bulk_mail_attested
		   AND reply.thread_key <> ''
		   AND EXISTS (
		         SELECT 1 FROM activity ours
		          WHERE ours.thread_key = reply.thread_key
		            AND ours.counterparty_email = $1
		            AND ours.kind = reply.kind
		            AND ours.counterparty_outbound_attested
		            AND ` + auth.ActivityAvailableClause("ours") + `)
		   AND NOT ` + CounterpartyHeldOn("reply") + `)`
}

// AddressAnsweredTx reports whether the address has answered the workspace in
// a way that may publish a contact kept only because nobody had answered.
func AddressAnsweredTx(ctx context.Context, tx pgx.Tx, email string) (bool, error) {
	folded := normalizeEmail(email)
	if folded == "" {
		return false, nil
	}
	var answered bool
	if err := tx.QueryRow(ctx, `SELECT `+addressAnswered(), folded).Scan(&answered); err != nil {
		return false, fmt.Errorf("capture: reading whether the address answered: %w", err)
	}
	return answered, nil
}

// AnsweredContact names one owner-scoped contact the evidence above says may
// be published, and the address that answered.
type AnsweredContact struct {
	ContactID ids.UUID
	Email     string
}

// AnsweredContactsTx lists owner-scoped contacts narrowed for `reason` whose
// address has answered, so a reconciling pass can publish what the live
// capture hook missed — including rows narrowed before a colleague's reply
// counted.
//
// The reason is the caller's value, not spelled here: the vocabulary belongs to
// the contacts module. The evidence is addressAnswered, the fragment the live
// check runs, with the row's own address substituted.
func AnsweredContactsTx(ctx context.Context, tx pgx.Tx, reason string, limit int) ([]AnsweredContact, error) {
	evidence := strings.ReplaceAll(addressAnswered(), "$1", "pe.email")
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT ON (p.id) p.id, pe.email
		  FROM contact p
		  JOIN contact_email pe ON pe.contact_id = p.id AND pe.archived_at IS NULL
		 WHERE p.visibility = 'owner' AND p.narrowing_reason = $1
		   AND p.archived_at IS NULL AND p.merged_into_id IS NULL
		   AND `+evidence+`
		 ORDER BY p.id, pe.email
		 LIMIT $2`, reason, limit)
	if err != nil {
		return nil, fmt.Errorf("capture: listing contacts whose address answered: %w", err)
	}
	defer rows.Close()
	var out []AnsweredContact
	for rows.Next() {
		var c AnsweredContact
		if err := rows.Scan(&c.ContactID, &c.Email); err != nil {
			return nil, fmt.Errorf("capture: reading a contact whose address answered: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("capture: listing contacts whose address answered: %w", err)
	}
	return out, nil
}
