// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Merging a contact case: every selected member into one survivor, in one
// transaction. A preview runs this same decision and rolls it back, so what a
// reviewer is shown is what the merge would write.

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// ContactCaseDecision is what a reviewer chose from a case.
type ContactCaseDecision struct {
	Revision   string
	MemberIDs  []ids.UUID
	SurvivorID ids.UUID
	OwnerID    ids.UUID
}

// CaseSelectionError refuses a selection the case cannot take: 422 with the
// contract's code.
type CaseSelectionError struct{ Code, Msg string }

func (e *CaseSelectionError) Error() string { return "contacts: " + e.Msg }

// MessageFault carries the code a client branches on.
func (e *CaseSelectionError) MessageFault() (code, message string) { return e.Code, e.Msg }

var (
	errCaseNotConnected = &CaseSelectionError{
		Code: "not_connected",
		Msg:  "the selected contacts are not all joined by open duplicate pairs in this case",
	}
	errCaseDismissedPair = &CaseSelectionError{
		Code: "dismissed_pair_selected",
		Msg:  "two of the selected contacts were marked as different people; leave one of them out",
	}
	errCaseOwnerNotEligible = &CaseSelectionError{
		Code: "owner_not_eligible",
		Msg:  "the owner must be a current owner of one of the selected contacts",
	}
	errCaseTooLarge = &CaseSelectionError{
		Code: "too_large",
		Msg:  "this case has more contacts than one review may merge; decide its pairs one by one",
	}
	errCaseChanged = &pairDecisionError{
		code: "case_changed",
		msg:  "the case changed since it was read; read it again and review the new version",
	}
	// errCasePreview rolls a preview's transaction back once its outcome is built.
	errCasePreview = errors.New("contacts: a case preview writes nothing")
)

// PreviewContactCaseMerge answers what MergeContactCase would do, and writes
// nothing.
func (s *Store) PreviewContactCaseMerge(ctx context.Context, contactID ids.UUID, d ContactCaseDecision) (crmcontracts.ContactCaseOutcome, error) {
	return s.decideContactCase(ctx, contactID, d, false)
}

// MergeContactCase merges the selected members of a case into the survivor.
func (s *Store) MergeContactCase(ctx context.Context, contactID ids.UUID, d ContactCaseDecision) (crmcontracts.ContactCaseOutcome, error) {
	return s.decideContactCase(ctx, contactID, d, true)
}

func (s *Store) decideContactCase(ctx context.Context, contactID ids.UUID, d ContactCaseDecision, commit bool) (crmcontracts.ContactCaseOutcome, error) {
	if err := auth.RequireActingForAHuman(ctx, "a duplicate case is merged"); err != nil {
		return crmcontracts.ContactCaseOutcome{}, err
	}
	if err := auth.Require(ctx, contactEntity, principal.ActionUpdate); err != nil {
		return crmcontracts.ContactCaseOutcome{}, err
	}
	selected, err := caseSelection(d)
	if err != nil {
		return crmcontracts.ContactCaseOutcome{}, err
	}
	active, err := s.activeColumns(ctx, contactEntity)
	if err != nil {
		return crmcontracts.ContactCaseOutcome{}, err
	}
	var out crmcontracts.ContactCaseOutcome
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		if out, err = s.decideContactCaseTx(ctx, tx, contactID, d, selected, active); err != nil {
			return err
		}
		if !commit {
			return errCasePreview
		}
		return nil
	})
	if !commit && errors.Is(err, errCasePreview) {
		return out, nil
	}
	if err != nil {
		return crmcontracts.ContactCaseOutcome{}, err
	}
	out.Committed = true
	return out, nil
}

// caseSelection checks the request's shape before any lock is taken.
func caseSelection(d ContactCaseDecision) ([]ids.UUID, error) {
	if err := httperr.RequireBodyID("survivor_id", d.SurvivorID); err != nil {
		return nil, err
	}
	if err := httperr.RequireBodyID("owner_id", d.OwnerID); err != nil {
		return nil, err
	}
	selected := slices.Clone(d.MemberIDs)
	slices.SortFunc(selected, compareUUID)
	selected = slices.Compact(selected)
	switch {
	case len(selected) != len(d.MemberIDs) || len(selected) < 2:
		return nil, &DedupeInputError{Field: "member_ids", Msg: "must name at least two different contacts"}
	case len(selected) > contactCaseMaxMembers:
		return nil, errCaseTooLarge
	case !slices.Contains(selected, d.SurvivorID):
		return nil, &DedupeInputError{Field: "survivor_id", Msg: "must be one of member_ids"}
	}
	return selected, nil
}

// decideContactCaseTx locks every selected member, then reads and checks the
// case under those locks. A concurrent merge of the same case then shows as a
// changed revision.
func (s *Store) decideContactCaseTx(ctx context.Context, tx pgx.Tx, contactID ids.UUID, d ContactCaseDecision,
	selected []ids.UUID, active []fieldcatalog.Column,
) (crmcontracts.ContactCaseOutcome, error) {
	policy, err := s.sharedMergeReviewsForDecision(ctx, tx)
	if err != nil {
		return crmcontracts.ContactCaseOutcome{}, err
	}
	if err := s.lockCaseMembers(ctx, tx, selected); err != nil {
		return crmcontracts.ContactCaseOutcome{}, err
	}
	c, err := walkContactCase(ctx, tx, contactID)
	if err != nil {
		return crmcontracts.ContactCaseOutcome{}, err
	}
	if len(c.Members) == 0 {
		return crmcontracts.ContactCaseOutcome{}, caseGone(ctx, tx, contactID)
	}
	if err := checkCaseSelection(c, selected); err != nil {
		return crmcontracts.ContactCaseOutcome{}, err
	}
	if c.revision(policy) != d.Revision {
		return crmcontracts.ContactCaseOutcome{}, errCaseChanged
	}
	grantees := c.liveOwners(selected)
	if !slices.Contains(grantees, d.OwnerID) {
		return crmcontracts.ContactCaseOutcome{}, errCaseOwnerNotEligible
	}
	survivor, _ := c.member(d.SurvivorID)
	ctx, mode, err := admitCaseDecision(ctx, tx, c, selected, survivor, d.OwnerID, policy)
	if err != nil {
		return crmcontracts.ContactCaseOutcome{}, err
	}
	return s.executeCaseMerge(ctx, tx, caseMergePlan{
		c: c, selected: selected, survivor: survivor, owner: d.OwnerID, mode: mode, policy: policy,
	}, active)
}

// lockCaseMembers takes every selected member's stop lock in one call, then
// holds each live member in id order: the order every contact merge keeps. A
// member already gone is left to the walk, which no longer finds it.
func (s *Store) lockCaseMembers(ctx context.Context, tx pgx.Tx, selected []ids.UUID) error {
	subjects := make([]commsauthz.StopSubject, 0, len(selected))
	for _, id := range selected {
		subjects = append(subjects, commsauthz.ContactStopSubject(ids.From[ids.ContactKind](id)))
	}
	if err := lockStopsOrSkip(ctx, tx, s.stopCarrier, subjects...); err != nil {
		return err
	}
	for _, id := range selected {
		if err := auth.LockSubjectLive(ctx, tx, contactEntity, id); err != nil && !errors.Is(err, apperrors.ErrNotFound) {
			return fmt.Errorf("contacts: locking the case members: %w", err)
		}
	}
	return nil
}

// caseGone answers for a contact the reviewer saw in a case that is in none
// now. A merge or a dismissal changed the case under them.
func caseGone(ctx context.Context, tx pgx.Tx, contactID ids.UUID) error {
	if err := auth.EnsureVisible(ctx, tx, contactEntity, contactID); err != nil {
		return err
	}
	return errCaseChanged
}

// checkCaseSelection holds the selection to the case: members only, the case
// small enough, no pair a colleague dismissed, and joined by open pairs.
func checkCaseSelection(c contactCase, selected []ids.UUID) error {
	for _, id := range selected {
		if _, ok := c.member(id); !ok {
			return errCaseNotConnected
		}
	}
	if c.TooLarge {
		return errCaseTooLarge
	}
	among := func(e caseEdge) bool { return slices.Contains(selected, e.Left) && slices.Contains(selected, e.Right) }
	reached := map[ids.UUID]bool{selected[0]: true}
	for grew := true; grew; {
		grew = false
		for _, e := range c.Edges {
			if !among(e) {
				continue
			}
			if e.Disposition == dispositionNotDuplicate {
				return errCaseDismissedPair
			}
			if reached[e.Left] != reached[e.Right] {
				reached[e.Left], reached[e.Right], grew = true, true, true
			}
		}
	}
	if len(reached) != len(selected) {
		return errCaseNotConnected
	}
	return nil
}

// admitCaseDecision answers under which authority the merge runs. Ordinary:
// the caller may change every selected member and hand the survivor to the
// owner. Delegated: the policy is on and the caller is a live full-seat owner
// of a selected member. The context then admits the selected contacts and
// their owners, and nothing else.
func admitCaseDecision(ctx context.Context, tx pgx.Tx, c contactCase, selected []ids.UUID,
	survivor caseMember, owner ids.UUID, policy bool,
) (context.Context, crmcontracts.ContactCaseOutcomeMode, error) {
	ordinary := true
	for _, id := range selected {
		writable, err := auth.WritableBy(ctx, tx, contactEntity, id)
		if err != nil {
			return ctx, "", err
		}
		ordinary = ordinary && writable
	}
	var assignee error
	if ordinary && !sameOwner(survivor.OwnerID, &owner) {
		if assignee = auth.EnsureAssignee(ctx, tx, owner); assignee != nil && !errors.As(assignee, new(*auth.AssigneeNotAllowedError)) {
			return ctx, "", assignee
		}
		ordinary = assignee == nil
	}
	if ordinary {
		return ctx, crmcontracts.ContactCaseOutcomeModeOrdinary, nil
	}
	if actor, ok := principal.Actor(ctx); ok && policy && c.actorOwnsAMember(actor, selected) {
		admitted, err := auth.WithContactCaseAdmission(ctx, selected, c.liveOwners(selected))
		return admitted, crmcontracts.ContactCaseOutcomeModeDelegated, err
	}
	if assignee != nil {
		return ctx, "", assignee
	}
	return ctx, "", apperrors.ErrPermissionDenied
}

// caseMergePlan is a checked decision, ready to write.
type caseMergePlan struct {
	c        contactCase
	selected []ids.UUID
	survivor caseMember
	owner    ids.UUID
	mode     crmcontracts.ContactCaseOutcomeMode
	policy   bool
}

func (p caseMergePlan) survivorID() ids.ContactID { return ids.From[ids.ContactKind](p.survivor.ID) }

// carriedOwners collects what each merge did about a former owner, one answer
// per owner: a grant outranks a skip.
type carriedOwners map[ids.UUID]authz.OwnerAccessCarry

func (o carriedOwners) add(carry *authz.OwnerAccessCarry) {
	if carry == nil {
		return
	}
	if prior, seen := o[carry.Owner]; seen && prior.Access != "" {
		return
	}
	o[carry.Owner] = *carry
}
