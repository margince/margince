// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/config"
)

// modelPathSpec names the boot knobs selectModelPath switches on, so a call
// site labels each flag instead of passing anonymous booleans.
type modelPathSpec struct {
	routingPath     string
	fake            bool
	capturePayloads bool
}

// selectModelPath resolves the model path: a routing config for real
// deployments, the offline fake behind an explicit dev flag, or the
// zero path — the runner and the embed lane simply don't start without
// a declared model; nothing is picked silently.
func selectModelPath(ctx context.Context, spec modelPathSpec, pool *pgxpool.Pool, log *slog.Logger) (compose.ModelPath, map[string]map[string]bool, error) {
	// The stored binding, seeded from the routing file when this installation
	// has none yet. Resolved before the switch rather than inside an arm,
	// because "is a model bound" is now a question about the installation and
	// not about whether this process was handed a --ai-routing path.
	cfg, err := compose.ResolveRouting(ctx, pool, spec.routingPath, config.FromOS, log)
	if err != nil {
		return compose.ModelPath{}, nil, err
	}
	candidates := modelPathCandidates(cfg, spec.fake)
	if len(candidates) == 0 {
		// Neither a binding nor --ai-fake: the runner and the embed lane simply
		// don't start, and nothing is picked silently.
		return compose.ModelPath{}, nil, nil
	}
	if !cfg.Unconfigured() {
		// A task whose whole fallback ladder has no bound tier is not a
		// boot error (a deployment may legitimately not run every
		// workload), but it must be loud: log it now, not discover it
		// from a refused call.
		for _, w := range cfg.UnboundLadderWarnings() {
			log.Warn(w)
		}
	}
	for i, candidate := range candidates {
		// The bound model ids travel with the path because they describe the
		// SAME binding: the cost refresh narrows a provider catalog to the
		// models this deployment actually calls.
		path, bound, err := modelPathWithBoundModels(ctx, candidate, pool, spec.capturePayloads, log)
		// Only an unservable BINDING is worth another candidate. A database
		// fault from the embed marker is not: no fallback repairs it, and
		// booting past it would launch a worker whose marker is unestablished
		// while reporting a storage outage as a missing credential.
		var unservable *compose.UnservableBindingError
		if err == nil || i == len(candidates)-1 || !errors.As(err, &unservable) {
			return path, bound, err
		}
		// Loud: a bound installation quietly serving canned text would be the
		// worse of the two failures.
		log.WarnContext(ctx, "the stored model binding cannot be served, and --ai-fake was requested: falling back to the offline fake for this boot. The runner answers with canned text until the binding resolves — bind a servable model under Settings -> AI, or supply the missing credential",
			"error", err)
	}
	return compose.ModelPath{}, nil, nil
}

// modelPathCandidates names the bindings this boot may run on, best first. The
// second entry is a FALLBACK from the first, reached only when the stored
// binding cannot be built.
//
// The fallback is not generosity, and it is why this list exists rather than a
// switch: a dev stack's bootstrap seeds a cloud binding, the engineer running
// it may hold no key for that vendor, and refusing the boot costs them every
// queued job over an AI lane they were not using. A deployment passes no
// --ai-fake, so it still fails closed on a binding it cannot serve.
//
// cmd/api's resolveModelPath resolves the same two candidates on the same
// condition. The roles must agree: a worker that exits where the api falls back
// leaves the queue unrun behind an api that looks healthy, which reads as a
// broken feature rather than an unconfigured stack.
func modelPathCandidates(cfg ai.RoutingConfig, fake bool) []ai.RoutingConfig {
	var candidates []ai.RoutingConfig
	if !cfg.Unconfigured() {
		candidates = append(candidates, cfg)
	}
	if fake {
		// A real RoutingConfig over the fake provider rather than a direct fake
		// client: the worker always has a pool, so --ai-fake rides the real
		// Router (tiering, the budget guardrail, metering, call tracing) with
		// only the provider swapped.
		candidates = append(candidates, ai.FakeRoutingConfig())
	}
	return candidates
}

// modelPathWithBoundModels assembles the path and reports which models the SAME
// binding names, so the two can never describe different routing configs.
func modelPathWithBoundModels(ctx context.Context, cfg ai.RoutingConfig, pool *pgxpool.Pool, capturePayloads bool, log *slog.Logger) (compose.ModelPath, map[string]map[string]bool, error) {
	path, err := compose.NewModelPath(ctx, cfg, pool, capturePayloads, log)
	return path, cfg.BoundModelIDsByProvider(), err
}
