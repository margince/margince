// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package inquiries

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

type Assignment struct {
	db       *database.DB
	tasks    *activities.Store
	contacts *contacts.Store
	routing  Routing
}

func NewAssignment(db *database.DB, records *contacts.Store, routing Routing) Assignment {
	return Assignment{db: db, tasks: activities.NewStore(db), contacts: records, routing: routing}
}

func (s Assignment) Preview(ctx context.Context, id ids.ActivityID, in crmcontracts.WebsiteInquiryAssignmentRequest) (crmcontracts.WebsiteInquiryAssignment, error) {
	var out crmcontracts.WebsiteInquiryAssignment
	err := database.WithWorkspaceSnapshot(ctx, s.db.Pool(), func(ctx context.Context) error {
		var err error
		out, err = s.plan(ctx, id, in)
		return err
	})
	return out, err
}

func (s Assignment) Apply(ctx context.Context, id ids.ActivityID, version int64, in crmcontracts.WebsiteInquiryAssignmentRequest) (crmcontracts.WebsiteInquiryAssignment, error) {
	var out crmcontracts.WebsiteInquiryAssignment
	run := func() error {
		return database.WithWorkspaceSnapshot(ctx, s.db.Pool(), func(ctx context.Context) error {
			planned, err := s.plan(ctx, id, in)
			if err != nil {
				return err
			}
			if planned.Task.Version == nil || int64(*planned.Task.Version) != version || planned.RecordVersion == nil || in.RecordVersion == nil || *planned.RecordVersion != *in.RecordVersion || in.RecordId == nil || *in.RecordId != planned.RecordId || in.RecordType == nil || string(*in.RecordType) != string(planned.RecordType) {
				return apperrors.ErrVersionSkew
			}
			if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
				return s.lockTarget(ctx, tx, datasource.EntityType(planned.RecordType), ids.UUID(planned.RecordId))
			}); err != nil {
				return err
			}
			out, err = s.assign(ctx, id, version, in, planned)
			return err
		})
	}
	err := storekit.RetryLockCycles(ctx, run)
	return out, err
}

// lockTarget holds the inquiry's record before its task, preserving assignment lock order.
func (s Assignment) lockTarget(ctx context.Context, tx pgx.Tx, recordType datasource.EntityType, id ids.UUID) error {
	switch recordType {
	case datasource.EntityLead:
		return s.contacts.LockInquiryLeadTx(ctx, tx, id)
	case datasource.EntityContact:
		return s.contacts.LockInquiryContactTx(ctx, tx, id)
	}
	return apperrors.ErrInvalidArgument
}

func (s Assignment) plan(ctx context.Context, id ids.ActivityID, in crmcontracts.WebsiteInquiryAssignmentRequest) (crmcontracts.WebsiteInquiryAssignment, error) {
	var out crmcontracts.WebsiteInquiryAssignment
	if err := httperr.RequireBodyID("recipient_id", ids.UUID(in.RecipientId)); err != nil {
		return out, err
	}
	task, source, err := s.acceptedTask(ctx, id)
	if err != nil {
		return out, err
	}
	target, err := inquiryAssignmentTarget(task)
	if err != nil {
		return out, err
	}
	out = crmcontracts.WebsiteInquiryAssignment{Task: task, RecordType: crmcontracts.WebsiteInquiryAssignmentRecordType(target.Type), RecordId: crmcontracts.Id(target.ID), RecipientId: in.RecipientId}
	if err := s.recordOwner(ctx, target, &out); err != nil {
		return out, err
	}
	if out.RecordVersion == nil {
		return out, apperrors.ErrConflict
	}
	if err := s.routing.CheckRecipient(ctx, ids.UUID(in.RecipientId), target, source); err != nil {
		return out, err
	}
	actor, err := storekit.Actor(ctx)
	if err != nil {
		return out, err
	}
	if in.Mode == "take" && (ids.UUID(in.RecipientId) != actor.UserID || (target.Type == datasource.EntityLead && out.RecordOwnerId != nil && ids.UUID(*out.RecordOwnerId) != actor.UserID)) {
		return out, apperrors.ErrPermissionDenied
	}
	out.ChangesRecordOwner = target.Type == datasource.EntityLead && (out.RecordOwnerId == nil || *out.RecordOwnerId != in.RecipientId)
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		if !out.ChangesRecordOwner {
			return auth.EnsureActivityWritable(ctx, tx, id.UUID)
		}
		if err := auth.Require(ctx, "lead", principal.ActionUpdate); err != nil {
			return err
		}
		if in.Mode == "take" {
			return auth.EnsureClaimable(ctx, tx, "lead", target.ID)
		}
		return auth.EnsureAssignable(ctx, tx, "lead", target.ID, ids.UUID(in.RecipientId))
	})
	return out, err
}

func (s Assignment) acceptedTask(ctx context.Context, id ids.ActivityID) (crmcontracts.Activity, ids.UUID, error) {
	task, err := s.tasks.GetActivityContent(ctx, id, storekit.LiveOnly)
	if err != nil {
		return task, ids.Nil, err
	}
	if task.Kind != crmcontracts.ActivityKindTask || task.IsDone == nil || *task.IsDone || task.SourceSystem == nil || task.SourceId == nil {
		return task, ids.Nil, apperrors.ErrConflict
	}
	key, known := provenance.WebsiteInquirySubmission(*task.SourceSystem, *task.SourceId)
	if !known {
		return task, ids.Nil, apperrors.ErrInvalidArgument
	}
	source, err := s.tasks.ActivityBySource(ctx, provenance.WebFormSource, key, storekit.LiveOnly)
	if err != nil {
		return task, ids.Nil, err
	}
	source, err = s.tasks.GetActivityContent(ctx, ids.From[ids.ActivityKind](ids.UUID(source.Id)), storekit.LiveOnly)
	if err != nil {
		return task, ids.Nil, err
	}
	if source.Kind != crmcontracts.ActivityKindNote || source.WebForm == nil || (task.SourceActivityId != nil && *task.SourceActivityId != source.Id) {
		return task, ids.Nil, apperrors.ErrConflict
	}
	target, err := inquiryAssignmentTarget(task)
	if err != nil {
		return task, ids.Nil, err
	}
	sourceTarget, err := inquiryAssignmentTarget(source)
	if err != nil {
		return task, ids.Nil, err
	}
	if target != sourceTarget {
		return task, ids.Nil, apperrors.ErrConflict
	}
	return task, ids.UUID(source.Id), nil
}

func inquiryAssignmentTarget(row crmcontracts.Activity) (datasource.EntityRef, error) {
	var target datasource.EntityRef
	if row.Links == nil {
		return target, apperrors.ErrNotFound
	}
	for _, link := range *row.Links {
		if link.EntityType != "lead" && link.EntityType != "contact" {
			continue
		}
		next := datasource.EntityRef{Type: datasource.EntityType(link.EntityType), ID: ids.UUID(link.EntityId)}
		if !target.ID.IsZero() && target != next {
			return target, apperrors.ErrConflict
		}
		target = next
	}
	if target.ID.IsZero() {
		return target, apperrors.ErrNotFound
	}
	return target, nil
}

func (s Assignment) recordOwner(ctx context.Context, target datasource.EntityRef, out *crmcontracts.WebsiteInquiryAssignment) error {
	owners, err := s.contacts.WorkOwners(ctx, string(target.Type), []ids.UUID{target.ID})
	if err != nil {
		return err
	}
	owner, known := owners[target.ID]
	if !known {
		return apperrors.ErrNotFound
	}
	version := crmcontracts.RowVersion(owner.Version)
	out.RecordVersion = &version
	if owner.OwnerID != nil {
		value := crmcontracts.Id(*owner.OwnerID)
		out.RecordOwnerId = &value
	}
	return nil
}

func (s Assignment) assign(ctx context.Context, id ids.ActivityID, version int64, in crmcontracts.WebsiteInquiryAssignmentRequest, out crmcontracts.WebsiteInquiryAssignment) (crmcontracts.WebsiteInquiryAssignment, error) {
	recordVersion := int64(*out.RecordVersion)
	if out.ChangesRecordOwner && in.Mode == "take" {
		claimed, err := s.contacts.ClaimRecord(ctx, "lead", ids.UUID(out.RecordId), &recordVersion)
		if err != nil {
			return out, err
		}
		updated := crmcontracts.RowVersion(claimed.Version)
		out.RecordVersion, out.RecordOwnerId = &updated, &in.RecipientId
	}
	recipient := ids.From[ids.UserKind](ids.UUID(in.RecipientId))
	if _, err := s.tasks.UpdateActivity(ctx, id, activities.UpdateActivityInput{AssigneeID: &recipient, IfVersion: &version}); err != nil {
		return out, err
	}
	if out.ChangesRecordOwner && in.Mode == "assign_and_follow_up" {
		lead, err := s.contacts.UpdateLead(ctx, ids.From[ids.LeadKind](ids.UUID(out.RecordId)), contacts.UpdateLeadInput{OwnerID: &recipient, IfVersion: &recordVersion})
		if err != nil {
			return out, err
		}
		out.RecordVersion, out.RecordOwnerId = lead.Version, &in.RecipientId
	}
	var err error
	out.Task, err = s.tasks.GetActivityContent(ctx, id, storekit.LiveOnly)
	return out, err
}
