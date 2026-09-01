// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// The ROUTED lane: how a deployment's own tier→model map becomes the binding
// each task is certified against, and what must hold before a paid call.
//
// Split from runner.go because it answers a question that file does not: runner.go
// drives a run given a binding, and this decides WHICH binding a task gets when
// the run was pointed at a config instead of a model.

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/margince/margince/backend/internal/modules/ai"
)

// resolveBinding is the model a routed run certifies a task against: the first
// rung of its ladder that the deployment actually BINDS.
//
// First bound, not simply first, because that is what production does. The
// router filters a task's ladder to the rungs with a client and serves the
// leading survivor (ai.attemptLadder) — so a deployment that leaves premium
// unbound still serves a premium-led task on cheap_cloud, and a certification
// lane that read only ladder[0] would refuse to measure a task its own
// deployment runs every day. One invariant, spelled on both sides: production
// picks the first bound rung, so certification certifies the first bound rung.
//
// The rungs BELOW it are reachable too, under budget pressure
// (ai.ServableTiers walks that closure) and want their own runs — a sweep, not a
// pooled record that could not say which model answered.
func resolveBinding(routing ai.RoutingConfig, task ai.Task) (ai.ProviderConfig, ai.Tier, bool) {
	for _, tier := range ai.TaskLadder(task) {
		binding, bound := routing.Tiers[tier]
		if bound && binding.Provider != "" && binding.Model != "" {
			return binding, tier, true
		}
	}
	return ai.ProviderConfig{}, "", false
}

// validateRoutedBindings refuses a routed run that could not produce a
// trustworthy verdict, BEFORE the first paid call.
//
// The candidate-is-not-the-judge check runs for every task the routing resolves,
// not just one: cert_judge's own ladder leads at premium, so against a config
// binding claude-haiku-4.5 there the grader collides with the candidate for every
// premium-led task. Caught here that costs nothing; caught per task it would
// surface midway through a paid corpus, after the tasks before it had been
// billed.
func validateRoutedBindings(cfg RunnerConfig, log *slog.Logger) error {
	if cfg.JudgeBinding.Provider == "" || cfg.JudgeBinding.Model == "" {
		return errors.New("no judge binding — set MARGINCE_AICERT_JUDGE_MODEL=provider:model; " +
			"the judge is a SECOND model on purpose and is NOT resolved from the routing, because " +
			"cert_judge's own rung would collide with every candidate sharing it")
	}
	if !cfg.Profile.Valid() {
		return fmt.Errorf("the supplied routing declares profile %q, which is not an environment class; "+
			"a record is filed under it, so a run states which one it measured", cfg.Profile)
	}
	// Every rung a task can REACH, not just the one it leads on. A task whose
	// degrade target is unbound runs fine until the budget band moves it there,
	// then fails in production on a rung certification never looked at — and the
	// cert run is the one place that gap is cheap to see, because it already has
	// the routing in hand.
	var unreachable []string
	for _, task := range ai.AllTasks() {
		if ai.Status(task) != ai.StatusShipped {
			continue
		}
		for _, tier := range ai.ServableTiers(task) {
			binding, bound := cfg.Routing.Tiers[tier]
			if !bound || binding.Provider == "" || binding.Model == "" {
				unreachable = append(unreachable, fmt.Sprintf("%s→%s", task, tier))
			}
		}
	}
	if len(unreachable) > 0 {
		// A warning rather than a refusal: the leading rung is what this run
		// measures, and an unbound degrade target is a gap in the DEPLOYMENT, not
		// a reason to refuse to measure what is bound.
		log.Warn("aicert: some tiers a task can degrade to are unbound in the supplied routing — those rungs will fail in production and this run does not measure them",
			"unbound", strings.Join(unreachable, ", "))
	}

	var collisions []string
	for _, task := range ai.AllTasks() {
		binding, _, ok := resolveBinding(*cfg.Routing, task)
		if !ok {
			continue // reported per task at run time, where it costs one record
		}
		if binding.Provider == cfg.JudgeBinding.Provider && binding.Model == cfg.JudgeBinding.Model {
			collisions = append(collisions, string(task))
		}
	}
	if len(collisions) > 0 {
		return fmt.Errorf("the routing binds %s:%s to the leading rung of %d task(s) — %s — and that is "+
			"also the judge; a model grading itself is certified by construction, so name a different "+
			"MARGINCE_AICERT_JUDGE_MODEL",
			cfg.JudgeBinding.Provider, cfg.JudgeBinding.Model, len(collisions), strings.Join(collisions, ", "))
	}
	return nil
}
