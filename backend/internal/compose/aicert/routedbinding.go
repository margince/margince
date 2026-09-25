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
	"context"
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

// taskBindings is the candidate one task is certified against and the judge
// that grades it. An error costs that task its record and no other: another
// task's rung may be bound perfectly well, and one gap must not cost every
// record.
func taskBindings(ctx context.Context, cfg RunnerConfig, task ai.Task, log *slog.Logger) (candidate, judge ai.ProviderConfig, err error) {
	candidate = cfg.Binding
	if cfg.Routing != nil {
		resolved, rung, ok := resolveBinding(*cfg.Routing, task)
		if !ok {
			return ai.ProviderConfig{}, ai.ProviderConfig{}, fmt.Errorf("aicert: task %s: no rung of its ladder %v is bound in the supplied routing, so there is no model to certify it against — and production could not serve it either",
				task, ai.TaskLadder(task))
		}
		candidate = resolved
		log.InfoContext(ctx, "aicert: routed", "task", string(task), "tier", string(rung), "model", resolved.Model)
	}
	judge, err = cfg.judgeFor(candidate)
	if err != nil {
		return ai.ProviderConfig{}, ai.ProviderConfig{}, fmt.Errorf("task %s: %w", task, err)
	}
	log.InfoContext(ctx, "aicert: judged by", "task", string(task), "judge", judge.Model)
	return candidate, judge, nil
}

// validateRoutedBindings refuses a routed run that could not produce a
// trustworthy verdict, BEFORE the first paid call.
//
// The candidate-is-not-the-judge check runs for every task the run will certify,
// not just one: cert_judge's own ladder leads at premium, so against a config
// binding the judge's model there the grader collides with the candidate for
// every premium-led task. Caught here that costs nothing; caught per task it
// would surface midway through a paid corpus, after the tasks before it had been
// billed.
func validateRoutedBindings(cfg RunnerConfig, tasks []ai.Task, log *slog.Logger) error {
	if cfg.JudgeBinding.Provider == "" || cfg.JudgeBinding.Model == "" {
		return errors.New("no judge binding — set MARGINCE_AICERT_JUDGE_MODEL=provider:model; " +
			"the judge is a SECOND model on purpose and is NOT resolved from the routing, because " +
			"cert_judge's own rung would collide with every candidate sharing it")
	}
	if !cfg.Routing.Profile.Valid() {
		return fmt.Errorf("the supplied routing declares profile %q, which is not an environment class; "+
			"a record is filed under it, so a run states which one it measured", cfg.Routing.Profile)
	}
	warnUnboundDegradeTargets(cfg, log)
	return refuseSelfJudgedTasks(cfg, tasks)
}

// refuseSelfJudgedTasks names every task this run certifies whose candidate is
// the judge. A task the run will not certify cannot collide, so TASK= narrows it.
func refuseSelfJudgedTasks(cfg RunnerConfig, tasks []ai.Task) error {
	var collisions []string
	for _, task := range tasks {
		candidate := cfg.Binding
		if cfg.Routing != nil {
			resolved, _, ok := resolveBinding(*cfg.Routing, task)
			if !ok {
				continue // reported per task at run time, where it costs one record
			}
			candidate = resolved
		}
		if _, err := cfg.judgeFor(candidate); err != nil {
			collisions = append(collisions, fmt.Sprintf("%s (%s:%s)", task, candidate.Provider, candidate.Model))
		}
	}
	if len(collisions) > 0 {
		return fmt.Errorf("%d task(s) would be graded by their own candidate — %s: %w",
			len(collisions), strings.Join(collisions, ", "), errSelfJudged)
	}
	return nil
}

// errSelfJudged is judgeFor's refusal: the candidate is the judge.
var errSelfJudged = errors.New("a model grading itself is certified by construction, and one judge " +
	"grades every task of a run — pick a judge this run does not certify with JUDGE= (MARGINCE_AICERT_JUDGE_MODEL)")

// judgeFor is the judge that grades a task certified against candidate, and the
// one place the candidate-is-not-the-judge rule is spelled: both validations and
// the run itself ask it, so what was checked up front is what grades.
func (c RunnerConfig) judgeFor(candidate ai.ProviderConfig) (ai.ProviderConfig, error) {
	if sameModel(candidate, c.JudgeBinding) || cliJudgesOwnFamily(candidate, c.JudgeBinding) {
		return ai.ProviderConfig{}, errSelfJudged
	}
	return c.JudgeBinding, nil
}

// sameModel is the identity a self-grading check compares: provider and model,
// not the host, because one model behind two base URLs still grades itself.
func sameModel(a, b ai.ProviderConfig) bool {
	return a.Provider == b.Provider && a.Model == b.Model
}

// recordProfile is the environment class this run files its records under.
//
// A routed run takes the ROUTING's profile, not the config field beside it. The
// profile is part of a record's identity and is the claim ValidateTierBinding
// enforces, so it has to come from the same document that named the models — a
// caller that set Routing and left Profile at some other value would otherwise
// file records under an environment class the binding never declared, and
// nothing downstream could tell.
//
// Decided here rather than trusted from the caller because Run is the last place
// that knows both, and a programmatic caller has no reason to have kept them
// consistent — the paid lane is not the only way in.
func (c RunnerConfig) recordProfile() ai.Profile {
	if c.Routing != nil {
		return c.Routing.Profile
	}
	return c.Profile
}

// warnUnboundDegradeTargets says which rungs a task can fall to that this
// deployment does not bind.
//
// A warning rather than a refusal: the rung this run MEASURES is bound, and an
// unbound degrade target is a gap in the deployment rather than a reason to
// decline to measure what is there. It is worth saying at all because the cert
// run is the one place the gap is cheap to see — it already holds the routing —
// and in production it surfaces only once the budget band moves a task onto a
// rung nothing can serve.
func warnUnboundDegradeTargets(cfg RunnerConfig, log *slog.Logger) {
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
		log.Warn("aicert: some tiers a task can degrade to are unbound in the supplied routing — those rungs will fail in production and this run does not measure them",
			"unbound", strings.Join(unreachable, ", "))
	}
}
