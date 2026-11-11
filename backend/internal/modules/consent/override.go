// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Taking back a standing exception: a rep's earlier vouch that a machine-level,
// non-absolute refusal for one category may be overruled for one contact. No
// door records a new one; a rep lets one refused message go with DirectSend.
// The rows already on file still count at the gate until they are revoked.
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
	// THEN the chain's own key, in that order and never the other way round.
	// The subject lock cannot hold the walk below still: a chain outlives the
	// subject it started on, and the merge that would extend it locks two
	// subjects this caller never named. overridechain.go carries that argument
	// and the lock order both.
	root, err := lockOverrideFamilyOf(ctx, tx, in.OverrideID)
	if err != nil {
		return err
	}
	// EnsureRetractable, which IS EnsureWritable and says so: this write
	// RELEASES rather than adds, and it reaches an archived subject on purpose.
	if err := auth.EnsureRetractable(ctx, tx, sub.entityType, sub.id); err != nil {
		return err
	}
	// SETTLED AGAINST A MERGE: the survivor is resolved so the chain-reach
	// check below accepts the vouch through either home the caller may hold.
	surviving, err := survivingSubject(ctx, tx, sub.id)
	if err != nil {
		return err
	}
	// THE SURVIVOR IS CHECKED TOO. The row accepted below may sit on the
	// survivor, a record this caller never named and whose scope the check
	// above never read: a seat that may write the retired contact but not the
	// one it merged into would otherwise take back the survivor's own vouches,
	// and learn from the answer which ids exist there. A plain read, so it adds
	// no lock the merge could cycle on.
	if surviving != sub.id {
		if err := auth.EnsureRetractable(ctx, tx, sub.entityType, surviving); err != nil {
			return err
		}
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}

	// The row must be live, and its chain must touch the contact named or that
	// contact's survivor. The chain is one subject's vouch under successive ids.
	// So the listed handle works through any id of that subject the caller holds.
	// A row whose chain touches no record the caller may reach stays hidden.
	var decided string
	err = tx.QueryRow(ctx, `
		WITH RECURSIVE chain AS (
		    SELECT id, contact_id FROM communication_override WHERE id IN ($4, $1)
		  UNION
		    SELECT carried.id, carried.contact_id
		      FROM communication_override carried
		      JOIN chain ON carried.carried_from = chain.id
		)
		SELECT decided_by_level FROM communication_override
		 WHERE id = $1 AND revoked_at IS NULL
		   AND EXISTS (SELECT 1 FROM chain WHERE contact_id IN ($2, $3))
		 FOR UPDATE`, in.OverrideID, sub.id, surviving, root).Scan(&decided)
	if errors.Is(err, pgx.ErrNoRows) {
		// A row that is already revoked, whose chain never touches the subject
		// named or its survivor, or that never existed all answer alike: a
		// caller learns nothing about rows they were not going to be allowed
		// to touch.
		return apperrors.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("consent: reading the override: %w", err)
	}

	// CanRevoke, not CanOverrule: taking back a vouch is the one place an admin
	// may act on a peer admin's row, because admin is the top human authority and
	// no higher seat exists to reach an admin-recorded override. A subject-level
	// decision stays beyond every seat — CanRevoke keeps that square closed.
	// An override holds a user's or an admin's level, and a user's revokes
	// neither. So the admin gate asks the same question, and records it.
	if !mayRevokeOverride(ctx, level, commsauthz.AuthorityLevel(decided)) {
		return fmt.Errorf("this override was recorded at a level you may not revoke: %w", apperrors.ErrPermissionDenied)
	}

	revoked, err := revokeOverrideChain(ctx, tx, root, in.OverrideID)
	if err != nil {
		return err
	}

	// The REVOKER's words ride every audit row — the same split liftAdmittedTx
	// keeps: the rep's own reason for vouching stays on the override row, and
	// this is the installation explaining why it took the vouch back.
	return auditAndEmitRevokedOverrides(ctx, tx, revoked, revokeNote{
		recordedAtLevel: commsauthz.AuthorityLevel(decided),
		revokedByLevel:  level,
		by:              by,
		reason:          in.Reason,
		rows:            len(revoked),
	})
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

func mayRevokeOverride(ctx context.Context, caller, recorded commsauthz.AuthorityLevel) bool {
	return auth.RequireAdmin(ctx) == nil && caller.CanRevoke(recorded)
}
