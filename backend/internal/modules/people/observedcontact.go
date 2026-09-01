// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Applying what the person themselves stated, on a date.
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
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// The rest of the shared vocabulary: the fields a card and a signature both
// state, as constants so a typo cannot file a value under a key no reader looks
// for. fieldTitle and fieldPhone are declared where their own writers first
// needed them.
const (
	fieldRole     = "role"
	fieldOrgName  = "org_name"
	fieldAddress  = "address"
	fieldLinkedin = "linkedin"
	fieldWebsite  = "website"
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

	// Nil rather than the zero time, so the statement below takes the
	// transaction's own clock. A zero time.Time sent as a timestamp is year 1,
	// which is older than every row on the table — it would supersede nothing
	// and look exactly like a statement that simply lost.
	var observedAt *time.Time
	if !f.ObservedAt.IsZero() {
		at := f.ObservedAt
		observedAt = &at
	}
	landed, err := writePersonProfileField(ctx, tx, personID, personProfileFieldRow{
		Field: f.Field, Value: f.Value, EvidenceSnippet: f.Evidence, SourceRef: f.SourceRef,
		Source: f.Source, CapturedBy: f.CapturedBy, Confidence: f.Confidence,
		ObservedAt: observedAt, Superseded: seeded,
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
	// The transaction's own clock when the caller states no date — a card
	// carries none. Read from the database rather than the process so it
	// compares against the stored dates on the same clock that wrote them, and
	// so the four statements below all use ONE date rather than drifting apart
	// across the transaction.
	observedAt := p.ObservedAt
	if observedAt.IsZero() {
		if err := tx.QueryRow(ctx, `SELECT now()`).Scan(&observedAt); err != nil {
			return observedSkipped, fmt.Errorf("people: dating an undated statement: %w", err)
		}
	}

	// A number the record already carries is confirmed rather than duplicated,
	// and either way the caller is done with it.
	known, err := confirmKnownNumber(ctx, tx, personID, normalized, observedAt)
	if err != nil || known != observedSkipped {
		return known, err
	}

	// A number of this type stated LATER than this one already stands, so this
	// statement is stale and adds nothing. Asked before the replace below,
	// because "no older row to replace" and "a newer row already answers" are
	// different situations that would otherwise both end in an insert: a
	// re-delivered old mail would file its number beside the current one, and
	// the record would carry two work numbers with no way to tell which rings.
	// STRICTLY newer, not "at least as new". A business card states two work
	// numbers as one statement and they share its date; > would let the first
	// one written reject the second as already superseded, and the card would
	// silently import half its numbers. A tie is two numbers the person gave
	// together, which is a person with two numbers.
	var newerStands bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM person_phone
			WHERE person_id = $1 AND phone_type = $2 AND archived_at IS NULL
			  AND observed_at > $3)`,
		personID, phoneType, observedAt).Scan(&newerStands); err != nil {
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
		personID, phoneType, observedAt).Scan(&supersededID); err != nil &&
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
		personID, normalized, phoneType, p.Source, p.CapturedBy, observedAt, supersededID)
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

// observedCard is one business card, as a dated statement by the person on it.
type observedCard struct {
	Entry      VCardEntry
	Evidence   string
	SourceRef  string
	Source     string
	CapturedBy string
	// ObservedAt is when the card was handed over. Zero means now, which is the
	// honest answer for an import: a .vcf carries no date of its own, and the
	// moment a human chose to import it is what there is.
	ObservedAt time.Time
}

// applyObservedCard writes everything a card states, and reports which fields
// moved so the caller can audit them.
//
// The same writer the signature pass uses, deliberately: a card and a signature
// are one kind of input — the contact describing themselves — and two writers
// would let the ARRIVAL ROUTE decide whether a stale number gets corrected.
//
// Emails are the exception and stay additive. An address the card omits is not
// an address retired, and no automatic path may take away a way to reach
// somebody; the card's own addresses are added on the create path and left
// alone here.
func applyObservedCard(ctx context.Context, tx pgx.Tx, personID ids.PersonID, c observedCard) ([]string, error) {
	var applied []string
	for _, f := range cardFields(c.Entry) {
		outcome, err := applyObservedField(ctx, tx, personID, observedField{
			Field: f.name, Value: f.value, Evidence: c.Evidence, SourceRef: c.SourceRef,
			Source: c.Source, CapturedBy: c.CapturedBy, ObservedAt: c.ObservedAt,
		})
		if err != nil {
			return nil, err
		}
		if outcome == observedApplied {
			applied = append(applied, f.name)
		}
	}
	wroteEvidence := false
	for _, phone := range c.Entry.Phones {
		outcome, err := applyObservedPhone(ctx, tx, personID, observedPhone{
			Phone: phone.Value, PhoneType: phone.Kind, SourceRef: c.SourceRef,
			Source: c.Source, CapturedBy: c.CapturedBy, ObservedAt: c.ObservedAt,
		})
		if err != nil {
			return nil, err
		}
		if outcome != observedApplied {
			continue
		}
		if !slices.Contains(applied, fieldPhone) {
			applied = append(applied, fieldPhone)
		}
		// The evidence line for the number, written once for the first number
		// this card LANDS: the sidecar holds one row per field while a card may
		// state several numbers, so writing it per number would be the same row
		// overwritten and the undo would offer whichever happened to be last.
		// The signature pass writes this line too, and it is what gives a
		// replaced number something to undo.
		if !wroteEvidence {
			wroteEvidence = true
			if _, err := applyObservedField(ctx, tx, personID, observedField{
				Field: fieldPhone, Value: phone.Value, Evidence: c.Evidence,
				SourceRef: c.SourceRef, Source: c.Source, CapturedBy: c.CapturedBy,
				ObservedAt: c.ObservedAt,
			}); err != nil {
				return nil, err
			}
		}
	}
	return applied, nil
}

// cardField is one of the card's single-answer fields, named in the shared
// vocabulary.
type cardField struct{ name, value string }

// cardFields reads a card into that vocabulary, dropping what it does not
// state.
//
// TITLE fills `title` and NOT `role`. They are different claims — a title is
// what somebody is called, a role is what they do in a deal — and person360
// attaches role evidence to the employment role, so a card saying "VP Finance"
// would make an unrelated role like "Decision maker" display with that as its
// evidence.
//
// URL is split rather than filed as linkedin, which is what the old import did
// to every card: a company website landed under a person's LinkedIn profile and
// the page then showed it as one. Host, not guesswork.
func cardFields(entry VCardEntry) []cardField {
	out := make([]cardField, 0, 5)
	add := func(name, value string) {
		if v := strings.TrimSpace(value); v != "" {
			out = append(out, cardField{name: name, value: v})
		}
	}
	add(fieldTitle, entry.Title)
	add(fieldOrgName, entry.Organization)
	add(fieldAddress, entry.Address)
	if u := strings.TrimSpace(entry.URL); u != "" {
		if isLinkedinURL(u) {
			add(fieldLinkedin, u)
		} else {
			add(fieldWebsite, u)
		}
	}
	return out
}

// isLinkedinURL reports whether a URL names a LinkedIn profile, by host rather
// than by substring: a website whose path merely mentions linkedin is not one.
//
// Subdomains count, because LinkedIn's own localized profile links carry them —
// de.linkedin.com and www.linkedin.com are the same site, and a reader who
// pasted either meant their profile. Hostname() rather than Host, so an
// explicit port does not turn a profile into a website.
func isLinkedinURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		// A bare "linkedin.com/in/x" with no scheme parses as a path.
		host, _, _ = strings.Cut(strings.ToLower(raw), "/")
		host, _, _ = strings.Cut(host, ":")
	}
	return host == "linkedin.com" || strings.HasSuffix(host, ".linkedin.com")
}
