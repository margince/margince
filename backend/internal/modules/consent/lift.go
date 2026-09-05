// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Lifting a suppression, and who may.
//
// Writing one was the previous change; until this one nothing could take it
// back, so every stop a rep recorded was permanent whatever level it carried.
// That made the level a claim rather than a rule — the contract said an admin
// could lift a rep's row and no code could.
//
// ONE RULE, and it is commsauthz.AuthorityLevel.CanOverrule: you may lift a
// decision made BELOW your level, never at or above it. This file asks that
// question and does not re-answer it. A second comparison here — a rank check,
// a role switch, an "is admin" shortcut — would be the copy that stops matching
// the day a level is added, and the one that stops matching decides whether
// mail reaches somebody who asked us to stop.
//
// The subject's own act is the tier nothing satisfies. An Art. 21 objection and
// a withdrawal are theirs, not the installation's, so no seat lifts them and
// this door does not offer a way to try.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// LiftInput names the suppression to take back and why.
type LiftInput struct {
	PersonID ids.PersonID
	// SuppressionID is the row, not the person: a subject may carry more than
	// one stop, and lifting "the suppression" would silently take back whichever
	// the query happened to return first.
	SuppressionID ids.UUID
	// Reason is why it is being lifted, in the lifter's words. A stop that gets
	// taken back is the change most worth explaining — the row said somebody
	// asked us not to write, and this says why we are writing again.
	Reason string
}

// Lift revokes one suppression, if this caller outranks the level that wrote it.
func (s *Store) Lift(ctx context.Context, in LiftInput) error {
	sub, level, err := admitLift(ctx, in)
	if err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		return s.liftAdmittedTx(ctx, tx, in, sub, level)
	})
}

// admitLift settles what is decidable before a connection is taken.
func admitLift(ctx context.Context, in LiftInput) (subject, commsauthz.AuthorityLevel, error) {
	sub, err := consentSubject(RecordInput{PersonID: in.PersonID})
	if err != nil {
		return subject{}, "", err
	}
	if in.SuppressionID.IsZero() {
		return subject{}, "", &ValidationError{
			Field:  "suppression_id",
			Reason: "name the suppression to lift; a subject may carry more than one",
		}
	}
	if strings.TrimSpace(in.Reason) == "" {
		return subject{}, "", &ValidationError{
			Field:  fieldReason,
			Reason: "taking back a stop needs a reason somebody can review later",
		}
	}
	if len([]rune(in.Reason)) > liftReasonMax {
		return subject{}, "", &ValidationError{
			Field: fieldReason,
			Reason: fmt.Sprintf("a reason is at most %d characters; this one is %d",
				liftReasonMax, len([]rune(in.Reason))),
		}
	}
	if err := auth.Require(ctx, "person", principal.ActionUpdate); err != nil {
		return subject{}, "", err
	}
	return sub, authorityOf(ctx), nil
}

// fieldReason names the reason field once, so the two refusals and the audit
// payload spell it the same way.
const fieldReason = "reason"

// liftReasonMax bounds the reason. It is the contract's own maxLength written
// again here because nothing generated enforces it — unchecked, one caller
// stores a megabyte in an audit payload every later reader of this person's
// history is served in full.
const liftReasonMax = 500

// liftAdmittedTx reads the row's authority, compares it, and revokes.
func (s *Store) liftAdmittedTx(
	ctx context.Context, tx pgx.Tx, in LiftInput, sub subject, level commsauthz.AuthorityLevel,
) error {
	// EnsureRetractable, which IS EnsureWritable and says so: this is a write
	// that RELEASES rather than adds, and it reaches an archived subject on
	// purpose. The named spelling is what distinguishes that deliberate reach
	// from a live filter somebody forgot.
	if err := auth.EnsureRetractable(ctx, tx, sub.entityType, sub.id); err != nil {
		return err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}

	// The row is read INSIDE the transaction that revokes it, so the level this
	// decision rests on is the level still on the row when the write lands. Read
	// outside and a concurrent lift could change it between the check and the
	// update — the shape that lets a rep's request act on an admin's answer.
	var decided string
	err = tx.QueryRow(ctx, `
		SELECT decided_by_level FROM communication_suppression
		 WHERE id = $1 AND person_id = $2 AND revoked_at IS NULL
		 FOR UPDATE`, in.SuppressionID, sub.id).Scan(&decided)
	if errors.Is(err, pgx.ErrNoRows) {
		// A row that is already revoked, belongs to another subject, or never
		// existed all answer alike: a caller learns nothing about rows they were
		// not going to be allowed to touch.
		return apperrors.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("consent: reading the suppression: %w", err)
	}

	if !level.CanOverrule(commsauthz.AuthorityLevel(decided)) {
		return fmt.Errorf(
			"this stop was recorded at a level you do not outrank: %w", apperrors.ErrPermissionDenied)
	}

	if _, err = tx.Exec(ctx, `
		UPDATE communication_suppression
		   SET revoked_at = now()
		 WHERE id = $1 AND revoked_at IS NULL`, in.SuppressionID); err != nil {
		return fmt.Errorf("consent: revoking the suppression: %w", err)
	}

	auditID, err := storekit.AuditEvent(ctx, tx, "update", sub.entityType, sub.id,
		map[string]any{
			"lifted_suppression": in.SuppressionID.String(),
			"recorded_at_level":  decided,
			"lifted_by_level":    string(level),
			"lifted_by":          by,
			// The LIFTER's words, and the audit entry is their home. Suppress
			// keeps the SUBJECT's reason off the audit payload because those
			// words are theirs; this reason is the installation explaining
			// itself, and the trail is exactly who should be able to read it.
			fieldReason: in.Reason,
		})
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, sub.id, suppressionLiftedPayload(decided, level))
}

// suppressionLiftedPayload says a stop was taken back and by whose authority.
//
// It carries the two levels rather than the reason, for the same reason the
// recording event does: the words belong to the people who wrote them, and an
// event reaches readers the explanation was not given to.
func suppressionLiftedPayload(recorded string, by commsauthz.AuthorityLevel) crmcontracts.PublicEventConsentSuppressionLifted {
	return crmcontracts.PublicEventConsentSuppressionLifted{
		RecordedAtLevel: recorded,
		LiftedByLevel:   string(by),
	}
}
