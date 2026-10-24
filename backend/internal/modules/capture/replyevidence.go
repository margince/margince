// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Whether an address has answered the workspace in a way that may publish the
// contact capture is keeping to one seat.
//
// One question has two askers. The verdict decides whether a `contact` answer
// stays the owner's for want of a reply. The reply hook and its reconciling
// pass publish such a contact once a reply exists. Both read AddressAnsweredTx,
// so a reply counts the same whichever of them sees it first.
//
// A reply to any seat counts, not only to the seat that owns the record. A
// colleague who corresponds with the address is the workspace working with it.
// The narrowing it lifts only keeps a rep's unanswered prospecting quiet. A
// reply on a held thread never counts.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// addressAnswered is the evidence, with $1 the folded address: an exchange
// (answeredExchange) whose reply is not held and was not captured into a
// mailbox that holds its mail.
//
// A held mailbox promises that nothing it imports is shared, so its replies
// publish nobody, the owner's included. A classified mailbox's reply waits for
// its verdict like any other and still counts.
func addressAnswered() string {
	return `EXISTS (
		SELECT 1
		  FROM activity reply
		 WHERE ` + answeredExchange("reply", "$1") + `
		   AND NOT ` + CounterpartyHeldOn("reply") + `
		   AND NOT ` + importedIntoHeldMailbox("reply") + `)`
}

// importedIntoHeldMailbox is a message one of whose imports was made under the
// held posture, or into a seat whose mailbox holds its mail now.
func importedIntoHeldMailbox(alias string) string {
	return fmt.Sprintf(`EXISTS (
		SELECT 1 FROM capture_import held_import
		 WHERE held_import.activity_id = %[1]s.id
		   AND (held_import.posture_at_import = 'held' OR EXISTS (
		         SELECT 1 FROM capture_connection held_mailbox
		          WHERE held_mailbox.user_id = held_import.user_id
		            AND held_mailbox.mail_posture = 'held')))`, alias)
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

// AnsweredContactsTx lists up to limit owner-scoped contacts narrowed for
// `reason` whose address has answered, in id order after the pass's stored
// position. A reconciling pass publishes what the live capture hook missed,
// and rows narrowed before a colleague's reply counted.
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
		   AND p.id > coalesce((SELECT after_contact_id FROM capture_answered_sweep_cursor), '00000000-0000-0000-0000-000000000000'::uuid)
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

// AdvanceAnsweredSweepTx stores where the pass goes on from. A page shorter
// than the pass asked for reached the end, and the next pass starts over, so a
// contact skipped for a failure is offered again on the next round.
func AdvanceAnsweredSweepTx(ctx context.Context, tx pgx.Tx, page []AnsweredContact, limit int) error {
	var after *ids.UUID
	if len(page) == limit && limit > 0 {
		after = &page[len(page)-1].ContactID
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO capture_answered_sweep_cursor (singleton, after_contact_id, updated_at)
		VALUES (true, $1, now())
		ON CONFLICT (singleton) DO UPDATE
		   SET after_contact_id = EXCLUDED.after_contact_id, updated_at = EXCLUDED.updated_at`, after); err != nil {
		return fmt.Errorf("capture: storing where the answered-contact pass stopped: %w", err)
	}
	return nil
}
