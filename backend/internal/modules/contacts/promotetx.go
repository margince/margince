// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// promoteLeadTx is QualifyLead's transactional body, for a caller that has
// already asked for the grants and opened the transaction the promotion
// must land in.
func (s *Store) promoteLeadTx(ctx context.Context, tx pgx.Tx, id ids.LeadID, in PromoteLeadInput, by string, active []fieldcatalog.Column) (PromoteOutcome, error) {
	var out PromoteOutcome
	// The lead lock comes BEFORE the promotability read: two
	// concurrent promotes of one lead must serialize here, so the
	// loser re-reads status=promoted and answers 409 instead of
	// minting a second contact. IncludeArchived keeps the re-promote
	// 409-with-pointer diagnostic reachable.
	//
	// The lead's stop lock comes before even that, for the reason
	// mergeContactTx gives.
	if err := lockStopsOrSkip(ctx, tx, s.stopCarrier, commsauthz.LeadStopSubject(id)); err != nil {
		return PromoteOutcome{}, err
	}
	if _, err := storekit.LockRow(ctx, tx, "lead", id.UUID, storekit.IncludeArchived); err != nil {
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

	contactID, mergeFields, err := s.promoteTarget(ctx, tx, lead, by, &out.Merged)
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
