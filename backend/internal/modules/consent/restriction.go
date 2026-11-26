// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Recording a restriction of processing (GDPR Art. 18). The restriction is a
// processing_restriction stop, written by the same stop writer as every kind.
// It comes from the contact's stops or from fulfilling a restriction request.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// admitRestriction admits a human admin signed in to the app. A restriction
// stops the product's own machinery for a contact, so no agent, system or
// connector principal records one. auth.RequireHuman admits the last two.
func admitRestriction(ctx context.Context) error {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || !auth.AdminGranted(ctx) {
		return fmt.Errorf("a restriction of processing is recorded by an admin in the app: %w", apperrors.ErrPermissionDenied)
	}
	return nil
}

// recordRestrictionTx writes one processing_restriction stop on a contact, at
// the subject's level, with its audit and outbox rows.
func (s *Store) recordRestrictionTx(ctx context.Context, tx pgx.Tx, contactID ids.UUID, reason string) error {
	in := SuppressInput{ContactID: ids.From[ids.ContactKind](contactID), Kind: kindProcessingRestriction, Reason: reason}
	sub, level, err := admitSuppress(ctx, in)
	if err != nil {
		return err
	}
	return s.suppressAdmittedTx(ctx, tx, in, sub, level, commsauthz.RecordedByColleague)
}

// FulfilRestriction records the restriction a request asks for and closes the
// request, in one transaction. A replay on a closed request records nothing.
func (s *Store) FulfilRestriction(ctx context.Context, id ids.UUID, in UpdateDSRInput) (dsrRow, error) {
	if err := requireDSRAdmin(ctx, principal.ActionUpdate); err != nil {
		return dsrRow{}, err
	}
	if err := admitRestriction(ctx); err != nil {
		return dsrRow{}, err
	}
	unresolvedSubject := &ValidationError{
		Field:  fieldSubjectRef,
		Reason: "a restriction request must name a contact id before it can be fulfilled",
	}
	var out dsrRow
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := requireDSRAssignee(ctx, tx, in); err != nil {
			return err
		}
		current, err := lockDSR(ctx, tx, id)
		if err != nil {
			return err
		}
		if current.Kind != dsrKindRestriction {
			return &ValidationError{Field: fieldKind, Reason: "only a restriction request records a restriction"}
		}
		if verr := validateDSRUpdate(current, in); verr != nil {
			return verr
		}
		if !closedDSRStatus(current.Status) {
			contactID, resolved := resolveDSRSubject(current)
			if !resolved {
				return unresolvedSubject
			}
			err := s.recordRestrictionTx(ctx, tx, contactID, "privacy request "+current.ID.String())
			if errors.Is(err, apperrors.ErrNotFound) {
				return unresolvedSubject
			}
			if err != nil {
				return err
			}
		}
		out, err = s.applyDSRUpdateTx(ctx, tx, id, current, in)
		return err
	})
	return out, err
}
