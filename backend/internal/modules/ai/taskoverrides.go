// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// An admin's per-task request settings: how hard the task's model thinks, and
// how long one call may take.
//
// A setting of its own rather than part of ai.routing: it changes nothing a
// binding is, so it must not move the routing version every cached brief is
// keyed on, and an admin tuning one task's timeout should not conflict with
// another rebinding a tier.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/platform/settings"
)

// TaskOverridesKey is the settings key the overrides are stored under.
const TaskOverridesKey = "ai.task_overrides"

// TaskOverride is one task's settings. A zero field keeps the product's own
// behaviour for it, so an empty override is no override.
type TaskOverride struct {
	// Thinking is the exact level every site of the task is sent at. It
	// outranks the binding's level and the site's floor.
	Thinking string `json:"thinking,omitempty"`
	// DecisionTimeoutMs bounds the decision-model call of a deciding task.
	DecisionTimeoutMs int `json:"decision_timeout_ms,omitempty"`
	// AttemptTimeoutMs bounds each model call on the task's ladder.
	AttemptTimeoutMs int `json:"attempt_timeout_ms,omitempty"`
}

// TaskOverrides holds one override per task that has one, keyed by task.
type TaskOverrides map[Task]TaskOverride

// The bounds an override is held to. The defaults are today's behaviour, so an
// installation with no overrides calls exactly as it did before they existed.
const (
	MinDecisionTimeout = 5 * time.Second
	MaxDecisionTimeout = 60 * time.Second
	MinAttemptTimeout  = 10 * time.Second
	MaxAttemptTimeout  = CallCeiling
)

// TaskThinkingLevels are the levels an override may name: the four every
// adapter maps (docs/reference/ai-thinking.md), the same words a site floor
// takes. The contract's AiTaskOverride mirrors these and the bounds above,
// held by gates/airoutingschema_test.go.
var TaskThinkingLevels = []string{effortMinimal, effortLow, effortMedium, effortHigh}

// TaskOverridesSetting stores the overrides installation-wide, under the same
// object as the binding: deciding how an installation's requests are sent is
// the routing admin's decision.
var TaskOverridesSetting = settings.Define[TaskOverrides](
	TaskOverridesKey,
	routingSettingsObject,
	"update",
	TaskOverrides{},
	validateTaskOverrides,
).AsInstallationIdentity()

// validateTaskOverrides refuses each bad field by its path, `<task>.<field>`.
//
// A task the contract no longer declares is not refused: a stored override
// outlives a release that drops its task, and refusing it would make every
// other task's override unsaveable. Calls never read it (Effective), and the
// preview lists it as stale.
func validateTaskOverrides(v TaskOverrides) error {
	var errs []error
	for _, task := range slices.Sorted(maps.Keys(v)) {
		if _, known := taskLadders[task]; !known {
			continue
		}
		errs = append(errs, v[task].validate(string(task), TaskDecides(task)))
	}
	return joinFaults(errs...)
}

func (o TaskOverride) validate(task string, decides bool) error {
	var errs []error
	if o.Thinking != "" && !slices.Contains(TaskThinkingLevels, o.Thinking) {
		errs = append(errs, invalidAt(task+".thinking",
			fmt.Sprintf("must be one of %s. You wrote %q.", strings.Join(TaskThinkingLevels, ", "), o.Thinking)))
	}
	if o.DecisionTimeoutMs != 0 && !decides {
		errs = append(errs, invalidAt(task+".decision_timeout_ms", "applies only to a task that asks a decision model first; remove it"))
	} else if o.DecisionTimeoutMs != 0 {
		errs = append(errs, withinBounds(task+".decision_timeout_ms", o.DecisionTimeoutMs, MinDecisionTimeout, MaxDecisionTimeout))
	}
	if o.AttemptTimeoutMs != 0 {
		errs = append(errs, withinBounds(task+".attempt_timeout_ms", o.AttemptTimeoutMs, MinAttemptTimeout, MaxAttemptTimeout))
	}
	return joinFaults(errs...)
}

func withinBounds(path string, ms int, low, high time.Duration) error {
	// Compared in milliseconds: converting first would let a huge value wrap
	// into the range.
	if int64(ms) < low.Milliseconds() || int64(ms) > high.Milliseconds() {
		return invalidAt(path, fmt.Sprintf("must be between %d and %d ms. You wrote %d.", low.Milliseconds(), high.Milliseconds(), ms))
	}
	return nil
}

// EffectiveTask is what one task's calls are sent with once its override is
// applied over the product's own values.
type EffectiveTask struct {
	Thinking        string
	DecisionTimeout time.Duration
	AttemptTimeout  time.Duration
}

// TaskDefaults is what a task's calls are sent with when nothing overrides it.
func TaskDefaults() EffectiveTask {
	return EffectiveTask{DecisionTimeout: DecisionCallTimeout, AttemptTimeout: CallCeiling}
}

// Effective is what task's calls are sent with. An override stored for a task
// the contract no longer declares is ignored (Stale lists it).
func (v TaskOverrides) Effective(task Task) EffectiveTask {
	out := TaskDefaults()
	o := v[task]
	if _, known := taskLadders[task]; !known {
		return out
	}
	out.Thinking = o.Thinking
	if o.DecisionTimeoutMs != 0 {
		out.DecisionTimeout = time.Duration(o.DecisionTimeoutMs) * time.Millisecond
	}
	if o.AttemptTimeoutMs != 0 {
		out.AttemptTimeout = time.Duration(o.AttemptTimeoutMs) * time.Millisecond
	}
	return out
}

// Stale lists the stored overrides for tasks the contract no longer declares.
func (v TaskOverrides) Stale() []Task {
	var stale []Task
	for _, task := range slices.Sorted(maps.Keys(v)) {
		if _, known := taskLadders[task]; !known {
			stale = append(stale, task)
		}
	}
	return stale
}

// Revision identifies a stored value, for the If-Match a save is held to.
func (v TaskOverrides) Revision() string {
	// A map of strings and ints: marshal cannot fail on it, and it sorts the
	// keys, so the digest is the same in every process.
	encoded, _ := json.Marshal(v) //nolint:errchkjson // string-keyed map of scalars; marshal cannot fail
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

// SetTaskOverrides publishes the overrides every later call on this Router is
// sent with. A call reads them once (callSettings), so it never mixes two.
func (r *Router) SetTaskOverrides(v TaskOverrides) {
	r.overrides.Store(&v)
}

// TaskOverridesRevision is the revision this Router is serving, which the
// watcher compares against the stored one.
func (r *Router) TaskOverridesRevision() string {
	return r.taskOverrides().Revision()
}

func (r *Router) taskOverrides() TaskOverrides {
	if v := r.overrides.Load(); v != nil {
		return *v
	}
	return TaskOverrides{}
}

// taskSettings is what task's calls are sent with right now.
func (r *Router) taskSettings(task Task) EffectiveTask {
	return r.taskOverrides().Effective(task)
}

// callSettings is what one logical call's task is sent with, read on first use
// and kept, so a save mid-call cannot give two of its attempts different
// deadlines or record one it was not sent under.
func (r *Router) callSettings(lc *logicalCall, task Task) EffectiveTask {
	if lc.settings == nil {
		settings := r.taskSettings(task)
		lc.settings = &settings
	}
	return *lc.settings
}
