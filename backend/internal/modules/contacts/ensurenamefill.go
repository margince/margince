// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Completing the NAME of a contact capture already has. It sits apart from the
// ensure ladder that calls it because it is the ladder's one strictly additive
// write: everywhere else the engine decides which record a message belongs to,
// and here it improves one it has already decided on — under a guard that lets
// it only ever ADD, and an audit image that says what each column held.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The split-name columns this fill writes, so its statement, its image and its
// event read the same names. contact.go and merge.go still spell them inline;
// bringing those onto these is a wider change than this one.
const (
	columnFirstName = "first_name"
	columnLastName  = "last_name"
)

// fillMissingContactName completes a contact the ladder landed on by exact
// address, and completes ONLY what is missing.
//
// Every incumbent reached here already exists, so this is the one path that can
// improve a record created before the parser — or by an import, or by hand with
// only a full name typed in. It is strictly additive: each column carries its
// own IS NULL guard, so a name a human entered is never rewritten by whatever a
// mail header happens to spell, and re-running it converges instead of flapping
// between two spellings of the same contact.
//
// Unconfident parses write nothing: `schluepmann` is not evidence of a surname
// with no given name, it is evidence that the local part did not say.
func fillMissingContactName(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, parsed ParsedName, res *EnsureCounterpartyResult) error {
	filled, err := completeContactName(ctx, tx, contactID, parsed)
	if err != nil {
		return err
	}
	if filled {
		res.NameFilled = true
	}
	return nil
}

// completeContactName is the fill itself, for the callers that are not the mail
// ladder. It answers whether it wrote, so each caller records that in its own
// terms.
//
// A calendar invitation is the second caller and the reason this is split out.
// It names an attendee in full — "Chris Erler" where the mail ladder only ever
// saw `chris@…` — and it arrives through the participant rows rather than
// through a counterparty ensure. Every guard documented above is what makes
// feeding it safe, so they are shared rather than restated at that call site.
func completeContactName(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, parsed ParsedName) (bool, error) {
	if !parsed.Confident {
		return false, nil
	}
	// BOTH columns must be empty, and both are written together. A parse is
	// confident about the PAIR "Bob Jones" — grafting its surname onto a first
	// name a human typed would build "Alice Jones", a contact neither source ever
	// named. The predicate is also the concurrency guard: a writer who filled
	// either half between the dedupe read and this write keeps it, because
	// Postgres re-checks the predicate after waiting on their lock.
	// full_name moves with the split columns only while it is capture's own
	// guess. A name a human, an agent, an import or an API caller chose stays.
	//
	// It used to move only where full_name still equalled one of the two parts
	// exactly. That reached a record displaying "Lars" beside columns saying Lars
	// Jankowfsky, and nothing else: the display names that actually go stale are
	// the ones a calendar organizer typed into their own address book — "Bw" for
	// Björn Welter, "Juan" for Judith Andresen, "Chris" for Christoph Erler.
	// None of them share a character with the name we later learned, so no test
	// on the SHAPE of the string can find them. Who wrote it is the question,
	// and displayNameIsCapturesGuessTx answers it.
	//
	// The row is LOCKED before it is read, so the value recorded as the before is
	// the same one the write replaces. Read without the lock — as a sub-select in
	// RETURNING — a human editing full_name between the two leaves this write
	// recording their value as the after of a change it never made, which plants
	// a machine claim on the field human precedence is arbitrated by.
	var previousFullName string
	err := tx.QueryRow(ctx,
		`SELECT full_name FROM contact WHERE id = $1 FOR UPDATE`, contactID).Scan(&previousFullName)
	// No row means the contact is gone — erasure deletes it — so there is no
	// name left to complete.
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("contacts: reading the name contact %s carries: %w", contactID, err)
	}
	guessed, err := displayNameIsCapturesGuessTx(ctx, tx, contactID, parsed)
	if err != nil {
		return false, err
	}
	var fullName string
	err = tx.QueryRow(ctx, `
		UPDATE contact
		   SET first_name = $2,
		       last_name  = $3,
		       full_name  = CASE WHEN $5 THEN $4 ELSE full_name END
		 WHERE id = $1
		   AND first_name IS NULL AND last_name IS NULL
		RETURNING full_name`,
		contactID, parsed.First, parsed.Last, parsed.Full, guessed).Scan(&fullName)
	// No row is the guard doing its job, not a failure: the row already
	// carried a name, and it is not this call's to replace.
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("contacts: filling the missing name of contact %s: %w", contactID, err)
	}
	// A mutation that changes a contact's stored name is auditable like any other,
	// and every audited mutation ships its event in the same transaction —
	// without both, the row changes with no record of what did it and nothing
	// downstream learns the name it was waiting for.
	//
	// The split columns were both empty — the WHERE clause above is that
	// guarantee, not an assumption — while full_name moved only on the branch
	// that rewrote it, which is why the images are narrowed rather than asserted.
	// The event's delta and the audit image describe one change, so the name the
	// CASE rewrote is announced as well as recorded. Reported only when it moved:
	// the arm leaves full_name alone unless it held one of the two split values.
	changed := map[string]any{columnFirstName: parsed.First, columnLastName: parsed.Last}
	if fullName != previousFullName {
		changed[fieldFullName] = fullName
	}
	before, after := storekit.ChangedColumns(
		map[string]any{columnFirstName: nil, columnLastName: nil, fieldFullName: previousFullName},
		map[string]any{columnFirstName: parsed.First, columnLastName: parsed.Last, fieldFullName: fullName},
	)
	auditID, err := storekit.Audit(ctx, tx, "update", entityContact, contactID.UUID, before, after)
	if err != nil {
		return false, err
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, contactID.UUID,
		crmcontracts.PublicEventContactUpdated{ChangedFields: changed}); err != nil {
		return false, err
	}
	return true, nil
}
