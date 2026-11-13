// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package inquiries

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// owners adds who owns each task's record, and whether the reader may take
// the task, to facts.
func (w *Work) owners(ctx context.Context, tasks []crmcontracts.Activity, notes map[string]crmcontracts.Activity, facts map[ids.UUID]*crmcontracts.WebsiteInquiryWork) error {
	targets, grouped, err := inquiryWorkTargets(tasks, facts)
	if err != nil {
		return err
	}
	ownership, err := w.workOwnership(ctx, grouped)
	if err != nil {
		return err
	}
	eligible, err := w.readerCanTake(ctx)
	if err != nil {
		return err
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		return apperrors.ErrPermissionDenied
	}
	for _, task := range tasks {
		id := ids.UUID(task.Id)
		target := targets[id]
		owner, known := ownership[string(target.Type)][target.ID]
		if !known {
			continue
		}
		detail := facts[id]
		detail.RecordOwnerKnown = new(true)
		if owner.OwnerID != nil {
			value := crmcontracts.Id(*owner.OwnerID)
			detail.RecordOwnerId = &value
		}
		if !eligible {
			continue
		}
		take, err := mayTake(ctx, actor, task, target, owner, notes)
		if err != nil {
			return err
		}
		if take {
			detail.CanTake = new(true)
		}
	}
	return nil
}

// inquiryWorkTargets is the one lead or contact each inquiry task is filed
// under, also grouped by record type. A task with none, or two, is left out.
func inquiryWorkTargets(tasks []crmcontracts.Activity, facts map[ids.UUID]*crmcontracts.WebsiteInquiryWork) (map[ids.UUID]datasource.EntityRef, map[string][]ids.UUID, error) {
	targets := map[ids.UUID]datasource.EntityRef{}
	grouped := map[string][]ids.UUID{}
	for _, task := range tasks {
		if facts[ids.UUID(task.Id)] == nil {
			continue
		}
		target, err := inquiryAssignmentTarget(task)
		if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrConflict) {
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		targets[ids.UUID(task.Id)] = target
		grouped[string(target.Type)] = append(grouped[string(target.Type)], target.ID)
	}
	return targets, grouped, nil
}

// workOwnership reads the owners of the grouped records. A record type the
// reader may not read is left out.
func (w *Work) workOwnership(ctx context.Context, grouped map[string][]ids.UUID) (map[string]map[ids.UUID]contacts.WorkOwnership, error) {
	ownership := map[string]map[ids.UUID]contacts.WorkOwnership{}
	for kind, ids := range grouped {
		rows, err := w.contacts.WorkOwners(ctx, kind, ids)
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			continue
		}
		if err != nil {
			return nil, err
		}
		ownership[kind] = rows
	}
	return ownership, nil
}

// mayTake reports whether the reader may take a task they do not already
// hold. The task and its source note must name the same record, and a lead
// must be the reader's own or unowned and writable.
func mayTake(ctx context.Context, actor principal.Principal, task crmcontracts.Activity, target datasource.EntityRef, owner contacts.WorkOwnership, notes map[string]crmcontracts.Activity) (bool, error) {
	if task.SourceSystem == nil || task.SourceId == nil || (task.AssigneeId != nil && ids.UUID(*task.AssigneeId) == actor.UserID && (target.Type != datasource.EntityLead || owner.OwnerID != nil)) {
		return false, nil
	}
	if matches, err := sourceNoteMatches(task, target, notes); err != nil || !matches {
		return false, err
	}
	if target.Type != datasource.EntityLead {
		return true, nil
	}
	if owner.OwnerID != nil {
		return *owner.OwnerID == actor.UserID, nil
	}
	return auth.AllowsWrite(ctx, leadObject, principal.ActionUpdate), nil
}

// sourceNoteMatches reports whether the reader can read the task's source
// note, and the note names the same record as the task.
func sourceNoteMatches(task crmcontracts.Activity, target datasource.EntityRef, notes map[string]crmcontracts.Activity) (bool, error) {
	key, _ := provenance.WebsiteInquirySubmission(*task.SourceSystem, *task.SourceId)
	note, readable := notes[key]
	if !readable || note.WebForm == nil || (task.SourceActivityId != nil && *task.SourceActivityId != note.Id) {
		return false, nil
	}
	sourceTarget, err := inquiryAssignmentTarget(note)
	if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrConflict) {
		return false, nil
	}
	return err == nil && sourceTarget == target, err
}

func (w *Work) readerCanTake(ctx context.Context) (bool, error) {
	for _, check := range []func(context.Context) error{auth.RequireHumanOrItsAgent, func(ctx context.Context) error { return auth.Require(ctx, "activity", principal.ActionUpdate) }} {
		if err := check(ctx); err != nil {
			if errors.Is(err, apperrors.ErrPermissionDenied) {
				return false, nil
			}
			return false, err
		}
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		return false, apperrors.ErrPermissionDenied
	}
	err := w.db.Tx(ctx, func(tx pgx.Tx) error { return auth.EnsureAssignee(ctx, tx, actor.UserID) })
	if inquiryRecipientDenied(err) {
		return false, nil
	}
	return err == nil, err
}
