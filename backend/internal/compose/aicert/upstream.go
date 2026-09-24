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
	"strings"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// brokerProvider is the provider a broker binding names. The ai package keeps
// its provider constants unexported, so the spelling is held against its own
// defaulting by TestARecordNamesTheUpstreamPreferencesTheRouterApplies.
const brokerProvider = "openai_compatible"

// upstreamApplies reports whether a binding is the broker case upstream
// preferences reach — the predicate ai.NewLocalRouter defaults by, mirrored
// because it is unexported there and held equal to it by the test above.
func upstreamApplies(binding ai.ProviderConfig) bool {
	return binding.Provider == brokerProvider && ai.IsOpenRouterHost(binding.BaseURL)
}

// effectiveUpstream is the upstream preferences a binding is served under: its
// own declaration, the product default for an undeclared broker binding, and
// nil for a binding no preference reaches. An explicit empty block stays empty,
// because it is the operator asking for the broker's own routing.
func effectiveUpstream(binding ai.ProviderConfig) *ai.OpenRouterRouting {
	switch {
	case binding.Routing != nil:
		return binding.Routing
	case upstreamApplies(binding):
		return ai.DefaultOpenRouterRouting()
	default:
		return nil
	}
}

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

// refuseUnreachableUpstream refuses preferences on a binding they cannot reach:
// accepted, they would be applied to nothing and the record would name them.
func refuseUnreachableUpstream(cfg RunnerConfig) error {
	var errs []error
	for _, b := range []struct {
		role    bindingRole
		binding ai.ProviderConfig
	}{{candidateRole, cfg.Binding}, {judgeRole, cfg.JudgeBinding}} {
		if b.binding.Routing != nil && !upstreamApplies(b.binding) {
			errs = append(errs, fmt.Errorf("%s names upstream preferences, but the %s %s:%s is not a broker binding on an OpenRouter host; unset it",
				b.role.upstreamEnv, b.role.name, b.binding.Provider, b.binding.Model))
		}
	}
	return errors.Join(errs...)
}

// noHostServes reports whether a broker answered that no upstream host matches
// the request's preferences. Read from the broker's own sentence because the ai
// package raises no sentinel for it; a miss only costs the specific hint.
func noHostServes(err error) bool {
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "no endpoints found") || strings.Contains(text, "no allowed providers")
}

// unservable is a binding's call failure with the edit that fixes it named: a
// preference no host meets fails identically on every attempt, and a rejected
// request is the binding or the request shape rather than the model.
func unservable(role bindingRole, err error) error {
	switch {
	case noHostServes(err):
		return fmt.Errorf("no host the broker offers for the %s matches the upstream preferences it is served under — "+
			"set %s='{}' (%s) for the broker's own routing, or preferences a host of this model meets%s; no record is written: %w",
			role.name, role.upstreamVar, role.upstreamEnv, role.routedHint, err)
	case errors.Is(err, model.ErrRequestRejected):
		return fmt.Errorf("the provider rejected the %s's request, and would on every run — check %s, its key and its base URL; "+
			"no record is written, because a refused request measures the binding, not the model: %w", role.name, role.modelVar, err)
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
		routing, err := ladderForTask(p.role.name, p.binding, cfg.recordProfile(), p.ladderTask)
		if err != nil {
			continue // certifyTask reports it per task, where it costs that task's record alone
		}
		router, err := compose.NewLocalRouterForCert(routing, append([]ai.LocalOption{ai.WithoutResultCache()}, p.opts...)...)
		if err != nil {
			continue // as above
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
		if key := candidate.Provider + "|" + candidate.Model + "|" + candidate.BaseURL; !seen[key] {
			seen[key] = true
			probes = append(probes, preflightProbe{candidateRole, candidate, task, task, hooks.candidateOpts})
		}
	}
	if len(tasks) > 0 {
		probes = append(probes, preflightProbe{judgeRole, cfg.JudgeBinding, tasks[0], ai.TaskCertJudge, hooks.judgeOpts})
	}
	return probes
}
