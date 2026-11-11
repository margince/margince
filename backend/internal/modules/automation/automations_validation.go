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
	if before.Key == inquiryRecipeName && in.Params != nil {
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
	}

	resuming := in.Enabled != nil && *in.Enabled && !before.Enabled
	if !resuming && (in.Params == nil || sameParams(before.Params, in.Params)) {
		return nil
	}
	entry, ok := CatalogEntryByKey(before.Key)
	if !ok {
		return fmt.Errorf("automation %s names catalog key %q the registry no longer carries", before.ID, before.Key)
	}
	params := in.Params
	if params == nil {
		if err := json.Unmarshal(before.Params, &params); err != nil {
			return fmt.Errorf("automation %s: reading its stored params: %w", before.ID, err)
		}
	} else {
		if err := requireAuthorCeiling(ctx, entry); err != nil {
			return err
		}
	}
	if err := entry.Validate(params); err != nil {
		return err
	}
	if err := confirmScheduledPopulation(before, in); err != nil {
		return err
	}
	return s.validateRefs(ctx, entry, params)
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
	if entry.Key == renewalReminderName {
		_, err := renewalPreviewParams(ctx, s.catalog, Automation{Key: entry.Key}, AutomationPreviewInput{Params: params})
		return err
	}
	if _, quiet := activityScanHandlers[entry.Key]; quiet {
		if s.quiet == nil {
			return &ParamError{Field: "key", Reason: "quiet reminders are not available"}
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
	if entry.Key == stageChangeCreateTaskName || entry.Key == stageChangeNotifyName {
		return validateStageScope(ctx, s.stages, params)
	}
	if entry.Key == inquiryRecipeName {
		if s.inquiries == nil {
			return &ParamError{Field: "key", Reason: "website inquiry handling is not available"}
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
	if entry.ValidateRefs == nil {
		return nil
	}
	if s.lists == nil {
		return &ParamError{Field: "key", Reason: "needs lists, which are not switched on in this installation"}
	}
	return entry.ValidateRefs(ctx, s.lists, params)
}

func (s *AutomationStore) WithStageLookup(stages StageLookup) *AutomationStore {
	s.stages = stages
	return s
}

func (h Handlers) WithStageLookup(stages StageLookup) Handlers {
	h.automations.WithStageLookup(stages)
	return h
}
