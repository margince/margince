// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"fmt"
	"log/slog"
	"reflect"
	"strings"
)

// LadderWarnings is every finding about cfg's task ladders that an operator
// should read off the log instead of learning from a refused call: a task
// whose ladder binds nothing, and a task whose ladder binds only one upstream.
func (cfg RoutingConfig) LadderWarnings() []string {
	return append(cfg.UnboundLadderWarnings(), cfg.SingleRungLadderWarnings()...)
}

// LogLadderWarnings writes cfg's ladder findings to log, one line each. The
// two boots and a saved routing document all read the same findings through
// here, so a ladder that is one rung deep is named wherever it comes from.
func LogLadderWarnings(log *slog.Logger, cfg RoutingConfig) {
	for _, w := range cfg.LadderWarnings() {
		log.Warn(w)
	}
}

// SingleRungLadderWarnings names the tasks whose ladder has two or more bound
// rungs that all resolve to one upstream. Such a ladder reads as a fallback
// and behaves as one rung: a refusal at that upstream burns every rung in
// milliseconds. It is a warning, not a refusal, because pointing two tiers at
// one model is legal and sometimes deliberate. A ladder with a single bound
// rung is not reported; document_extract and corpus_ask bind one tier by design.
//
// Tasks that share an upstream share one warning, so a preset that binds every
// tier to one local model costs one line at boot, not one per task.
func (cfg RoutingConfig) SingleRungLadderWarnings() []string {
	var order []string
	tasksOf := map[string][]string{}
	for _, task := range AllTasks() {
		rungs := cfg.boundRungs(task)
		if len(rungs) < 2 || !allSameUpstream(rungs) {
			continue
		}
		key := fmt.Sprintf("%s %s", rungs[0].Provider, rungs[0].Model)
		if _, seen := tasksOf[key]; !seen {
			order = append(order, key)
		}
		tasksOf[key] = append(tasksOf[key], string(task))
	}
	warnings := make([]string, 0, len(order))
	for _, key := range order {
		warnings = append(warnings, fmt.Sprintf("tasks %s: every bound rung of the ladder is %s with one upstream selection, "+
			"so a refusal there has no fallback; bind a different model or upstream on a higher tier",
			strings.Join(tasksOf[key], ", "), key))
	}
	return warnings
}

func (cfg RoutingConfig) boundRungs(task Task) []ProviderConfig {
	var rungs []ProviderConfig
	for _, tier := range taskLadders[task] {
		if binding, ok := cfg.Tiers[tier]; ok {
			rungs = append(rungs, binding)
		}
	}
	return rungs
}

func allSameUpstream(rungs []ProviderConfig) bool {
	for _, next := range rungs[1:] {
		if !sameUpstream(rungs[0], next) {
			return false
		}
	}
	return true
}

// sameUpstream compares what decides where a call lands: the model, the
// endpoint (not its spelling) and the broker's upstream selection, resolved
// the way a call is served so an unwritten default equals a written one.
func sameUpstream(a, b ProviderConfig) bool {
	return a.Provider == b.Provider && a.Model == b.Model && a.Location == b.Location &&
		sameEndpoint(a.BaseURL, b.BaseURL) &&
		reflect.DeepEqual(UpstreamPreferencesFor(a), UpstreamPreferencesFor(b))
}
