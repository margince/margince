// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Applying what the person themselves stated, on a date.
//
// A mail signature is the contact describing their own details, at a moment
// that can be dated. A business card is the same kind of input and belongs
// here too; the card import still fills only blanks (vcardimport.go), so today
// which one arrives decides whether a stale number is corrected. Moving it is
// the next piece of this work, not a claim this file can make yet.
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
	"github.com/margince/margince/backend/internal/shared/kernel/values"
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
)

// applyObservedField writes one dated statement, superseding what is older.
//
// The sidecar row is the decision: its ON CONFLICT carries the date comparison,
// so a column mirror below runs only when the sidecar actually moved. That is
// why the column no longer needs an emptiness predicate of its own — the
// question "is this newer" was already asked and answered in one statement,
// where two racing passes cannot both win it.
//
// The person's own row is what the column write must not resurrect, so it keeps
// archived_at: the sidecar writer holds the subject live, but a mirror is a
// second statement and Art. 17 erasure can commit between them.
func applyObservedField(ctx context.Context, tx pgx.Tx, personID ids.PersonID, f observedField) (observedOutcome, error) {
	// The subject before any row this transaction writes, and the write
	// authority before any of it: Art. 17 erasure holds the subject and then
	// deletes what hangs off it, so taking them the other way round deadlocks
	// against the eraser and fails an erasure when it loses.
	if err := auth.HoldWritableLive(ctx, tx, "person", personID.UUID); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return observedSkipped, nil
		}
		return observedSkipped, err
	}

	// A column a human typed into leaves no sidecar row, so the value about to
	// be replaced would otherwise be lost to the undo buffer. Read it first and
	// seed the row with it: the supersede clause keeps whatever the row already
	// carried, and this is what the row carries when nothing wrote it yet.
	seeded, err := seedFromColumn(ctx, tx, personID, f)
	if err != nil {
		return observedSkipped, err
	}

	observedAt := f.ObservedAt
	landed, err := writePersonProfileField(ctx, tx, personID, personProfileFieldRow{
		Field: f.Field, Value: f.Value, EvidenceSnippet: f.Evidence, SourceRef: f.SourceRef,
		Source: f.Source, CapturedBy: f.CapturedBy, Confidence: f.Confidence,
		ObservedAt: &observedAt, Superseded: seeded,
	}, supersedeOnNewerObservation)
	if err != nil || !landed {
		return observedSkipped, err
	}

	column, mirrored := observedFieldColumn(f.Field)
	if !mirrored {
		// role, linkedin, org_name, website: no column to fill. org_name in
		// particular must never touch an organization — the promotion pass
		// weighs these rows and decides that separately.
		return observedApplied, nil
	}
	tag, err := tx.Exec(ctx, `
		UPDATE person SET `+column+` = $2 WHERE id = $1 AND archived_at IS NULL`, personID, f.Value)
	if err != nil {
		return observedSkipped, fmt.Errorf("people: observed %s fill: %w", f.Field, err)
	}
	if tag.RowsAffected() == 0 {
		// The subject went between the two statements: the evidence row must
		// not claim a value the record does not carry.
		return observedSkipped, revokeSignatureEvidence(ctx, tx, personID, f.Field)
	}
	return observedApplied, nil
}

// observedFieldColumn maps a field to the person column that displays it, when
// one exists.
//
// address is deliberately absent. The person carries six structured address
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
// UpdatePerson writes person.title and leaves this table untouched, so a stale
// sidecar row can sit beside a title nobody here wrote — and seeding only when
// the sidecar is ABSENT would then record the stale row's value as the thing
// replaced, quietly losing what the human actually typed. Comparing the two is
// what tells those apart: a column that disagrees with the sidecar was written
// by somebody else, and it is the value a reader wants back.
func seedFromColumn(ctx context.Context, tx pgx.Tx, personID ids.PersonID, f observedField) (string, error) {
	column, mirrored := observedFieldColumn(f.Field)
	if !mirrored {
		return "", nil
	}
	var current, sidecar *string
	if err := tx.QueryRow(ctx, `
		SELECT p.`+column+`,
		       (SELECT f.value FROM person_profile_field f
		         WHERE f.person_id = p.id AND f.field = $2)
		FROM person p
		WHERE p.id = $1 AND p.archived_at IS NULL`,
		personID, f.Field).Scan(&current, &sidecar); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("people: reading %s before it is superseded: %w", f.Field, err)
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

// observedPhone is one dated statement of a number.
type observedPhone struct {
	Phone      string
	PhoneType  string
	SourceRef  string
	Source     string
	CapturedBy string
	ObservedAt time.Time
}

// applyObservedPhone adds a number, or replaces the one of its type that is
// older than it.
//
// Additive across types on purpose: a mobile in a signature says nothing about
// the desk number, and a pass that replaced it would delete a working way to
// reach somebody on no evidence at all. Within one type it is recency, because
// two work numbers for one person is what a changed number looks like when
// nothing supersedes.
//
// An identical number still advances observed_at. The row then says "still true
// as of this date", which is what stops a late-delivered OLDER mail from
// winning afterwards — without it, a number confirmed a dozen times still
// carries the date of its first sighting.
func applyObservedPhone(ctx context.Context, tx pgx.Tx, personID ids.PersonID, p observedPhone) (observedOutcome, error) {
	parsed, err := values.ParsePhone(p.Phone)
	if err != nil {
		// Declined, not failed: a number this reader cannot parse is one field
		// skipped, exactly like an absent one. Propagating it would abandon the
		// other fields of the same signature over one contact's formatting,
		// which is the choice readSignatureValue already made for this shape.
		//nolint:nilerr // a footer this reader cannot parse is a skipped field, not a fault
		return observedSkipped, nil
	}
	// Held before the first child row, for applyObservedField's reason: the
	// eraser takes the subject first and this must not race it the other way.
	if err := auth.HoldWritableLive(ctx, tx, "person", personID.UUID); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return observedSkipped, nil
		}
		return observedSkipped, err
	}
	normalized := parsed.String()
	phoneType := p.PhoneType
	if phoneType == "" {
		phoneType = emailTypeWork
	}

	// A number the record already carries is confirmed rather than duplicated,
	// and either way the caller is done with it.
	known, err := confirmKnownNumber(ctx, tx, personID, normalized, p.ObservedAt)
	if err != nil || known != observedSkipped {
		return known, err
	}

	// A number of this type stated LATER than this one already stands, so this
	// statement is stale and adds nothing. Asked before the replace below,
	// because "no older row to replace" and "a newer row already answers" are
	// different situations that would otherwise both end in an insert: a
	// re-delivered old mail would file its number beside the current one, and
	// the record would carry two work numbers with no way to tell which rings.
	var newerStands bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM person_phone
			WHERE person_id = $1 AND phone_type = $2 AND archived_at IS NULL
			  AND observed_at >= $3)`,
		personID, phoneType, p.ObservedAt).Scan(&newerStands); err != nil {
		return observedSkipped, fmt.Errorf("people: looking for a number stated later: %w", err)
	}
	if newerStands {
		return observedSkipped, nil
	}

	// The row of this type this statement replaces, if it is older. Chosen by
	// id so the archive and the insert name the same row: several live numbers
	// of one type are permitted, and the primary is the one displayed.
	var supersededID *ids.UUID
	if err := tx.QueryRow(ctx, `
		SELECT id FROM person_phone
		WHERE person_id = $1 AND phone_type = $2 AND archived_at IS NULL AND observed_at < $3
		ORDER BY is_primary DESC, position, created_at
		LIMIT 1`,
		personID, phoneType, p.ObservedAt).Scan(&supersededID); err != nil &&
		!errors.Is(err, pgx.ErrNoRows) {
		return observedSkipped, fmt.Errorf("people: looking for the number this replaces: %w", err)
	}

	if supersededID != nil {
		if _, err := tx.Exec(ctx, `
			UPDATE person_phone SET archived_at = now() WHERE id = $1 AND archived_at IS NULL`,
			supersededID); err != nil {
			return observedSkipped, fmt.Errorf("people: retiring the replaced number: %w", err)
		}
	}

	// is_primary only when this type has no live primary left: the partial
	// unique index permits exactly one, and claiming it from a number this
	// statement said nothing about would silently re-rank the record.
	tag, err := tx.Exec(ctx, `
		INSERT INTO person_phone
		  (person_id, phone, phone_type, is_primary, position, source, captured_by, observed_at, superseded_phone_id)
		SELECT $1, $2, $3,
		  NOT EXISTS (
			SELECT 1 FROM person_phone
			WHERE person_id = $1 AND phone_type = $3 AND is_primary AND archived_at IS NULL),
		  COALESCE((SELECT MAX(position) + 1 FROM person_phone
			WHERE person_id = $1 AND archived_at IS NULL), 0),
		  $4, $5, $6, $7
		WHERE EXISTS (SELECT 1 FROM person WHERE id = $1 AND archived_at IS NULL)`,
		personID, normalized, phoneType, p.Source, p.CapturedBy, p.ObservedAt, supersededID)
	if err != nil {
		return observedSkipped, fmt.Errorf("people: writing the observed number: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return observedSkipped, nil
	}
	return observedApplied, nil
}

// confirmKnownNumber handles a number the record already carries: it advances
// the date rather than filing a duplicate, and reports observedSkipped when
// this is a number the caller still has to write.
//
// Advancing on an identical value is what makes the row say "still true as of
// this date". Without it a number confirmed a dozen times keeps the date of its
// first sighting, and a late-delivered OLDER mail would then outrank it.
func confirmKnownNumber(ctx context.Context, tx pgx.Tx, personID ids.PersonID, normalized string, observedAt time.Time) (observedOutcome, error) {
	tag, err := tx.Exec(ctx, `
		UPDATE person_phone SET observed_at = $3
		WHERE person_id = $1 AND phone = $2 AND archived_at IS NULL AND observed_at < $3`,
		personID, normalized, observedAt)
	if err != nil {
		return observedSkipped, fmt.Errorf("people: confirming a known number: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return observedApplied, nil
	}
	// The number is here but was already dated at or after this statement, so
	// there is nothing to advance and nothing to add.
	var live bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM person_phone
			WHERE person_id = $1 AND phone = $2 AND archived_at IS NULL)`,
		personID, normalized).Scan(&live); err != nil {
		return observedSkipped, fmt.Errorf("people: looking for a known number: %w", err)
	}
	if live {
		return observedConfirmed, nil
	}
	return observedSkipped, nil
}
