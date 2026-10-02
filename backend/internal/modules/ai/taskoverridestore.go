// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// TaskOverridesPreview is what a draft of the overrides would do: every field
// a save refuses, what each task would then be sent with, and the stored
// overrides for tasks this build no longer runs.
type TaskOverridesPreview struct {
	Errors    *[]crmcontracts.AiFieldError
	Effective map[Task]EffectiveTask
	Stale     []Task
}

// GetTaskOverrides reads the stored overrides.
func (s *RoutingStore) GetTaskOverrides(ctx context.Context) (TaskOverrides, error) {
	if err := auth.Require(ctx, routingSettingsObject, principal.ActionRead); err != nil {
		return nil, err
	}
	return settings.Get(ctx, s.settings, TaskOverridesSetting)
}

// PreviewTaskOverrides judges next without writing it.
func (s *RoutingStore) PreviewTaskOverrides(ctx context.Context, next TaskOverrides) (TaskOverridesPreview, error) {
	if err := auth.Require(ctx, routingSettingsObject, principal.ActionRead); err != nil {
		return TaskOverridesPreview{}, err
	}
	if err := auth.Require(ctx, routingSettingsObject, principal.ActionUpdate); err != nil {
		return TaskOverridesPreview{}, err
	}
	stored, err := settings.Get(ctx, s.settings, TaskOverridesSetting)
	if err != nil {
		return TaskOverridesPreview{}, err
	}
	out := TaskOverridesPreview{Effective: map[Task]EffectiveTask{}, Stale: next.Stale()}
	if faults := faultsOf(next.refusedAgainst(stored)); len(faults) > 0 {
		out.Errors = faults.toContract()
	}
	for _, task := range AllTasks() {
		out.Effective[task] = next.Effective(task)
	}
	return out, nil
}

// ReplaceTaskOverrides stores next when expected names the stored revision,
// or unconditionally when it is empty. The write is audit-only, as every
// settings write is.
func (s *RoutingStore) ReplaceTaskOverrides(ctx context.Context, next TaskOverrides, expected string) (TaskOverrides, error) {
	if err := auth.Require(ctx, routingSettingsObject, principal.ActionUpdate); err != nil {
		return nil, err
	}
	err := s.settings.WriteTx(ctx, func(tx pgx.Tx) error {
		if err := settings.LockForWrite(ctx, tx, TaskOverridesKey); err != nil {
			return err
		}
		current, err := settings.GetTx(ctx, tx, TaskOverridesSetting)
		if err != nil {
			return err
		}
		if expected != "" && current.Revision() != expected {
			return apperrors.ErrVersionSkew
		}
		if err := next.refusedAgainst(current); err != nil {
			return err
		}
		return settings.SetTx(ctx, s.settings, tx, TaskOverridesSetting, next.withoutEmpty())
	})
	if err != nil {
		return nil, err
	}
	return next.withoutEmpty(), nil
}

// refusedAgainst lists the fields of v a save refuses. An override for a task
// this build does not run is refused only when it is new or changed: one that
// was stored before its task was dropped rides along untouched.
func (v TaskOverrides) refusedAgainst(stored TaskOverrides) error {
	errs := []error{validateTaskOverrides(v)}
	for _, task := range slices.Sorted(maps.Keys(v)) {
		if _, known := taskLadders[task]; known {
			continue
		}
		if was, kept := stored[task]; !kept || was != v[task] {
			errs = append(errs, invalidAt(string(task), fmt.Sprintf("is not a task this installation runs; remove the override for %s", task)))
		}
	}
	return joinFaults(errs...)
}

// withoutEmpty drops the overrides that override nothing, so a reset task
// leaves no row behind and the table stops calling it custom.
func (v TaskOverrides) withoutEmpty() TaskOverrides {
	out := TaskOverrides{}
	for task, o := range v {
		if o != (TaskOverride{}) {
			out[task] = o
		}
	}
	return out
}

// Wire leaves a zero field absent: zero is "keep the product's value", which
// the contract spells as a missing key.
func (o TaskOverride) Wire() crmcontracts.AiTaskOverride {
	var wire crmcontracts.AiTaskOverride
	if o.Thinking != "" {
		level := crmcontracts.AiTaskOverrideThinking(o.Thinking)
		wire.Thinking = &level
	}
	if o.DecisionTimeoutMs != 0 {
		wire.DecisionTimeoutMs = &o.DecisionTimeoutMs
	}
	if o.AttemptTimeoutMs != 0 {
		wire.AttemptTimeoutMs = &o.AttemptTimeoutMs
	}
	return wire
}
