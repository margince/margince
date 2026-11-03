// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// executeCaseMerge writes a checked decision. The owner changes first. Each
// member's merge then compares its former owner with the chosen one, so the
// new owner gets no grant.
func (s *Store) executeCaseMerge(ctx context.Context, tx pgx.Tx, plan caseMergePlan, active []fieldcatalog.Column) (crmcontracts.ContactCaseOutcome, error) {
	if err := settleCasePairs(ctx, tx, plan); err != nil {
		return crmcontracts.ContactCaseOutcome{}, err
	}
	carried := carriedOwners{}
	if err := s.handOverSurvivor(ctx, tx, plan, active, carried); err != nil {
		return crmcontracts.ContactCaseOutcome{}, err
	}
	var merged []openapi_types.UUID
	for _, id := range plan.selected {
		if id == plan.survivor.ID {
			continue
		}
		_, carry, err := s.mergeContactCarryingTx(ctx, tx, ids.From[ids.ContactKind](id), plan.survivorID(), nil, active)
		if err != nil {
			return crmcontracts.ContactCaseOutcome{}, err
		}
		carried.add(carry)
		merged = append(merged, openapi_types.UUID(id))
	}
	out := plan.outcome(carried, merged)
	// Evidence, not an image: the decision changes no field of the survivor
	// beyond what each member's own merge row records.
	if _, err := storekit.AuditEventWithEvidence(ctx, tx, "merge", contactEntity, plan.survivor.ID, nil,
		map[string]any{"merge_case": map[string]any{
			"members": plan.selected, "survivor": plan.survivor.ID, "chosen_owner": plan.owner,
			"shares": out.Shares, "mode": plan.mode, "policy": plan.policy,
		}}); err != nil {
		return crmcontracts.ContactCaseOutcome{}, fmt.Errorf("contacts: recording the case merge: %w", err)
	}
	survivor, err := readContact(ctx, tx, plan.survivorID(), storekit.LiveOnly, active)
	if err != nil {
		return crmcontracts.ContactCaseOutcome{}, fmt.Errorf("contacts: reading the surviving contact: %w", err)
	}
	out.Survivor = &survivor
	return out, nil
}

// settleCasePairs marks the open pairs among the selected members as merged.
// The decision already holds each member's lock; asking again keeps every write
// visibly behind its subject's lock.
func settleCasePairs(ctx context.Context, tx pgx.Tx, plan caseMergePlan) error {
	actor, _ := principal.Actor(ctx)
	for _, id := range plan.selected {
		if err := auth.LockSubjectLive(ctx, tx, contactEntity, id); err != nil {
			return err
		}
	}
	for _, e := range plan.c.Edges {
		if e.Disposition == dispositionOpen && slices.Contains(plan.selected, e.Left) && slices.Contains(plan.selected, e.Right) {
			if err := setDedupeDispositionTx(ctx, tx, e.ID, dispositionMerged, actor.UserID); err != nil {
				return err
			}
		}
	}
	return nil
}

// handOverSurvivor gives the survivor to the chosen owner, and keeps the
// previous owner's access.
func (s *Store) handOverSurvivor(ctx context.Context, tx pgx.Tx, plan caseMergePlan, active []fieldcatalog.Column, carried carriedOwners) error {
	previous := plan.survivor.OwnerID
	if sameOwner(previous, &plan.owner) {
		return nil
	}
	if err := auth.LockSubjectLive(ctx, tx, contactEntity, plan.survivor.ID); err != nil {
		return err
	}
	owner := ids.From[ids.UserKind](plan.owner)
	if _, err := s.updateContactInTx(ctx, tx, plan.survivorID(), UpdateContactInput{OwnerID: &owner}, active); err != nil {
		return fmt.Errorf("contacts: handing the survivor to its owner: %w", err)
	}
	carry, err := s.carryOwnerAccessTx(ctx, tx, plan.survivorID(), &plan.owner, previous, ids.Nil)
	if err != nil {
		return err
	}
	carried.add(carry)
	return nil
}

// outcome reports, per former owner other than the chosen one, the access they
// kept or why they got none.
func (p caseMergePlan) outcome(carried carriedOwners, merged []openapi_types.UUID) crmcontracts.ContactCaseOutcome {
	out := crmcontracts.ContactCaseOutcome{
		SurvivorId: openapi_types.UUID(p.survivor.ID), OwnerId: openapi_types.UUID(p.owner),
		MergedIds: merged, Mode: p.mode,
		Shares: []crmcontracts.ContactCaseShare{}, Skipped: []crmcontracts.ContactCaseSkippedOwner{},
	}
	owners := make([]ids.UUID, 0, len(carried))
	for owner := range carried {
		if owner != p.owner {
			owners = append(owners, owner)
		}
	}
	slices.SortFunc(owners, compareUUID)
	for _, owner := range owners {
		carry, name := carried[owner], p.ownerName(owner)
		if carry.Access != "" {
			out.Shares = append(out.Shares, crmcontracts.ContactCaseShare{
				UserId: openapi_types.UUID(owner), Name: name, Access: crmcontracts.ContactCaseShareAccess(carry.Access),
			})
			continue
		}
		out.Skipped = append(out.Skipped, crmcontracts.ContactCaseSkippedOwner{
			UserId: openapi_types.UUID(owner), Name: name, Reason: crmcontracts.ContactCaseSkippedOwnerReason(carry.Skipped),
		})
	}
	return out
}

func (p caseMergePlan) ownerName(owner ids.UUID) string {
	for _, m := range p.c.Members {
		if m.OwnerID != nil && *m.OwnerID == owner && m.OwnerName != nil {
			return *m.OwnerName
		}
	}
	return ""
}
