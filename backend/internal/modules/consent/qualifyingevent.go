// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Recording the one qualifying event nothing can derive.
//
// A `business_correspondence` verdict reads the most recent thing on the record
// that makes writing to somebody lawful, and three of the four kinds arrive on
// their own: an inbound message, an inquiry and an open deal are all facts the
// product already holds and the verdict derives them itself.
//
// The fourth happened away from every system. Somebody handed over a card at a
// trade fair, and the only record of it is the memory of the person who was
// there. This is where they write it down — and the note is required, because
// there is no message to cite and no deal to point at, so the note IS the
// evidence. A basis nobody can check is not accountability.
//
// It grants no marketing consent and could not: §7 UWG asks for express
// consent, and a card is not one. What it settles is the narrower question of
// whether an ordinary business email may be sent at all.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// kindInPerson is the one kind a human may assert. The other three are derived
// from records, and a hand-written one would be a second, unbacked answer to a
// question the data already settles.
//
// Held by: TestRecordingAnExchangeRefusesWhatItCannotStandBehind
// (backend/internal/modules/consent/qualifyingevent_integration_test.go), whose
// first case sends a derived kind and requires the refusal.
const kindInPerson = "in_person"

// noteMaxRunes bounds what the evidence may be. A sentence saying where a card
// changed hands is the whole need; a page of prose in this column is somebody
// using it as a notes field, and the record has one of those.
const noteMaxRunes = 500

// fieldNote is the field name a refusal about the note carries, which is what
// the surface highlights.
const fieldNote = "note"

// RecordQualifyingEventInput is one exchange, as the person who was there
// states it.
type RecordQualifyingEventInput struct {
	Kind       string
	Note       string
	OccurredAt time.Time
}

// RecordQualifyingEvent writes the exchange and returns it as it now stands.
//
// Gated on the SUBJECT rather than on a consent object: what this asserts is a
// fact about a person, it changes what may be sent to them, and the authority
// to make that assertion is the authority to write their record.
func (s *Store) RecordQualifyingEvent(
	ctx context.Context,
	personID ids.PersonID,
	in RecordQualifyingEventInput,
) (QualifyingEvent, error) {
	if in.Kind != kindInPerson {
		return QualifyingEvent{}, &InvalidQualifyingEventError{
			Field: "kind", Reason: "only an in-person exchange is recorded by hand; the rest are derived",
		}
	}
	note := strings.TrimSpace(in.Note)
	if note == "" {
		return QualifyingEvent{}, &InvalidQualifyingEventError{
			Field: fieldNote, Reason: "an in-person exchange has no other evidence, so it needs one",
		}
	}
	if len([]rune(note)) > noteMaxRunes {
		return QualifyingEvent{}, &InvalidQualifyingEventError{
			Field: fieldNote, Reason: fmt.Sprintf("keep it under %d characters", noteMaxRunes),
		}
	}
	if in.OccurredAt.IsZero() {
		return QualifyingEvent{}, &InvalidQualifyingEventError{
			Field: "occurred_at", Reason: "say when it happened, not when it was typed in",
		}
	}
	if err := auth.Require(ctx, "person", principal.ActionUpdate); err != nil {
		return QualifyingEvent{}, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return QualifyingEvent{}, err
	}

	out := QualifyingEvent{Kind: kindInPerson, Note: note, OccurredAt: in.OccurredAt}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		// The row-scope probe first: this changes what may be SENT to a person,
		// which is a change to their record however little of the row it
		// touches.
		if err := auth.EnsureWritableLive(ctx, tx, "person", personID.UUID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO consent_qualifying_event
				(person_id, kind, note, occurred_at, source, captured_by)
			VALUES ($1, $2, $3, $4, 'human', $5)`,
			personID, kindInPerson, note, in.OccurredAt, by); err != nil {
			return fmt.Errorf("consent: recording the in-person exchange: %w", err)
		}
		// Audit-only, like every other consent-basis write beside it: the
		// closed public-event catalog carries no type for a lawful basis, and
		// what an auditor asks — who said this happened, and what did they say
		// — is a row question rather than a subscriber's.
		_, err := storekit.AuditEvent(ctx, tx, "create", "person_consent", personID.UUID, map[string]any{
			"qualifying_event": kindInPerson,
			fieldNote:          note,
			"occurred_at":      in.OccurredAt.UTC().Format(time.RFC3339),
		})
		return err
	})
	if err != nil {
		return QualifyingEvent{}, err
	}
	return out, nil
}

// InvalidQualifyingEventError refuses a claim the record could not stand
// behind, naming the field so the surface can point at it.
type InvalidQualifyingEventError struct {
	Field  string
	Reason string
}

func (e *InvalidQualifyingEventError) Error() string { return e.Field + ": " + e.Reason }

// FieldFault carries the verdict to every surface, as this module's other
// refusals do.
func (e *InvalidQualifyingEventError) FieldFault() (field, code, message string) {
	return e.Field, "invalid", e.Reason
}
