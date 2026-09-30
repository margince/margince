// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Applying what the contact themselves stated, on a date.
//
// A mail signature and a business card are the same kind of input: the contact
// describing their own details, at a moment that can be dated. Both land here,
// so the rule they obey is written once — while the card import filled only
// blanks, whether a stale number got corrected depended on whether the details
// arrived by mail or on paper.
//
// The rule is recency, and it holds against a human's typed value too. A rep who
// typed a number in March is not more right than the contact who signed a new
// one in August; keeping March's would leave the rep calling a phone that no
// longer rings, which is the failure this whole path exists to prevent. What is
// replaced is kept — superseded_value on the field row, the archived row itself
// for a phone — so the rep can put it back in one click.
//
// The one thing recency does NOT outrank is a human's CORRECTION, and that test
// belongs to the CALLER rather than to this file: the ruling lives in the ai
// module's ledger, which this one may not read, and it has to be read inside the
// same transaction as the write or a correction made while a model was thinking
// is lost. ApplySignatureFields does it that way.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The rest of the shared vocabulary: the fields a card and a signature both
// state, as constants so a typo cannot file a value under a key no reader looks
// for. fieldTitle and fieldPhone are declared where their own writers first
// needed them.
const (
	fieldRole        = "role"
	fieldCompanyName = "company_name"
	fieldAddress     = "address"
	fieldLinkedin    = "linkedin"
	fieldWebsite     = "website"
)

// observedField is one dated statement about one field.
//
// SourceRef names what was read — the activity, the attachment — and is the
// evidence handle a reader follows back. Confidence is nil for a card, which is
// parsed rather than inferred, and set for a signature, which is not.
type observedField struct {
	Field      string
	Value      string
	Evidence   string
	SourceRef  string
	Source     string
	CapturedBy string
	Confidence *float64
	ObservedAt time.Time
}

// observedOutcome is what applying a statement did, which the callers report
// and count.
type observedOutcome int

const (
	// observedSkipped: the row was not written. An older or equal statement, an
	// unreadable value, or a subject that went.
	observedSkipped observedOutcome = iota
	// observedApplied: the record now carries this value.
	observedApplied
	// observedConfirmed: the record already carried it, stated at least as
	// recently. Nothing was written and nothing needs to be.
	observedConfirmed
	// observedReplaced: the value landed AND retired an older one, which is
	// the case an undo exists for.
	observedReplaced
)

// applyObservedField writes one dated statement of a single-answer field,
// superseding what is older. A phone is a list and goes through
// applyObservedNumbers instead.
//
// The sidecar row is the decision: its ON CONFLICT carries the date comparison,
// so a column mirror below runs only when the sidecar actually moved. That is
// why the column no longer needs an emptiness predicate of its own — the
// question "is this newer" was already asked and answered in one statement,
// where two racing passes cannot both win it.
//
// The contact's own row is what the column write must not resurrect, so it keeps
// archived_at: the sidecar writer holds the subject live, but a mirror is a
// second statement and Art. 17 erasure can commit between them.
func applyObservedField(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, f observedField) (observedOutcome, error) {
	// The subject before any row this transaction writes, and the write
	// authority before any of it: Art. 17 erasure holds the subject and then
	// deletes what hangs off it, so taking them the other way round deadlocks
	// against the eraser and fails an erasure when it loses.
	if err := auth.HoldWritableLive(ctx, tx, "contact", contactID.UUID); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return observedSkipped, nil
		}
		return observedSkipped, err
	}

	// A column a human typed into leaves no sidecar row, so the value about to
	// be replaced would otherwise be lost to the undo buffer. Read it first and
	// seed the row with it: the supersede clause keeps whatever the row already
	// carried, and this is what the row carries when nothing wrote it yet.
	seeded, err := seedFromColumn(ctx, tx, contactID, f)
	if err != nil {
		return observedSkipped, err
	}

	// Nil rather than the zero time, so the statement below takes the
	// transaction's own clock. A zero time.Time sent as a timestamp is year 1,
	// which is older than every row on the table — it would supersede nothing
	// and look exactly like a statement that simply lost.
	var observedAt *time.Time
	if !f.ObservedAt.IsZero() {
		at := f.ObservedAt
		observedAt = &at
	}
	landed, err := writeContactProfileField(ctx, tx, contactID, contactProfileFieldRow{
		Field: f.Field, Value: f.Value, EvidenceSnippet: f.Evidence, SourceRef: f.SourceRef,
		Source: f.Source, CapturedBy: f.CapturedBy, Confidence: f.Confidence,
		ObservedAt: observedAt, Superseded: seeded,
	}, supersedeOnNewerObservation)
	if err != nil || !landed {
		return observedSkipped, err
	}

	column, mirrored := observedFieldColumn(f.Field)
	if !mirrored {
		// role, linkedin, company_name, website: no column to fill. company_name in
		// particular must never touch a company — the promotion pass
		// weighs these rows and decides that separately.
		return observedApplied, nil
	}
	tag, err := tx.Exec(ctx, `
		UPDATE contact SET `+column+` = $2 WHERE id = $1 AND archived_at IS NULL`, contactID, f.Value)
	if err != nil {
		return observedSkipped, fmt.Errorf("contacts: observed %s fill: %w", f.Field, err)
	}
	if tag.RowsAffected() == 0 {
		// The subject went between the two statements: the evidence row must
		// not claim a value the record does not carry.
		return observedSkipped, revokeSignatureEvidence(ctx, tx, contactID, f.Field)
	}
	return observedApplied, nil
}

// observedFieldColumn maps a field to the contact column that displays it, when
// one exists.
//
// address is deliberately absent. The contact carries six structured address
// columns and both readers here produce ONE flattened line — the vCard parser
// drops the PO box and extended parts on purpose — so a mirror would have to
// guess which column the line belongs in, and would write a street into a
// country as readily as not.
func observedFieldColumn(field string) (string, bool) {
	if field == fieldTitle {
		return fieldTitle, true
	}
	return "", false
}

// seedFromColumn reads the value a mirror COLUMN holds when the sidecar does not
// already account for it, so a value somebody typed survives into the undo
// buffer.
//
// The column is the authority here, not the sidecar. An edit through
// UpdateContact writes contact.title and leaves this table untouched, so a stale
// sidecar row can sit beside a title nobody here wrote — and seeding only when
// the sidecar is ABSENT would then record the stale row's value as the thing
// replaced, quietly losing what the human actually typed. Comparing the two is
// what tells those apart: a column that disagrees with the sidecar was written
// by somebody else, and it is the value a reader wants back.
func seedFromColumn(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, f observedField) (string, error) {
	column, mirrored := observedFieldColumn(f.Field)
	if !mirrored {
		return "", nil
	}
	var current, sidecar *string
	if err := tx.QueryRow(ctx, `
		SELECT p.`+column+`,
		       (SELECT f.value FROM contact_profile_field f
		         WHERE f.contact_id = p.id AND f.field = $2)
		FROM contact p
		WHERE p.id = $1 AND p.archived_at IS NULL`,
		contactID, f.Field).Scan(&current, &sidecar); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("contacts: reading %s before it is superseded: %w", f.Field, err)
	}
	if current == nil || *current == f.Value {
		return "", nil
	}
	if sidecar != nil && *sidecar == *current {
		// The sidecar already holds what the column shows, so the row's own
		// value is the honest record of what is being replaced and the conflict
		// clause will keep it.
		return "", nil
	}
	return *current, nil
}
