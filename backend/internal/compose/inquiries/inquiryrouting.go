// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package inquiries

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/webforms"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// leadObject is the RBAC object and record type of a lead.
const leadObject = "lead"

// Routing decides who receives a website inquiry. Automations reach it as
// automation.InquiryRouting; admit is always the rule owner's context.
type Routing struct {
	db       *database.DB
	fence    webforms.HandlingFence
	provider datasource.SystemOfRecordProvider
}

// NewRouting reads records through provider and holds each form under fence.
func NewRouting(db *database.DB, fence webforms.HandlingFence, provider datasource.SystemOfRecordProvider) Routing {
	return Routing{db: db, fence: fence, provider: provider}
}

var _ automation.InquiryRouting = Routing{}

// Within runs one form's handling under its fence. A busy fence answers as
// handling that is unavailable for now.
func (r Routing) Within(ctx context.Context, form ids.UUID, run func(context.Context) error) error {
	err := r.fence.Within(ctx, form, run)
	if errors.Is(err, webforms.ErrHandlingBusy) {
		return webforms.HandlingUnavailable()
	}
	return err
}

// Validate checks a policy before it is saved, every owner in its routing
// pool included.
func (r Routing) Validate(ctx context.Context, policy crmcontracts.WebsiteInquiryPolicy) error {
	return r.validatePolicy(ctx, policy, true)
}

func (r Routing) validatePolicy(ctx context.Context, policy crmcontracts.WebsiteInquiryPolicy, requirePool bool) error {
	if _, err := webforms.NewStore(r.db, slog.Default()).GetForm(ctx, ids.UUID(policy.FormId)); err != nil {
		return err
	}
	for _, object := range []string{leadObject, "contact", "activity"} {
		if err := auth.Require(ctx, object, principal.ActionRead); err != nil {
			return err
		}
	}
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return err
	}
	if policy.Mode != crmcontracts.WebsiteInquiryModeCoordinator {
		if err := auth.Require(ctx, leadObject, principal.ActionUpdate); err != nil {
			return err
		}
	}
	candidates := []ids.UUID{ids.UUID(policy.CoordinatorId)}
	if requirePool && policy.Routing.Owners != nil {
		for _, id := range *policy.Routing.Owners {
			candidates = append(candidates, ids.UUID(id))
		}
	}
	if err := r.ensureEligible(ctx, candidates); err != nil {
		return err
	}
	coordinator, err := r.human(ctx, ids.UUID(policy.CoordinatorId))
	if err != nil {
		return err
	}
	for _, object := range []string{leadObject, "activity"} {
		if err := auth.Require(coordinator, object, principal.ActionRead); err != nil {
			return err
		}
	}
	return r.checkRequestType(ctx, policy)
}

// Choose picks the recipient of an inquiry and the reason, and writes the lead
// routing it decides.
func (r Routing) Choose(ctx, admit context.Context, policy crmcontracts.WebsiteInquiryPolicy, target datasource.EntityRef, source ids.UUID) (ids.UUID, string, error) {
	return r.pickRecipient(ctx, admit, policy, target, source, true)
}

// Preview picks the recipient as Choose does and writes nothing.
func (r Routing) Preview(ctx, admit context.Context, policy crmcontracts.WebsiteInquiryPolicy, target datasource.EntityRef, source ids.UUID) (ids.UUID, string, error) {
	return r.pickRecipient(ctx, admit, policy, target, source, false)
}

// ensureEligible refuses the policy when any candidate may not hold a task.
func (r Routing) ensureEligible(ctx context.Context, candidates []ids.UUID) error {
	return r.db.Tx(ctx, func(tx pgx.Tx) error {
		eligible, err := auth.EligibleAssignees(ctx, tx, candidates)
		if err != nil {
			return err
		}
		for _, id := range candidates {
			if !eligible[id] {
				return &auth.AssigneeNotAllowedError{}
			}
		}
		return nil
	})
}

func (r Routing) pickRecipient(ctx, admit context.Context, policy crmcontracts.WebsiteInquiryPolicy, target datasource.EntityRef, source ids.UUID, apply bool) (ids.UUID, string, error) {
	owner, err := r.owner(admit, target)
	if err != nil {
		return ids.Nil, "", err
	}
	if owner != nil {
		return r.existingOwner(admit, *owner, ids.UUID(policy.CoordinatorId), target, source)
	}
	if target.Type != datasource.EntityLead || policy.Mode == crmcontracts.WebsiteInquiryModeCoordinator {
		return ids.UUID(policy.CoordinatorId), "coordinator_review", nil
	}
	denied, err := r.poolDenied(admit, policy, target, source)
	if err != nil {
		return ids.Nil, "", err
	}
	if denied {
		return ids.UUID(policy.CoordinatorId), "recipient_no_longer_eligible", nil
	}
	decision, err := r.routeLead(ctx, admit, policy, target, apply)
	if err != nil {
		return ids.Nil, "", err
	}
	if decision.Assigned {
		return decision.OwnerID.UUID, decision.Reason, nil
	}
	if decision.Reason == "already_owned" {
		owner, err := r.owner(admit, target)
		if err != nil {
			return ids.Nil, "", err
		}
		if owner != nil {
			return r.existingOwner(admit, *owner, ids.UUID(policy.CoordinatorId), target, source)
		}
		return ids.Nil, "", apperrors.ErrConflict
	}
	return ids.UUID(policy.CoordinatorId), "coordinator_review", nil
}

// routeLead runs the policy's lead routing, and writes its decision only when
// apply is set.
func (r Routing) routeLead(ctx, admit context.Context, policy crmcontracts.WebsiteInquiryPolicy, target datasource.EntityRef, apply bool) (contacts.RoutingDecision, error) {
	raw, err := json.Marshal(policy.Routing)
	if err != nil {
		return contacts.RoutingDecision{}, err
	}
	config, err := contacts.ParseRoutingConfig(raw)
	if err != nil {
		return contacts.RoutingDecision{}, err
	}
	router := contacts.NewStore(r.db)
	if apply {
		return router.RouteLeadAs(ctx, admit, ids.From[ids.LeadKind](target.ID), config)
	}
	return router.PreviewLeadRouting(admit, ids.From[ids.LeadKind](target.ID), config)
}

// poolDenied reports whether any owner in the routing pool may no longer
// receive this inquiry.
func (r Routing) poolDenied(admit context.Context, policy crmcontracts.WebsiteInquiryPolicy, target datasource.EntityRef, source ids.UUID) (bool, error) {
	if policy.Routing.Owners == nil {
		return false, nil
	}
	for _, candidate := range *policy.Routing.Owners {
		if err := r.CheckRecipient(admit, ids.UUID(candidate), target, source); err != nil {
			if inquiryRecipientDenied(err) {
				return true, nil
			}
			return false, err
		}
	}
	return false, nil
}

func (r Routing) existingOwner(ctx context.Context, owner, coordinator ids.UUID, target datasource.EntityRef, source ids.UUID) (ids.UUID, string, error) {
	if err := r.CheckRecipient(ctx, owner, target, source); err == nil {
		return owner, "existing_owner", nil
	} else if inquiryRecipientDenied(err) {
		return coordinator, "owner_needs_review", nil
	} else {
		return ids.Nil, "", err
	}
}

// CheckRecipient refuses a colleague who may not hold the task, or who cannot
// read the target record or the source inquiry note.
func (r Routing) CheckRecipient(ctx context.Context, id ids.UUID, target datasource.EntityRef, source ids.UUID) error {
	human, err := r.recipient(ctx, id, target)
	if err != nil {
		return err
	}
	_, err = activities.NewStore(r.db).GetActivityContent(human, ids.From[ids.ActivityKind](source), storekit.LiveOnly)
	return err
}

func (r Routing) recipient(ctx context.Context, id ids.UUID, target datasource.EntityRef) (context.Context, error) {
	err := r.db.Tx(ctx, func(tx pgx.Tx) error { return auth.EnsureAssignee(ctx, tx, id) })
	if err != nil {
		return nil, err
	}
	human, err := r.human(ctx, id)
	if err != nil {
		return nil, err
	}
	_, err = r.provider.Read(human, target)
	return human, err
}

func (r Routing) human(ctx context.Context, id ids.UUID) (context.Context, error) {
	workspace, err := r.db.Workspace(ctx)
	if err != nil {
		return nil, err
	}
	rbac, err := identity.NewService(r.db.Pool()).EffectiveRBAC(ctx, workspace.UUID, id)
	if err != nil {
		return nil, err
	}
	return principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalHuman, ID: principal.HumanIDPrefix + id.String(), UserID: id, TeamIDs: rbac.TeamIDs, Permissions: rbac.Permissions}), nil
}

func (r Routing) owner(ctx context.Context, target datasource.EntityRef) (*ids.UUID, error) {
	record, err := r.provider.Read(ctx, target)
	if err != nil {
		return nil, err
	}
	var row struct {
		Owner *ids.UUID `json:"owner_id"`
	}
	if err := json.Unmarshal(record.Fields, &row); err != nil {
		return nil, err
	}
	return row.Owner, nil
}

func inquiryRecipientDenied(err error) bool {
	_, ineligible := errors.AsType[*auth.AssigneeNotAllowedError](err)
	return ineligible || errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied)
}

// ReuseArrivalTask is activities.Store.ReuseWebsiteArrivalTask, for the
// automation package, which cannot import activities.
func (r Routing) ReuseArrivalTask(ctx, admit context.Context, key string, source, recipient ids.UUID, due time.Time) (bool, error) {
	return activities.NewStore(r.db).ReuseWebsiteArrivalTask(ctx, admit, key, source, recipient, due)
}

func (r Routing) checkRequestType(ctx context.Context, policy crmcontracts.WebsiteInquiryPolicy) error {
	if policy.RequestTypeField == nil {
		return nil
	}
	var columns []fieldcatalog.Column
	err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		columns, err = customfields.ActiveColumnsIn(ctx, tx, leadObject)
		return err
	})
	if err != nil {
		return err
	}
	for _, column := range columns {
		if column.Name == *policy.RequestTypeField && column.Type == fieldcatalog.TypePicklist {
			return nil
		}
	}
	return &automation.ParamError{Field: "params.request_type_field", Reason: "choose an active lead picklist field mapped on the form"}
}

// ExistingTask finds the task already filed for a submission key. A task the
// caller cannot read is reported as found, with the refusal.
func (r Routing) ExistingTask(ctx, admit context.Context, key string) (crmcontracts.Activity, bool, error) {
	store := activities.NewStore(r.db)
	task, found, err := store.WebsiteInquiryTask(admit, key)
	if err != nil || !found {
		return task, found, err
	}
	if _, err := store.GetActivityContent(ctx, ids.From[ids.ActivityKind](ids.UUID(task.Id)), storekit.IncludeArchived); err != nil {
		return crmcontracts.Activity{}, true, err
	}
	return task, true, nil
}

// TaskSubmissions maps each readable task filed from a website submission to
// its submission key.
func (r Routing) TaskSubmissions(ctx context.Context, idsToRead []ids.ActivityID) (map[ids.UUID]string, error) {
	out := map[ids.UUID]string{}
	if len(idsToRead) == 0 {
		return out, nil
	}
	kind := "task"
	limit := len(idsToRead)
	rows, _, err := activities.NewStore(r.db).ListActivities(ctx, activities.ListActivitiesInput{IDs: idsToRead, Kind: &kind, ReadableOnly: true, IncludeArchived: true, Limit: &limit})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.SourceSystem == nil || row.SourceId == nil {
			continue
		}
		if key, known := provenance.WebsiteInquirySubmission(*row.SourceSystem, *row.SourceId); known {
			out[ids.UUID(row.Id)] = key
		}
	}
	return out, nil
}
