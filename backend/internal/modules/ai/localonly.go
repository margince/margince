// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// localOnlyLadder drops every rung bound to a network-hosted provider when task
// declares `local_only` in api/ai-tasks.yaml.
//
// The guarantee these tasks carry is about WHERE the prompt goes, and until
// this existed the only thing carrying it was the name of the tier on the
// ladder. `local_small` is a SIZE class in this tree, not a location — four of
// the five shipped presets bind it to a hosted vendor, and
// config/presets/consumer_class_brokered.yaml does so on purpose, to serve
// local-class weights from a broker. So the name could never have carried it.
//
// Narrowing at the rung rather than refusing the configuration is deliberate.
// A config-load refusal would make those presets unloadable — an outage in
// place of a privacy fix — and it would refuse them for tasks that have no such
// requirement and legitimately ride the same rung. Here only the affected call
// is affected.
//
// An unbound tier drops out too: routeMeta has no entry, the provider reads
// empty, and empty is not local. That is the same answer BoundLadder gives, and
// the right one — a rung nothing is bound to cannot serve the call either.
func localOnlyLadder(task Task, meta map[Tier]routeMeta, ladder []Tier) []Tier {
	if !LocalOnly(task) {
		return ladder
	}
	out := make([]Tier, 0, len(ladder))
	for _, tier := range ladder {
		if localProviders[meta[tier].provider] {
			out = append(out, tier)
		}
	}
	return out
}

// ErrLocalOnlyUnservable reports a local-only task that this configuration
// cannot serve: every rung of its ladder is bound to a hosted provider.
//
// A SENTINEL because the callers have to tell it from a provider that failed.
// A failure is worth retrying and worth alarming about; this is neither — it is
// a standing property of the binding, true on every row until somebody rebinds
// a rung, and a caller that retried it would fail the same way forever.
//
// What a caller does with it is theirs: the counterparty verdict asks a human
// instead, and the confidentiality verdict holds the thread. Both already have
// that path for an installation with no model at all, which is the same
// situation from the pass's point of view — there is no model it may ask.
var ErrLocalOnlyUnservable = errors.New("ai: no local provider is bound for a local-only task")

// localOnlyRefusal explains a local-only task whose every rung is hosted.
//
// It names what each rung is actually bound to, because the operator's next
// question is always "bound to what?" and the answer is in a config file they
// may not have written — a preset they copied, or a rebind made through the
// settings UI months ago.
func localOnlyRefusal(task Task, meta map[Tier]routeMeta) error {
	return fmt.Errorf("%w: task %s needs one of (%s), and its ladder binds %s",
		ErrLocalOnlyUnservable, task, localProviderNames(), boundProviderSummary(task, meta))
}

// localProviderNames lists the providers that can serve a local-only task, so a
// refusal says what an operator may bind instead of only what they may not.
func localProviderNames() string {
	names := make([]string, 0, len(localProviders))
	for name := range localProviders {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// boundProviderSummary renders each of task's rungs as `tier=provider`, in
// ladder order, naming an unbound rung as such rather than omitting it — a rung
// missing from the list would read as a rung that does not exist.
func boundProviderSummary(task Task, meta map[Tier]routeMeta) string {
	rungs := make([]string, 0, len(taskLadders[task]))
	for _, tier := range taskLadders[task] {
		provider := meta[tier].provider
		if provider == "" {
			provider = "unbound"
		}
		rungs = append(rungs, fmt.Sprintf("%s=%s", tier, provider))
	}
	return strings.Join(rungs, ", ")
}
