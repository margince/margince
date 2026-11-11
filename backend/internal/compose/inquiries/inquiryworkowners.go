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

func (w *Work) owners(ctx context.Context, tasks []crmcontracts.Activity, notes map[string]crmcontracts.Activity, facts map[ids.UUID]*crmcontracts.WebsiteInquiryWork) error {
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
			return err
		}
		targets[ids.UUID(task.Id)] = target
		grouped[string(target.Type)] = append(grouped[string(target.Type)], target.ID)
	}
	ownership := map[string]map[ids.UUID]contacts.WorkOwnership{}
	for kind, ids := range grouped {
		rows, err := w.contacts.WorkOwners(ctx, kind, ids)
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			continue
		}
		if err != nil {
			return err
		}
		ownership[kind] = rows
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
		if !eligible || task.SourceSystem == nil || task.SourceId == nil || (task.AssigneeId != nil && ids.UUID(*task.AssigneeId) == actor.UserID && (target.Type != datasource.EntityLead || owner.OwnerID != nil)) {
			continue
		}
		key, _ := provenance.WebsiteInquirySubmission(*task.SourceSystem, *task.SourceId)
		note, readable := notes[key]
		if !readable || note.WebForm == nil || (task.SourceActivityId != nil && *task.SourceActivityId != note.Id) {
			continue
		}
		sourceTarget, err := inquiryAssignmentTarget(note)
		if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrConflict) {
			continue
		}
		if err != nil {
			return err
		}
		if sourceTarget != target {
			continue
		}
		if target.Type == datasource.EntityLead {
			if owner.OwnerID != nil && *owner.OwnerID != actor.UserID {
				continue
			}
			if owner.OwnerID == nil && !auth.AllowsWrite(ctx, "lead", principal.ActionUpdate) {
				continue
			}
		}
		detail.CanTake = new(true)
	}
	return nil
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
