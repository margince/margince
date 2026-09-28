// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// promoteHeldLeadsTx promotes every live lead whose email this contact now
// holds onto the contact, in the caller's transaction.
//
// A lead and a contact for one address are the same contact filed twice. It happens
// when a contact is typed in, imported, or minted by mail capture for somebody
// who is already a lead — typically a lead carried over from the previous CRM
// who then writes to a connected mailbox. Left alone,
// the lead keeps its history and stays on the lead list as if nobody had spoken
// to them, and the contact holds the conversation. The promotion is the
// ordinary one (promoteLeadTx): its dedupe finds this contact by the shared
// address and MERGES, so the lead's activities, consent and stops move over,
// the contact gains converted_from_lead_id, and the lead is archived as
// promoted.
//
// Only a contact the whole workspace can see, and that is not quarantined,
// receives a lead. Capture mints an owner-scoped contact while a sender is
// unjudged and a quarantined one on an impersonation tell; folding a
// workspace lead into either would hide it, or hand its history to a record
// that may be an impostor. Such a contact receives the lead later, when an
// ensure over the same address finds it workspace-scoped.
//
// Without both grants the promotion needs, nothing happens: the contact write
// the caller was allowed to make still lands. Each lead is promoted under its
// own savepoint, and a lead the promotion refuses — out of the caller's reach,
// already promoted, nameless — is left as it is rather than failing the write
// that created the contact.
func (s *Store) promoteHeldLeadsTx(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, trigger PromoteTrigger, evidence *ids.ActivityID, by string) error {
	for object, action := range map[string]principal.Action{"lead": principal.ActionUpdate, "contact": principal.ActionCreate} {
		if err := auth.Require(ctx, object, action); err != nil {
			if errors.Is(err, apperrors.ErrPermissionDenied) {
				return nil
			}
			return err
		}
	}
	rows, err := tx.Query(ctx, `
		SELECT l.id
		  FROM contact c
		  JOIN contact_email ce ON ce.contact_id = c.id
		  JOIN lead l ON l.email = ce.email
		 WHERE c.id = $1
		   AND c.archived_at IS NULL AND c.merged_into_id IS NULL
		   AND c.visibility = 'workspace' AND c.quarantined_at IS NULL
		   AND l.archived_at IS NULL AND l.merged_into_id IS NULL
		   AND l.status <> 'promoted'
		 ORDER BY l.created_at, l.id`, contactID)
	if err != nil {
		return fmt.Errorf("contacts: leads holding the contact's address: %w", err)
	}
	leadIDs, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return fmt.Errorf("contacts: leads holding the contact's address: %w", err)
	}
	for _, leadID := range leadIDs {
		if err := s.promoteHeldLeadTx(ctx, tx, ids.From[ids.LeadKind](leadID), PromoteLeadInput{
			Trigger: string(trigger), EvidenceActivityID: evidence,
		}, by); err != nil {
			return err
		}
	}
	return nil
}

// promoteHeldLeadTx runs one promotion under a savepoint, so a refusal rolls
// back that lead alone. A fault that is not a refusal still fails the caller:
// a broken promotion must not be mistaken for a lead that was out of reach.
func (s *Store) promoteHeldLeadTx(ctx context.Context, tx pgx.Tx, leadID ids.LeadID, in PromoteLeadInput, by string) error {
	sp, err := tx.Begin(ctx)
	if err != nil {
		return fmt.Errorf("contacts: savepoint before promoting a held lead: %w", err)
	}
	if _, err := s.promoteLeadTx(ctx, sp, leadID, in, by, nil); err != nil {
		if rbErr := sp.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("contacts: roll back a refused lead promotion: %w", rbErr)
		}
		if promotionRefused(err) {
			return nil
		}
		return fmt.Errorf("contacts: promote the lead holding this address: %w", err)
	}
	if err := sp.Commit(ctx); err != nil {
		return fmt.Errorf("contacts: release the lead promotion savepoint: %w", err)
	}
	return nil
}

// promotionRefused says the promotion declined this lead for a reason that
// belongs to the lead or to the caller's reach, not to a fault.
func promotionRefused(err error) bool {
	var promoted *AlreadyPromotedError
	var nameless *PromoteNeedsIdentityError
	return errors.As(err, &promoted) || errors.As(err, &nameless) ||
		errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrConflict) ||
		errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrVersionSkew)
}
