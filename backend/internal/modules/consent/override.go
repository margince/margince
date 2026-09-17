// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// A rep vouching that we may write to a contact the engine refused for lack of
// evidence. It is not consent and not a basis; it flips only a machine-level,
// non-absolute refusal for the one category it names, and a subject stop still
// wins at the gate.
//
// The door mirrors Suppress (suppress.go) in almost every particular — same
// subject reach, same authority source, same write shape — with three
// deliberate differences:
//
//   - the reason is REQUIRED here. Suppress may relay a phone call with
//     nothing more to add; this write is the rep's own judgement call, and a
//     vouch that flips a refusal is the write most worth being able to
//     explain later.
//   - the field is a CATEGORY, validated against the engine's own send
//     vocabulary (commsauthz.Category), not a suppression kind.
//   - the level is the seat's OWN authority for every case. Suppress has an
//     objection special-case that stamps LevelSubject because Art. 21 makes
//     the objection the subject's act whoever types it; an override has no
//     such act to defer to — every row here is a rep staking their own name
//     on the send, so it always carries the seat's level.
import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// AllowInput is a rep vouching that a machine-level refusal in one category
// may be overruled for this contact.
type AllowInput struct {
	// ContactID is the only subject this door takes, for the same reason
	// SuppressInput is contact-only: a lead has no consent surface yet, and a
	// field naming a subject the authorization check cannot name would be a
	// door claiming a reach it does not have.
	ContactID ids.ContactID
	// Category is which send category this vouch covers. The engine resolves
	// every send to exactly one category, and the override applies to that
	// one only — a vouch for marketing says nothing about customer_service.
	Category string
	// Reason is why the rep is vouching, in their own words. Mandatory here,
	// unlike Suppress: this write overrules the engine's own answer, and the
	// record must say why a human decided to.
	Reason string
}

// auditFieldOverrideCategory is a DIFFERENT key from the wire field's own
// name "category", deliberately: "category" already names a field audited by
// other writers (activities/documents.go among them), and the same bare key
// from two writers would mislabel whichever ships second — the same
// collision auditFieldSuppressionKind avoids for "kind".
const auditFieldOverrideCategory = "override_category"

// Allow records a standing vouch that a machine-level refusal for one
// category may be overruled for this contact.
//
// The verb is deliberately narrow: it writes one row, at the caller's own
// authority level, for the one category it names. It grants no consent and
// creates no lawful basis — decideOne (authorizetransmit.go) reads it, and
// reads it only when the decision it is weighing is already CanBeOverruled.
func (s *Store) Allow(ctx context.Context, in AllowInput) error {
	sub, level, err := admitAllow(ctx, in)
	if err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		return s.allowAdmittedTx(ctx, tx, in, sub, level)
	})
}

// admitAllow settles everything decidable before a connection is taken: the
// subject, the category, the reason, and that this caller may write about
// that subject at all.
func admitAllow(ctx context.Context, in AllowInput) (subject, commsauthz.AuthorityLevel, error) {
	sub, err := consentSubject(RecordInput{ContactID: in.ContactID})
	if err != nil {
		return subject{}, "", err
	}
	if !commsauthz.Category(in.Category).KnownForOverride() {
		return subject{}, "", &ValidationError{
			Field:  "category",
			Reason: "name a category the engine can resolve a send to",
		}
	}
	// The FULL requirement, not the bound alone: unlike a suppression, a rep
	// relaying a phone call has nothing to add on this door — every row here
	// is a rep's own judgement call, and the record must say why.
	if err := requireReason(in.Reason, "recording an override"); err != nil {
		return subject{}, "", err
	}
	// "contact" as a literal for the same reason admitSuppress uses one: this
	// door reaches a contact route only, and a computed object name is a door
	// grantreachability_test.go cannot resolve — an authorization gate
	// nothing can scan is exactly the one worth keeping scannable.
	if err := auth.Require(ctx, "contact", principal.ActionUpdate); err != nil {
		return subject{}, "", err
	}
	// ALWAYS the seat's own authority — no objection-shaped special case.
	// Suppress stamps LevelSubject for an Art. 21 objection because that act
	// belongs to the subject however it reaches the record; an override has
	// no such act behind it, so it always carries the level of whoever is
	// vouching.
	return sub, authorityOf(ctx), nil
}

// allowAdmittedTx writes the row, its audit entry and its event together.
func (s *Store) allowAdmittedTx(
	ctx context.Context, tx pgx.Tx, in AllowInput, sub subject, level commsauthz.AuthorityLevel,
) error {
	// SERIALISED WITH EVERY OTHER WRITER OF THIS SUBJECT'S STOPS AND
	// OVERRIDES. lockSubjectSuppressions is reused rather than a dedicated
	// lock: its key is hashtextextended over the SUBJECT's uuid alone, naming
	// no table, so an override and a stop already queue behind the same key —
	// which is what a merge carrying both onto a survivor needs, and what the
	// suppress/lift pair already gets from sharing it.
	//
	// BEFORE EnsureWritable, for the reason suppressAdmittedTx gives: a merge
	// holds the contact row locked while it reaches for this same advisory
	// lock, and taking them the other way round inverts the order between the
	// two transactions and deadlocks.
	if err := lockSubjectSuppressions(ctx, tx, sub.id); err != nil {
		return err
	}
	// auth.EnsureWritable: row scope, capture privacy and write authority
	// together, the same probe suppressAdmittedTx runs. Not EnsureWritableLive
	// — an override stays recordable against an archived subject for the same
	// reason a suppression does: it is a fact about a decision made, not a
	// live-only convenience.
	if err := auth.EnsureWritable(ctx, tx, sub.entityType, sub.id); err != nil {
		return err
	}
	// SETTLED AGAINST A MERGE, after EnsureWritable and for the reason
	// suppressAdmittedTx settles it there: no reader of communication_override
	// walks merged_into_id, so an override written onto a retired id sits on a
	// record no send evaluates.
	subjectID, err := survivingSubject(ctx, tx, sub.id)
	if err != nil {
		return err
	}
	sub.id = subjectID
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}

	// ON CONFLICT DO NOTHING would be wrong here too: a second vouch is a
	// second occasion somebody decided to overrule the engine, possibly for a
	// different reason, and liveOverride (override_read.go) answers with the
	// strongest live row by AUTHORITY, so recording both is the honest answer.
	var overrideID ids.UUID
	if err = tx.QueryRow(ctx, `
		INSERT INTO communication_override
		    (`+sub.column+`, category, reason, decided_by_level, captured_by)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`,
		sub.id, in.Category, in.Reason, string(level), by).Scan(&overrideID); err != nil {
		return fmt.Errorf("consent: recording the override: %w", err)
	}

	// "update" on the contact, matching suppressAdmittedTx: a human changing
	// what may be sent to somebody, not a scrub — AuditEvent rather than
	// Audit because this is a new fact with no before-image to diff against.
	auditID, err := storekit.AuditEvent(ctx, tx, "update", sub.entityType, sub.id,
		map[string]any{auditFieldOverrideCategory: in.Category, auditFieldDecidedByLevel: string(level)})
	if err != nil {
		return err
	}
	// EmitEvent: the payload declares a STATIC entity, this door writes about
	// a contact and only a contact, so the fan-out gate resolves delivery
	// scope from the type rather than a runtime subject somebody ratifies by
	// hand. The reason stays OFF the payload, as suppress.go keeps it off
	// theirs: it is the rep's own explanation to whoever reviews this
	// contact's history, not something every subscriber needs to receive.
	return storekit.EmitEvent(ctx, tx, auditID, sub.id,
		overrideRecordedPayload(overrideID, in.Category, level))
}

// overrideRecordedPayload names WHICH row was written, what was vouched for and
// at which authority.
//
// The id is on the payload because it is the only place a caller ever learns it:
// the door answers 204 with no body, and the revoke door takes that id in its
// path. Without it a rep could record a vouch and never be able to take it back.
// The reason stays off, as suppress.go keeps it off theirs.
func overrideRecordedPayload(
	id ids.UUID, category string, level commsauthz.AuthorityLevel,
) crmcontracts.PublicEventConsentOverrideRecorded {
	return crmcontracts.PublicEventConsentOverrideRecorded{
		OverrideId:     openapi_types.UUID(id),
		Category:       category,
		DecidedByLevel: string(level),
	}
}

// RevokeOverrideInput names the override to take back and why.
type RevokeOverrideInput struct {
	ContactID ids.ContactID
	// OverrideID is the row, not the contact: a subject may carry more than one
	// vouch — one per category, sometimes several over time — and revoking "the
	// override" would silently take back whichever the query happened to return
	// first.
	OverrideID ids.UUID
	// Reason is why it is being revoked, in the revoker's own words. Required,
	// the same asymmetry requireReason states for a lift: a vouch that gets
	// taken back is the write most worth being able to explain later.
	Reason string
}

// RevokeOverride revokes one standing override, if this caller's level may
// revoke the one that recorded it.
//
// The shape mirrors Lift in subject reach, authority source and write shape,
// against communication_override rather than communication_suppression. The one
// difference from Lift is the rule it asks: commsauthz.AuthorityLevel.CanRevoke,
// not CanOverrule — a vouch is the one decision an admin may take back from a
// peer admin, because admin is the top human authority and nothing higher exists
// to reach an admin-recorded override. The suppression Lift keeps CanOverrule on
// purpose: a stop erring toward not-sending is the safe direction, so an
// admin-recorded stop needs no admin-revokes-admin escape.
func (s *Store) RevokeOverride(ctx context.Context, in RevokeOverrideInput) error {
	sub, level, err := admitRevokeOverride(ctx, in)
	if err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		return s.revokeOverrideAdmittedTx(ctx, tx, in, sub, level)
	})
}

// admitRevokeOverride settles what is decidable before a connection is taken.
func admitRevokeOverride(ctx context.Context, in RevokeOverrideInput) (subject, commsauthz.AuthorityLevel, error) {
	sub, err := consentSubject(RecordInput{ContactID: in.ContactID})
	if err != nil {
		return subject{}, "", err
	}
	if in.OverrideID.IsZero() {
		return subject{}, "", &ValidationError{
			Field:  "override_id",
			Reason: "name the override to revoke; a subject may carry more than one",
		}
	}
	if err := requireReason(in.Reason, "revoking an override"); err != nil {
		return subject{}, "", err
	}
	if err := auth.Require(ctx, "contact", principal.ActionUpdate); err != nil {
		return subject{}, "", err
	}
	return sub, authorityOf(ctx), nil
}

// revokeOverrideAdmittedTx reads the row's authority, compares it, and revokes.
func (s *Store) revokeOverrideAdmittedTx(
	ctx context.Context, tx pgx.Tx, in RevokeOverrideInput, sub subject, level commsauthz.AuthorityLevel,
) error {
	// FIRST, before anything that reads the subject's row, and for the same
	// deadlock-ordering reason liftAdmittedTx gives: a merge holds the contact
	// row locked while it reaches for this same advisory lock. lockSubjectSuppressions
	// is reused rather than a dedicated lock because its key names no table —
	// an override queues behind the same key a stop already does.
	if err := lockSubjectSuppressions(ctx, tx, sub.id); err != nil {
		return err
	}
	// EnsureRetractable, which IS EnsureWritable and says so: this write
	// RELEASES rather than adds, and it reaches an archived subject on purpose.
	if err := auth.EnsureRetractable(ctx, tx, sub.entityType, sub.id); err != nil {
		return err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}

	var decided string
	err = tx.QueryRow(ctx, `
		SELECT decided_by_level FROM communication_override
		 WHERE id = $1 AND contact_id = $2 AND revoked_at IS NULL
		 FOR UPDATE`, in.OverrideID, sub.id).Scan(&decided)
	if errors.Is(err, pgx.ErrNoRows) {
		// A row that is already revoked, belongs to another subject, or never
		// existed all answer alike: a caller learns nothing about rows they were
		// not going to be allowed to touch.
		return apperrors.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("consent: reading the override: %w", err)
	}

	// CanRevoke, not CanOverrule: taking back a vouch is the one place an admin
	// may act on a peer admin's row, because admin is the top human authority and
	// no higher seat exists to reach an admin-recorded override. A subject-level
	// decision stays beyond every seat — CanRevoke keeps that square closed.
	if !level.CanRevoke(commsauthz.AuthorityLevel(decided)) {
		return fmt.Errorf(
			"this override was recorded at a level you may not revoke: %w", apperrors.ErrPermissionDenied)
	}

	if _, err = tx.Exec(ctx, `
		UPDATE communication_override
		   SET revoked_at = now()
		 WHERE id = $1 AND revoked_at IS NULL`, in.OverrideID); err != nil {
		return fmt.Errorf("consent: revoking the override: %w", err)
	}

	auditID, err := storekit.AuditEvent(ctx, tx, "update", sub.entityType, sub.id,
		map[string]any{
			"revoked_override":  in.OverrideID.String(),
			"recorded_at_level": decided,
			"revoked_by_level":  string(level),
			"revoked_by":        by,
			// The REVOKER's words, and the audit entry is their home — the same
			// split liftAdmittedTx keeps: the rep's own reason for vouching stays
			// on the override row, and this is the installation explaining why it
			// took the vouch back.
			fieldReason: in.Reason,
		})
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, sub.id,
		overrideLiftedPayload(in.OverrideID, commsauthz.AuthorityLevel(decided), level))
}

// overrideLiftedPayload names which override was revoked, at which authority it
// was recorded, and at whose it was taken back. It carries BOTH levels so a
// subscriber can see the revoker was allowed to take the recorder's row back without
// joining a row that no longer says so — the same pairing
// suppressionLiftedPayload keeps. It still carries neither the category the
// override covered nor the reason either party gave: a consumer wanting the
// category reads the still-live communication_override rows for this contact,
// and the words belong to the contacts who wrote them.
func overrideLiftedPayload(
	revoked ids.UUID, recordedAtLevel, by commsauthz.AuthorityLevel,
) crmcontracts.PublicEventConsentOverrideLifted {
	return crmcontracts.PublicEventConsentOverrideLifted{
		OverrideId:      openapi_types.UUID(revoked),
		RecordedAtLevel: string(recordedAtLevel),
		RevokedByLevel:  string(by),
	}
}
