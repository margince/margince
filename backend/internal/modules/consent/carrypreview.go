// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

const stopCarryCandidates = `SELECT DISTINCT ON (live.kind, live.purpose_id)
 live.id, live.address, live.kind, live.source, live.decided_by_level, live.purpose_id
 FROM communication_suppression live
 WHERE ((@from_contact::uuid IS NOT NULL AND live.contact_id = @from_contact)
 OR (@from_lead::uuid IS NOT NULL AND live.lead_id = @from_lead))
 AND live.revoked_at IS NULL
 AND NOT EXISTS (SELECT 1 FROM communication_suppression held
 WHERE ((@to_contact::uuid IS NOT NULL AND held.contact_id = @to_contact)
 OR (@to_lead::uuid IS NOT NULL AND held.lead_id = @to_lead))
 AND held.kind = live.kind AND held.purpose_id IS NOT DISTINCT FROM live.purpose_id
 AND held.revoked_at IS NULL
 AND coalesce(array_position(@ladder::text[], held.decided_by_level), array_length(@ladder::text[], 1) + 1)
 >= coalesce(array_position(@ladder::text[], live.decided_by_level), array_length(@ladder::text[], 1) + 1))
 ORDER BY live.kind, live.purpose_id,
 coalesce(array_position(@ladder::text[], live.decided_by_level), array_length(@ladder::text[], 1) + 1) DESC,
 live.recorded_at DESC, live.id`

func carryAuthorityArgs(from, to commsauthz.StopSubject, by string) pgx.NamedArgs {
	return pgx.NamedArgs{"from_contact": zeroAsNull(from.ContactID.UUID), "from_lead": zeroAsNull(from.LeadID.UUID),
		"to_contact": zeroAsNull(to.ContactID.UUID), "to_lead": zeroAsNull(to.LeadID.UUID), "by": by, "ladder": authorityLadder()}
}

// A new contact has no row yet. Existing targets retain the carry's write scope;
// both subjects require read authority before the preview exposes their state.
func admitLeadCarryPreview(ctx context.Context, tx pgx.Tx, lead ids.LeadID, contact ids.ContactID) error {
	if err := auth.Require(ctx, entityLead, principal.ActionRead); err != nil {
		return err
	}
	if err := auth.Require(ctx, entityContact, principal.ActionRead); err != nil {
		return err
	}
	if err := auth.Require(ctx, entityLead, principal.ActionUpdate); err != nil {
		return err
	}
	if err := auth.Require(ctx, entityContact, principal.ActionCreate); err != nil {
		return err
	}
	if err := auth.EnsureWritable(ctx, tx, entityLead, lead.UUID); err != nil {
		return err
	}
	if !contact.IsZero() {
		return auth.EnsureWritable(ctx, tx, entityContact, contact.UUID)
	}
	return nil
}

func (s *Store) PreviewStopsTx(ctx context.Context, tx pgx.Tx, from, to commsauthz.StopSubject) ([]commsauthz.CarriedStop, error) {
	if err := admitCarryPreview(ctx, tx, from, to); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT candidates.id, kind, decided_by_level, purpose_id, coalesce(p.label, '') FROM (`+stopCarryCandidates+`) candidates LEFT JOIN consent_purpose p ON p.id = candidates.purpose_id ORDER BY kind, purpose_id LIMIT 501`,
		carryAuthorityArgs(from, to, ""))
	if err != nil {
		return nil, fmt.Errorf("read carried restrictions: %w", err)
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (commsauthz.CarriedStop, error) {
		var out commsauthz.CarriedStop
		err := row.Scan(&out.ID, &out.Kind, &out.Authority, &out.PurposeID, &out.PurposeLabel)
		return out, err
	})
	if err != nil {
		return nil, err
	}
	if len(out) > 500 {
		return nil, apperrors.ErrInvalidArgument
	}
	return out, nil
}

const carrySubjectPredicate = `(@from_contact::uuid IS NOT NULL AND contact_id = @from_contact)
 OR (@from_lead::uuid IS NOT NULL AND lead_id = @from_lead)`

func (s *Store) PreviewConsentLinksTx(ctx context.Context, tx pgx.Tx, from, to commsauthz.StopSubject) (commsauthz.CarriedConsentLinks, error) {
	if err := admitCarryPreview(ctx, tx, from, to); err != nil {
		return commsauthz.CarriedConsentLinks{}, err
	}
	if err := requireCarryEmailRead(ctx, from); err != nil {
		return commsauthz.CarriedConsentLinks{}, err
	}
	args := pgx.NamedArgs{"from_contact": zeroAsNull(from.ContactID.UUID), "from_lead": zeroAsNull(from.LeadID.UUID)}
	withdrawals, err := previewCarriedWithdrawals(ctx, tx, args)
	if err != nil {
		return commsauthz.CarriedConsentLinks{}, err
	}
	bases, err := previewCarriedBases(ctx, tx, args)
	if err != nil {
		return commsauthz.CarriedConsentLinks{}, err
	}
	out := commsauthz.CarriedConsentLinks{WithdrawalCredentials: withdrawals, CommunicationBases: bases}
	if !from.ContactID.IsZero() {
		out.Confirmations, err = previewCarriedConfirmations(ctx, tx, from.ContactID)
		if err != nil {
			return commsauthz.CarriedConsentLinks{}, err
		}
		out.DoubleOptIns, err = previewCarriedDoubleOptIns(ctx, tx, from.ContactID)
		if err != nil {
			return commsauthz.CarriedConsentLinks{}, err
		}
		out.QualifyingEvents, err = previewCarriedQualifyingEvents(ctx, tx, from.ContactID, to.ContactID)
		if err != nil {
			return commsauthz.CarriedConsentLinks{}, err
		}
	}
	return out, nil
}

func previewCarriedWithdrawals(ctx context.Context, tx pgx.Tx, args pgx.NamedArgs) ([]commsauthz.CarriedWithdrawalLink, error) {
	rows, err := tx.Query(ctx, `SELECT c.id, address, scope, p.label, issued_at, expires_at, revoked_at
 FROM withdrawal_credential c LEFT JOIN consent_purpose p ON p.id = c.purpose_id
 WHERE `+carrySubjectPredicate+` ORDER BY c.id LIMIT 501`, args)
	if err != nil {
		return nil, fmt.Errorf("read carried withdrawal links: %w", err)
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (commsauthz.CarriedWithdrawalLink, error) {
		var out commsauthz.CarriedWithdrawalLink
		err := row.Scan(&out.ID, &out.Address, &out.Scope, &out.PurposeLabel, &out.IssuedAt, &out.ExpiresAt, &out.RevokedAt)
		return out, err
	})
	if err != nil {
		return nil, err
	}
	if len(out) > 500 {
		return nil, apperrors.ErrInvalidArgument
	}
	return out, nil
}

func previewCarriedBases(ctx context.Context, tx pgx.Tx, args pgx.NamedArgs) ([]commsauthz.CarriedCommunicationBasis, error) {
	rows, err := tx.Query(ctx, `SELECT id, kind, source_activity_id, valid_from, valid_until, revoked_at
 FROM communication_basis WHERE `+carrySubjectPredicate+` ORDER BY id LIMIT 501`, args)
	if err != nil {
		return nil, fmt.Errorf("read carried communication bases: %w", err)
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (commsauthz.CarriedCommunicationBasis, error) {
		var out commsauthz.CarriedCommunicationBasis
		err := row.Scan(&out.ID, &out.Kind, &out.SourceActivityID, &out.ValidFrom, &out.ValidUntil, &out.RevokedAt)
		return out, err
	})
	if err != nil {
		return nil, err
	}
	if len(out) > 500 {
		return nil, apperrors.ErrInvalidArgument
	}
	for _, basis := range out {
		if basis.SourceActivityID != nil {
			if err := authorizeCarryEvidence(ctx, tx, "activity", *basis.SourceActivityID); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

func admitCarryPreview(ctx context.Context, tx pgx.Tx, from, to commsauthz.StopSubject) error {
	if from.IsZero() || (!from.ContactID.IsZero() && !from.LeadID.IsZero()) || (!to.ContactID.IsZero() && !to.LeadID.IsZero()) {
		return apperrors.ErrInvalidArgument
	}
	if !from.LeadID.IsZero() && to.LeadID.IsZero() {
		return admitLeadCarryPreview(ctx, tx, from.LeadID, to.ContactID)
	}
	if to.IsZero() {
		return apperrors.ErrInvalidArgument
	}
	if err := admitACarry(ctx, tx, from, to); err != nil {
		return err
	}
	for _, subject := range []commsauthz.StopSubject{from, to} {
		object := entityContact
		if !subject.LeadID.IsZero() {
			object = entityLead
		}
		if err := auth.Require(ctx, object, principal.ActionRead); err != nil {
			return err
		}
	}
	return nil
}

func requireCarryEmailRead(ctx context.Context, from commsauthz.StopSubject) error {
	object := entityContact
	if !from.LeadID.IsZero() {
		object = entityLead
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		return apperrors.ErrPermissionDenied
	}
	masked := auth.MaskedFields(actor, object, true)
	if slices.Contains(masked, "email") || slices.Contains(masked, "emails") {
		return apperrors.ErrPermissionDenied
	}
	return nil
}
