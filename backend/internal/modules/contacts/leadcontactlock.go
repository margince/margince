// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// promoteAttempts bounds how often a promotion starts again after its match
// moved. Each restart needs a concurrent merge or email edit to land in a
// window of a few statements, so three is generous.
const promoteAttempts = 3

// lockContactThenLead locks a contact and then a lead. MergeContact locks its
// contacts and then rewrites lead rows, so a writer taking the lead first
// would close a deadlock cycle with it.
//
// A zero contact locks the lead alone. A promotion passes IncludeArchived, so
// a candidate a merge has retired still locks and the move shows. between runs
// after the contact lock and before the lead lock, where a promotion takes its
// duplicate-lane locks (lockorder.go); nil runs nothing.
//
// Held by: TestOneLockOrderOverTheLeadAndItsContact (backend/gates/leadcontactlockorder_test.go)
func lockContactThenLead(
	ctx context.Context, tx pgx.Tx, contactID ids.ContactID, contactFilter storekit.ArchivedFilter, leadID ids.LeadID,
	between func() error,
) error {
	if !contactID.IsZero() {
		if _, err := storekit.LockRow(ctx, tx, "contact", contactID.UUID, contactFilter); err != nil {
			return fmt.Errorf("lock the lead's contact: %w", err)
		}
	}
	if between != nil {
		if err := between(); err != nil {
			return err
		}
	}
	if _, err := storekit.LockRow(ctx, tx, "lead", leadID.UUID, storekit.IncludeArchived); err != nil {
		return err
	}
	return nil
}

// lockLeadForContact takes the locks of a write that lands a lead on a contact,
// in the order lockorder.go and lockContactThenLead fix. Consent's stop locks
// come before any row. Then come the last-activity clocks an employment at the
// company moves, the candidate contact, its lanes, and the lead. A zero
// candidate names no contact; a nil company moves no clock.
// The promotion's industry carry (leadIndustryCarry) also relies on this company lock.
func (s *Store) lockLeadForContact(ctx context.Context, tx pgx.Tx, id ids.LeadID, candidate ids.ContactID, company *ids.CompanyID) (ContactCandidate, error) {
	if err := auth.EnsureWritable(ctx, tx, "lead", id.UUID); err != nil {
		return ContactCandidate{}, err
	}
	subjects := []commsauthz.StopSubject{commsauthz.LeadStopSubject(id)}
	var clocks storekit.LastActivityTargets
	if !candidate.IsZero() {
		subjects = append(subjects, commsauthz.ContactStopSubject(candidate))
		clocks.Contacts = []ids.UUID{candidate.UUID}
	}
	if err := lockStopsOrSkip(ctx, tx, s.stopCarrier, subjects...); err != nil {
		return ContactCandidate{}, err
	}
	if company != nil {
		clocks.Companies = []ids.UUID{company.UUID}
		if err := storekit.LockLastActivityTargets(ctx, tx, clocks); err != nil {
			return ContactCandidate{}, err
		}
	}
	var lanes ContactCandidate
	err := lockContactThenLead(ctx, tx, candidate, storekit.IncludeArchived, id, func() error {
		var err error
		lanes, err = s.lockLeadLanes(ctx, tx, id)
		return err
	})
	return lanes, err
}

// promoteTargetMovedError says the contact or company a promotion locked is no
// longer the lead's under the lock. QualifyLead starts over with the new one; a
// caller that cannot start over reads it as a conflict.
type promoteTargetMovedError struct{}

func (e *promoteTargetMovedError) Error() string {
	return "the contact or company this lead names changed while it was being promoted"
}

// refuseMovedCompany answers promoteTargetMovedError when the lead's linked
// company is not the one locked before the lead. Taking it now would lock a
// company after its lead, the reverse of a company merge.
func refuseMovedCompany(ctx context.Context, tx pgx.Tx, id ids.LeadID, locked *ids.CompanyID) error {
	linked, err := leadLinkedCompany(ctx, tx, id)
	if err != nil {
		return err
	}
	if !sameCompany(linked, locked) {
		return &promoteTargetMovedError{}
	}
	return nil
}

// sameCompany compares two optional company ids, where nil is no company.
func sameCompany(a, b *ids.CompanyID) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// Is lets a caller with no retry of its own treat the move as the conflict it
// is, without knowing this type.
func (e *promoteTargetMovedError) Is(target error) bool { return target == apperrors.ErrConflict }

// promoteCandidate names the contact a promotion would merge into, read
// without any lock so the contact can be locked before the lead. The answer is
// a hint: promoteTarget resolves the match again under both locks.
//
// The lead's row-scope probe runs first, so a caller who may not change this
// lead learns nothing from it and locks no contact. A lead that cannot be
// promoted names no candidate; promotableLead gives the refusal under the lock.
func (s *Store) promoteCandidate(ctx context.Context, tx pgx.Tx, id ids.LeadID) (ids.ContactID, error) {
	var none ids.ContactID
	if err := auth.EnsureWritable(ctx, tx, "lead", id.UUID); err != nil {
		return none, err
	}
	lead, err := readLead(ctx, tx, id, storekit.IncludeArchived, nil)
	if err != nil {
		return none, fmt.Errorf("read lead before resolving its contact: %w", err)
	}
	if lead.Status == crmcontracts.LeadStatusPromoted || lead.ArchivedAt != nil || leadIdentityName(lead) == "" {
		return none, nil
	}
	match, err := s.previewTarget(ctx, tx, lead)
	if err != nil {
		return none, err
	}
	if match.Decision != DecisionExactCollision {
		return none, nil
	}
	return match.ContactID, nil
}
