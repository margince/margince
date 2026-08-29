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
	if !parsed.Confident {
		return nil
	}
	// BOTH columns must be empty, and both are written together. A parse is
	// confident about the PAIR "Bob Jones" — grafting its surname onto a first
	// name a human typed would build "Alice Jones", a person neither source ever
	// named. The predicate is also the concurrency guard: a writer who filled
	// either half between the dedupe read and this write keeps it, because
	// Postgres re-checks the predicate after waiting on their lock.
	// full_name moves WITH the split columns, and only when it is still the
	// shorter thing the parser refused to split. A record displaying "Lars"
	// while its columns say Lars Jankowfsky is a fill that reported success and
	// changed nothing a human can see — the defect this predicate closes.
	// A full_name a person typed is longer or different, and is left alone.
	//
	// The row is LOCKED before it is read, so the value recorded as the before
	// is the same one the CASE re-evaluates against. Read without the lock — as
	// a sub-select in RETURNING — a human editing full_name between the two
	// leaves this write recording their value as the after of a change it never
	// made, which plants a machine claim on the field human precedence is
	// arbitrated by.
	var previousFullName string
	err := tx.QueryRow(ctx,
		`SELECT full_name FROM person WHERE id = $1 FOR UPDATE`, personID).Scan(&previousFullName)
	// No row means the person is gone — erasure deletes it — so there is no
	// name left to complete.
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("people: reading the name person %s carries: %w", personID, err)
	}
	var fullName string
	err = tx.QueryRow(ctx, `
		UPDATE person
		   SET first_name = $2,
		       last_name  = $3,
		       full_name  = CASE WHEN full_name = $2 OR full_name = $3
		                         THEN $4 ELSE full_name END
		 WHERE id = $1
		   AND first_name IS NULL AND last_name IS NULL
		RETURNING full_name`,
		personID, parsed.First, parsed.Last, parsed.Full).Scan(&fullName)
	// No row is the guard doing its job, not a failure: the row already
	// carried a name, and it is not this call's to replace.
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("people: filling the missing name of person %s: %w", personID, err)
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
		return err
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, personID.UUID,
		crmcontracts.PublicEventPersonUpdated{ChangedFields: changed}); err != nil {
		return err
	}
	res.NameFilled = true
	return nil
}
