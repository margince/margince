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
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/deployconfig"
	"github.com/margince/margince/backend/internal/platform/jobs"
)

// resolveModelPath is the ONE place the api process decides what serves
// its AI surfaces: the stored routing binding, the offline fake behind
// --ai-fake, or neither — a single three-way switch run() runs exactly
// once. coldStartOptions, offerDraftOptions and /readyz's AI line all
// consume the one *compose.ModelPath (and the state string) this
// returns, so the process holds one Router, one cache, one budget —
// never a doubled pair from two callers each resolving their own.
//
// The --ai-fake arm builds a real compose.ModelPath over
// ai.FakeRoutingConfig() rather than compose.FakeModelPath's direct
// client wiring: the api always has a pool, so --ai-fake safely rides
// the real Router (tiering, budget guardrail, metering, call tracing)
// with only the provider swapped for the deterministic fake — dev/test
// exercises the same wiring production does, not a bypass of it.
//
// A nil path (the neither-flag case) is not a boot error: an
// AI-unconfigured deployment is a legitimate, ready one (aistate.go);
// coldStartOptions/offerDraftOptions/writeAIMetrics all treat nil as
// "this role wires no AI surfaces" rather than panicking.
// routingVersionOf names the model binding a resolved path came from, for
// callers that cache model-written content keyed on it.
func routingVersionOf(cfg ai.RoutingConfig) string { return cfg.RoutingVersion() }

// modelPathSpec names the boot knobs resolveModelPath switches on, so a call
// site labels each flag instead of passing anonymous booleans.
type modelPathSpec struct {
	routingPath     string
	fakeBrain       bool
	capturePayloads bool
}

// modelPathSpecFrom gathers the model-path knobs from where each is declared:
// the routing choice from the process flags, the capture posture from the
// deployment config.
func modelPathSpecFrom(cfg apiConfig, deployCfg deployconfig.Config) modelPathSpec {
	return modelPathSpec{
		routingPath:     cfg.routingPath,
		fakeBrain:       cfg.fakeBrain,
		capturePayloads: deployCfg.AI.CapturePayloads,
	}
}

func resolveModelPath(ctx context.Context, spec modelPathSpec, pool *pgxpool.Pool, log *slog.Logger) (*compose.ModelPath, string, ai.PublicProfile, string, error) {
	// WHERE the binding comes from is a question about the installation, and
	// answering it needs the database. Which surfaces that binding lights up is
	// a question about this process. They are separated so the second stays
	// answerable without a database — the wiring switch is what a unit test can
	// pin, and it is where a silently-picked default would hide.
	cfg, err := compose.ResolveRouting(ctx, pool, spec.routingPath, config.FromOS, log)
	if err != nil {
		return nil, "", ai.PublicProfile{}, "", err
	}
	return modelPathFor(ctx, cfg, spec, pool, log)
}

// modelPathFor wires the surfaces a resolved binding lights up: the declared
// path for a bound installation, the offline fake behind an explicit dev flag,
// or the honest-absent posture. Nothing is picked silently.
func modelPathFor(ctx context.Context, cfg ai.RoutingConfig, spec modelPathSpec, pool *pgxpool.Pool, log *slog.Logger) (*compose.ModelPath, string, ai.PublicProfile, string, error) {
	if !cfg.Unconfigured() {
		// A task whose whole fallback ladder has no bound tier is not a
		// boot error (a deployment may legitimately not run every
		// workload), but it must be loud: log it now, not discover it
		// from a refused call.
		for _, w := range cfg.UnboundLadderWarnings() {
			log.Warn(w)
		}
	}
	// The bindings this boot may run on, best first. A list rather than a switch
	// because the second entry is a FALLBACK from the first — reached only when
	// the stored binding cannot be built — and a role resolves its model path in
	// exactly one place (backend/gates/arch_test.go holds that), so trying two
	// candidates cannot mean two construction sites.
	//
	// The fallback is not generosity. A dev stack's bootstrap seeds a cloud
	// binding from `seeds.ai_routing`, and the engineer running it may hold no
	// key for that vendor; refusing the boot there costs them the whole stack
	// over an AI lane they were not using, while --ai-fake on their own command
	// line says what they want instead. A deployment passes no such flag, so it
	// still fails closed on a binding it cannot serve.
	var candidates []struct {
		cfg   ai.RoutingConfig
		state string
	}
	if !cfg.Unconfigured() {
		candidates = append(candidates, struct {
			cfg   ai.RoutingConfig
			state string
		}{cfg, compose.AIStateConfigured})
	}
	if spec.fakeBrain {
		candidates = append(candidates, struct {
			cfg   ai.RoutingConfig
			state string
		}{ai.FakeRoutingConfig(), compose.AIStateFake})
	}

	for i, candidate := range candidates {
		modelPath, err := compose.NewModelPath(ctx, candidate.cfg, pool, spec.capturePayloads, log)
		if err != nil {
			// Only an unservable BINDING is worth another candidate. A
			// database fault from the embed marker is not: no fallback repairs
			// it, and booting past it would serve the AI surfaces with an
			// unestablished marker while reporting a storage outage as a
			// missing credential.
			var unservable *compose.UnservableBindingError
			if i == len(candidates)-1 || !errors.As(err, &unservable) {
				return nil, "", ai.PublicProfile{}, "", err
			}
			// Loud: a bound installation quietly serving canned text would be
			// the worse of the two failures.
			log.WarnContext(ctx, "the stored model binding cannot be served, and --ai-fake was requested: falling back to the offline fake for this boot. The AI surfaces answer with canned text until the binding resolves — bind a servable model under Settings -> AI, or supply the missing credential",
				"error", err)
			continue
		}
		return &modelPath, candidate.state,
			ai.NewPublicProfile(candidate.state, candidate.cfg), routingVersionOf(candidate.cfg), nil
	}
	return nil, compose.AIStateUnconfigured,
		ai.NewPublicProfile(compose.AIStateUnconfigured, ai.RoutingConfig{}), "", nil
}

// coldStartOptions wires the cold-start read-back's model surface over
// an already-resolved model path: a real deployment or --ai-fake lights
// it up, no path leaves the operation an explicit 501 (same posture as
// the worker's runner lane).
func coldStartOptions(modelPath *compose.ModelPath, routingVersion string) []compose.Option {
	if modelPath == nil {
		return nil
	}
	// The read-back and per-org enrichment share the fetch + extraction
	// seam, so both light up together on the one resolved model path;
	// the Morning-Brief L2 re-order rides its own routed lane.
	fetch := compose.NewWebFetcher()
	return []compose.Option{
		compose.WithColdStart(fetch, modelPath.ColdStart),
		compose.WithScrape(fetch, modelPath.ColdStart),
		compose.WithBrief(modelPath.BriefRanking),
		compose.WithAccountBrief(modelPath.Summarize, routingVersion),
		compose.WithCompanyDossier(modelPath.Summarize, routingVersion),
		compose.WithGrowthFit(modelPath.GrowthFit, routingVersion),
		compose.WithReplyDraft(modelPath.DraftReply),
		// The account-started draft rides the same draft_reply lane as the
		// reply-side one: it is the same task with a different input shape.
		compose.WithAccountDraft(modelPath.DraftReply),
		// The person-side draft rides the same lane for the same reason: one
		// drafting task, a different input shape.
		compose.WithPersonDraft(modelPath.DraftReply),
		compose.WithDealStatusWriter(modelPath.DealHealth, routingVersion),
		compose.WithRoleProposals(modelPath.ProposeRoles),
		// The ask to a colleague rides the drafting lane: it is the same task
		// with a different reader, which is what a site is for.
		compose.WithIntroRequestDraft(modelPath.DraftReply),
	}
}

// offerDraftOptions wires the AI-drafted offer regeneration's model +
// retrieval surface (arc 4b) over the SAME resolved model path
// coldStartOptions consumes — a role that lights up one AI surface
// lights up both rather than growing a second resolution for what is,
// at boot time, one decision ("does this role have a model?"). Absent a
// path, regenerateOffer stays the mechanical clone alone (offerregenerate.go's
// honest "offerDrafter unwired" path) — never a silently different behavior.
func offerDraftOptions(pool *pgxpool.Pool, modelPath *compose.ModelPath) []compose.Option {
	if modelPath == nil {
		return nil
	}
	retriever := search.NewRetriever(search.NewStore(compose.InstallationDB(pool)), modelPath.Embedder)
	return []compose.Option{compose.WithOfferDraft(modelPath.OfferDraft, retriever)}
}

// jobEnqueueOptions wires the api-role transports that hand work to the
// worker role over an insert-only River client — the deep-read start and
// the voice-build create both enqueue, they never work jobs
// (jobs.NewInserter documents that Start is never called on it). The deep
// read additionally carries the cold-start completer when this role has a
// model path (the workbench read-back); nil keeps it enqueue-only.
func jobEnqueueOptions(pool *pgxpool.Pool, logger *slog.Logger, modelPath *compose.ModelPath) ([]compose.Option, error) {
	inserter, err := jobs.NewInserter(pool, logger)
	if err != nil {
		return nil, err
	}
	deepRead := compose.WithDeepRead(inserter, nil)
	if modelPath != nil {
		deepRead = compose.WithDeepRead(inserter, modelPath.ColdStart)
	}
	return []compose.Option{
		deepRead, compose.WithVoiceBuildEnqueue(inserter), compose.WithRateRefresh(inserter),
		compose.WithTechnicalEnrich(inserter),
		compose.WithTranscriptRead(inserter),
		compose.WithDocumentRead(inserter),
		compose.WithKnowledgeIngest(inserter),
		knowledgeAskOption(modelPath, logger),
		// An address that lands queues a coordinate lookup. The api role only
		// ENQUEUES — the provider lives on the worker (JobRunnerConfig.Geocoder)
		// — and that split is what keeps the single-requester rule enforceable:
		// several api replicas may queue, exactly one worker asks.
		compose.WithGeocoding(inserter),
		compose.WithVatChecking(inserter),
	}, nil
}

// embedReindexOption wires the /embeddings/reindex* ops over the resolved
// model path's embed lane and its own insert-only River client (the api
// enqueues the fleet-wide re-embed, the worker role works it — the same
// api-enqueues/worker-works split as deepReadOption). Without a model
// path there is no router to hand WithEmbedReindex, which self-gates on
// a nil router exactly like it self-gates on a nil inserter — so the
// returned Option is always real, and the three ops stay their generated
// 501 by that same omission, never a special-cased nil Option here.
func embedReindexOption(pool *pgxpool.Pool, modelPath *compose.ModelPath, logger *slog.Logger) (compose.Option, error) {
	inserter, err := jobs.NewInserter(pool, logger)
	if err != nil {
		return nil, err
	}
	var router *ai.Router
	if modelPath != nil {
		router = modelPath.Router()
	}
	return compose.WithEmbedReindex(router, inserter), nil
}

// knowledgeAskOption wires the corpus ask over whatever this role resolved: the
// retrieval embed lane, and the chat lane that writes the prose over it.
//
// Both may be absent and neither absence is an error. With no embed lane the
// ask reports retrieval_unavailable — nothing was searched — and with no chat
// lane it reports unreviewed, carrying the retrieved passages: they are what
// the search found, and nothing read them. The option is always applied,
// because the endpoint's 501 is for an installation that composed no retrieval
// AT ALL, not for one whose lanes are unconfigured.
func knowledgeAskOption(modelPath *compose.ModelPath, log *slog.Logger) compose.Option {
	if modelPath == nil {
		return compose.WithCorpusAsk(nil, nil, log)
	}
	return compose.WithCorpusAsk(modelPath.Embedder, modelPath.CorpusAsk, log)
}
