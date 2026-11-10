// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/modules/webforms"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

type inquirySettings struct {
	db       *database.DB
	forms    *webforms.Store
	policies *automation.AutomationStore
	fence    *webFormFence
}

func newInquirySettings(db *database.DB, forms *webforms.Store) inquirySettings {
	return inquirySettings{db: db, forms: forms, policies: automation.NewAutomationStore(db).WithInquiryRouting(newInquiryRouting(db)), fence: newWebFormFence(db)}
}

func (s inquirySettings) Save(ctx context.Context, id ids.UUID, version int64, in crmcontracts.WebsiteInquiryHandlingRequest) (crmcontracts.WebsiteInquiryHandling, error) {
	if err := auth.Require(ctx, "automation", principal.ActionRead); err != nil {
		return crmcontracts.WebsiteInquiryHandling{}, err
	}
	var out crmcontracts.WebsiteInquiryHandling
	err := s.fence.Within(ctx, id, func(leased context.Context) error {
		run := func() error {
			return database.WithWorkspaceSnapshot(leased, s.db.Pool(), func(bound context.Context) error {
				var err error
				out, err = s.save(bound, id, version, in)
				return err
			})
		}
		return storekit.RetryLockCycles(leased, run)
	})
	return out, err
}

func (s inquirySettings) save(ctx context.Context, id ids.UUID, version int64, in crmcontracts.WebsiteInquiryHandlingRequest) (crmcontracts.WebsiteInquiryHandling, error) {
	form, err := s.forms.GetForm(ctx, id)
	if err != nil {
		return crmcontracts.WebsiteInquiryHandling{}, err
	}
	if form.Version != version {
		return crmcontracts.WebsiteInquiryHandling{}, apperrors.ErrVersionSkew
	}
	policy, err := s.savePolicy(ctx, form, in)
	if err != nil {
		return crmcontracts.WebsiteInquiryHandling{}, err
	}
	form, err = s.forms.UpdateForm(ctx, id, webforms.UpdateInput{HandlingAutomationID: &policy.ID.UUID, IfVersion: &version})
	if err != nil {
		return crmcontracts.WebsiteInquiryHandling{}, err
	}
	policy, err = s.policies.Update(ctx, policy.ID, automation.UpdateAutomationInput{Enabled: &in.Enabled, IfVersion: &policy.Version})
	if err != nil {
		return crmcontracts.WebsiteInquiryHandling{}, err
	}
	wire, err := automation.WireAutomation(policy)
	if err != nil {
		return crmcontracts.WebsiteInquiryHandling{}, err
	}
	return crmcontracts.WebsiteInquiryHandling{Form: webforms.WireForm(form), Automation: wire}, nil
}

func (s inquirySettings) savePolicy(ctx context.Context, form webforms.Form, in crmcontracts.WebsiteInquiryHandlingRequest) (automation.Automation, error) {
	raw, err := json.Marshal(in.Policy)
	if err != nil {
		return automation.Automation{}, err
	}
	var params map[string]any
	if err := json.Unmarshal(raw, &params); err != nil {
		return automation.Automation{}, err
	}
	if in.AutomationId == nil {
		if form.HandlingAutomationID != nil {
			return automation.Automation{}, apperrors.ErrConflict
		}
		return s.policies.Create(ctx, automation.CreateAutomationInput{Key: provenance.WebsiteInquirySource, Name: in.Name, Params: params})
	}
	id := ids.From[ids.AutomationKind](ids.UUID(*in.AutomationId))
	if form.HandlingAutomationID != nil && *form.HandlingAutomationID != id.UUID {
		return automation.Automation{}, apperrors.ErrConflict
	}
	if in.AutomationVersion == nil {
		return automation.Automation{}, httperr.Validation("automation_version", "required", "the reviewed policy needs its version")
	}
	current, err := s.policies.Get(ctx, id)
	if err != nil {
		return automation.Automation{}, err
	}
	if current.Key != provenance.WebsiteInquirySource {
		return automation.Automation{}, httperr.Validation("automation_id", "wrong_recipe", "choose a website inquiry policy")
	}
	policyVersion := int64(*in.AutomationVersion)
	return s.policies.Update(ctx, id, automation.UpdateAutomationInput{Name: &in.Name, Params: params, IfVersion: &policyVersion})
}
