// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
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
	if err := auth.Require(ctx, "lead", principal.ActionUpdate); err != nil {
		return unlessDenied(err)
	}
	if err := auth.Require(ctx, "contact", principal.ActionCreate); err != nil {
		return unlessDenied(err)
	}
	rows, err := tx.Query(ctx, `
		SELECT l.id
		  FROM contact c
		  JOIN contact_email ce ON ce.contact_id = c.id AND ce.archived_at IS NULL
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
		if err := s.promoteHeldLeadTx(ctx, tx, ids.From[ids.LeadKind](leadID), contactID, PromoteLeadInput{
			Trigger: string(trigger), EvidenceActivityID: evidence,
		}, by); err != nil {
			return err
		}
	}
	return nil
}

// PromoteHeldLeadsOnReplyTx is promoteHeldLeadsTx for the capture step that
// publishes a contact once its address has answered: the contact was kept to
// its owner when the mail arrived, so the ensure took no lead, and the reply
// that now publishes it is the inbound_reply the promotion needs.
func (s *Store) PromoteHeldLeadsOnReplyTx(ctx context.Context, tx pgx.Tx, contactID ids.ContactID) error {
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	return s.promoteHeldLeadsTx(ctx, tx, contactID, TriggerInboundReply, nil, by)
}

// promoteHeldLeadTx runs one promotion under a savepoint, so a refusal rolls
// back that lead alone. A fault that is not a refusal still fails the caller:
// a broken promotion must not be mistaken for a lead that was out of reach.
//
// The promotion resolves its own target from the lead's email, read under the
// lead's lock. The lead was chosen without that lock, so its email can have
// moved in between, and the promotion would then land on another contact — or
// mint one — that none of the checks above were applied to. Anything but a
// merge into THIS contact is rolled back and the lead left as it is.
func (s *Store) promoteHeldLeadTx(ctx context.Context, tx pgx.Tx, leadID ids.LeadID, contactID ids.ContactID, in PromoteLeadInput, by string) error {
	// A named savepoint, released on both paths: pgx's nested-transaction
	// rollback only rolls back TO the savepoint and leaves it open, so a bulk
	// import that met many refusals would pile them up until it committed.
	if _, err := tx.Exec(ctx, "SAVEPOINT promote_held_lead"); err != nil {
		return fmt.Errorf("contacts: savepoint before promoting a held lead: %w", err)
	}
	out, err := s.promoteLeadTx(ctx, tx, leadID, in, by, nil)
	if err == nil && (!out.Merged || ids.UUID(out.Contact.Id) != contactID.UUID) {
		err = apperrors.ErrConflict
	}
	if err != nil {
		if _, rbErr := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT promote_held_lead; RELEASE SAVEPOINT promote_held_lead"); rbErr != nil {
			return fmt.Errorf("contacts: roll back a refused lead promotion: %w", rbErr)
		}
		if promotionRefused(err) {
			return nil
		}
		return fmt.Errorf("contacts: promote the lead holding this address: %w", err)
	}
	if _, err := tx.Exec(ctx, "RELEASE SAVEPOINT promote_held_lead"); err != nil {
		return fmt.Errorf("contacts: release the lead promotion savepoint: %w", err)
	}
	return nil
}

// unlessDenied answers nil for a permission refusal, which only means this
// caller cannot promote, and the error itself for anything else.
func unlessDenied(err error) error {
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil
	}
	return err
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
