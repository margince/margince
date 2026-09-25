// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// What a broker binding is served under, and what a run does when that cannot
// be met: the upstream preferences a record names, the refusal that says which
// variable lifts them, and the pre-flight that finds an unservable binding
// before the corpus pays for it.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// bindingRole names one of a run's two bindings by the variables that set it,
// so a refusal sends the reader to the one they have to edit.
type bindingRole struct {
	name, modelVar string
	// upstreamVar is the make variable and upstreamEnv the one it exports;
	// routedHint is where the preferences live when ROUTING= named the binding.
	upstreamVar, upstreamEnv, routedHint string
}

var (
	candidateRole = bindingRole{
		name: "candidate", modelVar: "MODEL= (MARGINCE_AICERT_MODEL)",
		upstreamVar: "UPSTREAM", upstreamEnv: "MARGINCE_AICERT_UPSTREAM",
		routedHint: "; under ROUTING=, that tier's own routing block",
	}
	judgeRole = bindingRole{
		name: "judge", modelVar: "JUDGE= (MARGINCE_AICERT_JUDGE_MODEL)",
		upstreamVar: "JUDGE_UPSTREAM", upstreamEnv: "MARGINCE_AICERT_JUDGE_UPSTREAM",
	}
)

// profileFor is the profile this role's binding is validated under, in a run
// that files its records under record.
//
// The candidate is held to the record's profile, because that profile is the
// record's claim about where the certified deployment sends its text. The
// judge is not part of that deployment. It is the lane's own grader, it is sent
// only the hand-authored corpus and the candidate's answer to it — never an
// installation's data — and the record names it (judge_served_model). Holding
// it to the candidate's profile made a sovereign record depend on a judge that
// fits beside the candidate on the same machine, which on the 24 GB machines
// these presets are written for left only judges too small to grade reliably.
func (r bindingRole) profileFor(record ai.Profile) ai.Profile {
	if r == judgeRole {
		return ai.ProfileCloudFrontier
	}
	return record
}

// refuseUnreachableUpstream refuses preferences on a binding they cannot reach:
// accepted, they would be applied to nothing and the record would name them.
func refuseUnreachableUpstream(cfg RunnerConfig) error {
	var errs []error
	for _, b := range []struct {
		role    bindingRole
		binding ai.ProviderConfig
	}{{candidateRole, cfg.Binding}, {judgeRole, cfg.JudgeBinding}} {
		if b.binding.Routing != nil && !ai.UpstreamPreferencesApply(b.binding) {
			errs = append(errs, fmt.Errorf("%s names upstream preferences, but the %s %s:%s is not a broker binding on an OpenRouter host; unset it",
				b.role.upstreamEnv, b.role.name, b.binding.Provider, b.binding.Model))
		}
	}
	return errors.Join(errs...)
}

// unservable is a binding's call failure with the edit that fixes it named: a
// preference no host meets fails identically on every attempt, and a rejected
// request is the vendor refusing the request's own shape rather than the model
// answering it.
func unservable(role bindingRole, err error) error {
	switch {
	case errors.Is(err, ai.ErrNoUpstreamHost):
		return fmt.Errorf("no host the broker offers for the %s matches the upstream preferences it is served under — "+
			"set %s='{}' (%s) for the broker's own routing, or preferences a host of this model meets%s; no record is written: %w",
			role.name, role.upstreamVar, role.upstreamEnv, role.routedHint, err)
	case errors.Is(err, model.ErrRequestRejected):
		return fmt.Errorf("the vendor behind the %s (%s) refused the shape of the request itself — its response schema or a "+
			"parameter it names — and would on every run; no record is written, because a refused request measures the "+
			"request against that vendor, not the model: %w", role.name, role.modelVar, err)
	default:
		return err
	}
}

// preflightPrompt and preflightMaxTokens are the pre-flight's whole request:
// enough to prove a host serves the binding, and no more.
const (
	preflightPrompt    = "Reply with the single word: ready."
	preflightMaxTokens = 64
)

// preflight sends one small call to every binding the run will use, before the
// corpus: a key, slug or preference that cannot be served fails in seconds
// rather than after the scenarios ahead of it were paid for.
func preflight(ctx context.Context, cfg RunnerConfig, tasks []ai.Task, hooks *certifyHooks, log *slog.Logger) error {
	if hooks == nil {
		hooks = &certifyHooks{}
	}
	var errs []error
	for _, p := range preflightProbes(cfg, tasks, hooks) {
		// A binding that cannot be set up is certifyTask's to report, per task,
		// where it costs that task's record alone; the pre-flight only notes it.
		routing, err := ladderForTask(p.role.name, p.binding, p.role.profileFor(cfg.recordProfile()), p.ladderTask)
		if err != nil {
			log.DebugContext(ctx, "aicert: pre-flight skipped a binding it could not route", "role", p.role.name, "err", err)
			continue
		}
		router, err := compose.NewLocalRouterForCert(routing, append([]ai.LocalOption{ai.WithoutResultCache()}, p.opts...)...)
		if err != nil {
			log.DebugContext(ctx, "aicert: pre-flight skipped a binding it could not build", "role", p.role.name, "err", err)
			continue
		}
		_, _, err = router.Complete(ctx, p.servedTask, model.Request{
			Messages:  []model.Message{{Role: roleUser, Content: preflightPrompt}},
			MaxTokens: preflightMaxTokens,
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("pre-flight: the %s %s:%s could not be served, so no scenario was run: %w",
				p.role.name, p.binding.Provider, p.binding.Model, unservable(p.role, err)))
			continue
		}
		log.InfoContext(ctx, "aicert: pre-flight served", "role", p.role.name, "model", p.binding.Model)
	}
	return errors.Join(errs...)
}

// preflightProbe is one binding to ask, bound on ladderTask's ladder and asked
// as servedTask, the task it will actually serve.
type preflightProbe struct {
	role                   bindingRole
	binding                ai.ProviderConfig
	ladderTask, servedTask ai.Task
	opts                   []ai.LocalOption
}

// preflightProbes is each distinct candidate the tasks resolve to, then the one
// judge. A task whose rung is unbound is left to taskBindings to report.
func preflightProbes(cfg RunnerConfig, tasks []ai.Task, hooks *certifyHooks) []preflightProbe {
	var probes []preflightProbe
	seen := map[string]bool{}
	for _, task := range tasks {
		candidate := cfg.Binding
		if cfg.Routing != nil {
			resolved, _, ok := resolveBinding(*cfg.Routing, task)
			if !ok {
				continue
			}
			candidate = resolved
		}
		if key := bindingKey(candidate); !seen[key] {
			seen[key] = true
			probes = append(probes, preflightProbe{candidateRole, candidate, task, task, hooks.candidateOpts})
		}
	}
	if len(tasks) > 0 {
		probes = append(probes, preflightProbe{judgeRole, cfg.JudgeBinding, tasks[0], ai.TaskCertJudge, hooks.judgeOpts})
	}
	return probes
}
