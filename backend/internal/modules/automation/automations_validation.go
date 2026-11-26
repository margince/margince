// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"encoding/json"
	"fmt"
)

// Editing params or resuming rechecks the catalog and current references.
// A paused rule must not resume against a list or form its author can no longer use.
func (s *AutomationStore) revalidate(ctx context.Context, before Automation, in UpdateAutomationInput) error {
	if err := inquiryKeepsItsForm(before, in); err != nil {
		return err
	}

	resuming := resumesRule(before, in)
	handsOn := handsOnAnEnabledRule(before, in)
	paramsKept := in.Params == nil || sameParams(before.Params, in.Params)
	if !resuming && !handsOn && paramsKept {
		return nil
	}
	entry, ok := CatalogEntryByKey(before.Key)
	if !ok {
		return fmt.Errorf("automation %s names catalog key %q the registry no longer carries", before.ID, before.Key)
	}
	refs, err := s.referenceContext(ctx, entry, in, handsOn)
	if err != nil {
		return err
	}
	params, err := paramsToCheck(ctx, entry, before, in)
	if err != nil {
		return err
	}
	// An owner-only hand-on keeps the params the rule was saved with. Only the
	// new owner's access to each record they name decides it.
	if handsOn && !resuming && paramsKept {
		return s.handOnAccess(refs, entry, params)
	}
	if err := entry.Validate(params); err != nil {
		return err
	}
	if err := confirmScheduledPopulation(before, in); err != nil {
		return err
	}
	return s.validateRefs(refs, entry, params)
}

// inquiryKeepsItsForm refuses an edit that moves an inquiry policy to another
// website form.
func inquiryKeepsItsForm(before Automation, in UpdateAutomationInput) error {
	if before.Key != inquiryRecipeName || in.Params == nil {
		return nil
	}
	next, err := json.Marshal(in.Params)
	if err != nil {
		return err
	}
	oldPolicy, err := ParseInquiryPolicy(before.Params)
	if err != nil {
		return err
	}
	newPolicy, err := ParseInquiryPolicy(next)
	if err != nil {
		return err
	}
	if oldPolicy.FormId != newPolicy.FormId {
		return inquiryParamError("form_id", "an inquiry policy stays with its website form")
	}
	return nil
}

// referenceContext is whose grants decide the reference checks. A rule handed
// on acts on its new owner's grants and row access from now on, as on a full
// save by that owner.
func (s *AutomationStore) referenceContext(ctx context.Context, entry CatalogEntry, in UpdateAutomationInput, handsOn bool) (context.Context, error) {
	if !handsOn {
		return ctx, nil
	}
	bound, err := s.authorizerContext(ctx, in.OwnerID.UUID)
	if err != nil {
		return nil, err
	}
	if err := requireAuthorCeiling(bound, entry); err != nil {
		return nil, err
	}
	return bound, nil
}

// paramsToCheck is the params the rule runs with after this update. New params
// must be ones the caller may author; otherwise the stored ones stay.
func paramsToCheck(ctx context.Context, entry CatalogEntry, before Automation, in UpdateAutomationInput) (map[string]any, error) {
	if in.Params != nil {
		return in.Params, requireAuthorCeiling(ctx, entry)
	}
	var params map[string]any
	if err := json.Unmarshal(before.Params, &params); err != nil {
		return nil, fmt.Errorf("automation %s: reading its stored params: %w", before.ID, err)
	}
	return params, nil
}

// resumesRule reports an update that turns a paused rule back on.
func resumesRule(before Automation, in UpdateAutomationInput) bool {
	return in.Enabled != nil && *in.Enabled && !before.Enabled
}

// handsOnAnEnabledRule reports an update that gives a rule enabled after it to
// another owner.
func handsOnAnEnabledRule(before Automation, in UpdateAutomationInput) bool {
	if in.OwnerID == nil || (before.OwnerID != nil && before.OwnerID.UUID == in.OwnerID.UUID) {
		return false
	}
	if in.Enabled != nil {
		return *in.Enabled
	}
	return before.Enabled
}

func scheduledPopulationRule(key string) bool {
	_, quiet := activityScanHandlers[key]
	_, dated := dateFieldScanHandlers[key]
	return quiet || dated
}

func confirmScheduledPopulation(before Automation, in UpdateAutomationInput) error {
	if !scheduledPopulationRule(before.Key) {
		return nil
	}
	enabled := before.Enabled
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	changesPopulation := in.Params != nil && !sameParams(before.Params, in.Params)
	if enabled && (!before.Enabled || changesPopulation) && !in.InitialRunConfirmed {
		return &ParamError{Field: "initial_run_confirmed", Reason: "preview the initial run and confirm that existing matching records may receive work"}
	}
	return nil
}

// validateRefs asks the entry's reference check, when it has one.
func (s *AutomationStore) validateRefs(ctx context.Context, entry CatalogEntry, params map[string]any) error {
	if entry.ValidateRefs != nil && s.lists == nil {
		return &ParamError{Field: keyCatalogKey, Reason: "needs lists, which are not switched on in this installation"}
	}
	if err := validateRequiredSetup(entry, params); err != nil {
		return err
	}
	if entry.Key == renewalReminderName {
		_, err := renewalPreviewParams(ctx, s.catalog, Automation{Key: entry.Key}, AutomationPreviewInput{Params: params})
		return err
	}
	if isQuietRule(entry.Key) {
		return s.validateQuietRefs(ctx, params)
	}
	if entry.Key == stageChangeCreateTaskName || entry.Key == stageChangeNotifyName {
		return validateStageScope(ctx, s.stages, params)
	}
	if entry.Key == inquiryRecipeName {
		return s.validateInquiryRefs(ctx, params)
	}
	if entry.ValidateRefs == nil {
		return nil
	}
	return entry.ValidateRefs(ctx, s.lists, params)
}

// validateQuietRefs asks the quiet population scan whether the population the
// params name is one the caller may use.
func (s *AutomationStore) validateQuietRefs(ctx context.Context, params map[string]any) error {
	if s.quiet == nil {
		return &ParamError{Field: keyCatalogKey, Reason: "quiet reminders are not available"}
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	population, err := quietPopulation(raw)
	if err != nil {
		return err
	}
	return s.quiet.ValidateQuietPopulation(ctx, population)
}

// validateInquiryRefs asks inquiry routing whether the policy's form, colleagues
// and fields are ones the caller may use.
func (s *AutomationStore) validateInquiryRefs(ctx context.Context, params map[string]any) error {
	if s.inquiries == nil {
		return &ParamError{Field: keyCatalogKey, Reason: "website inquiry handling is not available"}
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	policy, err := ParseInquiryPolicy(raw)
	if err != nil {
		return err
	}
	return s.inquiries.Validate(ctx, policy)
}

// WithStageLookup lets stage rules check that the stages they name belong to
// their pipeline.
func (s *AutomationStore) WithStageLookup(stages StageLookup) *AutomationStore {
	s.stages = stages
	return s
}

// WithStageLookup hands stages to the handlers' automation store.
func (h Handlers) WithStageLookup(stages StageLookup) Handlers {
	h.automations.WithStageLookup(stages)
	return h
}
