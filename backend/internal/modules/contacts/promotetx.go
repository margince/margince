// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// promoteLeadTx is QualifyLead's transactional body, for a caller that has
// already asked for the grants and opened the transaction the promotion
// must land in. candidate is the contact the lead's email matched before any
// lock, or zero for none. A different match under the locks is
// promoteTargetMovedError.
func (s *Store) promoteLeadTx(ctx context.Context, tx pgx.Tx, id ids.LeadID, in PromoteLeadInput, by string, active []fieldcatalog.Column, candidate ids.ContactID) (PromoteOutcome, error) {
	var out PromoteOutcome
	company, err := leadLinkedCompany(ctx, tx, id)
	if err != nil {
		return PromoteOutcome{}, err
	}
	// The lead lock comes before the promotability read. Two promotes of one
	// lead serialize here, and the loser reads status=promoted and answers 409.
	lanes, err := s.lockLeadForContact(ctx, tx, id, candidate, company)
	if err != nil {
		return PromoteOutcome{}, err
	}
	if err := refuseMovedCompany(ctx, tx, id, company); err != nil {
		return PromoteOutcome{}, err
	}
	lead, err := promotableLead(ctx, tx, id, in)
	if err != nil {
		return PromoteOutcome{}, err
	}
	// Under the lock, against the row this transaction will write. An
	// agent's released approval was granted against a version, and
	// redemption verified it in a transaction that has since committed —
	// the agent's own 🟢 writes can land in the window between. Unpinned,
	// the promotion then mints a contact from lead fields nobody released.
	if err := refuseIfVersionMoved("lead", lead.Version, writeOptions{atVersion: in.IfVersion}); err != nil {
		return PromoteOutcome{}, err
	}

	contactID, mergeFields, err := s.promoteTarget(ctx, tx, lead, by, candidate, lanes, in.intoCandidate, &out.Merged)
	if err != nil {
		return PromoteOutcome{}, err
	}
	if err := carryLeadConsent(ctx, tx, id, contactID, by); err != nil {
		return PromoteOutcome{}, fmt.Errorf("carry lead consent: %w", err)
	}
	// And the lead's STOPS, which live in consent's table rather than ours
	// — see stopcarry.go. A lead who asked us to stop and was then promoted
	// would otherwise arrive as a contact carrying no stop at all, which is
	// the same silent resumption the contact merge produced.
	if err := s.carryStopsTx(ctx, tx,
		commsauthz.LeadStopSubject(id),
		commsauthz.ContactStopSubject(contactID)); err != nil {
		return PromoteOutcome{}, fmt.Errorf("carry the lead's stops: %w", err)
	}
	if err := s.carryConsentSatellitesTx(ctx, tx,
		commsauthz.LeadStopSubject(id),
		commsauthz.ContactStopSubject(contactID)); err != nil {
		return PromoteOutcome{}, fmt.Errorf("carry the lead's consent links: %w", err)
	}
	if err := s.carryLeadCompany(ctx, tx, id, contactID, deref(lead.Title)); err != nil {
		return PromoteOutcome{}, err
	}
	carried, err := carryLeadActivities(ctx, tx, id, contactID)
	if err != nil {
		return PromoteOutcome{}, err
	}

	out.DealID, err = s.openQualifiedDeal(ctx, tx, id, lead, contactID, in.Deal)
	if err != nil {
		return PromoteOutcome{}, err
	}
	out.Contact, err = finalizeLeadPromotion(ctx, tx, id, in, lead, contactID, out.Merged, mergeFields, active, out.DealID, carried)
	return out, err
}

// lockLeadLanes takes the duplicate-lane locks a promotion's new contact will be
// created under, between the contact lock and the lead lock (lockorder.go). The
// lead is read before its lock, so promoteTarget compares the lanes with the
// locked lead and starts over when they moved. A lead that is gone takes no
// lane lock, and the lead lock that follows answers it.
func (s *Store) lockLeadLanes(ctx context.Context, tx pgx.Tx, leadID ids.LeadID) (ContactCandidate, error) {
	lead, err := readLead(ctx, tx, leadID, storekit.IncludeArchived, nil)
	if errors.Is(err, apperrors.ErrNotFound) {
		return ContactCandidate{}, nil
	}
	if err != nil {
		return ContactCandidate{}, err
	}
	lanes, err := s.leadContactCandidate(ctx, tx, lead)
	if err != nil {
		return ContactCandidate{}, err
	}
	return lanes, lockContactLanes(ctx, tx, lanes.Emails, lanes.FullName)
}
