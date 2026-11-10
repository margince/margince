// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

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
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

type inquiryRouting struct {
	db    *database.DB
	fence *webFormFence
}

func newInquiryRouting(db *database.DB) inquiryRouting {
	return inquiryRouting{db: db, fence: newWebFormFence(db)}
}

var _ automation.InquiryRouting = inquiryRouting{}

func (r inquiryRouting) Within(ctx context.Context, form ids.UUID, run func(context.Context) error) error {
	err := r.fence.Within(ctx, form, run)
	if errors.Is(err, webforms.ErrHandlingBusy) {
		return webforms.HandlingUnavailable()
	}
	return err
}

func (r inquiryRouting) Validate(ctx context.Context, policy crmcontracts.WebsiteInquiryPolicy) error {
	if _, err := webforms.NewStore(r.db, slog.Default()).GetForm(ctx, ids.UUID(policy.FormId)); err != nil {
		return err
	}
	for _, object := range []string{"lead", "contact", "activity"} {
		if err := auth.Require(ctx, object, principal.ActionRead); err != nil {
			return err
		}
	}
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return err
	}
	if policy.Mode != crmcontracts.WebsiteInquiryModeCoordinator {
		if err := auth.Require(ctx, "lead", principal.ActionUpdate); err != nil {
			return err
		}
	}
	candidates := []ids.UUID{ids.UUID(policy.CoordinatorId)}
	if policy.Routing.Owners != nil {
		for _, id := range *policy.Routing.Owners {
			candidates = append(candidates, ids.UUID(id))
		}
	}
	if err := r.db.Tx(ctx, func(tx pgx.Tx) error {
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
	}); err != nil {
		return err
	}
	coordinator, err := r.human(ctx, ids.UUID(policy.CoordinatorId))
	if err != nil {
		return err
	}
	for _, object := range []string{"lead", "activity"} {
		if err := auth.Require(coordinator, object, principal.ActionRead); err != nil {
			return err
		}
	}
	return r.checkRequestType(ctx, policy)
}

func (r inquiryRouting) Choose(ctx, admit context.Context, policy crmcontracts.WebsiteInquiryPolicy, target datasource.EntityRef) (ids.UUID, string, error) {
	return r.choose(ctx, admit, policy, target, true)
}

func (r inquiryRouting) Preview(ctx, admit context.Context, policy crmcontracts.WebsiteInquiryPolicy, target datasource.EntityRef) (ids.UUID, string, error) {
	return r.choose(ctx, admit, policy, target, false)
}

func (r inquiryRouting) choose(ctx, admit context.Context, policy crmcontracts.WebsiteInquiryPolicy, target datasource.EntityRef, apply bool) (ids.UUID, string, error) {
	owner, err := r.owner(admit, target)
	if err != nil {
		return ids.Nil, "", err
	}
	if owner != nil {
		if _, err := r.recipient(ctx, *owner, target); err == nil {
			return *owner, "existing_owner", nil
		} else if !inquiryRecipientDenied(err) {
			return ids.Nil, "", err
		}
		return ids.UUID(policy.CoordinatorId), "owner_needs_review", nil
	}
	if target.Type != datasource.EntityLead || policy.Mode == crmcontracts.WebsiteInquiryModeCoordinator {
		return ids.UUID(policy.CoordinatorId), "coordinator_review", nil
	}
	raw, err := json.Marshal(policy.Routing)
	if err != nil {
		return ids.Nil, "", err
	}
	config, err := contacts.ParseRoutingConfig(raw)
	if err != nil {
		return ids.Nil, "", err
	}
	router := contacts.NewStore(r.db)
	var decision contacts.RoutingDecision
	if apply {
		decision, err = router.RouteLeadAs(ctx, admit, ids.From[ids.LeadKind](target.ID), config)
	} else {
		decision, err = router.PreviewLeadRouting(admit, ids.From[ids.LeadKind](target.ID), config)
	}
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
			return *owner, "existing_owner", nil
		}
		return ids.Nil, "", apperrors.ErrConflict
	}
	return ids.UUID(policy.CoordinatorId), "coordinator_review", nil
}

func (r inquiryRouting) CheckRecipient(ctx context.Context, id ids.UUID, target datasource.EntityRef, source ids.UUID) error {
	human, err := r.recipient(ctx, id, target)
	if err != nil {
		return err
	}
	_, err = activities.NewStore(r.db).GetActivityContent(human, ids.From[ids.ActivityKind](source), storekit.LiveOnly)
	return err
}

func (r inquiryRouting) recipient(ctx context.Context, id ids.UUID, target datasource.EntityRef) (context.Context, error) {
	err := r.db.Tx(ctx, func(tx pgx.Tx) error { return auth.EnsureAssignee(ctx, tx, id) })
	if err != nil {
		return nil, err
	}
	human, err := r.human(ctx, id)
	if err != nil {
		return nil, err
	}
	_, err = NewProviderFor(r.db).Read(human, target)
	return human, err
}

func (r inquiryRouting) human(ctx context.Context, id ids.UUID) (context.Context, error) {
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

func (r inquiryRouting) owner(ctx context.Context, target datasource.EntityRef) (*ids.UUID, error) {
	record, err := NewProviderFor(r.db).Read(ctx, target)
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

func (r inquiryRouting) ReuseArrivalTask(ctx, admit context.Context, key string, source, recipient ids.UUID, due time.Time) (bool, error) {
	return activities.NewStore(r.db).ReuseWebsiteArrivalTask(ctx, admit, key, source, recipient, due)
}

func (r inquiryRouting) checkRequestType(ctx context.Context, policy crmcontracts.WebsiteInquiryPolicy) error {
	if policy.RequestTypeField == nil {
		return nil
	}
	var columns []fieldcatalog.Column
	err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		columns, err = customfields.ActiveColumnsIn(ctx, tx, "lead")
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
