// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Completing the NAME of a person capture already has. It sits apart from the
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
// event read the same names. person.go and merge.go still spell them inline;
// bringing those onto these is a wider change than this one.
const (
	columnFirstName = "first_name"
	columnLastName  = "last_name"
)

// displayNameSetByHumanTx answers whether a person typed this contact's display
// name, either when the record was made or at any time since.
//
// Two questions, because a name can be typed at two moments and only one of them
// leaves an update behind.
//
// WHO MINTED THE ROW: person.captured_by, whose prefix is the principal type
// that created it — human, connector or agent. A contact somebody added by hand,
// imported from a vCard or typed into quick capture carries `human:`, and the
// name it was born with is that person's. Everything a connector or an agent
// minted carries the machine, and those are the rows this fill exists to
// improve. captured_by is stamped once at create and never moves, which is
// exactly right for a question about the create and wrong for anything else.
//
// WHO CHANGED IT SINCE: a human audit row whose action is 'update' and whose
// image names full_name. It has to be an update rather than any human row,
// because a capture runs under the SEAT whose mailbox it is — a rep's sync acts
// on their behalf, so the create it writes reads actor_type 'human' too, while
// the name on it came from a mail header rather than from that person typing.
// Counting those would read every connector-minted contact as human-named and
// refuse to improve any of them, which is this whole function inverted.
//
// field_provenance is the mechanism this eventually belongs in, and it is
// already wired for other fields — but no writer has ever stamped a name column,
// so it is empty for every contact that exists today. A guard reading it would
// report "no human" for a name a person typed last week.
//
// Whatever they set it to. Somebody who set the display name expressed an intent
// about that field, and one who set it back to what it already said expressed
// the same intent as one who changed it.
func displayNameSetByHumanTx(ctx context.Context, tx pgx.Tx, personID ids.PersonID) (bool, error) {
	var byHuman bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM person
		                WHERE id = $2 AND captured_by LIKE 'human:%')
		    OR EXISTS (SELECT 1 FROM audit_log
		                WHERE entity_type = $1 AND entity_id = $2
		                  AND actor_type = 'human' AND action = 'update'
		                  AND (after ? $3 OR before ? $3))`,
		entityPerson, personID.UUID, fieldFullName).Scan(&byHuman); err != nil {
		return false, fmt.Errorf("people: reading who named person %s: %w", personID, err)
	}
	return byHuman, nil
}

// fillMissingPersonName completes a person the ladder landed on by exact
// address, and completes ONLY what is missing.
//
// Every incumbent reached here already exists, so this is the one path that can
// improve a record created before the parser — or by an import, or by hand with
// only a full name typed in. It is strictly additive: each column carries its
// own IS NULL guard, so a name a human entered is never rewritten by whatever a
// mail header happens to spell, and re-running it converges instead of flapping
// between two spellings of the same person.
//
// Unconfident parses write nothing: `schluepmann` is not evidence of a surname
// with no given name, it is evidence that the local part did not say.
func fillMissingPersonName(ctx context.Context, tx pgx.Tx, personID ids.PersonID, parsed ParsedName, res *EnsureCounterpartyResult) error {
	filled, err := completePersonName(ctx, tx, personID, parsed)
	if err != nil {
		return err
	}
	if filled {
		res.NameFilled = true
	}
	return nil
}

// completePersonName is the fill itself, for the callers that are not the mail
// ladder. It answers whether it wrote, so each caller records that in its own
// terms.
//
// A calendar invitation is the second caller and the reason this is split out.
// It names an attendee in full — "Chris Erler" where the mail ladder only ever
// saw `chris@…` — and it arrives through the participant rows rather than
// through a counterparty ensure. Every guard documented above is what makes
// feeding it safe, so they are shared rather than restated at that call site.
func completePersonName(ctx context.Context, tx pgx.Tx, personID ids.PersonID, parsed ParsedName) (bool, error) {
	if !parsed.Confident {
		return false, nil
	}
	// BOTH columns must be empty, and both are written together. A parse is
	// confident about the PAIR "Bob Jones" — grafting its surname onto a first
	// name a human typed would build "Alice Jones", a person neither source ever
	// named. The predicate is also the concurrency guard: a writer who filled
	// either half between the dedupe read and this write keeps it, because
	// Postgres re-checks the predicate after waiting on their lock.
	// full_name moves WITH the split columns unless a person set it. When we
	// learn somebody's name we display it — that is the whole rule, and the only
	// thing that outranks it is a human having typed a name already.
	//
	// It used to move only where full_name still equalled one of the two parts
	// exactly. That reached a record displaying "Lars" beside columns saying Lars
	// Jankowfsky, and nothing else: the display names that actually go stale are
	// the ones a calendar organizer typed into their own address book — "Bw" for
	// Björn Welter, "Juan" for Judith Andresen, "Chris" for Christoph Erler.
	// None of them share a character with the name we later learned, so no test
	// on the SHAPE of the string can find them. Who wrote it is the question, and
	// the audit log answers it.
	//
	// The row is LOCKED before it is read, so the value recorded as the before is
	// the same one the write replaces. Read without the lock — as a sub-select in
	// RETURNING — a human editing full_name between the two leaves this write
	// recording their value as the after of a change it never made, which plants
	// a machine claim on the field human precedence is arbitrated by.
	var previousFullName string
	err := tx.QueryRow(ctx,
		`SELECT full_name FROM person WHERE id = $1 FOR UPDATE`, personID).Scan(&previousFullName)
	// No row means the person is gone — erasure deletes it — so there is no
	// name left to complete.
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("people: reading the name person %s carries: %w", personID, err)
	}
	humanNamed, err := displayNameSetByHumanTx(ctx, tx, personID)
	if err != nil {
		return false, err
	}
	var fullName string
	err = tx.QueryRow(ctx, `
		UPDATE person
		   SET first_name = $2,
		       last_name  = $3,
		       full_name  = CASE WHEN $5 THEN full_name ELSE $4 END
		 WHERE id = $1
		   AND first_name IS NULL AND last_name IS NULL
		RETURNING full_name`,
		personID, parsed.First, parsed.Last, parsed.Full, humanNamed).Scan(&fullName)
	// No row is the guard doing its job, not a failure: the row already
	// carried a name, and it is not this call's to replace.
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("people: filling the missing name of person %s: %w", personID, err)
	}
	// A mutation that changes a person's stored name is auditable like any other,
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
	auditID, err := storekit.Audit(ctx, tx, "update", entityPerson, personID.UUID, before, after)
	if err != nil {
		return false, err
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, personID.UUID,
		crmcontracts.PublicEventPersonUpdated{ChangedFields: changed}); err != nil {
		return false, err
	}
	return true, nil
}
